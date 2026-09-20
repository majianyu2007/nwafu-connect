package auth

import (
	"crypto/rand"
	"errors"
	"crypto/rsa"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestPasswordAuthenticationReturnsGatewayFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/passport/v1/auth/psw" {
			http.NotFound(writer, request)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":401,"message":"invalid credentials","data":{}}`))
	}))
	defer server.Close()

	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatal(err)
	}
	session := testSessionForServer(server)
	session.pubKey = key.PublicKey.N.Text(16)
	session.pubKeyExp = strconv.Itoa(key.PublicKey.E)

	_, err = session.pswImpl("student", "wrong-password", "LDAP", "")
	if err == nil || !strings.Contains(err.Error(), "invalid credentials") {
		t.Fatalf("password failure = %v, want gateway error", err)
	}
	if session.ticket != "" {
		t.Fatalf("failed password authentication stored ticket %q", session.ticket)
	}
}

func TestLegacySMSFlowReturnsGatewayFailures(t *testing.T) {
	tests := []struct {
		name string
		path string
		call func(*Session) error
	}{
		{
			name: "send",
			path: "/passport/v1/public/sendSms",
			call: func(session *Session) error {
				_, err := session.sendSms("13800138000", "LDAP", "")
				return err
			},
		},
		{
			name: "verify",
			path: "/passport/v1/auth/smsCheckCode",
			call: func(session *Session) error {
				_, err := session.smsCheckCodeImpl("123456", "13800138000", "LDAP", "")
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != test.path {
					http.NotFound(writer, request)
					return
				}
				writer.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(writer, `{"code":429,"message":"try later","data":{"graphCheckCodeEnable":0}}`)
			}))
			defer server.Close()

			err := test.call(testSessionForServer(server))
			if err == nil || !strings.Contains(err.Error(), "try later") {
				t.Fatalf("SMS failure = %v, want gateway error", err)
			}
		})
	}
}

func TestLegacySMSFlowReturnsCaptchaChallenge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":471,"message":"captcha required","data":{"graphCheckCodeEnable":1}}`))
	}))
	defer server.Close()

	challenge, err := testSessionForServer(server).sendSms("13800138000", "LDAP", "")
	if err != nil {
		t.Fatalf("captcha challenge returned error: %v", err)
	}
	if challenge != 1 {
		t.Fatalf("captcha challenge = %d, want 1", challenge)
	}
}

func TestAuthConfigReturnsGatewayFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"code":503,"message":"gateway unavailable"}`))
	}))
	defer server.Close()

	_, _, err := testSessionForServer(server).authConfig(false, true)
	if err == nil || !strings.Contains(err.Error(), "gateway unavailable") {
		t.Fatalf("auth config failure = %v, want gateway error", err)
	}
}

func testSessionForServer(server *httptest.Server) *Session {
	session := NewSession("vpn.example.com", nil)
	session.baseURL = server.URL
	session.client = server.Client()
	return session
}


type failingAuthResponseReader struct {
	err error
}

func (reader failingAuthResponseReader) Read([]byte) (int, error) {
	return 0, reader.err
}

func TestReadAuthHTTPResponseRejectsTransportStatusAndSizeFailures(t *testing.T) {
	readFailure := errors.New("connection reset")
	tests := []struct {
		name     string
		response *http.Response
		limit    int64
		want     string
	}{
		{
			name:     "read failure",
			response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(failingAuthResponseReader{err: readFailure})},
			limit:    64,
			want:     readFailure.Error(),
		},
		{
			name:     "HTTP failure",
			response: &http.Response{StatusCode: http.StatusBadGateway, Body: io.NopCloser(strings.NewReader("upstream failed"))},
			limit:    64,
			want:     "HTTP status 502",
		},
		{
			name:     "oversized body",
			response: &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("12345"))},
			limit:    4,
			want:     "exceeds 4 bytes",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer test.response.Body.Close()
			_, err := readAuthHTTPResponse(test.response, "test operation", test.limit)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("readAuthHTTPResponse() error = %v, want %q", err, test.want)
			}
		})
	}
}