package auth

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	mathrand "math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/client/authchallenge"
	"github.com/majianyu2007/nwafu-connect/log"
)

const (
	UserAgent    = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) aTrustTray/2.4.10.50 Chrome/83.0.4103.94 Electron/9.0.2 Safari/537.36 aTrustTray-Linux-Plat-Ubuntu-x64 SPCClientType"
	maxAttempts  = 5
	maxAuthSteps = 8
)

var sharedParams = url.Values{
	"clientType": {"SDPClient"},
	"platform":   {"Linux"},
	"lang":       {"en-US"},
}

func WithSharedParams(extra url.Values) url.Values {
	combined := make(url.Values, len(sharedParams)+len(extra))
	for k, v := range sharedParams {
		combined[k] = append([]string(nil), v...)
	}

	for k, v := range extra {
		for _, val := range v {
			// notice: not Add()
			combined.Set(k, val)
		}
	}

	return combined
}

type Cookie struct {
	Host   string `json:"host"`
	Scheme string `json:"scheme"`
	Name   string `json:"name"`
	Value  string `json:"value"`
}

type ClientAuthData struct {
	Cookies           []Cookie        `json:"cookies"`
	DeviceID          string          `json:"device_id"`
	MACAddresses      []string        `json:"mac_addresses,omitempty"`
	ServerVersionInfo json.RawMessage `json:"server_version_info,omitempty"`
}

type ServerVersionInfo struct {
	Code int `json:"code"`
	Data struct {
		Capacities struct {
			ZeroRTT struct {
				Version string `json:"version"`
			} `json:"zeroRTT"`
		} `json:"capacities"`
		Options struct {
			Tun0RTT struct {
				Enable bool `json:"enable"`
			} `json:"tun0rtt"`
		} `json:"options"`
	} `json:"data"`
}

func ParseServerVersionInfo(data []byte) (ServerVersionInfo, error) {
	var info ServerVersionInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return info, fmt.Errorf("failed to parse aTrust server manifest: %w", err)
	}
	if info.Code != 0 {
		return info, fmt.Errorf("aTrust server manifest failed with code %d", info.Code)
	}
	return info, nil
}

func (i ServerVersionInfo) TCPTunnelZeroRTT() bool {
	return i.Data.Capacities.ZeroRTT.Version != "" && i.Data.Options.Tun0RTT.Enable
}

// Prompter handles interactive authentication prompts (click captcha and
// verification codes) when running embedded in a host process without a
// terminal or local browser page.
type Prompter interface {
	GraphCheckCode(image []byte) (string, error)
	VerificationCode(kind, title, hint string, allowSkip bool) (string, error)
}

type prompterChallengeHandler struct {
	prompter Prompter
}

func newPrompterChallengeHandler(prompter Prompter) authchallenge.Handler {
	return prompterChallengeHandler{prompter: prompter}
}

func (h prompterChallengeHandler) HandleCodeChallenge(challenge authchallenge.CodeChallenge) (authchallenge.CodeResponse, error) {
	title := "输入验证码"
	switch challenge.Kind {
	case authchallenge.CodeSMS:
		title = "输入短信验证码"
	case authchallenge.CodeTOTP:
		title = "输入动态验证码"
	case authchallenge.CodeRadius:
		title = "输入 RADIUS 验证码"
	case authchallenge.CodeDevice:
		title = "选择要解绑的终端"
	}
	code, err := h.prompter.VerificationCode(string(challenge.Kind), title, challenge.Message, challenge.CanSkipSecondaryAuth)
	if err != nil {
		return authchallenge.CodeResponse{}, err
	}
	code = strings.TrimSpace(code)
	skip := false
	if challenge.CanSkipSecondaryAuth {
		if stripped, found := strings.CutPrefix(code, "$"); found {
			code = strings.TrimSpace(stripped)
			skip = true
		}
	}
	if code == "" {
		return authchallenge.CodeResponse{}, errors.New("empty verification code")
	}
	return authchallenge.CodeResponse{Code: code, SkipSecondaryAuth: skip}, nil
}

