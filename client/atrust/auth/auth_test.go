package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNewSessionRejectsUntrustedTLSCertificate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	session := NewSession(strings.TrimPrefix(server.URL, "https://"), nil)
	response, err := session.client.Get(server.URL)
	if err == nil {
		response.Body.Close()
		t.Fatal("authentication client accepted an untrusted TLS certificate")
	}
}

func TestLoginResultPersistsCurrentSessionCookies(t *testing.T) {
	session := NewSession("vpn.example.com", nil)
	session.client.Jar.SetCookies(
		&url.URL{Scheme: "https", Host: "vpn.example.com"},
		[]*http.Cookie{
			{Name: "sid", Value: "refreshed-session"},
			{Name: "route", Value: "node-a"},
		},
	)

	result := session.loginResult("student")
	if result.Username != "student" || result.SID != "refreshed-session" {
		t.Fatalf("login result = %#v", result)
	}
	if len(result.Cookies) != 2 {
		t.Fatalf("persisted cookie count = %d, want 2", len(result.Cookies))
	}
	for _, cookie := range result.Cookies {
		if cookie.Host != "vpn.example.com" || cookie.Scheme != "https" {
			t.Fatalf("persisted cookie scope = %#v", cookie)
		}
	}
}
