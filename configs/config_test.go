package configs

import "testing"

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.ServerAddress != "vpn.nwafu.edu.cn" {
		t.Fatalf("ServerAddress = %q, want vpn.nwafu.edu.cn", cfg.ServerAddress)
	}
	if cfg.ServerPort != 443 {
		t.Fatalf("ServerPort = %d, want 443", cfg.ServerPort)
	}
	if cfg.AuthType != "auth/psw" || cfg.LoginDomain != "LDAP" {
		t.Fatalf("auth defaults = (%q, %q), want (auth/psw, LDAP)", cfg.AuthType, cfg.LoginDomain)
	}
	if cfg.SocksBind != "127.0.0.1:1080" || cfg.HTTPBind != "127.0.0.1:1081" {
		t.Fatalf("proxy defaults = %q, %q", cfg.SocksBind, cfg.HTTPBind)
	}
	if cfg.RemoteDNSServer != "auto" || cfg.SecondaryDNSServer != "auto" {
		t.Fatalf("DNS defaults = %q, %q", cfg.RemoteDNSServer, cfg.SecondaryDNSServer)
	}
	if cfg.SessionRefreshInterval != 1800 || cfg.UpdateBestNodesInterval != 300 {
		t.Fatalf("intervals = (%d, %d), want (1800, 300)", cfg.SessionRefreshInterval, cfg.UpdateBestNodesInterval)
	}
	if !cfg.QYWechatQRCodeTerminal || !cfg.QYWechatQRCodeBrowser || cfg.QYWechatQRCodeFile != "qywechat_qrcode.png" {
		t.Fatalf("QYWechat defaults = (%q, %v, %v)", cfg.QYWechatQRCodeFile, cfg.QYWechatQRCodeTerminal, cfg.QYWechatQRCodeBrowser)
	}
}