func (h prompterChallengeHandler) HandleTextCaptcha(challenge authchallenge.TextCaptchaChallenge) (authchallenge.TextCaptchaResponse, error) {
	return authchallenge.UnsupportedHandler{}.HandleTextCaptcha(challenge)
}

func (h prompterChallengeHandler) HandleClickCaptcha(challenge authchallenge.ClickCaptchaChallenge) (authchallenge.ClickCaptchaResponse, error) {
	raw, err := h.prompter.GraphCheckCode(challenge.Image)
	if err != nil {
		return authchallenge.ClickCaptchaResponse{}, err
	}
	return authchallenge.ParseClickCaptchaResponse(raw)
}

func (h prompterChallengeHandler) HandleExternalLogin(challenge authchallenge.ExternalLoginChallenge) (authchallenge.ExternalLoginResponse, error) {
	return authchallenge.UnsupportedHandler{}.HandleExternalLogin(challenge)
}

type Session struct {
	ctx          context.Context
	client       *http.Client
	deviceID     string
	macAddresses []string
	signKey      []byte
	username     string
	totpSecret   string

	baseHost string
	baseURL  string

	rid            string
	env            string
	csrfToken      string
	pubKey         string
	pubKeyExp      string
	antiReplayRand string
	ticket         string
	nextService    string

	response         map[string]json.RawMessage
	challengeHandler authchallenge.Handler
}

func NewSession(server string, tlsKeyLogWriter io.Writer, dialContext ...client.DialContextFunc) *Session {
	return NewSessionContext(context.Background(), server, tlsKeyLogWriter, dialContext...)
}

func NewSessionContext(ctx context.Context, server string, tlsKeyLogWriter io.Writer, dialContext ...client.DialContextFunc) *Session {
	if ctx == nil {
		ctx = context.Background()
	}
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if tr.TLSClientConfig == nil {
		tr.TLSClientConfig = &tls.Config{}
	} else {
		tr.TLSClientConfig = tr.TLSClientConfig.Clone()
	}
	tr.TLSClientConfig.KeyLogWriter = tlsKeyLogWriter
	if len(dialContext) > 0 && dialContext[0] != nil {
		tr.DialContext = dialContext[0]
	}
	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{Transport: tr, Jar: jar, Timeout: 20 * time.Second}

	return &Session{
		ctx:      ctx,
		client:   httpClient,
		baseHost: server,
		baseURL:  "https://" + server,
		rid:      base64.StdEncoding.EncodeToString([]byte(server)),
		response: make(map[string]json.RawMessage),
	}
}

func (s *Session) request(req *http.Request) (*http.Response, error) {
	if s.ctx == nil {
		return s.client.Do(req)
	}
	ctx, cancel := context.WithCancel(req.Context())
	stop := context.AfterFunc(s.ctx, cancel)

	resp, err := s.client.Do(req.WithContext(ctx))
	if err != nil {
		stop()
		cancel()
		return nil, err
	}

	resp.Body = &releaseOnClose{ReadCloser: resp.Body, release: func() { stop(); cancel() }}
	return resp, nil
}

type releaseOnClose struct {
	io.ReadCloser
	release func()
}

func (b *releaseOnClose) Close() error {
	err := b.ReadCloser.Close()
	b.release()
	return err
}

type QYWechatQrcodeConfig struct {
	AppID       string `json:"appid"`
	AgentID     string `json:"agentid"`
	RedirectURI string `json:"redirect_uri"`
	State       string `json:"state"`
}

type AuthInfo struct {
	LoginDomain            string               `json:"loginDomain"`
	AuthID                 string               `json:"authId"`
	AuthType               string               `json:"authType"`
	AuthName               string               `json:"authName"`
	SubType                string               `json:"subType"`
	LoginURL               string               `json:"loginUrl"`
	QYWechatQrcodeConf     QYWechatQrcodeConfig `json:"qywechatQrcodeConf,omitempty"`
	ThirdAuthQrcodeTimeout int                  `json:"thirdAuthQrcodeTimeout,omitempty"`
}

type LoginOptions struct {
	DeviceID         string
	MACAddresses     []string
	Cookies          []Cookie
	TOTPSecret       string
	ChallengeHandler authchallenge.Handler
	Prompter         Prompter
}

