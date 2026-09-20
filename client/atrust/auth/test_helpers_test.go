package auth

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTLSTestSession(server *httptest.Server) *Session {
	session := NewSession(strings.TrimPrefix(server.URL, "https://"), nil)
	session.client = server.Client()
	jar, _ := cookiejar.New(nil)
	session.client.Jar = jar
	return session
}

func TestNewSessionUsesTLSKeyLogWriter(t *testing.T) {
	writer := new(strings.Builder)
	session := NewSession("vpn.example.com", writer)
	transport, ok := session.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", session.client.Transport)
	}
	if transport.TLSClientConfig.KeyLogWriter != writer {
		t.Fatal("session transport did not preserve the TLS key-log writer")
	}
}
