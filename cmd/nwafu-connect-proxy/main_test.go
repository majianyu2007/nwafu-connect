package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

func TestProxyStreamCarriesStdioThroughConnect(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverDone := make(chan error, 1)
	go func() {
		connection, err := listener.Accept()
		if err != nil {
			serverDone <- err
			return
		}
		defer connection.Close()
		request, err := http.ReadRequest(bufio.NewReader(connection))
		if err != nil {
			serverDone <- err
			return
		}
		if request.Method != http.MethodConnect || request.Host != "ssh.internal:22" {
			serverDone <- fmt.Errorf("unexpected CONNECT request: %s %s", request.Method, request.Host)
			return
		}
		if _, err := io.WriteString(connection, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
			serverDone <- err
			return
		}
		payload := make([]byte, 4)
		if _, err := io.ReadFull(connection, payload); err != nil {
			serverDone <- err
			return
		}
		if string(payload) != "ping" {
			serverDone <- fmt.Errorf("tunneled payload = %q", payload)
			return
		}
		_, err = io.WriteString(connection, "pong")
		serverDone <- err
	}()

	var output bytes.Buffer
	if err := proxyStream(listener.Addr().String(), "ssh.internal:22", strings.NewReader("ping"), &output); err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if output.String() != "pong" {
		t.Fatalf("tunneled response = %q, want pong", output.String())
	}
}

func TestProxyStreamRejectsInvalidTarget(t *testing.T) {
	if err := proxyStream("127.0.0.1:1", "missing-port", strings.NewReader(""), io.Discard); err == nil || !strings.Contains(err.Error(), "invalid target address") {
		t.Fatalf("proxyStream error = %v", err)
	}
}