type LoginResult struct {
	Username string
	SID      string
	Cookies  []Cookie
}

type LoginMethod interface {
	AuthType() string
	LoginDomain() string
	login(s *Session, authInfo AuthInfo) error
}

func (s *Session) randSdpId() string {
	const chars = "0123456789abcdef"
	b := make([]byte, 32)
	for i := range b {
		b[i] = chars[mathrand.Intn(len(chars))]
	}
	return string(b)
}

func (s *Session) GetAuthInfoList() ([]AuthInfo, error) {
	_, authInfoList, err := s.authConfig(false, false)
	return authInfoList, err
}

func (s *Session) handler() authchallenge.Handler {
	if s.challengeHandler != nil {
		return s.challengeHandler
	}
	return authchallenge.NewCLIHandler(authchallenge.CLIOptions{})
}

func (s *Session) withGraphCheckCode(process func(graphCheckCode string) (int, error), graphCodeFile string) error {
	graphCheckCode := ""
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		graphCheckCodeEnable, err := process(graphCheckCode)
		if err != nil {
			return err
		}
		if graphCheckCodeEnable == 0 {
			return nil
		}
		if attempt == maxAttempts {
			return fmt.Errorf("graph check code failed after %d attempts", maxAttempts)
		}

		log.Println("Graph check code required")
		imgData, err := s.checkCode()
		if err != nil {
			return err
		}

		message := ""
		if graphCodeFile != "" {
			message = fmt.Sprintf("Graph check code saved to %s. Please enter the code:", graphCodeFile)
		}
		response, err := s.handler().HandleClickCaptcha(authchallenge.ClickCaptchaChallenge{
			Image:      imgData,
			OutputPath: graphCodeFile,
			Message:    message,
		})
		if err != nil {
			return fmt.Errorf("failed to get graph check code: %w", err)
		}

		graphCheckCode, err = graphCheckCodeFromResponse(response, imgData)
		if err != nil {
			return fmt.Errorf("invalid graph check code input: %w", err)
		}
	}
	return fmt.Errorf("graph check code failed after %d attempts", maxAttempts)
}

func (s *Session) continueAuth(step authStep) error {
	for i := 0; i < maxAuthSteps && step.Service != ""; i++ {
		var err error
		switch step.Service {
		case "auth/authCheck":
			step, err = s.authCheck()
		case "auth/sms":
			step, err = s.completeSMS(step)
		case "auth/customSms":
			step, err = s.completeCustomSMS()
		case "auth/totp":
			step, err = s.completeTOTP(step)
		case "auth/radius", "auth/challenge":
			step, err = s.completeRadius(step.Service)
		case "auth/accessCheck":
			step, err = s.accessCheck()
		case "auth/preEnhancedAuth", "auth/enhancedConfirm", "auth/enhancedDone":
			step, err = s.completeEnhancedAuth(step)
		case "auth/bindAuthDevice":
			step, err = s.bindAuthDevice()
		case "auth/token":
			return fmt.Errorf("unsupported token authentication subtype")
		default:
			return fmt.Errorf("unsupported authentication service: %s", step.Service)
		}
		if err != nil {
			return err
		}
	}
	if step.Service != "" {
		return fmt.Errorf("authentication did not complete after %d steps (next: %s)", maxAuthSteps, step.Service)
	}
	return nil
}

func (s *Session) completeSMS(step authStep) (authStep, error) {
	switch step.SMSMode {
	case smsWithAuthID:
		// Gateways that issue an authId require a mod=1 authConfig refresh before
		// querying the phone number and sending the SMS.
		if _, _, err := s.authConfig(true, true); err != nil {
			return authStep{}, err
		}
	case smsWithoutAuthID:
		// SARI-style gateways refresh auth config after sending the SMS.
	default:
		return authStep{}, fmt.Errorf("unknown SMS authentication mode")
	}

	phoneNumbers, err := s.phoneNumber(step.AuthID)
	if err != nil {
		log.Printf("Warning: failed to get phone number: %v", err)
	} else if len(phoneNumbers) > 0 {
		log.Printf("Found %d registered phone number(s) for SMS authentication", len(phoneNumbers))
	}

	if err := s.authSms(step); err != nil {
		return authStep{}, err
	}

	if step.SMSMode == smsWithoutAuthID {
		if _, _, err := s.authConfig(true, true); err != nil {
			return authStep{}, err
		}
	}

	return s.smsCheckCode(step)
}

