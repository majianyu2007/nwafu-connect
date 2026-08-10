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
