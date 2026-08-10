package auth

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strconv"

	"github.com/majianyu2007/nwafu-connect/log"
)

type PasswordLogin struct {
	Username      string
	Password      string
	Domain        string
	GraphCodeFile string
}

func (m PasswordLogin) AuthType() string {
	return "auth/psw"
}

func (m PasswordLogin) LoginDomain() string {
	return m.Domain
}

func (m PasswordLogin) login(s *Session, _ AuthInfo) error {
	return s.loginAuthPsw(m.Username, m.Password, m.Domain, m.GraphCodeFile)
}

func (s *Session) loginAuthPsw(username, password, loginDomain, graphCodeFile string) error {
	process := func(graphCheckCode string) (int, error) {
		return s.pswImpl(username, password, loginDomain, graphCheckCode)
	}
	return s.withGraphCheckCode(process, graphCodeFile)
}

func (s *Session) pswImpl(username, password, loginDomain, graphCheckCode string) (int, error) {
	log.Println("Perform POST /passport/v1/auth/psw")

	N, ok := new(big.Int).SetString(s.pubKey, 16)
	if !ok || N.Sign() <= 0 {
		return 0, fmt.Errorf("invalid password encryption public key")
	}
	E, err := strconv.Atoi(s.pubKeyExp)
	if err != nil || E < 2 {
		return 0, fmt.Errorf("invalid password encryption public exponent %q", s.pubKeyExp)
	}
	pub := &rsa.PublicKey{N: N, E: E}

	msg := []byte(password + "_" + s.antiReplayRand)
	cipherBytes, err := rsa.EncryptPKCS1v15(rand.Reader, pub, msg)
	if err != nil {
		return 0, err
	}
	encryptedPwd := hex.EncodeToString(cipherBytes)

	data := map[string]interface{}{
		"username":    username + "@" + loginDomain,
		"password":    encryptedPwd,
		"rememberPwd": "0",
	}

	if graphCheckCode != "" {
		data["graphCheckCode"] = graphCheckCode
	}
	postBody, err := json.Marshal(data)
	if err != nil {
		return 0, fmt.Errorf("encode password authentication request: %w", err)
	}

	u := s.baseURL + "/passport/v1/auth/psw"
	req, err := http.NewRequest("POST", u+"?"+WithSharedParams(nil).Encode(), bytes.NewReader(postBody))
	if err != nil {
		return 0, fmt.Errorf("create password authentication request: %w", err)
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
	body, err := readAuthHTTPResponse(resp, "password authentication", 8<<20)
	if err != nil {
		return 0, err
	}
	log.DebugPrintf("Received psw: %s", string(body))

	var re struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Ticket               string `json:"ticket"`
			GraphCheckCodeEnable int    `json:"graphCheckCodeEnable"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &re); err != nil {
		return 0, fmt.Errorf("decode password authentication response: %w", err)
	}
	log.DebugPrintf("Parsed psw: %+v", re)
	if re.Code != 0 && re.Data.GraphCheckCodeEnable == 0 {
		return 0, fmt.Errorf("password authentication failed with code %d: %s", re.Code, re.Message)
	}
	if re.Data.GraphCheckCodeEnable == 0 && re.Data.Ticket == "" {
		return 0, fmt.Errorf("password authentication response did not include a ticket")
	}

	s.ticket = re.Data.Ticket
	return re.Data.GraphCheckCodeEnable, nil
}
