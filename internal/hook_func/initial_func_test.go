package hook_func

import (
	"context"
	"net"
	"testing"

	"github.com/majianyu2007/nwafu-connect/configs"
)

func TestCheckBindPortLegalRejectsOccupiedAndConflictingListeners(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	if err := checkBindPortLegal(context.Background(), configs.Config{HTTPBind: occupied.Addr().String()}); err == nil {
		t.Fatal("occupied HTTP proxy address was accepted")
	}

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	if err := checkBindPortLegal(context.Background(), configs.Config{HTTPBind: address, SocksBind: address}); err == nil {
		t.Fatal("conflicting HTTP and SOCKS5 listener addresses were accepted")
	}
}

func TestCheckBindPortLegalSkipsManagedBrowserListeners(t *testing.T) {
	configuration := configs.Config{
		BrowserMode:   true,
		HTTPBind:      "not an address",
		SocksBind:     "not an address",
		DNSServerBind: "not an address",
	}
	if err := checkBindPortLegal(context.Background(), configuration); err != nil {
		t.Fatalf("managed browser listener preflight = %v", err)
	}
}
