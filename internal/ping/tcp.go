package ping

import (
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/majianyu2007/nwafu-connect/log"
)

// TCPing ...
type TCPing struct {
	target *Target
	stop   chan struct{}
	done   chan struct{}
	result *Result

	startOnce sync.Once
	stopOnce  sync.Once
}

var _ Pinger = (*TCPing)(nil)

// NewTCPing return a new TCPing
func NewTCPing() *TCPing {
	return &TCPing{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// SetTarget set target for TCPing
func (tcping *TCPing) SetTarget(target *Target) {
	tcping.target = target
	if tcping.result == nil {
		tcping.result = &Result{Target: target}
	}
}

// Result return the result
func (tcping *TCPing) Result() *Result {
	return tcping.result
}

func (tcping *TCPing) Start() <-chan struct{} {
	tcping.startOnce.Do(func() {
		go func() {
			defer close(tcping.done)
			interval := tcping.target.Interval
			if interval <= 0 {
				interval = time.Millisecond
			}
			timer := time.NewTimer(0)
			defer timer.Stop()
			for {
				select {
				case <-timer.C:
					duration, remoteAddr, err := tcping.ping()
					tcping.result.Counter++

					if err != nil {
						log.DebugPrintf("Ping %s - failed: %s\n", tcping.target, err)
					} else {
						log.DebugPrintf("Ping %s(%s) - Connected - time=%s\n", tcping.target, remoteAddr, duration)

						if tcping.result.MinDuration == 0 {
							tcping.result.MinDuration = duration
						}
						if tcping.result.MaxDuration == 0 {
							tcping.result.MaxDuration = duration
						}
						tcping.result.SuccessCounter++
						if duration > tcping.result.MaxDuration {
							tcping.result.MaxDuration = duration
						} else if duration < tcping.result.MinDuration {
							tcping.result.MinDuration = duration
						}
						tcping.result.TotalDuration += duration
					}
					if tcping.target.Counter > 0 && tcping.result.Counter >= tcping.target.Counter {
						return
					}
					timer.Reset(interval)
				case <-tcping.stop:
					return
				}
			}
		}()
	})
	return tcping.done
}

// Stop terminates a running probe. It is safe to call more than once.
func (tcping *TCPing) Stop() {
	tcping.stopOnce.Do(func() { close(tcping.stop) })
}

func (tcping *TCPing) ping() (time.Duration, net.Addr, error) {
	var remoteAddr net.Addr
	duration, errIfce := timeIt(func() interface{} {
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", tcping.target.Host, tcping.target.Port), tcping.target.Timeout)
		if err != nil {
			return err
		}
		remoteAddr = conn.RemoteAddr()
		conn.Close()
		return nil
	})
	if errIfce != nil {
		err := errIfce.(error)
		return 0, remoteAddr, err
	}
	return time.Duration(duration), remoteAddr, nil
}
