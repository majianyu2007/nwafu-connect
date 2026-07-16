package settingswindow

import (
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"github.com/majianyu2007/nwafu-connect/internal/desktopconfig"
)

func TestNativeWindowSavesConfigurationWithoutBrowserUI(t *testing.T) {
	application := test.NewApp()
	defer application.Quit()
	root := t.TempDir()
	store := desktopconfig.Store{ConfigPath: filepath.Join(root, "config.toml"), PreferencesPath: filepath.Join(root, "desktop.json"), ClientDataPath: filepath.Join(root, "client-data.json")}
	callbackCalled := false
	settings, err := New(application, store, fyne.NewStaticResource("icon.png", []byte("icon")), func(preferences desktopconfig.Preferences) error {
		callbackCalled = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if settings.Native().Title() != "NWAFU Connect 设置" {
		t.Fatalf("native window title = %q", settings.Native().Title())
	}
	settings.server.SetText("vpn.nwafu.edu.cn")
	settings.port.SetText("443")
	settings.authType.SetSelected("用户名与密码")
	settings.username.SetText("student")
	settings.password.SetText("secret")
	settings.browserURL.SetText("https://lib.nwafu.edu.cn/")
	settings.launchAtLogin.SetChecked(true)
	settings.save()
	if !callbackCalled {
		t.Fatal("native settings save callback was not called")
	}
	configuration, preferences, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.Username == nil || *configuration.Username != "student" || configuration.Password == nil || *configuration.Password != "secret" {
		t.Fatal("native settings fields were not persisted")
	}
	if !preferences.LaunchAtLogin {
		t.Fatal("native launch-at-login preference was not persisted")
	}
}
