package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/majianyu2007/nwafu-connect/log"
	"github.com/pquerna/otp/totp"
)

func (s *Session) completeCampusTOTP(step authStep) (authStep, error) {
	var code string
	if s.totpSecret == "" {
		var err error
		code, err = readVerificationCode(
			"输入动态验证码",
			"请输入身份验证器当前显示的一次性验证码。",
			false,
		)
		if err != nil {
			return authStep{}, err
		}
	} else {
		var err error
		code, err = totp.GenerateCode(s.totpSecret, time.Now())
		if err != nil {
			return authStep{}, fmt.Errorf("generate TOTP code: %w", err)
		}
	}

	return s.checkTOTP(step, code)
}

func (s *Session) checkTOTP(step authStep, code string) (authStep, error) {
	log.Println("Perform POST /passport/v1/auth/token")

	payload := struct {
		Action            string `json:"action"`
		TaskID            string `json:"taskId"`
		TOTPToken         string `json:"totpToken"`
		IsPrevEffect      bool   `json:"isPrevEffect"`
		AuthID            string `json:"authId"`
		SkipSecondaryAuth string `json:"skipSecondaryAuth"`
	}{
		Action:            "auth",
		TaskID:            step.TaskID,
		TOTPToken:         code,
		IsPrevEffect:      step.IsPrevEffect,
		AuthID:            step.AuthID,
		SkipSecondaryAuth: "0",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return authStep{}, err
	}

	u := s.baseURL + "/passport/v1/auth/token"
	req, err := http.NewRequest("POST", u+"?"+WithSharedParams(nil).Encode(), bytes.NewReader(body))
	if err != nil {
		return authStep{}, err
	}
	s.setAuthJSONHeaders(req)
	if s.env != "" {
		req.Header.Set("x-sdp-env", s.env)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return authStep{}, err
	}
	defer resp.Body.Close()

	body, err = readAuthHTTPResponse(resp, "TOTP authentication", 8<<20)
	if err != nil {
		return authStep{}, err
	}
	log.DebugPrintf("Received TOTP response: %s", string(body))

	var result struct {
		Code    int          `json:"code"`
		Message string       `json:"message"`
		Data    authStepData `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return authStep{}, err
	}
	if result.Code != 0 {
		return authStep{}, fmt.Errorf("TOTP authentication failed with code %d: %s", result.Code, result.Message)
	}

	return authStepFromData(result.Data), nil
}
