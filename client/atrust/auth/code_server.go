package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/majianyu2007/nwafu-connect/log"
)

const verificationCodeTimeout = 5 * time.Minute

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
:root { color-scheme: light; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; color: #17251f; background: #eef4f1; }
* { box-sizing: border-box; }
body { min-height: 100vh; margin: 0; display: grid; place-items: center; padding: 24px; }
main { width: min(100%, 420px); padding: 30px; border: 1px solid #d5e1dc; border-radius: 18px; background: #fff; box-shadow: 0 18px 55px rgba(20, 58, 45, .12); }
.brand { margin-bottom: 28px; color: #0d4f3c; font-size: 13px; font-weight: 700; letter-spacing: .08em; }
h1 { margin: 0 0 9px; font-size: 25px; letter-spacing: -.02em; }
p { margin: 0 0 22px; color: #65756e; font-size: 14px; line-height: 1.65; }
label { display: block; margin-bottom: 8px; font-size: 13px; font-weight: 650; }
input[type="text"] { width: 100%; min-height: 50px; padding: 0 14px; border: 1px solid #bdcec7; border-radius: 10px; outline: 0; color: #17251f; font: 600 19px ui-monospace, SFMono-Regular, Menlo, monospace; letter-spacing: .12em; }
input[type="text"]:focus { border-color: #0d4f3c; box-shadow: 0 0 0 3px rgba(13, 79, 60, .14); }
.skip { display: flex; gap: 9px; align-items: flex-start; margin: 16px 0 0; color: #52645c; font-size: 13px; font-weight: 500; line-height: 1.45; }
.skip input { margin-top: 2px; accent-color: #0d4f3c; }
button { width: 100%; min-height: 48px; margin-top: 22px; border: 0; border-radius: 10px; background: #0d4f3c; color: #fff; cursor: pointer; font-size: 15px; font-weight: 700; }
button:hover { background: #0a3f30; }
button:focus-visible { outline: 3px solid #2e705c; outline-offset: 3px; }
.note { margin: 18px 0 0; color: #829089; font-size: 12px; text-align: center; }
</style>
</head>
<body>
<main>
<div class="brand">NWAFU CONNECT</div>
<h1>{{.Title}}</h1>
<p>{{.Hint}}</p>
<form method="post" action="/submit">
<input type="hidden" name="token" value="{{.Token}}">
<label for="code">验证码</label>
<input id="code" name="code" type="text" inputmode="numeric" autocomplete="one-time-code" maxlength="64" required autofocus>
{{if .AllowSkip}}<label class="skip"><input type="checkbox" name="skip" value="1"><span>本次跳过后续二次认证（网关支持时）</span></label>{{end}}
<button type="submit">继续登录</button>
</form>
<div class="note">页面仅在本机临时开放，提交后会自动关闭。</div>
</main>
</body>
</html>`))

const verificationSuccessHTML = `<!doctype html><html lang="zh-CN"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>验证完成</title><style>body{min-height:100vh;margin:0;display:grid;place-items:center;background:#eef4f1;color:#17251f;font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}main{padding:32px;text-align:center}strong{display:block;margin-bottom:8px;color:#0d4f3c;font-size:24px}span{color:#65756e}</style></head><body><main><strong>验证信息已提交</strong><span>可以关闭此页面并返回 NWAFU Connect。</span></main></body></html>`

func readVerificationCode(title, hint string, allowSkip bool) (string, error) {
	if stdinCanProvideCode() {
		log.Printf("%s: ", title)
		var code string
		if _, err := fmt.Scanln(&code); err == nil {
			code = strings.TrimSpace(code)
			if code != "" {
				return code, nil
			}
		} else {
			log.DebugPrintf("Terminal verification-code input unavailable: %v", err)
		}
	}
	return serveVerificationCodeInBrowser(verificationPrompt{
		Title:     title,
		Hint:      hint,
		AllowSkip: allowSkip,
	}, verificationCodeTimeout)
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

func serveVerificationCodeInBrowser(prompt verificationPrompt, timeout time.Duration) (string, error) {
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
	openBrowser(address)

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
