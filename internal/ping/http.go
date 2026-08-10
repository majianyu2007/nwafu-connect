package ping

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/majianyu2007/nwafu-connect/log"
)

// HTTPing ...
type HTTPing struct {
	target *Target
	stop   chan struct{}
	done   chan struct{}
	result *Result
	Method string

	startOnce sync.Once
	stopOnce  sync.Once
}

var _ Pinger = (*HTTPing)(nil)

// NewHTTPing return new HTTPing
func NewHTTPing(method string) *HTTPing {
	return &HTTPing{
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
		Method: method,
	}
}

// SetTarget ...
func (ping *HTTPing) SetTarget(target *Target) {
	ping.target = target
	if ping.result == nil {
		ping.result = &Result{Target: target}
	}
}

// Start begins probing and returns a channel closed when probing finishes.
func (ping *HTTPing) Start() <-chan struct{} {
	ping.startOnce.Do(func() {
		go func() {
			defer close(ping.done)
			interval := ping.target.Interval
			if interval <= 0 {
				interval = time.Millisecond
			}
			timer := time.NewTimer(0)
			defer timer.Stop()
			for {
				select {
				case <-timer.C:
					duration, resp, remoteAddr, err := ping.ping()
					ping.result.Counter++

					if err != nil {
						log.DebugPrintf("Ping %s - failed: %s\n", ping.target, err)
					} else {
						length, _ := io.Copy(io.Discard, resp.Body)
						_ = resp.Body.Close()
						log.DebugPrintf("Ping %s(%s) - %s is open - time=%s method=%s status=%d bytes=%d\n", ping.target, remoteAddr, ping.target.Protocol, duration, ping.Method, resp.StatusCode, length)
						if ping.result.MinDuration == 0 {
							ping.result.MinDuration = duration
						}
						if ping.result.MaxDuration == 0 {
							ping.result.MaxDuration = duration
						}
						ping.result.SuccessCounter++
						if duration > ping.result.MaxDuration {
							ping.result.MaxDuration = duration
						} else if duration < ping.result.MinDuration {
							ping.result.MinDuration = duration
						}
						ping.result.TotalDuration += duration
					}
					if ping.target.Counter > 0 && ping.result.Counter >= ping.target.Counter {
						return
					}
					timer.Reset(interval)
				case <-ping.stop:
					return
				}
			}
		}()
	})
	return ping.done
}

// Result return ping result
func (ping *HTTPing) Result() *Result {
	return ping.result
}

// Stop terminates a running probe. It is safe to call more than once.
func (ping *HTTPing) Stop() {
	ping.stopOnce.Do(func() { close(ping.stop) })
}

func (ping *HTTPing) ping() (time.Duration, *http.Response, net.Addr, error) {
	var resp *http.Response
	var body io.Reader
	if ping.Method == "POST" {
		body = bytes.NewBufferString("{}")
	}
	req, err := http.NewRequest(ping.Method, ping.target.String(), body)
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set(http.CanonicalHeaderKey("User-Agent"), "tcping")
	var remoteAddr net.Addr
	trace := &httptrace.ClientTrace{
		GotConn: func(connInfo httptrace.GotConnInfo) {
			remoteAddr = connInfo.Conn.RemoteAddr()
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))
	duration, errIfce := timeIt(func() interface{} {
		client := http.Client{Timeout: ping.target.Timeout}
		resp, err = client.Do(req)
		return err
	})
	if errIfce != nil {
		err := errIfce.(error)
		return 0, nil, nil, err
	}
	return time.Duration(duration), resp, remoteAddr, nil
}
