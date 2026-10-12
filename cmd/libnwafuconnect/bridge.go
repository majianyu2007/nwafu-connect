package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/majianyu2007/nwafu-connect/embed"
)

// session is the part of *embed.Session the bridge uses (fakeable in tests).
type session interface {
	ProxyAddr() string
	Status() embed.Status
	Close() error
}

type startFunc func(ctx context.Context, cfg embed.Config, p embed.Prompter) (session, error)

func defaultStart(ctx context.Context, cfg embed.Config, p embed.Prompter) (session, error) {
	s, err := embed.Start(ctx, cfg, p)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// response is the JSON every exported call returns.
type response struct {
	OK    bool    `json:"ok"`
	State string  `json:"state"`
	Proxy string  `json:"proxy,omitempty"`
	Error string  `json:"error,omitempty"`
	Needs *string `json:"needs"`
	// Image is the base64 captcha image when Needs == "captcha".
	Image string `json:"image,omitempty"`
	Hint  string `json:"hint,omitempty"`
	// ClientIP is the tunnel address assigned by the gateway.
	ClientIP string `json:"clientIp,omitempty"`
}

func (r response) json() string {
	b, err := json.Marshal(r)
	if err != nil {
		return `{"ok":false,"state":"failed","error":"encode response","needs":null}`
	}
	return string(b)
}

func errorResponse(state string, err error) response {
	return response{OK: false, State: state, Error: err.Error()}
}

// Bridge states.
const (
	stateIdle     = "idle"
	stateStarting = "starting"
	stateAwaiting = "awaiting"
	stateRunning  = "running"
	stateFailed   = "failed"
	stateStopped  = "stopped"
)

type event struct {
	needs string
	image []byte
	hint  string
}

// run is one login attempt and, if it succeeds, its session.
type run struct {
	cancel  context.CancelFunc
	prompts chan event
	answers chan string
	done    chan struct{} // closed after sess/err are set

	// Written by the login goroutine before done is closed.
	sess session
	err  error

	// Guarded by bridge.mu.
	pending  *event
	finished bool
}

// chanPrompter turns blocking Prompter calls into prompts for the host.
type chanPrompter struct{ r *run }

func (p chanPrompter) ask(ctx context.Context, ev event) (string, error) {
	select {
	case p.r.prompts <- ev:
	case <-ctx.Done():
		return "", ctx.Err()
	}
	select {
	case v := <-p.r.answers:
		return v, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (p chanPrompter) Captcha(ctx context.Context, image []byte) (string, error) {
	return p.ask(ctx, event{needs: embed.KindCaptcha, image: image, hint: "点击图片中的文字，按顺序提交坐标 [[x,y],...]"})
}

func (p chanPrompter) Code(ctx context.Context, kind, hint string) (string, error) {
	return p.ask(ctx, event{needs: kind, hint: hint})
}

// bridge holds the single session the C ABI manages. The mutex is never held
// while waiting on the network, so Status and Stop stay responsive while a
// Start or Submit call is blocked.
type bridge struct {
	mu    sync.Mutex
	start startFunc
	run   *run
}

func newBridge(start startFunc) *bridge {
	if start == nil {
		start = defaultStart
	}
	return &bridge{start: start}
}

// Start parses configJSON and begins logging in. It blocks until the
// session is up, fails, or needs an interactive answer.
func (b *bridge) Start(configJSON string) string {
	var cfg embed.Config
	if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
		return errorResponse(stateIdle, fmt.Errorf("invalid config JSON: %w", err)).json()
	}
	if err := cfg.Validate(); err != nil {
		return errorResponse(stateIdle, err).json()
	}

	b.mu.Lock()
	if b.run != nil {
		state := b.stateLocked()
		if state == stateStarting || state == stateAwaiting || state == stateRunning {
			b.mu.Unlock()
			return errorResponse(state, errors.New("a session is already active; call NcStop first")).json()
		}
		// Previous run failed or stopped: release it before starting again.
		if b.run.sess != nil {
			_ = b.run.sess.Close()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &run{
		cancel:  cancel,
		prompts: make(chan event),
		answers: make(chan string),
		done:    make(chan struct{}),
	}
	b.run = r
	b.mu.Unlock()

	go func() {
		defer close(r.done)
		sess, err := b.start(ctx, cfg, chanPrompter{r: r})
		if err != nil && sess != nil {
			_ = sess.Close()
			sess = nil
		}
		if sess != nil && ctx.Err() != nil {
			// Stopped while finishing: do not leak a session nobody owns.
			_ = sess.Close()
			sess, err = nil, ctx.Err()
		}
		r.sess, r.err = sess, err
	}()
	return b.wait(r).json()
}

// Submit answers the pending prompt and blocks for the next step.
func (b *bridge) Submit(kind, value string) string {
	b.mu.Lock()
	r := b.run
	if r == nil || r.finished || r.pending == nil {
		state := b.stateLocked()
		b.mu.Unlock()
		return errorResponse(state, errors.New("no input is pending")).json()
	}
	if kind != r.pending.needs {
		resp := promptResponse(*r.pending)
		b.mu.Unlock()
		resp.Error = fmt.Sprintf("expected %q input, got %q", resp.needsValue(), kind)
		return resp.json()
	}
	value = strings.TrimSpace(value)
	if value == "" {
		resp := promptResponse(*r.pending)
		b.mu.Unlock()
		resp.Error = "empty value"
		return resp.json()
	}
	r.pending = nil
	b.mu.Unlock()

	select {
	case r.answers <- value:
	case <-r.done:
	}
	return b.wait(r).json()
}

func (r response) needsValue() string {
	if r.Needs == nil {
		return ""
	}
	return *r.Needs
}

func promptResponse(ev event) response {
	needs := ev.needs
	resp := response{OK: true, State: stateAwaiting, Needs: &needs, Hint: ev.hint}
	if len(ev.image) > 0 {
		resp.Image = base64.StdEncoding.EncodeToString(ev.image)
	}
	return resp
}

// wait blocks until the login goroutine asks for input or finishes.
func (b *bridge) wait(r *run) response {
	select {
	case ev := <-r.prompts:
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.run != r {
			return response{OK: false, State: stateStopped, Error: "stopped"}
		}
		r.pending = &ev
		return promptResponse(ev)
	case <-r.done:
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	r.finished = true
	if b.run != r {
		return response{OK: false, State: stateStopped, Error: "stopped"}
	}
	if r.err != nil {
		return errorResponse(stateFailed, r.err)
	}
	st := r.sess.Status()
	return response{OK: true, State: stateRunning, Proxy: r.sess.ProxyAddr(), ClientIP: st.ClientIP}
}

func (b *bridge) stateLocked() string {
	r := b.run
	switch {
	case r == nil:
		return stateIdle
	case !r.finished && r.pending != nil:
		return stateAwaiting
	case !r.finished:
		return stateStarting
	case r.err != nil:
		return stateFailed
	case r.sess != nil:
		switch r.sess.Status().State {
		case embed.StateFailed:
			return stateFailed
		case embed.StateStopped:
			return stateStopped
		}
		return stateRunning
	}
	return stateStopped
}

// Status reports the bridge state without blocking on the network.
func (b *bridge) Status() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	r := b.run
	if r == nil {
		return response{OK: true, State: stateIdle}.json()
	}
	if !r.finished {
		if r.pending != nil {
			return promptResponse(*r.pending).json()
		}
		return response{OK: true, State: stateStarting}.json()
	}
	resp := response{OK: true, State: b.stateLocked()}
	if r.err != nil {
		resp.Error = r.err.Error()
	}
	if r.sess != nil {
		st := r.sess.Status()
		resp.Proxy = st.ProxyAddr
		resp.ClientIP = st.ClientIP
		if st.Error != "" {
			resp.Error = st.Error
		}
	}
	return resp.json()
}

// Stop cancels a pending login or closes the running session.
func (b *bridge) Stop() string {
	b.mu.Lock()
	r := b.run
	b.run = nil
	b.mu.Unlock()

	if r == nil {
		return response{OK: true, State: stateIdle}.json()
	}
	r.cancel()
	<-r.done
	if r.sess != nil {
		if err := r.sess.Close(); err != nil {
			return errorResponse(stateStopped, err).json()
		}
	}
	return response{OK: true, State: stateStopped}.json()
}
