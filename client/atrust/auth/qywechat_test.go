package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/boombuler/barcode"
	barcodeqr "github.com/boombuler/barcode/qr"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQYWechatQRCodeURLUsesGatewayConfiguration(t *testing.T) {
	session := &Session{
		baseHost: "vpn.nwafu.edu.cn",
		baseURL:  "https://vpn.nwafu.edu.cn",
	}
	authInfo := AuthInfo{
		QYWechatQrcodeConf: QYWechatQrcodeConfig{
			AppID:       "ww-app",
			AgentID:     "agent-1",
			RedirectURI: "https://vpn.nwafu.edu.cn:443/passport/v1/auth/qywechat?sfDomain=wechat",
			State:       "state-1",
		},
	}

	qrCodeURL, err := session.qyWechatQRCodeURL(authInfo, "wechat")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(qrCodeURL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "https" || parsed.Host != "open.work.weixin.qq.com" || parsed.Path != "/wwopen/sso/qrConnect" {
		t.Fatalf("unexpected QR code endpoint: %s", parsed.String())
	}
	want := map[string]string{
		"appid":        "ww-app",
		"agentid":      "agent-1",
		"redirect_uri": authInfo.QYWechatQrcodeConf.RedirectURI,
		"state":        "state-1",
		"login_type":   "jssdk",
		"href":         "https://vpn.nwafu.edu.cn/portal/wechat_qrcode.css",
	}
	for key, value := range want {
		if parsed.Query().Get(key) != value {
			t.Errorf("query %s = %q, want %q", key, parsed.Query().Get(key), value)
		}
	}
}

func TestQYWechatScanSessionDownloadsImageAndPollsCallback(t *testing.T) {
	imageData := testQRCodePNG(t)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/wwopen/sso/qrConnect":
			_, _ = io.WriteString(w, `<script>window.settings = { key : "key-1" };</script>`)
		case "/wwopen/sso/qrImg":
			if r.URL.Query().Get("key") != "key-1" {
				t.Errorf("image key = %q, want key-1", r.URL.Query().Get("key"))
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageData)
		case "/wwopen/sso/l/qrConnect":
			if r.URL.Query().Get("key") != "key-1" {
				t.Errorf("poll key = %q, want key-1", r.URL.Query().Get("key"))
			}
			_, _ = io.WriteString(w, `jsonpCallback({"status":"QRCODE_SCAN_SUCC","auth_code":"code-1"})`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	conf := QYWechatQrcodeConfig{
		AppID:       "ww-app",
		RedirectURI: "https://vpn.nwafu.edu.cn/passport/v1/auth/qywechat?sfDomain=wechat",
		State:       "state-1",
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	scan, err := newQYWechatScanSession(ctx, server.URL+"/wwopen/sso/qrConnect", conf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(scan.image, imageData) {
		t.Fatal("downloaded QR code image changed")
	}
	callback, err := scan.waitForCallback(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if callback.Query().Get("code") != "code-1" || callback.Query().Get("state") != "state-1" || callback.Query().Get("appid") != "ww-app" {
		t.Fatalf("unexpected callback: %s", callback)
	}
}

func TestRenderAndSaveQYWechatQRCode(t *testing.T) {
	imageData := testQRCodePNG(t)
	rendered, err := renderQRCodeTerminal(imageData)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered, "\x1b[30;47m") || !strings.ContainsAny(rendered, "█▀▄") {
		t.Fatal("terminal QR code is missing ANSI colors or block modules")
	}
	if lines := strings.Count(rendered, "\n"); lines < 10 || lines > 100 {
		t.Fatalf("terminal QR code lines = %d, want a compact QR code", lines)
	}

	path := filepath.Join(t.TempDir(), "qywechat_qrcode.png")
	if err := saveQYWechatQRCode(path, imageData); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(saved, imageData) {
		t.Fatal("saved QR code image changed")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("QR code mode = %o, want 600", info.Mode().Perm())
	}
}

func TestQYWechatPageReportsStatus(t *testing.T) {
	page, err := serveQYWechatPage(testQRCodePNG(t))
	if err != nil {
		t.Fatal(err)
	}
	defer page.close()

	readStatus := func() qyWechatPageStatus {
		response, err := http.Get(page.address + "/status")
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("status endpoint returned %d", response.StatusCode)
		}
		var status qyWechatPageStatus
		if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
			t.Fatal(err)
		}
		return status
	}

	if status := readStatus(); status.State != "waiting" || status.Message == "" {
		t.Fatalf("initial page status = %+v", status)
	}
	page.setStatus("success", "认证成功，可以关闭此页面。")
	if status := readStatus(); status.State != "success" || status.Message != "认证成功，可以关闭此页面。" {
		t.Fatalf("updated page status = %+v", status)
	}
}

func TestValidateQYWechatCallbackURL(t *testing.T) {
	valid, err := url.Parse("https://vpn.nwafu.edu.cn:443/passport/v1/auth/qywechat?sfDomain=wechat&code=code-1&state=state-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateQYWechatCallbackURL(valid, "vpn.nwafu.edu.cn", "wechat", "state-1"); err != nil {
		t.Fatalf("valid callback rejected: %v", err)
	}

	tests := []struct {
		name     string
		callback string
	}{
		{name: "wrong host", callback: "https://attacker.example/passport/v1/auth/qywechat?sfDomain=wechat&code=code-1&state=state-1"},
		{name: "wrong domain", callback: "https://vpn.nwafu.edu.cn/passport/v1/auth/qywechat?sfDomain=LDAP&code=code-1&state=state-1"},
		{name: "missing code", callback: "https://vpn.nwafu.edu.cn/passport/v1/auth/qywechat?sfDomain=wechat&state=state-1"},
		{name: "wrong state", callback: "https://vpn.nwafu.edu.cn/passport/v1/auth/qywechat?sfDomain=wechat&code=code-1&state=other"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			callback, err := url.Parse(test.callback)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateQYWechatCallbackURL(callback, "vpn.nwafu.edu.cn", "wechat", "state-1"); err == nil {
				t.Fatal("invalid callback accepted")
			}
		})
	}
}

func TestQYWechatCallbackExtractsPortalTicket(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/passport/v1/auth/qywechat" {
			t.Errorf("path = %q, want qywechat callback", r.URL.Path)
		}
		portalData, _ := json.Marshal(map[string]string{"ticket": "ticket-1"})
		w.Header().Set("Location", server.URL+"/portal/qrcode_middle.html?data="+url.QueryEscape(string(portalData)))
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	session := &Session{
		client:   server.Client(),
		baseHost: strings.TrimPrefix(server.URL, "https://"),
		baseURL:  server.URL,
	}
	if err := session.qyWechat(server.URL + "/passport/v1/auth/qywechat?sfDomain=wechat&code=code-1&state=state-1"); err != nil {
		t.Fatal(err)
	}
	if session.ticket != "ticket-1" {
		t.Fatalf("ticket = %q, want ticket-1", session.ticket)
	}
}

func testQRCodePNG(t *testing.T) []byte {
	t.Helper()
	encoded, err := barcodeqr.Encode("https://open.work.weixin.qq.com/wwopen/sso/qrConnect?key=test", barcodeqr.M, barcodeqr.Auto)
	if err != nil {
		t.Fatal(err)
	}
	scaled, err := barcode.Scale(encoded, 300, 300)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := png.Encode(&output, scaled); err != nil {
		t.Fatal(err)
	}
	return output.Bytes()
}
