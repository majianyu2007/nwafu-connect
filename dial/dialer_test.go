package dial

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/internal/ippool"
	"github.com/majianyu2007/nwafu-connect/internal/zcdns"
)

type recordingStack struct {
	tcpAddress *net.TCPAddr
}

func (s *recordingStack) Run() {}

func (s *recordingStack) SetupResolve(zcdns.LocalServer) {}

func (s *recordingStack) SetupIPPool(*ippool.IPPool[client.DomainResource]) {}

func (s *recordingStack) DialTCP(_ context.Context, address *net.TCPAddr) (net.Conn, error) {
	s.tcpAddress = address
	clientConnection, serverConnection := net.Pipe()
	_ = serverConnection.Close()
	return clientConnection, nil
}

func (s *recordingStack) DialUDP(_ context.Context, _ *net.UDPAddr) (net.Conn, error) {
	return nil, errors.New("unexpected UDP dial")
}

func TestAlwaysUseVPNPassthroughUnauthorizedDestination(t *testing.T) {
	vpnStack := &recordingStack{}
	dialer := NewDialer(vpnStack, nil, []client.IPResource{}, true, "")

	_, err := dialer.DialIPPort(context.Background(), "tcp", "127.0.0.1:1")
	if errors.Is(err, ErrACLDenied) {
		t.Fatalf("DialIPPort() should fall back to direct connection, got ErrACLDenied")
	}
	if vpnStack.tcpAddress != nil {
		t.Fatalf("unauthorized destination reached VPN stack: %s", vpnStack.tcpAddress)
	}
}

func TestAlwaysUseVPNRoutesAuthorizedDestination(t *testing.T) {
	vpnStack := &recordingStack{}
	resources := []client.IPResource{{
		IPMin:    net.ParseIP("210.27.83.19"),
		IPMax:    net.ParseIP("210.27.83.20"),
		PortMin:  80,
		PortMax:  443,
		Protocol: "tcp",
	}}
	dialer := NewDialer(vpnStack, nil, resources, true, "")

	connection, err := dialer.DialIPPort(context.Background(), "tcp", "210.27.83.20:80")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if got, want := vpnStack.tcpAddress.String(), "210.27.83.20:80"; got != want {
		t.Fatalf("VPN destination = %q, want %q", got, want)
	}
}