func (s *Session) Login(method LoginMethod, opts LoginOptions) (LoginResult, error) {
	if len(opts.Cookies) > 0 {
		for _, cookie := range opts.Cookies {
			if cookie.Host != s.baseHost || cookie.Scheme != "https" || cookie.Name == "" {
				continue
			}
			c := &http.Cookie{
				Name:  cookie.Name,
				Value: cookie.Value,
			}
			s.client.Jar.SetCookies(&url.URL{Host: cookie.Host, Scheme: cookie.Scheme}, []*http.Cookie{c})
		}
	}

	s.deviceID = opts.DeviceID
	s.macAddresses = opts.MACAddresses
	s.totpSecret = opts.TOTPSecret
	switch {
	case opts.ChallengeHandler != nil:
		s.challengeHandler = opts.ChallengeHandler
	case opts.Prompter != nil:
		s.challengeHandler = newPrompterChallengeHandler(opts.Prompter)
	default:
		s.challengeHandler = authchallenge.NewCLIHandler(authchallenge.CLIOptions{})
	}
	envBytes, err := json.Marshal(struct {
		DeviceID string `json:"deviceId"`
	}{DeviceID: opts.DeviceID})
	if err != nil {
		return LoginResult{}, err
	}
	s.env = base64.StdEncoding.EncodeToString(envBytes)

	isLogin, authInfoList, err := s.authConfig(false, true)
	if err != nil {
		return LoginResult{}, err
	}
	if isLogin == 1 {
		log.Println("Already logged in")
		username, err := s.onlineInfo()
		if err != nil {
			return LoginResult{}, err
		}
		return s.loginResult(username), nil
	}

	if method == nil {
		return LoginResult{}, fmt.Errorf("login method is nil, but user is not logged in")
	}
	var foundAuthInfo *AuthInfo
	for _, authInfo := range authInfoList {
		if authInfo.AuthType == method.AuthType() && authInfo.LoginDomain == method.LoginDomain() {
			foundAuthInfo = &authInfo
			break
		}
	}
	if foundAuthInfo == nil {
		summaries := make([]string, 0, len(authInfoList))
		for _, info := range authInfoList {
			summaries = append(summaries, fmt.Sprintf("%s/%s (%s)", info.AuthType, info.LoginDomain, info.AuthName))
		}
		log.Printf("Available authentication methods: %s", strings.Join(summaries, ", "))
		return LoginResult{}, fmt.Errorf("auth type/login domain combination not found: auth type: %s, login domain: %s", method.AuthType(), method.LoginDomain())
	}

	log.Printf("Starting login with auth type: %s, login domain: %s", method.AuthType(), method.LoginDomain())
	err = method.login(s, *foundAuthInfo)
	if err != nil {
		return LoginResult{}, err
	}

	err = s.reportEnv()
	if err != nil {
		return LoginResult{}, err
	}

	nextService := s.nextService
	if nextService == "" {
		nextService = "auth/authCheck"
	}
	err = s.continueAuth(authStep{Service: nextService})
	if err != nil {
		return LoginResult{}, err
	}

	username, err := s.onlineInfo()
	if err != nil {
		return LoginResult{}, err
	}

	return s.loginResult(username), nil
}

func (s *Session) loginResult(username string) LoginResult {
	sid, cookies := sessionCookies(s)
	return LoginResult{
		Username: username,
		SID:      sid,
		Cookies:  cookies,
	}
}

func sessionCookies(s *Session) (string, []Cookie) {
	cookies := make([]Cookie, 0)
	sid := ""
	for _, cookie := range s.client.Jar.Cookies(&url.URL{Host: s.baseHost, Scheme: "https"}) {
		if cookie.Name == "sid" {
			sid = cookie.Value
		}
		cookies = append(cookies, Cookie{
			Host:   s.baseHost,
			Scheme: "https",
			Name:   cookie.Name,
			Value:  cookie.Value,
		})
	}
	return sid, cookies
}
