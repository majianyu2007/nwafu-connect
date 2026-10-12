package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/majianyu2007/nwafu-connect/embed"
)

type fakeSession struct {
	addr   string
	closed atomic.Int32
	state  embed.State
}

func (f *fakeSession) ProxyAddr() string { return f.addr }
func (f *fakeSession) Status() embed.Status {
	st := embed.Status{State: f.state, ProxyAddr: f.addr, ClientIP: "10.0.0.2"}
	if f.closed.Load() > 0 {
		st.State = embed.StateStopped
	}
	return st
}
func (f *fakeSession) Close() error { f.closed.Add(1); return nil }

const validConfig = `{"username":"u","password":"p"}`

func decode(t *testing.T, s string) response {
	t.Helper()
	var r response
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		t.Fatalf("bad JSON %q: %v", s, err)
	}
	return r
}

func TestStartRejectsBadConfig(t *testing.T) {
	b := newBridge(func(context.Context, embed.Config, embed.Prompter) (session, error) {
		t.Fatal("start must not be called")
		return nil, nil
	})
	for _, in := range []string{`not json`, `{}`, `{"username":"u","password":"p","port":70000}`, `{"username":"u","password":"p","httpBind":"nope"}`} {
		r := decode(t, b.Start(in))
		if r.OK || r.Error == "" || r.State != stateIdle || r.Needs != nil {
			t.Errorf("Start(%s) = %+v, want idle error", in, r)
		}
	}
	if r := decode(t, b.Status()); r.State != stateIdle {
		t.Errorf("status after bad config = %+v", r)
	}
}

func TestNeedsIsExplicitNull(t *testing.T) {
	out := response{OK: true, State: stateRunning}.json()
	var raw map[string]any
	_ = json.Unmarshal([]byte(out), &raw)
	if v, ok := raw["needs"]; !ok || v != nil {
		t.Fatalf("needs should be present and null: %s", out)
	}
}

func TestStartWithoutPromptsRuns(t *testing.T) {
	fs := &fakeSession{addr: "127.0.0.1:5555", state: embed.StateRunning}
	var got embed.Config
	b := newBridge(func(_ context.Context, cfg embed.Config, _ embed.Prompter) (session, error) {
		got = cfg
		return fs, nil
	})
	r := decode(t, b.Start(`{"username":"u","password":"p","clientDataPath":"/tmp/x.json"}`))
	if !r.OK || r.State != stateRunning || r.Proxy != fs.addr || r.ClientIP != "10.0.0.2" || r.Needs != nil {
		t.Fatalf("start = %+v", r)
	}
	if got.Username != "u" || got.ClientDataPath != "/tmp/x.json" {
		t.Fatalf("config not passed through: %+v", got)
	}
	if r := decode(t, b.Start(validConfig)); r.OK || r.State != stateRunning {
		t.Fatalf("second start = %+v, want refusal", r)
	}
	if r := decode(t, b.Status()); r.State != stateRunning || r.Proxy != fs.addr {
		t.Fatalf("status = %+v", r)
	}
	if r := decode(t, b.Stop()); !r.OK || r.State != stateStopped {
		t.Fatalf("stop = %+v", r)
	}
	if fs.closed.Load() != 1 {
		t.Fatalf("session closed %d times", fs.closed.Load())
	}
	if r := decode(t, b.Status()); r.State != stateIdle {
		t.Fatalf("status after stop = %+v", r)
	}
}

