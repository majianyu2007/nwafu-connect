package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	_ "image/png"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	barcodeqr "github.com/boombuler/barcode/qr"
	"github.com/liyue201/goqr"
	"github.com/majianyu2007/nwafu-connect/log"
)

const (
	qyWechatQRCodeEndpoint = "https://open.work.weixin.qq.com/wwopen/sso/qrConnect"
	defaultQYWechatTimeout = 60 * time.Second
)
const qyWechatStatusScript = `
const statusElement = document.getElementById("status");
async function refreshStatus() {
	try {
		const response = await fetch("/status", { cache: "no-store" });
		if (!response.ok) {
			throw new Error("status request failed");
		}
		const status = await response.json();
		statusElement.textContent = status.message;
		statusElement.dataset.state = status.state;
		if (status.state !== "success" && status.state !== "error") {
			window.setTimeout(refreshStatus, 500);
		}
	} catch {
		statusElement.textContent = "无法读取认证状态，请查看终端日志。";
		statusElement.dataset.state = "error";
	}
}
refreshStatus();
`

var (
	qyWechatPage = template.Must(template.New("qywechat").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>NWAFU Connect 企业微信登录</title>
<style>
* { box-sizing: border-box; }
html, body { margin: 0; min-height: 100%; }
body { min-height: 100vh; display: grid; place-items: center; padding: 24px; background: #f5f7fa; color: #1f2329; font: 16px system-ui, sans-serif; }
main { width: min(460px, 100%); display: flex; flex-direction: column; align-items: center; padding: 32px 24px; text-align: center; background: white; border-radius: 12px; box-shadow: 0 8px 30px rgba(0,0,0,.08); }
h1 { margin: 0 0 12px; }
.instructions { margin: 0 0 24px; }
img { display: block; width: min(360px, 100%); height: auto; margin: 0 auto 20px; image-rendering: pixelated; }
#status { min-height: 24px; margin: 0; font-weight: 600; }
#status[data-state="processing"] { color: #245bdb; }
#status[data-state="success"] { color: #16803c; }
#status[data-state="error"] { color: #c62828; }
</style>
</head>
<body>
<main>
<h1>企业微信扫码登录</h1>
<p class="instructions">请使用企业微信扫描二维码并确认登录。</p>
<img src="/qrcode.png" alt="企业微信登录二维码">
<p id="status" role="status" aria-live="polite">等待扫码…</p>
</main>
<script src="/status.js"></script>
</body>
</html>`))
	qyWechatKeyPattern = regexp.MustCompile(`key\s*:\s*"([A-Za-z0-9_-]+)"`)
)

type QYWechatLogin struct {
	Domain      string
	QRCodeFile  string
	PrintQRCode bool
	OpenBrowser bool
}

func (m QYWechatLogin) AuthType() string {
	return "auth/qywechat"
}

func (m QYWechatLogin) LoginDomain() string {
	return m.Domain
}

func (m QYWechatLogin) login(s *Session, authInfo AuthInfo) error {
	return s.loginAuthQYWechat(authInfo, m)
}

type qyWechatScanSession struct {
	client      *http.Client
	origin      *url.URL
	key         string
	appID       string
	redirectURI string
	state       string
	image       []byte
}

type qyWechatPollResult struct {
	Status   string `json:"status"`
	AuthCode string `json:"auth_code"`
}

type qyWechatPageStatus struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

type qyWechatPageServer struct {
	address string
	server  *http.Server
	mu      sync.RWMutex
	status  qyWechatPageStatus
}

func (s *Session) loginAuthQYWechat(authInfo AuthInfo, method QYWechatLogin) error {
	qrURL, err := s.qyWechatQRCodeURL(authInfo, method.Domain)
	if err != nil {
		return err
	}

	timeout := time.Duration(authInfo.ThirdAuthQrcodeTimeout) * time.Second
	if timeout <= 0 {
		timeout = defaultQYWechatTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	scan, err := newQYWechatScanSession(ctx, qrURL, authInfo.QYWechatQrcodeConf)
	if err != nil {
		return err
	}
	if !method.PrintQRCode && method.QRCodeFile == "" && !method.OpenBrowser {
		return fmt.Errorf("enterprise WeChat QR code has no enabled output")
	}
	if method.QRCodeFile != "" {
		if err := saveQYWechatQRCode(method.QRCodeFile, scan.image); err != nil {
			return err
		}
		log.Printf("Enterprise WeChat QR code saved to %s", method.QRCodeFile)
	}
	if method.PrintQRCode {
		rendered, err := renderQRCodeTerminal(scan.image)
		if err != nil {
			return fmt.Errorf("render enterprise WeChat QR code: %w", err)
		}
		fmt.Println("Scan this QR code with enterprise WeChat:")
		fmt.Print(rendered)
	}

	var page *qyWechatPageServer
	if method.OpenBrowser {
		page, err = serveQYWechatPage(scan.image)
		if err != nil {
			return err
		}
		defer page.closeAfter(5 * time.Second)
		log.Printf("Open %s to scan the enterprise WeChat QR code", page.address)
		openBrowser(page.address)
	}
	setPageStatus := func(state, message string) {
		if page != nil {
			page.setStatus(state, message)
		}
	}
	failPage := func(message string, failure error) error {
		setPageStatus("error", message)
		if page != nil {
			time.Sleep(time.Second)
		}
		return failure
	}
	notifyScanStatus := func(status string) {
		switch status {
		case "QRCODE_SCAN_ING":
			setPageStatus("processing", "已扫码，请在企业微信中确认登录。")
		case "QRCODE_SCAN_SUCC":
			setPageStatus("processing", "扫码已确认，正在完成 VPN 认证…")
		case "QRCODE_SCAN_FAIL":
			setPageStatus("waiting", "登录已取消，请重新扫描二维码。")
		case "QRCODE_SCAN_NEVER":
			setPageStatus("waiting", "等待扫码…")
		case "QRCODE_SCAN_ERR":
			setPageStatus("error", "二维码已失效，请重新启动登录。")
		}
	}

	callback, err := scan.waitForCallback(ctx, notifyScanStatus)
	if err != nil {
		return failPage("扫码登录失败，请查看终端日志。", err)
	}
	if err := validateQYWechatCallbackURL(callback, s.baseHost, method.Domain, scan.state); err != nil {
		return failPage("登录回调校验失败，请查看终端日志。", err)
	}
	if err := s.qyWechat(callback.String()); err != nil {
		return failPage("企业微信认证失败，请查看终端日志。", err)
	}
	if _, _, err := s.authConfig(true, false); err != nil {
		return failPage("VPN 登录失败，请查看终端日志。", err)
	}
	setPageStatus("success", "认证成功，可以关闭此页面。")
	return nil
}

func (s *Session) qyWechatQRCodeURL(authInfo AuthInfo, loginDomain string) (string, error) {
	conf := authInfo.QYWechatQrcodeConf
	if conf.AppID == "" || conf.AgentID == "" || conf.RedirectURI == "" || conf.State == "" {
		return "", fmt.Errorf("incomplete enterprise WeChat QR code configuration")
	}
	redirectURL, err := url.Parse(conf.RedirectURI)
	if err != nil {
		return "", fmt.Errorf("invalid enterprise WeChat redirect URI: %w", err)
	}
	if err := validateQYWechatRedirectURL(redirectURL, s.baseHost, loginDomain); err != nil {
		return "", err
	}

	qrURL, _ := url.Parse(qyWechatQRCodeEndpoint)
	query := qrURL.Query()
	query.Set("appid", conf.AppID)
	query.Set("agentid", conf.AgentID)
	query.Set("redirect_uri", conf.RedirectURI)
	query.Set("state", conf.State)
	query.Set("login_type", "jssdk")
	query.Set("href", s.baseURL+"/portal/wechat_qrcode.css")
	qrURL.RawQuery = query.Encode()
	return qrURL.String(), nil
}

func newQYWechatScanSession(ctx context.Context, qrURL string, conf QYWechatQrcodeConfig) (*qyWechatScanSession, error) {
	client := &http.Client{Timeout: 65 * time.Second}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, qrURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("load enterprise WeChat QR code session: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("enterprise WeChat QR code session returned status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	match := qyWechatKeyPattern.FindSubmatch(body)
	if len(match) != 2 {
		return nil, fmt.Errorf("enterprise WeChat QR code key not found")
	}
	origin := &url.URL{Scheme: response.Request.URL.Scheme, Host: response.Request.URL.Host}
	imageURL := origin.ResolveReference(&url.URL{Path: "/wwopen/sso/qrImg", RawQuery: url.Values{"key": {string(match[1])}}.Encode()})
	imageData, err := fetchQYWechatQRCodeImage(ctx, client, imageURL)
	if err != nil {
		return nil, err
	}
	return &qyWechatScanSession{
		client:      client,
		origin:      origin,
		key:         string(match[1]),
		appID:       conf.AppID,
		redirectURI: conf.RedirectURI,
		state:       conf.State,
		image:       imageData,
	}, nil
}

func fetchQYWechatQRCodeImage(ctx context.Context, client *http.Client, imageURL *url.URL) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL.String(), nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("download enterprise WeChat QR code: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("enterprise WeChat QR code image returned status %d", response.StatusCode)
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "image/png") {
		return nil, fmt.Errorf("enterprise WeChat QR code image is not PNG")
	}
	imageData, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if _, _, err := image.Decode(bytes.NewReader(imageData)); err != nil {
		return nil, fmt.Errorf("decode enterprise WeChat QR code image: %w", err)
	}
	return imageData, nil
}

func (s *qyWechatScanSession) waitForCallback(ctx context.Context, notify func(string)) (*url.URL, error) {
	status := ""
	lastNotified := ""
	for {
		result, err := s.poll(ctx, status)
		if err != nil {
			return nil, err
		}
		if notify != nil && result.Status != lastNotified {
			notify(result.Status)
			lastNotified = result.Status
		}
		switch result.Status {
		case "QRCODE_SCAN_SUCC":
			if result.AuthCode == "" {
				return nil, fmt.Errorf("enterprise WeChat callback code is empty")
			}
			callback, err := url.Parse(s.redirectURI)
			if err != nil {
				return nil, err
			}
			query := callback.Query()
			query.Set("code", result.AuthCode)
			query.Set("state", s.state)
			query.Set("appid", s.appID)
			callback.RawQuery = query.Encode()
			return callback, nil
		case "QRCODE_SCAN_ING":
			if status != result.Status {
				log.Println("Enterprise WeChat QR code scanned; waiting for confirmation")
			}
			status = result.Status
		case "QRCODE_SCAN_FAIL":
			log.Println("Enterprise WeChat login was canceled; scan again to retry")
			status = result.Status
		case "QRCODE_SCAN_NEVER":
			status = ""
		case "QRCODE_SCAN_ERR":
			return nil, fmt.Errorf("enterprise WeChat QR code expired")
		default:
			return nil, fmt.Errorf("unknown enterprise WeChat QR code status: %s", result.Status)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("enterprise WeChat QR code timed out: %w", ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func (s *qyWechatScanSession) poll(ctx context.Context, status string) (qyWechatPollResult, error) {
	pollURL := s.origin.ResolveReference(&url.URL{Path: "/wwopen/sso/l/qrConnect"})
	query := pollURL.Query()
	query.Set("callback", "jsonpCallback")
	query.Set("key", s.key)
	query.Set("redirect_uri", s.redirectURI)
	query.Set("appid", s.appID)
	query.Set("_", fmt.Sprintf("%d", time.Now().UnixMilli()))
	if status != "" {
		query.Set("statusCode", status)
		query.Set("lastStatus", status)
	}
	pollURL.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, pollURL.String(), nil)
	if err != nil {
		return qyWechatPollResult{}, err
	}
	response, err := s.client.Do(request)
	if err != nil {
		return qyWechatPollResult{}, fmt.Errorf("poll enterprise WeChat QR code: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return qyWechatPollResult{}, fmt.Errorf("enterprise WeChat QR code poll returned status %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return qyWechatPollResult{}, err
	}
	const prefix = "jsonpCallback("
	payload := strings.TrimSpace(string(body))
	if !strings.HasPrefix(payload, prefix) || !strings.HasSuffix(payload, ")") {
		return qyWechatPollResult{}, fmt.Errorf("invalid enterprise WeChat QR code poll response")
	}
	payload = strings.TrimSuffix(strings.TrimPrefix(payload, prefix), ")")
	var result qyWechatPollResult
	if err := json.Unmarshal([]byte(payload), &result); err != nil {
		return qyWechatPollResult{}, err
	}
	return result, nil
}

func serveQYWechatPage(imageData []byte) (*qyWechatPageServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start enterprise WeChat QR code server: %w", err)
	}
	page := &qyWechatPageServer{
		address: "http://" + listener.Addr().String(),
		status:  qyWechatPageStatus{State: "waiting", Message: "等待扫码…"},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self'; style-src 'unsafe-inline'; script-src 'self'; connect-src 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if err := qyWechatPage.Execute(w, nil); err != nil {
			log.Printf("Render enterprise WeChat page failed: %v", err)
		}
	})
	mux.HandleFunc("/qrcode.png", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(imageData)
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(page.getStatus()); err != nil {
			log.Printf("Render enterprise WeChat status failed: %v", err)
		}
	})
	mux.HandleFunc("/status.js", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = io.WriteString(w, qyWechatStatusScript)
	})
	page.server = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := page.server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("Enterprise WeChat QR code server failed: %v", err)
		}
	}()
	return page, nil
}

func (s *qyWechatPageServer) setStatus(state, message string) {
	s.mu.Lock()
	s.status = qyWechatPageStatus{State: state, Message: message}
	s.mu.Unlock()
}

func (s *qyWechatPageServer) getStatus() qyWechatPageStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *qyWechatPageServer) close() {
	shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = s.server.Shutdown(shutdownContext)
}

func (s *qyWechatPageServer) closeAfter(delay time.Duration) {
	time.AfterFunc(delay, s.close)
}

func saveQYWechatQRCode(path string, imageData []byte) error {
	if err := os.WriteFile(path, imageData, 0600); err != nil {
		return fmt.Errorf("save enterprise WeChat QR code to %s: %w", path, err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("secure enterprise WeChat QR code file %s: %w", path, err)
	}
	return nil
}

func renderQRCodeTerminal(imageData []byte) (string, error) {
	decoded, _, err := image.Decode(bytes.NewReader(imageData))
	if err != nil {
		return "", err
	}
	results, err := goqr.Recognize(decoded)
	if err != nil {
		return "", fmt.Errorf("decode QR code payload: %w", err)
	}
	if len(results) != 1 || len(results[0].Payload) == 0 {
		return "", fmt.Errorf("expected one QR code payload, got %d", len(results))
	}
	encoded, err := barcodeqr.Encode(string(results[0].Payload), barcodeqr.M, barcodeqr.Auto)
	if err != nil {
		return "", fmt.Errorf("re-encode QR code for terminal: %w", err)
	}

	bounds := encoded.Bounds()
	coreSize := bounds.Dx()
	if coreSize == 0 || coreSize != bounds.Dy() {
		return "", fmt.Errorf("invalid terminal QR code geometry")
	}
	const quietZone = 2
	gridSize := coreSize + quietZone*2
	isDark := func(x, y int) bool {
		if x < quietZone || y < quietZone || x >= gridSize-quietZone || y >= gridSize-quietZone {
			return false
		}
		return darkPixel(encoded, bounds.Min.X+x-quietZone, bounds.Min.Y+y-quietZone)
	}

	var output strings.Builder
	for y := 0; y < gridSize; y += 2 {
		output.WriteString("\x1b[30;47m")
		for x := 0; x < gridSize; x++ {
			top := isDark(x, y)
			bottom := y+1 < gridSize && isDark(x, y+1)
			switch {
			case top && bottom:
				output.WriteRune('█')
			case top:
				output.WriteRune('▀')
			case bottom:
				output.WriteRune('▄')
			default:
				output.WriteByte(' ')
			}
		}
		output.WriteString("\x1b[0m\n")
	}
	return output.String(), nil
}

func darkPixel(value image.Image, x, y int) bool {
	red, green, blue, alpha := value.At(x, y).RGBA()
	return alpha >= 0x8000 && uint64(red)+uint64(green)+uint64(blue) < 3*0x8000
}

func validateQYWechatRedirectURL(redirectURL *url.URL, baseHost, loginDomain string) error {
	if redirectURL.Scheme != "https" || redirectURL.User != nil || redirectURL.Fragment != "" || !sameHTTPSAuthority(redirectURL, baseHost) {
		return fmt.Errorf("invalid enterprise WeChat redirect URI: host not match")
	}
	if redirectURL.Path != "/passport/v1/auth/qywechat" {
		return fmt.Errorf("invalid enterprise WeChat redirect URI: path not match")
	}
	domains := redirectURL.Query()["sfDomain"]
	if len(domains) != 1 || domains[0] != loginDomain {
		return fmt.Errorf("invalid enterprise WeChat redirect URI: login domain not match")
	}
	return nil
}

func validateQYWechatCallbackURL(callbackURL *url.URL, baseHost, loginDomain, state string) error {
	if err := validateQYWechatRedirectURL(callbackURL, baseHost, loginDomain); err != nil {
		return fmt.Errorf("%s", strings.Replace(err.Error(), "redirect URI", "callback URL", 1))
	}
	query := callbackURL.Query()
	codes := query["code"]
	if len(codes) != 1 || codes[0] == "" {
		return fmt.Errorf("invalid enterprise WeChat callback URL: code not found")
	}
	states := query["state"]
	if len(states) != 1 || states[0] != state {
		return fmt.Errorf("invalid enterprise WeChat callback URL: state not match")
	}
	return nil
}

func sameHTTPSAuthority(value *url.URL, baseHost string) bool {
	expected, err := url.Parse("https://" + baseHost)
	if err != nil {
		return false
	}
	valuePort := value.Port()
	if valuePort == "" {
		valuePort = "443"
	}
	expectedPort := expected.Port()
	if expectedPort == "" {
		expectedPort = "443"
	}
	return strings.EqualFold(value.Hostname(), expected.Hostname()) && valuePort == expectedPort
}

func (s *Session) qyWechat(callback string) error {
	log.Println("Perform GET /passport/v1/auth/qywechat")
	request, err := http.NewRequest(http.MethodGet, callback, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", UserAgent)
	request.Header.Set("x-csrf-token", s.csrfToken)
	request.Header.Set("x-sdp-rid", s.rid)
	request.Header.Set("x-sdp-traceid", s.randSdpId())

	previousCheckRedirect := s.client.CheckRedirect
	s.client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	defer func() { s.client.CheckRedirect = previousCheckRedirect }()

	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	log.DebugPrintf("Received enterprise WeChat callback data: %s", string(body))
	if response.StatusCode != http.StatusFound {
		return fmt.Errorf("enterprise WeChat callback returned status %d", response.StatusCode)
	}
	ticket, err := parsePortalTicketFromRedirect(response.Header.Get("Location"), s.baseHost)
	if err != nil {
		return err
	}
	s.ticket = ticket
	return nil
}
