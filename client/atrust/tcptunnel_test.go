package atrust

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/resolve"
)

func TestBuildTCPTunnelAuthRequestEscapesFieldsAndSignsPayload(t *testing.T) {
	signKey := []byte("0123456789abcdef0123456789abcdef")
	vpnClient := &Client{
		Username:     "student\"\\\n名字",
		SID:          "sid\"\\value",
		DeviceID:     "device-id",
		ConnectionID: "connection-id",
		SignKey:      hex.EncodeToString(signKey),
	}
	payload, err := buildTCPTunnelAuthRequest(
		vpnClient,
		"application\"id",
		"library.example:443",
		"google-chrome-stable",
		"/usr/bin/google-chrome-stable",
	)
	if err != nil {
		t.Fatal(err)
	}

	var request tcpTunnelAuthRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatalf("authentication request is not valid JSON: %v", err)
	}
	if request.Username != vpnClient.Username {
		t.Fatalf("username = %q, want %q", request.Username, vpnClient.Username)
	}
	if request.SID != vpnClient.SID {
		t.Fatalf("SID = %q, want %q", request.SID, vpnClient.SID)
	}
	if request.AppID != "application\"id" {
		t.Fatalf("app ID = %q", request.AppID)
	}
	if request.XRequestSig == "" {
		t.Fatal("request signature is empty")
	}

	signature := request.XRequestSig
	request.XRequestSig = ""
	unsigned, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if want := calcXRequestSig(signKey, unsigned); signature != want {
		t.Fatalf("signature = %q, want %q", signature, want)
	}
}

func TestBuildTCPTunnelAuthRequestRejectsInvalidSignKey(t *testing.T) {
	_, err := buildTCPTunnelAuthRequest(&Client{SignKey: "not-hex"}, "app", "host:443", "browser", "/browser")
	if err == nil {
		t.Fatal("invalid sign key was accepted")
	}
}

func TestDialTCPRejectsInvalidOrUnauthorizedRoutingContext(t *testing.T) {
	vpnClient := &Client{}
	address := &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 443}

	invalidContext := context.WithValue(context.Background(), resolve.ContextKeyDomainResource, "not a resource")
	if _, err := vpnClient.DialTCP(invalidContext, address); err == nil {
		t.Fatal("invalid routing context was accepted")
	}

	unauthorizedContext := context.WithValue(context.Background(), resolve.ContextKeyDomainResource, client.DomainResourceSet{{
		PortMin:  80,
		PortMax:  80,
		Protocol: "tcp",
		AppID:    "web",
	}})
	if _, err := vpnClient.DialTCP(unauthorizedContext, address); !errors.Is(err, client.ErrResourceNotFound) {
		t.Fatalf("unauthorized destination error = %v, want ErrResourceNotFound", err)
	}

	if _, err := vpnClient.DialTCP(context.Background(), address); !errors.Is(err, client.ErrResourceNotFound) {
		t.Fatalf("destination without resource error = %v, want ErrResourceNotFound", err)
	}
}

func TestTCPTunnelConnZeroLengthIOIsNonBlocking(t *testing.T) {
	connection := &tcpTunnelConn{}
	if n, err := connection.Read(nil); n != 0 || err != nil {
		t.Fatalf("Read(nil) = %d, %v", n, err)
	}
	if n, err := connection.Write(nil); n != 0 || err != nil {
		t.Fatalf("Write(nil) = %d, %v", n, err)
	}
}

func TestTCPTunnelConnSkipsEmptyFramesAndRejectsUnknownHeaders(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	defer clientConnection.Close()
	defer serverConnection.Close()
	go func() {
		_ = writeAll(serverConnection, []byte{0x01, 0x00, 0x00, 0x00, 0x7f, 0x01})
	}()
	connection := &tcpTunnelConn{reader: bufio.NewReader(clientConnection)}
	if _, err := connection.Read(make([]byte, 1)); err == nil || !strings.Contains(err.Error(), "unexpected TCP tunnel response header") {
		t.Fatalf("Read() error = %v, want unexpected-header error", err)
	}
}


func TestWaitForTCPConnectWaitsForDestinationStatus(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	defer clientConnection.Close()
	defer serverConnection.Close()
	reader := bufio.NewReader(clientConnection)
	probeReceived := make(chan struct{})
	releaseStatus := make(chan struct{})
	defer func() {
		select {
		case <-releaseStatus:
		default:
			close(releaseStatus)
		}
	}()
	serverErr := make(chan error, 1)
	go func() {
		if err := writeAll(serverConnection, []byte{0x05, 0x81, 0x53, 0x00, 0x00, 0x02, 'O', 'K'}); err != nil {
			serverErr <- err
			return
		}
		var probe [4]byte
		if _, err := io.ReadFull(serverConnection, probe[:]); err != nil {
			serverErr <- err
			return
		}
		if probe != [4]byte{0x01, 0x00, 0x00, 0x00} {
			serverErr <- errors.New("unexpected TCP connect probe")
			return
		}
		close(probeReceived)
		<-releaseStatus
		serverErr <- writeAll(serverConnection, []byte{0x05, 0x05})
	}()

	connectContext, cancelConnect := context.WithTimeout(context.Background(), time.Second)
	defer cancelConnect()
	result := make(chan error, 1)
	go func() {
		result <- waitForTCPConnect(connectContext, clientConnection, reader)
	}()
	select {
	case <-probeReceived:
	case <-time.After(time.Second):
		t.Fatal("TCP tunnel connect probe was not sent")
	}
	select {
	case err := <-result:
		t.Fatalf("waitForTCPConnect() returned before destination status: %v", err)
	default:
	}
	close(releaseStatus)
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "connection refused") {
			t.Fatalf("waitForTCPConnect() error = %v, want connection refused", err)
		}
	case <-time.After(time.Second):
		t.Fatal("TCP tunnel destination status was not returned")
	}
	select {
	case err := <-serverErr:
		if err != nil {
			t.Fatalf("TCP tunnel fixture: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("TCP tunnel fixture did not finish")
	}
}

func TestWaitForTCPConnectAcceptsSuccessfulDestination(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	defer clientConnection.Close()
	defer serverConnection.Close()
	serverErr := make(chan error, 1)
	go func() {
		if err := writeAll(serverConnection, []byte{0x53, 0x00, 0x00, 0x02, 'O', 'K'}); err != nil {
			serverErr <- err
			return
		}
		var probe [4]byte
		if _, err := io.ReadFull(serverConnection, probe[:]); err != nil {
			serverErr <- err
			return
		}
		serverErr <- writeAll(serverConnection, []byte{0x05, 0x00})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := waitForTCPConnect(ctx, clientConnection, bufio.NewReader(clientConnection)); err != nil {
		t.Fatalf("waitForTCPConnect() error = %v", err)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("TCP tunnel fixture: %v", err)
	}
}

func TestWaitForTCPConnectHonorsCancellation(t *testing.T) {
	clientConnection, serverConnection := net.Pipe()
	defer clientConnection.Close()
	defer serverConnection.Close()
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- waitForTCPConnect(ctx, clientConnection, bufio.NewReader(clientConnection))
	}()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("waitForTCPConnect() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled TCP tunnel setup did not return")
	}
}