package authchallenge

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/majianyu2007/nwafu-connect/log"
)

const captchaTokenPlaceholder = "__NWAFU_CAPTCHA_FORM_TOKEN__"

const captchaPageHTML = `<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>NWAFU Connect · 图形验证</title>
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
  --pearl: #fafafc;
  --black: #000000;
}
* { margin: 0; padding: 0; box-sizing: border-box; }
html { -webkit-font-smoothing: antialiased; -moz-osx-font-smoothing: grayscale; }
body {
  min-height: 100vh;
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
  font-weight: 400;
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
.card {
  width: min(100%, 480px);
  background: var(--canvas);
  border: 1px solid var(--hairline);
  border-radius: 18px;
  padding: 32px;
  text-align: center;
}
.eyebrow {
  font-size: 12px;
  font-weight: 600;
  line-height: 1.29;
  letter-spacing: -0.12px;
  color: var(--primary);
  margin-bottom: 8px;
}
h1 {
  font-family: "SF Pro Display", -apple-system, BlinkMacSystemFont, "PingFang SC", system-ui, sans-serif;
  font-size: 28px;
  font-weight: 600;
  line-height: 1.14;
  letter-spacing: -0.28px;
  color: var(--ink);
  margin-bottom: 10px;
}
.hint {
  font-size: 17px;
  font-weight: 400;
  line-height: 1.47;
  letter-spacing: -0.374px;
  color: var(--ink-muted-48);
  margin-bottom: 24px;
}
.img-stage {
  display: flex;
  justify-content: center;
  padding: 16px;
  margin-bottom: 20px;
  border-radius: 11px;
  background: var(--parchment);
  border: 1px solid var(--divider-soft);
}
.img-wrap {
  position: relative;
  display: inline-block;
  cursor: crosshair;
  user-select: none;
}
.img-wrap img {
  display: block;
  border-radius: 8px;
  /* render at natural pixel size for accurate coordinates */
}
.marker {
  position: absolute;
  width: 28px;
  height: 28px;
  border-radius: 9999px;
  background: var(--primary);
  color: #ffffff;
  border: 2px solid #ffffff;
  font-size: 13px;
  font-weight: 600;
  line-height: 24px;
  text-align: center;
  transform: translate(-50%, -50%);
  pointer-events: none;
}
.actions {
  display: flex;
  justify-content: center;
  gap: 12px;
  margin-bottom: 16px;
}
.actions button {
  min-height: 44px;
  padding: 8px 16px;
  border: 3px solid var(--divider-soft);
  border-radius: 11px;
  background: var(--pearl);
  color: var(--ink-muted-80);
  font-family: inherit;
  font-size: 14px;
  font-weight: 400;
  line-height: 1.29;
  letter-spacing: -0.224px;
  cursor: pointer;
  transition: transform 0.14s ease, border-color 0.14s ease;
}
.actions button:active { transform: scale(0.95); }
.actions button:focus-visible { outline: 2px solid var(--primary-focus); outline-offset: 2px; }
.status {
  margin-bottom: 20px;
  font-size: 14px;
  font-weight: 400;
  line-height: 1.43;
  letter-spacing: -0.224px;
  color: var(--ink-muted-48);
  font-variant-numeric: tabular-nums;
}
#submitBtn {
  width: 100%;
  min-height: 46px;
  padding: 11px 22px;
  border: 0;
  border-radius: 9999px;
  background: var(--primary);
  color: #ffffff;
  font-family: inherit;
  font-size: 17px;
  font-weight: 400;
  line-height: 1.24;
  letter-spacing: -0.374px;
  cursor: pointer;
  transition: transform 0.14s ease, opacity 0.14s ease;
}
#submitBtn:active:not(:disabled) { transform: scale(0.95); }
#submitBtn:focus-visible { outline: 2px solid var(--primary-focus); outline-offset: 2px; }
#submitBtn:disabled { background: var(--hairline); color: var(--ink-muted-48); cursor: not-allowed; }
.error-banner {
  display: none;
  margin-top: 14px;
  padding: 10px 14px;
  border-radius: 8px;
  background: var(--parchment);
  color: var(--ink);
  font-size: 14px;
  border: 1px solid var(--hairline);
}
details.debug-panel {
  margin-top: 16px;
  text-align: left;
  border-top: 1px solid var(--divider-soft);
  padding-top: 12px;
}
details.debug-panel > summary {
  cursor: pointer;
  color: var(--ink-muted-48);
  font-size: 12px;
  letter-spacing: -0.12px;
}
.debug {
  margin-top: 8px;
  max-height: 100px;
  overflow-y: auto;
  font-size: 12px;
  color: var(--ink-muted-48);
  white-space: pre;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
.success {
  display: none;
  padding: 20px 8px 4px;
}
.success-title {
  font-family: "SF Pro Display", -apple-system, BlinkMacSystemFont, "PingFang SC", system-ui, sans-serif;
  font-size: 21px;
  font-weight: 600;
  line-height: 1.19;
  color: var(--ink);
  margin-bottom: 8px;
}
.success-copy {
  font-size: 14px;
  color: var(--ink-muted-48);
}
.note {
  margin-top: 20px;
  font-size: 12px;
  line-height: 1.4;
  letter-spacing: -0.12px;
  color: var(--ink-muted-48);
}
@media (max-width: 480px) {
  .stage { padding: 24px 16px; }
  .card { padding: 24px 20px; }
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
    <span class="global-nav-meta">安全验证</span>
  </div>
</header>
<main class="stage">
  <div class="card">
    <div class="eyebrow">aTrust 身份核验</div>
    <h1 id="headline">请按顺序点击图中文字</h1>
    <p class="hint" id="hint">在下方验证码图片上依次点击对应文字的位置</p>
    <div class="img-stage" id="imgStage">
      <div class="img-wrap" id="imgWrap">
        <img id="captchaImg" src="/captcha.img" alt="验证码" draggable="false">
      </div>
    </div>
    <div class="actions">
      <button id="undoBtn" type="button" onclick="undoClick()">撤销上一个</button>
      <button id="clearBtn" type="button" onclick="clearClicks()">全部清除</button>
    </div>
    <p class="status" id="status">已选择 0 个点</p>
    <button id="submitBtn" type="button" onclick="submitClicks()" disabled>继续登录</button>
    <div class="error-banner" id="errorBanner" role="alert"></div>
    <div class="success" id="success">
      <div class="success-title" id="successTitle">验证信息已提交</div>
      <div class="success-copy" id="successCopy">可以关闭此页面并返回 NWAFU Connect。</div>
    </div>
    <details class="debug-panel" id="debugPanel">
      <summary id="debugSummary">调试信息</summary>
      <pre class="debug" id="debug"></pre>
    </details>
    <p class="note" id="note">此页面仅在本机临时开放，提交后自动关闭。</p>
  </div>
</main>
<script>
var clicks = [];
var imgStage = document.getElementById('imgStage');
var imgWrap = document.getElementById('imgWrap');
var captchaImg = document.getElementById('captchaImg');
var statusEl = document.getElementById('status');
var submitBtn = document.getElementById('submitBtn');
var debugPanelEl = document.getElementById('debugPanel');
var debugSummaryEl = document.getElementById('debugSummary');
var debugEl = document.getElementById('debug');
var headlineEl = document.getElementById('headline');
var hintEl = document.getElementById('hint');
var undoBtnEl = document.getElementById('undoBtn');
var clearBtnEl = document.getElementById('clearBtn');
var successEl = document.getElementById('success');
var successTitleEl = document.getElementById('successTitle');
var successCopyEl = document.getElementById('successCopy');
var errorBannerEl = document.getElementById('errorBanner');
var noteEl = document.getElementById('note');
var isSubmitting = false;

var I18N = {
  zh: {
    title: 'NWAFU Connect · 图形验证',
    heading: '请按顺序点击图中文字',
    hint: '在下方验证码图片上依次点击对应文字的位置',
    undo: '撤销上一个', clear: '全部清除',
    status: function(n) { return '已选择 ' + n + ' 个点'; },
    submit: '继续登录', submitting: '正在提交…',
    successTitle: '验证信息已提交',
    successCopy: '可以关闭此页面并返回 NWAFU Connect。',
    submitFailed: '提交失败，请检查坐标后重试。',
    networkError: '网络错误，请重试。',
    imgAlt: '验证码',
    debugSummary: '调试信息',
    note: '此页面仅在本机临时开放，提交后自动关闭。'
  },
  en: {
    title: 'NWAFU Connect · Captcha',
    heading: 'Click the specified characters in order',
    hint: 'Click the matching characters on the captcha image in sequence.',
    undo: 'Undo', clear: 'Clear',
    status: function(n) { return 'Selected ' + n + ' point' + (n !== 1 ? 's' : ''); },
    submit: 'Continue', submitting: 'Submitting…',
    successTitle: 'Verification submitted',
    successCopy: 'You may close this page and return to NWAFU Connect.',
    submitFailed: 'Submission failed. Please retry.',
    networkError: 'Network error. Please retry.',
    imgAlt: 'Captcha',
    debugSummary: 'Debug',
    note: 'Served temporarily on localhost only and closed automatically after submission.'
  }
};

var lang = (function() {
  var l = ((navigator.languages && navigator.languages[0]) || navigator.language || '').toLowerCase();
  return l.indexOf('zh') === 0 ? 'zh' : 'en';
})();

function tr(key) { return (I18N[lang] || I18N.en)[key]; }

function applyLang() {
  document.documentElement.lang = lang === 'zh' ? 'zh-CN' : 'en';
  document.title = tr('title');
  headlineEl.textContent = tr('heading');
  hintEl.textContent = tr('hint');
  undoBtnEl.textContent = tr('undo');
  clearBtnEl.textContent = tr('clear');
  captchaImg.alt = tr('imgAlt');
  debugSummaryEl.textContent = tr('debugSummary');
  successTitleEl.textContent = tr('successTitle');
  successCopyEl.textContent = tr('successCopy');
  noteEl.textContent = tr('note');
  renderStatus();
}

function renderStatus() {
  statusEl.textContent = tr('status')(clicks.length);
  submitBtn.textContent = isSubmitting ? tr('submitting') : tr('submit');
  submitBtn.disabled = clicks.length === 0 || isSubmitting;
}

captchaImg.onload = function() { updateDebug(); };

captchaImg.addEventListener('click', function(e) {
  if (isSubmitting) return;
  errorBannerEl.style.display = 'none';
  var rect = captchaImg.getBoundingClientRect();
  var nw = captchaImg.naturalWidth || Math.round(rect.width);
  var nh = captchaImg.naturalHeight || Math.round(rect.height);
  var dw = rect.width || nw;
  var dh = rect.height || nh;

  var x = Math.max(0, Math.min(nw - 1, Math.round((e.clientX - rect.left) * nw / dw)));
  var y = Math.max(0, Math.min(nh - 1, Math.round((e.clientY - rect.top) * nh / dh)));

  clicks.push({x: x, y: y});
  renderMarkers();
  renderStatus();
  updateDebug();
});

function updateDebug() {
  var rect = captchaImg.getBoundingClientRect();
  var lines = ['natural: ' + captchaImg.naturalWidth + 'x' + captchaImg.naturalHeight
    + ', display: ' + Math.round(rect.width) + 'x' + Math.round(rect.height)
    + ', dpr: ' + window.devicePixelRatio];
  for (var i = 0; i < clicks.length; i++) {
    lines.push((i+1) + ': (' + clicks[i].x + ', ' + clicks[i].y + ')');
  }
  debugEl.textContent = lines.join('\n');
}

function renderMarkers() {
  var old = imgWrap.querySelectorAll('.marker');
  for (var i = 0; i < old.length; i++) old[i].remove();

  var rect = captchaImg.getBoundingClientRect();
  var nw = captchaImg.naturalWidth || Math.round(rect.width);
  var nh = captchaImg.naturalHeight || Math.round(rect.height);
  var dw = rect.width || nw;
  var dh = rect.height || nh;

  for (var i = 0; i < clicks.length; i++) {
    var m = document.createElement('div');
    m.className = 'marker';
    m.textContent = i + 1;
    m.style.left = (clicks[i].x * dw / nw) + 'px';
    m.style.top = (clicks[i].y * dh / nh) + 'px';
    imgWrap.appendChild(m);
  }
}

function undoClick() {
  if (isSubmitting) return;
  clicks.pop();
  renderMarkers();
  renderStatus();
  updateDebug();
}

function clearClicks() {
  if (isSubmitting) return;
  clicks = [];
  renderMarkers();
  renderStatus();
  updateDebug();
}

function showError(message) {
  errorBannerEl.textContent = message;
  errorBannerEl.style.display = 'block';
}

function submitClicks() {
  if (clicks.length === 0 || isSubmitting) return;
  errorBannerEl.style.display = 'none';
  var rect = captchaImg.getBoundingClientRect();
  var width = captchaImg.naturalWidth || Math.round(rect.width);
  var height = captchaImg.naturalHeight || Math.round(rect.height);
  var payload = JSON.stringify({
    coordinates: clicks.map(function(p) { return [p.x, p.y]; }),
    width: width, height: height
  });

  isSubmitting = true;
  renderStatus();

  var xhr = new XMLHttpRequest();
  xhr.open('POST', '/submit', true);
  xhr.setRequestHeader('Content-Type', 'application/x-www-form-urlencoded');
  xhr.onload = function() {
    if (xhr.status === 200) {
      imgStage.style.display = 'none';
      document.querySelector('.actions').style.display = 'none';
      statusEl.style.display = 'none';
      submitBtn.style.display = 'none';
      debugPanelEl.style.display = 'none';
      hintEl.style.display = 'none';
      headlineEl.style.display = 'none';
      successEl.style.display = 'block';
    } else {
      isSubmitting = false;
      renderStatus();
      showError(tr('submitFailed'));
    }
  };
  xhr.onerror = function() {
    isSubmitting = false;
    renderStatus();
    showError(tr('networkError'));
  };
  xhr.send('token=` + captchaTokenPlaceholder + `&code=' + encodeURIComponent(payload));
  return false;
}

applyLang();
</script>
</body>
</html>`

