package authchallenge

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/majianyu2007/nwafu-connect/log"
)

type verificationPrompt struct {
	Title     string
	Hint      string
	Token     string
	AllowSkip bool
}

var verificationCodeTemplate = template.Must(template.New("verification-code").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>NWAFU Connect · {{.Title}}</title>
<style>
:root {
  color-scheme: light;
  --primary: #0066cc;
  --primary-focus: #0071e3;
  --ink: #1d1d1f;
  --ink-muted-80: #333333;
  --ink-muted-48: #7a7a7a;
  --divider-soft: #f0f0f0;
  --hairline: #e0e0e0;
  --canvas: #ffffff;
  --parchment: #f5f5f7;
  --black: #000000;
}
* { box-sizing: border-box; }
html { -webkit-font-smoothing: antialiased; -moz-osx-font-smoothing: grayscale; }
body {
  min-height: 100vh;
  margin: 0;
  display: flex;
  flex-direction: column;
  background: var(--parchment);
  color: var(--ink);
  font-family: "SF Pro Text", -apple-system, BlinkMacSystemFont, "PingFang SC", system-ui, sans-serif;
  font-size: 17px;
  font-weight: 400;
  line-height: 1.47;
  letter-spacing: -0.374px;
}
.global-nav {
  height: 44px;
  background: var(--black);
  color: #ffffff;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 0 24px;
}
.global-nav-inner {
  width: min(980px, 100%);
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  line-height: 1;
  letter-spacing: -0.12px;
}
.global-nav-brand { font-weight: 600; color: #ffffff; }
.global-nav-meta { color: #cccccc; }
.stage {
  flex: 1;
  display: grid;
  place-items: center;
  padding: 48px 24px;
}
main {
  width: min(100%, 440px);
  padding: 32px;
  border: 1px solid var(--hairline);
  border-radius: 18px;
  background: var(--canvas);
}
.eyebrow {
  margin-bottom: 8px;
  color: var(--primary);
  font-size: 12px;
  font-weight: 600;
  line-height: 1.29;
  letter-spacing: -0.12px;
}
h1 {
  margin: 0 0 10px;
  font-family: "SF Pro Display", -apple-system, BlinkMacSystemFont, "PingFang SC", system-ui, sans-serif;
  font-size: 28px;
  font-weight: 600;
  line-height: 1.14;
  letter-spacing: -0.28px;
  color: var(--ink);
}
p {
  margin: 0 0 24px;
  color: var(--ink-muted-48);
  font-size: 17px;
  line-height: 1.47;
  letter-spacing: -0.374px;
}
label.field-label {
  display: block;
  margin-bottom: 8px;
  font-size: 14px;
  font-weight: 600;
  line-height: 1.29;
  letter-spacing: -0.224px;
  color: var(--ink);
}
input[type="text"] {
  width: 100%;
  min-height: 48px;
  padding: 12px 18px;
  border: 1px solid var(--hairline);
  border-radius: 11px;
  outline: 0;
  background: var(--canvas);
  color: var(--ink);
  font: 600 20px ui-monospace, "SF Mono", SFMono-Regular, Menlo, monospace;
  letter-spacing: .14em;
  transition: border-color 0.14s ease;
}
input[type="text"]:focus {
  border-color: var(--primary-focus);
  outline: 2px solid var(--primary-focus);
  outline-offset: -1px;
}
.skip {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  margin: 16px 0 0;
  color: var(--ink-muted-80);
  font-size: 14px;
  font-weight: 400;
  line-height: 1.43;
  letter-spacing: -0.224px;
  cursor: pointer;
}
.skip input {
  width: 18px;
  height: 18px;
  margin-top: 2px;
  accent-color: var(--primary);
}
button {
  width: 100%;
  min-height: 46px;
  margin-top: 24px;
  padding: 11px 22px;
  border: 0;
  border-radius: 9999px;
  background: var(--primary);
  color: #ffffff;
  cursor: pointer;
  font-family: inherit;
  font-size: 17px;
  font-weight: 400;
  line-height: 1.24;
  letter-spacing: -0.374px;
  transition: transform 0.14s ease;
}
button:active { transform: scale(0.95); }
button:focus-visible { outline: 2px solid var(--primary-focus); outline-offset: 2px; }
.note {
  margin: 20px 0 0;
  color: var(--ink-muted-48);
  font-size: 12px;
  line-height: 1.4;
  letter-spacing: -0.12px;
  text-align: center;
}
@media (max-width: 480px) {
  .stage { padding: 24px 16px; }
  main { padding: 24px 20px; }
  h1 { font-size: 24px; }
}
@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after { transition-duration: .01ms !important; }
}
</style>
</head>
<body>
<header class="global-nav">
  <div class="global-nav-inner">
    <span class="global-nav-brand">NWAFU Connect</span>
    <span class="global-nav-meta">身份验证</span>
  </div>
</header>
<div class="stage">
<main>
  <div class="eyebrow">aTrust 二次认证</div>
  <h1>{{.Title}}</h1>
  <p>{{.Hint}}</p>
  <form method="post" action="/submit">
    <input type="hidden" name="token" value="{{.Token}}">
    <label class="field-label" for="code">验证码</label>
    <input id="code" name="code" type="text" inputmode="numeric" autocomplete="one-time-code" maxlength="64" required autofocus>
    {{if .AllowSkip}}<label class="skip"><input type="checkbox" name="skip" value="1"><span>本次跳过后续二次认证（网关支持时）</span></label>{{end}}
    <button type="submit">继续登录</button>
  </form>
  <div class="note">此页面仅在本机临时开放，提交后自动关闭。</div>
</main>
</div>
</body>
</html>`))

const verificationSuccessHTML = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>NWAFU Connect · 验证完成</title><style>*{box-sizing:border-box}body{min-height:100vh;margin:0;display:flex;flex-direction:column;background:#f5f5f7;color:#1d1d1f;font-family:"SF Pro Text",-apple-system,BlinkMacSystemFont,"PingFang SC",system-ui,sans-serif;-webkit-font-smoothing:antialiased}.global-nav{height:44px;background:#000;color:#fff;display:flex;align-items:center;justify-content:center;padding:0 24px}.global-nav-inner{width:min(980px,100%);display:flex;align-items:center;justify-content:space-between;font-size:12px;letter-spacing:-0.12px}.stage{flex:1;display:grid;place-items:center;padding:48px 24px}main{width:min(100%,440px);padding:32px;border:1px solid #e0e0e0;border-radius:18px;background:#fff;text-align:center}.eyebrow{margin-bottom:8px;color:#0066cc;font-size:12px;font-weight:600}strong{display:block;margin-bottom:10px;font-family:"SF Pro Display",-apple-system,BlinkMacSystemFont,"PingFang SC",system-ui,sans-serif;font-size:28px;font-weight:600;line-height:1.14;letter-spacing:-0.28px;color:#1d1d1f}span{color:#7a7a7a;font-size:17px;line-height:1.47;letter-spacing:-0.374px}</style></head><body><header class="global-nav"><div class="global-nav-inner"><strong>NWAFU Connect</strong><span style="color:#cccccc;font-size:12px">身份验证</span></div></header><div class="stage"><main><div class="eyebrow">验证已提交</div><strong>验证信息已提交</strong><span>可以关闭此页面并返回 NWAFU Connect。</span></main></div></body></html>`

func verificationPromptForChallenge(challenge CodeChallenge) verificationPrompt {
	title, hint := "输入验证码", strings.TrimSpace(challenge.Message)
	switch challenge.Kind {
	case CodeSMS:
		title = "输入短信验证码"
		if hint == "" || strings.HasPrefix(hint, "Please enter") {
			hint = "请填写手机收到的短信验证码以继续登录。"
		}
	case CodeTOTP:
		title = "输入动态验证码"
		if hint == "" || strings.HasPrefix(hint, "Please enter") {
			hint = "请输入身份验证器当前显示的一次性验证码。"
		}
	case CodeRadius:
		title = "输入 RADIUS 验证码"
		if hint == "" || strings.HasPrefix(hint, "Please enter") {
			hint = "请输入网关要求的 RADIUS 挑战口令。"
		}
	case CodeDevice:
		title = "选择要解绑的终端"
		if hint == "" {
			hint = "授信终端数量已达上限，请按提示输入对应编号。"
		}
	}
	return verificationPrompt{
		Title:     title,
		Hint:      hint,
		AllowSkip: challenge.CanSkipSecondaryAuth,
	}
}

func stdinCanProvideCode() bool {
	stdinInfo, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	nullInfo, err := os.Stat(os.DevNull)
	return err != nil || !os.SameFile(stdinInfo, nullInfo)
}

func newLocalFormToken() (string, error) {
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("generate local form token: %w", err)
	}
	return hex.EncodeToString(tokenBytes), nil
}

func serveVerificationCodeInBrowser(prompt verificationPrompt, timeout time.Duration, output io.Writer) (string, error) {
	if timeout <= 0 {
		return "", fmt.Errorf("verification code timeout must be positive")
	}
	token, err := newLocalFormToken()
	if err != nil {
		return "", err
	}
	prompt.Token = token
	resultCh := make(chan string, 1)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("start verification code server: %w", err)
	}
	server := &http.Server{
		Handler:           newVerificationCodeHandler(prompt, resultCh),
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       15 * time.Second,
	}
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			_ = server.Close()
		}
	}()

	serveErrCh := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrCh <- err
		}
	}()

	address := "http://" + listener.Addr().String() + "/"
	log.Printf("Open %s to continue authentication", address)
	OpenBrowser(address, output)

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case code := <-resultCh:
		return code, nil
	case err := <-serveErrCh:
		return "", fmt.Errorf("verification code server failed: %w", err)
	case <-timer.C:
		return "", fmt.Errorf("verification code input timed out after %v", timeout)
	}
}

