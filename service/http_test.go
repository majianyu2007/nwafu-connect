package service

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/majianyu2007/nwafu-connect/dial"
	"github.com/majianyu2007/nwafu-connect/internal/hook_func"
)

func TestHTTPConnectFlushesSmallServerResponse(t *testing.T) {
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()

	releaseTarget := make(chan struct{})
	go func() {
		connection, acceptErr := target.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		_, _ = connection.Write([]byte("ok"))
		<-releaseTarget
	}()

	proxyAddress, err := StartHTTP("127.0.0.1:0", dial.NewDialer(nil, nil, nil, false, ""))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		close(releaseTarget)
		hook_func.ExecTerminalFunc(context.Background())
	}()

	connection, err := net.Dial("tcp", proxyAddress)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(connection, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", target.Addr(), target.Addr()); err != nil {
		t.Fatal(err)
	}

	reader := bufio.NewReader(connection)
	status, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, " 200 ") {
		t.Fatalf("CONNECT status = %q, want 200", strings.TrimSpace(status))
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	payload := make([]byte, 2)
	if _, err := io.ReadFull(reader, payload); err != nil {
		t.Fatalf("small tunneled response was not flushed: %v", err)
	}
	if string(payload) != "ok" {
		t.Fatalf("tunneled payload = %q, want ok", payload)
	}
}

func TestHTTPProxyReturnsForbiddenForUnauthorizedDestination(t *testing.T) {
	request := httptest.NewRequest(http.MethodConnect, "http://proxy.invalid", nil)
	request.Host = "203.0.113.10:443"
	response := httptest.NewRecorder()

	newHTTPProxyHandler(dial.NewDialer(nil, nil, nil, true, "")).ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("proxy status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestHTTPProxyTransportHasBoundedPool(t *testing.T) {
	proxy := newHTTPProxy(dial.NewDialer(nil, nil, nil, false, ""))
	defer proxy.close()
	transport, ok := proxy.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", proxy.client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("managed proxy transport inherited an environment proxy")
	}
	if transport.MaxIdleConns <= 0 || transport.MaxIdleConnsPerHost <= 0 {
		t.Fatalf("idle limits = global %d, per-host %d; want positive", transport.MaxIdleConns, transport.MaxIdleConnsPerHost)
	}
	if transport.MaxConnsPerHost <= 0 {
		t.Fatalf("MaxConnsPerHost = %d, want positive", transport.MaxConnsPerHost)
	}
	if transport.IdleConnTimeout <= 0 || transport.ResponseHeaderTimeout <= 0 {
		t.Fatal("outbound connection timeouts are not configured")
	}
}

func TestHTTPProxyCapsConcurrentTunnels(t *testing.T) {
	proxy := newHTTPProxy(dial.NewDialer(nil, nil, nil, false, ""))
	defer proxy.close()
	for index := range httpProxyMaxTunnels {
		if !proxy.acquireTunnel() {
			t.Fatalf("tunnel slot %d was rejected before the limit", index)
		}
	}
	if proxy.acquireTunnel() {
		proxy.releaseTunnel()
		t.Fatal("proxy accepted a tunnel beyond its configured limit")
	}
	for range httpProxyMaxTunnels {
		proxy.releaseTunnel()
	}
}

func TestHTTPProxyCloseCancelsPendingConnect(t *testing.T) {
	proxy := newHTTPProxy(dial.NewDialer(nil, nil, nil, false, ""))
	dialStarted := make(chan struct{})
	proxy.dialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		close(dialStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	request := httptest.NewRequest(http.MethodConnect, "http://proxy.invalid", nil)
	request.Host = "192.0.2.10:443"
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		proxy.ServeHTTP(response, request)
		close(done)
	}()
	select {
	case <-dialStarted:
	case <-time.After(time.Second):
		t.Fatal("proxy CONNECT dial did not start")
	}
	proxy.close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("proxy close did not cancel the pending CONNECT")
	}
}