func serveCaptchaInBrowser(imgData []byte, timeout time.Duration, output io.Writer) (ClickCaptchaResponse, error) {
	if timeout <= 0 {
		timeout = defaultCaptchaTimeout
	}
	token, err := newLocalFormToken()
	if err != nil {
		return ClickCaptchaResponse{}, err
	}
	pageHTML := strings.Replace(captchaPageHTML, captchaTokenPlaceholder, token, 1)
	resultCh := make(chan ClickCaptchaResponse, 1)

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		setCaptchaPageHeaders(w.Header())
		if !isLoopbackRequestHost(r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(pageHTML))
	})

	mux.HandleFunc("/captcha.img", func(w http.ResponseWriter, r *http.Request) {
		setCaptchaPageHeaders(w.Header())
		if !isLoopbackRequestHost(r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", http.DetectContentType(imgData))
		_, _ = w.Write(imgData)
	})

	mux.HandleFunc("/submit", func(w http.ResponseWriter, r *http.Request) {
		setCaptchaPageHeaders(w.Header())
		if !isLoopbackRequestHost(r.Host) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.PostForm.Get("token")), []byte(token)) != 1 {
			http.Error(w, "invalid form token", http.StatusForbidden)
			return
		}
		code := r.PostForm.Get("code")
		if code == "" {
			http.Error(w, "empty code", http.StatusBadRequest)
			return
		}
		response, err := parseClickCaptchaResponse(code)
		if err != nil {
			http.Error(w, "invalid code: "+err.Error(), http.StatusBadRequest)
			return
		}
		select {
		case resultCh <- response:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		default:
			http.Error(w, "already submitted", http.StatusConflict)
		}
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return ClickCaptchaResponse{}, fmt.Errorf("failed to start captcha server: %w", err)
	}

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       15 * time.Second,
	}
	defer func() {
		shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownContext); err != nil {
			_ = srv.Close()
		}
	}()

	serveErrCh := make(chan error, 1)
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrCh <- err
		}
	}()

	addr := fmt.Sprintf("http://%s", listener.Addr().String())
	if output != nil {
		_, _ = fmt.Fprintf(output, "Captcha server started at %s\n", addr)
	} else {
		log.Printf("Captcha server started at %s", addr)
	}

	OpenBrowser(addr, output)

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case response := <-resultCh:
		if output != nil {
			_, _ = fmt.Fprintln(output, "Captcha code received from browser")
		} else {
			log.Println("Captcha code received from browser")
		}
		return response, nil
	case err := <-serveErrCh:
		return ClickCaptchaResponse{}, fmt.Errorf("captcha server failed: %w", err)
	case <-timer.C:
		return ClickCaptchaResponse{}, fmt.Errorf("captcha input timed out after %v", timeout)
	}
}

