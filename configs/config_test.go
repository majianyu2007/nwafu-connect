package configs

import "testing"

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Protocol != "atrust" {
		t.Fatalf("Protocol = %q, want atrust", cfg.Protocol)
	}
	if cfg.ServerAddress != "" {
		t.Fatalf("ServerAddress = %q, want derived default", cfg.ServerAddress)
	}
	if cfg.ServerPort != 443 {
		t.Fatalf("ServerPort = %d, want 443", cfg.ServerPort)
	}
	if cfg.SocksBind != "127.0.0.1:1080" || cfg.HTTPBind != "127.0.0.1:1081" {
		t.Fatalf("proxy defaults = %q, %q", cfg.SocksBind, cfg.HTTPBind)
	}
}
