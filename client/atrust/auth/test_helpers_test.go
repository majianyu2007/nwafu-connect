package auth

import (
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTLSTestSession(server *httptest.Server) *Session {
	session := NewSession(strings.TrimPrefix(server.URL, "https://"), nil)
	if cert := server.Certificate(); cert != nil {
		pool := x509.NewCertPool()
		pool.AddCert(cert)
		if tr, ok := session.client.Transport.(*http.Transport); ok {
			tr.TLSClientConfig.RootCAs = pool
		}
	}
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
