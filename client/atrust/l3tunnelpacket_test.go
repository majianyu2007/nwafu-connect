package atrust

import (
	"fmt"
	zlog "github.com/majianyu2007/nwafu-connect/log"
	"testing"
)

func TestIsAuthTimeoutErr(t *testing.T) {
	err := fmt.Errorf("%w for 4:<client-ip>:<client-port>-<dns-server>:53", errL3TunnelAuthTimeout)
	if !isAuthTimeoutErr(err) {
		t.Fatal("expected wrapped l3 tunnel auth timeout to be recognized")
	}

	if isAuthTimeoutErr(nil) {
		t.Fatal("nil error must not be treated as auth timeout")
	}

	if isAuthTimeoutErr(fmt.Errorf("l3-tunnel auth timeout for unrelated text")) {
		t.Fatal("plain text error must not be treated as auth timeout")
	}
}

func TestDisabledPacketLoggingAllocatesNothing(t *testing.T) {
	zlog.DisableDebug()
	packet := make([]byte, 1500)
	if allocations := testing.AllocsPerRun(1000, func() {
		logPacket("send", packet)
	}); allocations != 0 {
		t.Fatalf("disabled packet logging allocations = %v, want 0", allocations)
	}
}
