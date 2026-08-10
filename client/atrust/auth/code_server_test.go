package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestVerificationCodeHandlerRequiresLocalHostAndFormToken(t *testing.T) {
	prompt := verificationPrompt{
		Title:     "输入短信验证码",
		Hint:      "测试提示",
		Token:     "secret-token",
		AllowSkip: true,
	}
	results := make(chan string, 1)
	handler := newVerificationCodeHandler(prompt, results)

	foreignRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43210/", nil)
	foreignRequest.Host = "attacker.example"
	foreignResponse := httptest.NewRecorder()
	handler.ServeHTTP(foreignResponse, foreignRequest)
	if foreignResponse.Code != http.StatusForbidden {
		t.Fatalf("foreign host status = %d, want %d", foreignResponse.Code, http.StatusForbidden)
	}

	pageRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:43210/", nil)
	pageResponse := httptest.NewRecorder()
	handler.ServeHTTP(pageResponse, pageRequest)
	if pageResponse.Code != http.StatusOK {
		t.Fatalf("page status = %d, want %d", pageResponse.Code, http.StatusOK)
	}
	if !strings.Contains(pageResponse.Body.String(), `value="secret-token"`) {
		t.Fatal("verification page is missing its form token")
	}
	if pageResponse.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("verification page is missing its content security policy")
	}

	wrongTokenResponse := postVerificationCode(handler, url.Values{
		"token": {"wrong-token"},
		"code":  {"123456"},
	})
	if wrongTokenResponse.Code != http.StatusForbidden {
		t.Fatalf("wrong-token status = %d, want %d", wrongTokenResponse.Code, http.StatusForbidden)
	}
	select {
	case code := <-results:
		t.Fatalf("wrong-token request submitted code %q", code)
	default:
	}

	acceptedResponse := postVerificationCode(handler, url.Values{
		"token": {prompt.Token},
		"code":  {"123456"},
		"skip":  {"1"},
	})
	if acceptedResponse.Code != http.StatusOK {
		t.Fatalf("accepted status = %d, want %d", acceptedResponse.Code, http.StatusOK)
	}
	select {
	case code := <-results:
		if code != "$123456" {
			t.Fatalf("submitted code = %q, want %q", code, "$123456")
		}
	default:
		t.Fatal("accepted form did not submit a verification code")
	}
}

func TestVerificationCodeHandlerRejectsMalformedCode(t *testing.T) {
	prompt := verificationPrompt{Token: "secret-token"}
	handler := newVerificationCodeHandler(prompt, make(chan string, 1))
	response := postVerificationCode(handler, url.Values{
		"token": {prompt.Token},
		"code":  {"123\n456"},
	})
	if response.Code != http.StatusBadRequest {
		t.Fatalf("malformed-code status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func postVerificationCode(handler http.Handler, values url.Values) *httptest.ResponseRecorder {
	request := httptest.NewRequest(
		http.MethodPost,
		"http://127.0.0.1:43210/submit",
		strings.NewReader(values.Encode()),
	)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
