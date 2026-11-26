package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionRefreshConfiguration(t *testing.T) {
	file := writeConfig(t, "session_refresh_interval = 900\n")
	for _, tt := range []struct {
		name string
		args []string
		env  []string
		want int
	}{
		{"default", nil, nil, 1800},
		{"file", []string{"--config", file}, nil, 900},
		{"environment", []string{"--config", file}, []string{"NWAFU_CONNECT_SESSION_REFRESH_INTERVAL=600"}, 600},
		{"cli", []string{"--config", file, "-session-refresh-interval", "300"}, []string{"NWAFU_CONNECT_SESSION_REFRESH_INTERVAL=600"}, 300},
		{"disabled", []string{"--config", file, "--session-refresh-interval=0"}, nil, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			options, _, err := loadStartupOptions(tt.args, func() []string { return tt.env })
			if err != nil {
				t.Fatal(err)
			}
			if got := options.Config.SessionRefreshInterval; got != tt.want {
				t.Fatalf("session refresh seconds = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigSourcePrecedence(t *testing.T) {
	configFile := writeConfig(t, `
server_address = "file.nwafu.edu.cn"
server_port = 444
username = "file-user"
tun_mode = true
browser_mode = true
`)

	options, _, err := loadStartupOptions([]string{
		"--config", configFile,
		"--server", "cli.nwafu.edu.cn",
		"--tun-mode=false",
	}, func() []string {
		return []string{
			"NWAFU_CONNECT_SERVER_ADDRESS=env.nwafu.edu.cn",
			"NWAFU_CONNECT_SERVER_PORT=555",
			"NWAFU_CONNECT_USERNAME=env-user",
		}
	})
	if err != nil {
		t.Fatal(err)
	}

	cfg := options.Config
	if cfg.ServerAddress != "cli.nwafu.edu.cn" {
		t.Fatalf("ServerAddress = %q, want CLI value", cfg.ServerAddress)
	}
	if cfg.ServerPort != 555 {
		t.Fatalf("ServerPort = %d, want environment value", cfg.ServerPort)
	}
	if cfg.Username != "env-user" {
		t.Fatalf("Username = %q, want environment value", cfg.Username)
	}
	if !cfg.BrowserMode {
		t.Fatal("BrowserMode = false, want TOML value")
	}
	if cfg.TUNMode {
		t.Fatal("TUNMode = true, want explicit CLI false")
	}
}

func TestLegacySingleDashFlagsRemainSupported(t *testing.T) {
	options, _, err := loadStartupOptions([]string{
		"-server=legacy.nwafu.edu.cn",
		"-browser-mode",
		"-browser-stay-running",
		"-qywechat-qrcode-browser=false",
	}, func() []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if options.Config.ServerAddress != "legacy.nwafu.edu.cn" ||
		!options.Config.BrowserMode ||
		!options.Config.BrowserStayRunning ||
		options.Config.QYWechatQRCodeBrowser {
		t.Fatalf("legacy flags were not applied: %+v", options.Config)
	}
}

func TestLegacyArgumentNormalizationDoesNotRewriteValues(t *testing.T) {
	for _, passwordFlag := range []string{"-password", "--password"} {
		options, _, err := loadStartupOptions([]string{
			"-username", "user",
			passwordFlag, "-server",
		}, func() []string { return nil })
		if err != nil {
			t.Fatal(err)
		}
		if options.Config.Password != "-server" {
			t.Fatalf("%s value = %q, want unchanged flag-like value", passwordFlag, options.Config.Password)
		}
	}
}

func TestUnchangedCLIValuesDoNotOverrideConfig(t *testing.T) {
	configFile := writeConfig(t, `
server_port = 8443
socks_bind = "127.0.0.1:2080"
mac_addresses = ["02:11:22:33:44:55"]
`)

	options, _, err := loadStartupOptions([]string{"--config", configFile}, func() []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if options.Config.ServerPort != 8443 {
		t.Fatalf("ServerPort = %d, want 8443", options.Config.ServerPort)
	}
	if options.Config.SocksBind != "127.0.0.1:2080" {
		t.Fatalf("SocksBind = %q, want TOML value", options.Config.SocksBind)
	}
	if len(options.Config.MACAddresses) != 1 || options.Config.MACAddresses[0] != "02:11:22:33:44:55" {
		t.Fatalf("MACAddresses = %v", options.Config.MACAddresses)
	}
}

func TestEnvironmentConfigPathAndAuthInfo(t *testing.T) {
	configFile := writeConfig(t, `
server_address = "vpn.example.edu.cn"
`)

	options, _, err := loadStartupOptions([]string{"--auth-info"}, func() []string {
		return []string{"NWAFU_CONNECT_CONFIG=" + configFile}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !options.AuthInfo {
		t.Fatal("AuthInfo = false")
	}
	if options.Config.ServerAddress != "vpn.example.edu.cn" {
		t.Fatalf("config not loaded before auth-info: %+v", options.Config)
	}
}

func TestCollectionOverridePreservesOtherNetworkType(t *testing.T) {
	configFile := writeConfig(t, `
[[port_forwarding]]
network_type = "tcp"
bind_address = "127.0.0.1:1"
remote_address = "10.0.0.1:1"

[[port_forwarding]]
network_type = "udp"
bind_address = "127.0.0.1:53"
remote_address = "10.0.0.21:53"
`)

	options, _, err := loadStartupOptions([]string{
		"--config", configFile,
		"--tcp-port-forwarding", "127.0.0.1:2222-my-host.nwafu.edu.cn:22",
	}, func() []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Config.PortForwardingList) != 2 {
		t.Fatalf("PortForwardingList = %+v", options.Config.PortForwardingList)
	}
	if options.Config.PortForwardingList[0].NetworkType != "udp" {
		t.Fatalf("expected UDP forwarding preserved: %+v", options.Config.PortForwardingList)
	}
	if options.Config.PortForwardingList[1].RemoteAddress != "my-host.nwafu.edu.cn:22" {
		t.Fatalf("expected hyphenated host parsed cleanly: %+v", options.Config.PortForwardingList)
	}
}

func TestUnknownConfigurationKeyIsRejected(t *testing.T) {
	configFile := writeConfig(t, "server_adress = \"typo.nwafu.edu.cn\"\n")
	if _, _, err := loadStartupOptions([]string{"--config", configFile}, func() []string { return nil }); err == nil {
		t.Fatal("unknown TOML key was accepted")
	}

	if _, _, err := loadStartupOptions(nil, func() []string {
		return []string{"NWAFU_CONNECT_SERVER_ADRESS=typo.nwafu.edu.cn"}
	}); err == nil {
		t.Fatal("unknown environment key was accepted")
	}
}

func TestEnvironmentCollectionsUseCLISyntax(t *testing.T) {
	options, _, err := loadStartupOptions(nil, func() []string {
		return []string{
			"NWAFU_CONNECT_TCP_PORT_FORWARDING=127.0.0.1:2-10.0.0.2:2",
			"NWAFU_CONNECT_UDP_PORT_FORWARDING=127.0.0.1:3-10.0.0.3:3",
			"NWAFU_CONNECT_CUSTOM_DNS=library.nwafu.edu.cn:10.0.0.10",
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(options.Config.PortForwardingList) != 2 {
		t.Fatalf("PortForwardingList = %+v", options.Config.PortForwardingList)
	}
	if len(options.Config.CustomDNSList) != 1 || options.Config.CustomDNSList[0].IP != "10.0.0.10" {
		t.Fatalf("CustomDNSList = %+v", options.Config.CustomDNSList)
	}
}

func TestValidateConnectConfigRequiresServerAddressEvenWithDebugCreds(t *testing.T) {
	options, _, err := loadStartupOptions([]string{
		"--server=",
		"--sid=s",
		"--device-id=d",
		"--resource-file=r.json",
	}, func() []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := validateConnectConfig(options.Config); err == nil {
		t.Fatal("empty server address was accepted with debug credentials")
	}
}

func TestValidateConnectConfigRejectsUnsupportedAuthType(t *testing.T) {
	options, _, err := loadStartupOptions([]string{"--auth-type=auth/unknown"}, func() []string { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := validateConnectConfig(options.Config); err == nil {
		t.Fatal("unsupported auth type was accepted")
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
