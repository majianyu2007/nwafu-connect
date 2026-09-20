package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/majianyu2007/nwafu-connect/client/authchallenge"
	"github.com/majianyu2007/nwafu-connect/log"
)

type SMSLogin struct {
	Phone         string
	Domain        string
	GraphCodeFile string
}

func (m SMSLogin) AuthType() string {
	return "auth/smsCheckCode"
}

func (m SMSLogin) LoginDomain() string {
	return m.Domain
}

func (m SMSLogin) login(s *Session, _ AuthInfo) error {
	if err := s.loginAuthSmsCheckCode(m.Phone, m.Domain, m.GraphCodeFile); err != nil {
		return err
	}
	s.username = smsUsername(m.Phone, m.Domain)
	return nil
}

func smsUsername(phone, domain string) string {
	return phone + "@" + domain
}

func (s *Session) loginAuthSmsCheckCode(phone, loginDomain, graphCodeFile string) error {
	sendSmsProcess := func(graphCheckCode string) (int, error) {
		return s.sendSms(phone, loginDomain, graphCheckCode)
	}
	err := s.withGraphCheckCode(sendSmsProcess, graphCodeFile)
	if err != nil {
		return err
	}

	challenge := authchallenge.CodeChallenge{
		Kind:    authchallenge.CodeSMS,
		Message: "Please enter the SMS verification code:",
	}
	response, err := s.challengeHandler.HandleCodeChallenge(challenge)
	if err != nil {
		return fmt.Errorf("complete primary SMS challenge: %w", err)
	}
	smsCheckCodeProcess := func(graphCheckCode string) (int, error) {
		return s.smsCheckCodeImpl(response.Code, phone, loginDomain, graphCheckCode)
	}
	return s.withGraphCheckCode(smsCheckCodeProcess, graphCodeFile)
}

func (s *Session) sendSms(phone, loginDomain, graphCheckCode string) (int, error) {
	log.Println("Perform POST /passport/v1/public/sendSms")

	data := map[string]interface{}{
		"phone":          phone + "@" + loginDomain,
		"graphCheckCode": graphCheckCode,
	}

	postBody, err := json.Marshal(data)
	if err != nil {
		return 0, fmt.Errorf("encode SMS send request: %w", err)
	}

	u := s.baseURL + "/passport/v1/public/sendSms"
	req, err := http.NewRequest("POST", u+"?"+WithSharedParams(nil).Encode(), bytes.NewReader(postBody))
	if err != nil {
		return 0, fmt.Errorf("create SMS send request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json;charset=utf-8")
	req.Header.Set("x-csrf-token", s.csrfToken)
	req.Header.Set("x-sdp-env", s.env)
	req.Header.Set("x-sdp-traceid", s.randSdpId())

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, err := readAuthHTTPResponse(resp, "SMS send", 8<<20)
	if err != nil {
		return 0, err
	}
	log.DebugPrintf("Received sendSms: %s", string(body))

	var re struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Tips                 string `json:"tips"`
			Interval             string `json:"interval"`
			GraphCheckCodeEnable int    `json:"graphCheckCodeEnable"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &re); err != nil {
		return 0, fmt.Errorf("decode SMS send response: %w", err)
	}
	log.DebugPrintf("Parsed sendSms: %+v", re)
	if re.Code != 0 && re.Data.GraphCheckCodeEnable == 0 {
		return 0, fmt.Errorf("send SMS failed with code %d: %s", re.Code, re.Message)
	}

	return re.Data.GraphCheckCodeEnable, nil
}

func (s *Session) smsCheckCodeImpl(code, phone, loginDomain, graphCheckCode string) (int, error) {
	log.Println("Perform POST /passport/v1/auth/smsCheckCode")

	data := map[string]interface{}{
		"code":  code,
		"phone": phone + "@" + loginDomain,
	}

	if graphCheckCode != "" {
		data["graphCheckCode"] = graphCheckCode
	}
	postBody, err := json.Marshal(data)
	if err != nil {
		return 0, fmt.Errorf("encode SMS verification request: %w", err)
	}

	u := s.baseURL + "/passport/v1/auth/smsCheckCode"
	req, err := http.NewRequest("POST", u+"?"+WithSharedParams(nil).Encode(), bytes.NewReader(postBody))
	if err != nil {
		return 0, fmt.Errorf("create SMS verification request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json;charset=utf-8")
	req.Header.Set("x-csrf-token", s.csrfToken)
	req.Header.Set("x-sdp-env", s.env)
	req.Header.Set("x-sdp-traceid", s.randSdpId())

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)
	body, err := readAuthHTTPResponse(resp, "SMS verification", 8<<20)
	if err != nil {
		return 0, err
	}
	log.DebugPrintf("Received smsCheckCode: %s", string(body))

	var re struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Ticket               string `json:"ticket"`
			GraphCheckCodeEnable int    `json:"graphCheckCodeEnable"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &re); err != nil {
		return 0, fmt.Errorf("decode SMS verification response: %w", err)
	}
	log.DebugPrintf("Parsed smsCheckCode: %+v", re)
	if re.Code != 0 && re.Data.GraphCheckCodeEnable == 0 {
		return 0, fmt.Errorf("SMS verification failed with code %d: %s", re.Code, re.Message)
	}
	if re.Data.GraphCheckCodeEnable == 0 && re.Data.Ticket == "" {
		return 0, fmt.Errorf("SMS verification response did not include a ticket")
	}

	if re.Data.Ticket == "" && re.Data.GraphCheckCodeEnable == 0 {
		return 0, fmt.Errorf("SMS authentication succeeded without a ticket")
	}
	s.ticket = re.Data.Ticket
	return re.Data.GraphCheckCodeEnable, nil
}