func TestCaptchaThenSMSFlow(t *testing.T) {
	img := []byte{0x89, 'P', 'N', 'G'}
	fs := &fakeSession{addr: "127.0.0.1:6000", state: embed.StateRunning}
	answers := make(chan [2]string, 2)
	b := newBridge(func(ctx context.Context, _ embed.Config, p embed.Prompter) (session, error) {
		c, err := p.Captcha(ctx, img)
		if err != nil {
			return nil, err
		}
		answers <- [2]string{"captcha", c}
		code, err := p.Code(ctx, embed.KindSMS, "hint")
		if err != nil {
			return nil, err
		}
		answers <- [2]string{"sms", code}
		return fs, nil
	})

	r := decode(t, b.Start(validConfig))
	if !r.OK || r.State != stateAwaiting || r.Needs == nil || *r.Needs != "captcha" {
		t.Fatalf("start = %+v", r)
	}
	if dec, _ := base64.StdEncoding.DecodeString(r.Image); string(dec) != string(img) {
		t.Fatalf("image = %q", r.Image)
	}
	if s := decode(t, b.Status()); s.State != stateAwaiting || *s.Needs != "captcha" {
		t.Fatalf("status while awaiting = %+v", s)
	}
	// Wrong kind and empty values keep the prompt pending.
	if r := decode(t, b.Submit("sms", "123")); r.Error == "" || *r.Needs != "captcha" {
		t.Fatalf("wrong kind = %+v", r)
	}
	if r := decode(t, b.Submit("captcha", "  ")); r.Error == "" || *r.Needs != "captcha" {
		t.Fatalf("empty = %+v", r)
	}

	r = decode(t, b.Submit("captcha", "[[1,2],[3,4]]"))
	if r.State != stateAwaiting || *r.Needs != "sms" || r.Image != "" || r.Hint != "hint" {
		t.Fatalf("after captcha = %+v", r)
	}
	r = decode(t, b.Submit("sms", " 654321 "))
	if !r.OK || r.State != stateRunning || r.Proxy != fs.addr {
		t.Fatalf("after sms = %+v", r)
	}
	if a := <-answers; a != [2]string{"captcha", "[[1,2],[3,4]]"} {
		t.Fatalf("captcha answer = %v", a)
	}
	if a := <-answers; a != [2]string{"sms", "654321"} {
		t.Fatalf("sms answer = %v", a)
	}
	if r := decode(t, b.Submit("sms", "1")); r.OK {
		t.Fatalf("submit with nothing pending = %+v", r)
	}
	b.Stop()
}

func TestStopWhileAwaitingCancelsLogin(t *testing.T) {
	cancelled := make(chan error, 1)
	b := newBridge(func(ctx context.Context, _ embed.Config, p embed.Prompter) (session, error) {
		_, err := p.Code(ctx, embed.KindTOTP, "")
		cancelled <- err
		return nil, err
	})
	if r := decode(t, b.Start(validConfig)); *r.Needs != "totp" {
		t.Fatalf("start = %+v", r)
	}
	if r := decode(t, b.Stop()); !r.OK || r.State != stateStopped {
		t.Fatalf("stop = %+v", r)
	}
	select {
	case err := <-cancelled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("prompt err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("login goroutine not cancelled")
	}
}

func TestStopDuringBlockingStart(t *testing.T) {
	entered := make(chan struct{})
	b := newBridge(func(ctx context.Context, _ embed.Config, _ embed.Prompter) (session, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	result := make(chan response, 1)
	go func() { result <- decode(t, b.Start(validConfig)) }()
	<-entered
	if s := decode(t, b.Status()); s.State != stateStarting {
		t.Fatalf("status during start = %+v", s)
	}
	b.Stop()
	select {
	case r := <-result:
		if r.OK {
			t.Fatalf("start after stop = %+v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

func TestFailedLoginCanRestart(t *testing.T) {
	calls := 0
	fs := &fakeSession{addr: "127.0.0.1:7000", state: embed.StateRunning}
	b := newBridge(func(context.Context, embed.Config, embed.Prompter) (session, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("login: bad password")
		}
		return fs, nil
	})
	r := decode(t, b.Start(validConfig))
	if r.OK || r.State != stateFailed || r.Error != "login: bad password" {
		t.Fatalf("first = %+v", r)
	}
	if s := decode(t, b.Status()); s.State != stateFailed || s.Error == "" {
		t.Fatalf("status = %+v", s)
	}
	if r := decode(t, b.Start(validConfig)); !r.OK || r.State != stateRunning {
		t.Fatalf("restart = %+v", r)
	}
	b.Stop()
}
