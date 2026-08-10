package ping

import (
	"testing"
	"time"
)

func TestTCPingCompletesConfiguredProbeCount(t *testing.T) {
	probe := NewTCPing()
	probe.SetTarget(&Target{
		Protocol: TCP,
		Host:     "127.0.0.1",
		Port:     1,
		Counter:  2,
		Interval: time.Millisecond,
		Timeout:  20 * time.Millisecond,
	})

	select {
	case <-probe.Start():
	case <-time.After(time.Second):
		t.Fatal("TCP probe did not report completion")
	}
	if got := probe.Result().Counter; got != 2 {
		t.Fatalf("probe count = %d, want 2", got)
	}
}

func TestTCPingStopIsIdempotentAndCompletes(t *testing.T) {
	probe := NewTCPing()
	probe.SetTarget(&Target{
		Protocol: TCP,
		Host:     "127.0.0.1",
		Port:     1,
		Interval: time.Hour,
		Timeout:  20 * time.Millisecond,
	})
	done := probe.Start()
	probe.Stop()
	probe.Stop()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stopped TCP probe did not report completion")
	}
}
