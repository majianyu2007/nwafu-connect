package desktop

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/majianyu2007/nwafu-connect/configs"
	"github.com/majianyu2007/nwafu-connect/internal/appdata"
	"github.com/majianyu2007/nwafu-connect/internal/desktopconfig"
)

func TestConfiguredRequiresCredentialsForSelectedAuthentication(t *testing.T) {
	server, passwordAuth, smsAuth, wechatAuth := "vpn.nwafu.edu.cn", "auth/psw", "auth/smsCheckCode", "auth/qywechat"
	username, password, phone := "student", "secret", "86-13800138000"
	tests := []struct {
		name          string
		configuration configs.ConfigTOML
		want          bool
	}{
		{name: "missing password", configuration: configs.ConfigTOML{ServerAddress: &server, AuthType: &passwordAuth, Username: &username}},
		{name: "password ready", configuration: configs.ConfigTOML{ServerAddress: &server, AuthType: &passwordAuth, Username: &username, Password: &password}, want: true},
		{name: "default server is valid", configuration: configs.ConfigTOML{AuthType: &passwordAuth, Username: &username, Password: &password}, want: true},
		{name: "SMS ready", configuration: configs.ConfigTOML{ServerAddress: &server, AuthType: &smsAuth, Phone: &phone}, want: true},
		{name: "WeCom opens interactive login", configuration: configs.ConfigTOML{ServerAddress: &server, AuthType: &wechatAuth}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := configured(test.configuration); got != test.want {
				t.Fatalf("configured() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestShouldReconnectAfterWakeOrNetworkRecovery(t *testing.T) {
	tests := []struct {
		name                 string
		desired              bool
		elapsed              time.Duration
		wasOnline, nowOnline bool
		want                 bool
	}{
		{name: "normal online tick", desired: true, elapsed: 15 * time.Second, wasOnline: true, nowOnline: true},
		{name: "wake from sleep", desired: true, elapsed: 2 * time.Minute, wasOnline: true, nowOnline: true, want: true},
		{name: "network recovered", desired: true, elapsed: 15 * time.Second, wasOnline: false, nowOnline: true, want: true},
		{name: "still offline", desired: true, elapsed: 15 * time.Second, wasOnline: false, nowOnline: false},
		{name: "manually stopped before wake", elapsed: 2 * time.Minute, wasOnline: true, nowOnline: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldReconnect(test.desired, test.elapsed, test.wasOnline, test.nowOnline); got != test.want {
				t.Fatalf("shouldReconnect() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestStartClearsReconnectIntentWhenConfigurationIsIncomplete(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	controller := &Controller{
		paths: appdata.Paths{BrowserState: filepath.Join(root, "browser-state.json")},
		store: desktopconfig.Store{
			ConfigPath:      filepath.Join(root, "config.toml"),
			PreferencesPath: filepath.Join(root, "desktop.json"),
		},
		corePath: "unused",
		ctx:      ctx,
		cancel:   cancel,
		status:   make(chan Status, 1),
	}

	if err := controller.Start(); err == nil {
		t.Fatal("Start() accepted incomplete authentication configuration")
	}
	if controller.desiredRunning {
		t.Fatal("failed Start() left automatic reconnect enabled")
	}
}

func TestStopRemovesStaleBrowserStateWithoutRunningCommand(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "browser-state.json")
	if err := os.WriteFile(statePath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	controller := &Controller{
		paths:  appdata.Paths{BrowserState: statePath},
		status: make(chan Status, 1),
	}

	controller.stopLocked()

	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("browser state still exists after stop: %v", err)
	}
}