func setCaptchaPageHeaders(header http.Header) {
	header.Set("Cache-Control", "no-store, max-age=0")
	header.Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; connect-src 'self'; form-action 'none'; frame-ancestors 'none'; img-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'")
	header.Set("Cross-Origin-Opener-Policy", "same-origin")
	header.Set("Cross-Origin-Resource-Policy", "same-origin")
	header.Set("Permissions-Policy", "camera=(), geolocation=(), microphone=(), usb=()")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
}

// OpenBrowser tries to open the given URL in the system's default browser.
func OpenBrowser(targetURL string, output io.Writer) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", targetURL)
	case "linux":
		cmd = exec.Command("xdg-open", targetURL)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", targetURL)
	default:
		if output != nil {
			_, _ = fmt.Fprintf(output, "Unsupported platform for auto-opening browser, please visit: %s\n", targetURL)
		} else {
			log.Printf("Unsupported platform for auto-opening browser, please visit: %s", targetURL)
		}
		return
	}
	if err := cmd.Start(); err != nil {
		if output != nil {
			_, _ = fmt.Fprintf(output, "Failed to open browser: %v. Please visit: %s\n", err, targetURL)
		} else {
			log.Printf("Failed to open browser: %v. Please visit: %s", err, targetURL)
		}
		return
	}
	go func() {
		if err := cmd.Wait(); err != nil {
			log.DebugPrintf("Browser opener exited: %v", err)
		}
	}()
}
