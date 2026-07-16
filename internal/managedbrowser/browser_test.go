package managedbrowser

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestBrowserArgsIsolateAndProxyTraffic(t *testing.T) {
	args := browserArgs("/tmp/profile", "127.0.0.1:43210", "https://lib.nwafu.edu.cn/")
	for _, expected := range []string{
		"--user-data-dir=/tmp/profile",
		"--proxy-server=http://127.0.0.1:43210",
		"--proxy-bypass-list=127.0.0.1;localhost",
		"--homepage=https://lib.nwafu.edu.cn/",
		"--show-home-button",
		"--disable-quic",
		"--disable-background-mode",
		"--force-webrtc-ip-handling-policy=disable_non_proxied_udp",
		"--password-store=basic",
		"--use-mock-keychain",
	} {
		if !slices.Contains(args, expected) {
			t.Fatalf("browser arguments missing %q: %v", expected, args)
		}
	}
	if args[len(args)-1] != "https://lib.nwafu.edu.cn/" {
		t.Fatalf("start URL = %q", args[len(args)-1])
	}
}

func TestStartRejectsInvalidNetworkConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		options Options
	}{
		{
			name: "invalid proxy",
			options: Options{
				ProxyAddress: "not-an-address",
				StartURL:     "https://lib.nwafu.edu.cn/",
			},
		},
		{
			name: "non-HTTP URL",
			options: Options{
				ProxyAddress: "127.0.0.1:43210",
				StartURL:     "file:///etc/passwd",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if process, err := Start(context.Background(), test.options); err == nil {
				_ = process
				t.Fatal("invalid browser configuration accepted")
			}
		})
	}
}

func TestFindExecutableUsesConfiguredFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "browser")
	if err := os.WriteFile(path, []byte("browser"), 0700); err != nil {
		t.Fatal(err)
	}
	resolved, err := FindExecutable(path)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != path {
		t.Fatalf("resolved executable = %q, want %q", resolved, path)
	}
}