func newVerificationCodeHandler(prompt verificationPrompt, resultCh chan<- string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		setVerificationCodeHeaders(writer.Header())
		if !isLoopbackRequestHost(request.Host) {
			http.Error(writer, "forbidden", http.StatusForbidden)
			return
		}
		if request.URL.Path != "/" {
			http.NotFound(writer, request)
			return
		}
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		writer.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := verificationCodeTemplate.Execute(writer, prompt); err != nil {
			log.Printf("Render verification code page failed: %v", err)
		}
	})
	mux.HandleFunc("/submit", func(writer http.ResponseWriter, request *http.Request) {
		setVerificationCodeHeaders(writer.Header())
		if !isLoopbackRequestHost(request.Host) {
			http.Error(writer, "forbidden", http.StatusForbidden)
			return
		}
		if request.Method != http.MethodPost {
			writer.Header().Set("Allow", http.MethodPost)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		request.Body = http.MaxBytesReader(writer, request.Body, 4<<10)
		if err := request.ParseForm(); err != nil {
			http.Error(writer, "invalid form", http.StatusBadRequest)
			return
		}
		if subtle.ConstantTimeCompare([]byte(request.PostForm.Get("token")), []byte(prompt.Token)) != 1 {
			http.Error(writer, "invalid form token", http.StatusForbidden)
			return
		}
		code := strings.TrimSpace(request.PostForm.Get("code"))
		prefixedSkip := false
		if prompt.AllowSkip {
			code, prefixedSkip = strings.CutPrefix(code, "$")
		}
		if !validVerificationCode(code) {
			http.Error(writer, "invalid verification code", http.StatusBadRequest)
			return
		}
		if prompt.AllowSkip && (prefixedSkip || request.PostForm.Get("skip") == "1") {
			code = "$" + code
		}
		select {
		case resultCh <- code:
			writer.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = writer.Write([]byte(verificationSuccessHTML))
		default:
			http.Error(writer, "already submitted", http.StatusConflict)
		}
	})
	return mux
}

func validVerificationCode(code string) bool {
	if code == "" || len(code) > 64 {
		return false
	}
	for _, character := range code {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func setVerificationCodeHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store, max-age=0")
	header.Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'; style-src 'unsafe-inline'")
	header.Set("Cross-Origin-Opener-Policy", "same-origin")
	header.Set("Cross-Origin-Resource-Policy", "same-origin")
	header.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(), usb=()")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
}

// IsLoopbackRequestHost reports whether an HTTP Host header targets loopback.
func IsLoopbackRequestHost(authority string) bool {
	return isLoopbackRequestHost(authority)
}

func isLoopbackRequestHost(authority string) bool {
	host, _, err := net.SplitHostPort(authority)
	if err != nil {
		return false
	}
	host = strings.TrimSuffix(strings.Trim(strings.TrimSpace(host), "[]"), ".")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
