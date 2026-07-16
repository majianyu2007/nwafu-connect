package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthStepFromDataMapsTOTP(t *testing.T) {
	step := authStepFromData(authStepData{
		NextService: "auth/token",
		TaskID:      "task-1",
		NextServiceList: []authServiceInfo{{
			AuthID:   "auth-1",
			AuthType: "auth/token",
			SubType:  "totp",
		}},
	})

	if step.Service != "auth/totp" {
		t.Fatalf("service = %q, want auth/totp", step.Service)
	}
	if step.AuthID != "auth-1" {
		t.Fatalf("auth ID = %q, want auth-1", step.AuthID)
	}
	if step.TaskID != "task-1" {
		t.Fatalf("task ID = %q, want task-1", step.TaskID)
	}
}

func TestCheckTOTPSendsGatewayPayload(t *testing.T) {
	type requestPayload struct {
		Action            string `json:"action"`
		TaskID            string `json:"taskId"`
		TOTPToken         string `json:"totpToken"`
		IsPrevEffect      bool   `json:"isPrevEffect"`
		AuthID            string `json:"authId"`
		SkipSecondaryAuth string `json:"skipSecondaryAuth"`
	}

	var got requestPayload
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/passport/v1/auth/token" {
			t.Errorf("path = %q, want /passport/v1/auth/token", r.URL.Path)
		}
		if r.URL.Query().Get("clientType") != "SDPClient" {
			t.Errorf("clientType = %q, want SDPClient", r.URL.Query().Get("clientType"))
		}
		if r.Header.Get("x-csrf-token") != "csrf" {
			t.Errorf("x-csrf-token = %q, want csrf", r.Header.Get("x-csrf-token"))
		}
		if r.Header.Get("x-sdp-env") != "env" {
			t.Errorf("x-sdp-env = %q, want env", r.Header.Get("x-sdp-env"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"","data":{}}`))
	}))
	defer server.Close()

	session := &Session{
		client:    server.Client(),
		baseURL:   server.URL,
		csrfToken: "csrf",
		env:       "env",
	}
	_, err := session.checkTOTP(authStep{
		AuthID:       "auth-1",
		TaskID:       "task-1",
		IsPrevEffect: true,
	}, "123456")
	if err != nil {
		t.Fatal(err)
	}

	want := requestPayload{
		Action:            "auth",
		TaskID:            "task-1",
		TOTPToken:         "123456",
		IsPrevEffect:      true,
		AuthID:            "auth-1",
		SkipSecondaryAuth: "0",
	}
	if got != want {
		t.Fatalf("payload = %#v, want %#v", got, want)
	}
}
