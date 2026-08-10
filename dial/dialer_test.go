package dial

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/internal/ippool"
	"github.com/majianyu2007/nwafu-connect/internal/zcdns"
)

type recordingStack struct {
	tcpAddress *net.TCPAddr
}

func (s *recordingStack) Run() error { return nil }

func (s *recordingStack) SetupResolve(zcdns.LocalServer) {}

func (s *recordingStack) SetupIPPool(*ippool.IPPool[client.DomainResourceSet]) {}

func (s *recordingStack) DialTCP(_ context.Context, address *net.TCPAddr) (net.Conn, error) {
	s.tcpAddress = address
	clientConnection, serverConnection := net.Pipe()
	_ = serverConnection.Close()
	return clientConnection, nil
}

func (s *recordingStack) DialUDP(_ context.Context, _ *net.UDPAddr) (net.Conn, error) {
	return nil, errors.New("unexpected UDP dial")
}

func TestAlwaysUseVPNRejectsDestinationOutsideGatewayResources(t *testing.T) {
	vpnStack := &recordingStack{}
	dialer := NewDialer(vpnStack, nil, []client.IPResource{}, true, "")

	_, err := dialer.DialIPPort(context.Background(), "tcp", "203.0.113.10:443")
	if !errors.Is(err, ErrACLDenied) {
		t.Fatalf("DialIPPort() error = %v, want ErrACLDenied", err)
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

func TestHTTPProxyConnectPreservesBufferedTunnelBytes(t *testing.T) {
	proxyAddress, completed := startHTTPConnectFixture(t, "HTTP/1.1 200 Connection Established\r\nContent-Length: 0\r\n\r\nok")
	dialer := &Dialer{dialDirectHTTPProxy: proxyAddress}

	connection, err := dialer.dialDirectWithHTTPProxy(context.Background(), "library.example:443")
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 2)
	if _, err := io.ReadFull(connection, payload); err != nil {
		t.Fatalf("read tunneled bytes buffered with CONNECT response: %v", err)
	}
	if got := string(payload); got != "ok" {
		t.Fatalf("tunneled payload = %q, want ok", got)
	}
	if err := <-completed; err != nil {
		t.Fatal(err)
	}
}

func TestHTTPProxyConnectRequiresSuccessfulStatus(t *testing.T) {
	proxyAddress, completed := startHTTPConnectFixture(t, "HTTP/1.1 502 Bad Gateway\r\nX-Diagnostic: 200\r\nContent-Length: 0\r\n\r\n")
	dialer := &Dialer{dialDirectHTTPProxy: proxyAddress}

	connection, err := dialer.dialDirectWithHTTPProxy(context.Background(), "library.example:443")
	if err == nil {
		connection.Close()
		t.Fatal("HTTP proxy status 502 was accepted")
	}
	if !strings.Contains(err.Error(), "502 Bad Gateway") {
		t.Fatalf("HTTP proxy error = %q, want response status", err)
	}
	if err := <-completed; err != nil {
		t.Fatal(err)
	}
}

func startHTTPConnectFixture(t *testing.T, response string) (string, <-chan error) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	completed := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			completed <- err
			return
		}
		defer connection.Close()
		request, err := http.ReadRequest(bufio.NewReader(connection))
		if err != nil {
			completed <- err
			return
		}
		if request.Method != http.MethodConnect || request.Host != "library.example:443" {
			completed <- fmt.Errorf("CONNECT request = %s %s", request.Method, request.Host)
			return
		}
		_, err = io.WriteString(connection, response)
		completed <- err
	}()
	return listener.Addr().String(), completed
}

func TestProxyHandshakesHonorContextCancellation(t *testing.T) {
	tests := []struct {
		name string
		call func(context.Context, string) (net.Conn, error)
	}{
		{
			name: "HTTP",
			call: func(ctx context.Context, proxyAddress string) (net.Conn, error) {
				return (&Dialer{dialDirectHTTPProxy: proxyAddress}).dialDirectWithHTTPProxy(ctx, "library.example:443")
			},
		},
		{
			name: "SOCKS5",
			call: func(ctx context.Context, proxyAddress string) (net.Conn, error) {
				return (&Dialer{dialDirectSocksProxy: proxyAddress}).dialDirectWithSocksProxy(ctx, "tcp", "library.example:443", false)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			accepted := make(chan net.Conn, 1)
			go func() {
				connection, acceptErr := listener.Accept()
				if acceptErr == nil {
					accepted <- connection
				}
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			started := time.Now()
			connection, err := test.call(ctx, listener.Addr().String())
			if connection != nil {
				connection.Close()
			}
			if err == nil {
				t.Fatal("stalled proxy handshake unexpectedly succeeded")
			}
			if elapsed := time.Since(started); elapsed > 2*time.Second {
				t.Fatalf("canceled proxy handshake took %v", elapsed)
			}
			select {
			case connection := <-accepted:
				connection.Close()
			case <-time.After(time.Second):
				t.Fatal("proxy fixture did not accept connection")
			}
		})
	}
}
