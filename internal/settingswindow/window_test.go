package settingswindow

import (
	"path/filepath"
	"testing"

	fynetest "fyne.io/fyne/v2/test"
	"github.com/majianyu2007/nwafu-connect/internal/desktopconfig"
)

func TestSavePreservesAdvancedTOMLFields(t *testing.T) {
	application := fynetest.NewApp()
	defer application.Quit()

	root := t.TempDir()
	store := desktopconfig.Store{
		ConfigPath:      filepath.Join(root, "config.toml"),
		PreferencesPath: filepath.Join(root, "desktop.json"),
		ClientDataPath:  filepath.Join(root, "client-data.json"),
	}
	configuration, preferences, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	username, password := "student", "secret"
	configuration.Username = &username
	configuration.Password = &password
	if err := store.Save(configuration, preferences); err != nil {
		t.Fatal(err)
	}

	window, err := New(application, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	window.advancedConfig.SetText(`keep_alive_url = "https://library.example/health"
`)
	window.save()

	saved, _, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.KeepAliveURL == nil || *saved.KeepAliveURL != "https://library.example/health" {
		t.Fatalf("advanced keep_alive_url = %v, want preserved value", saved.KeepAliveURL)
	}
}

func TestSaveFromAdvancedTabDoesNotClobberTOMLEdits(t *testing.T) {
	application := fynetest.NewApp()
	defer application.Quit()

	root := t.TempDir()
	store := desktopconfig.Store{
		ConfigPath:      filepath.Join(root, "config.toml"),
		PreferencesPath: filepath.Join(root, "desktop.json"),
		ClientDataPath:  filepath.Join(root, "client-data.json"),
	}
	configuration, preferences, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	username, password := "student", "secret"
	configuration.Username = &username
	configuration.Password = &password
	if err := store.Save(configuration, preferences); err != nil {
		t.Fatal(err)
	}

	window, err := New(application, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	window.tabs.Select(window.advancedTab)
	window.advancedConfig.SetText(`server_address = "custom.nwafu.edu.cn"
session_refresh_interval = 900
`)
	window.save()

	saved, _, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.ServerAddress == nil || *saved.ServerAddress != "custom.nwafu.edu.cn" {
		t.Fatalf("server_address = %v, want custom.nwafu.edu.cn", saved.ServerAddress)
	}
	if saved.SessionRefreshInterval == nil || *saved.SessionRefreshInterval != 900 {
		t.Fatalf("session_refresh_interval = %v, want 900", saved.SessionRefreshInterval)
	}
	if saved.Password == nil || *saved.Password != "secret" {
		t.Fatalf("existing password should be preserved when omitted from TOML edit")
	}
}

func TestAuthTypeSwitchUpdatesDefaultLoginDomain(t *testing.T) {
	application := fynetest.NewApp()
	defer application.Quit()

	root := t.TempDir()
	store := desktopconfig.Store{
		ConfigPath:      filepath.Join(root, "config.toml"),
		PreferencesPath: filepath.Join(root, "desktop.json"),
		ClientDataPath:  filepath.Join(root, "client-data.json"),
	}
	window, err := New(application, store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	window.authType.SetSelected("短信验证码")
	if got := window.loginDomain.Text; got != "sms73926" {
		t.Fatalf("loginDomain after SMS switch = %q, want sms73926", got)
	}
	window.authType.SetSelected("企业微信扫码")
	if got := window.loginDomain.Text; got != "wechat" {
		t.Fatalf("loginDomain after WeCom switch = %q, want wechat", got)
	}
	window.authType.SetSelected("用户名与密码")
	if got := window.loginDomain.Text; got != "LDAP" {
		t.Fatalf("loginDomain after password switch = %q, want LDAP", got)
	}
}
