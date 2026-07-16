package desktopconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreDefaultsAndPrivateRoundTrip(t *testing.T) {
	root := t.TempDir()
	store := Store{ConfigPath: filepath.Join(root, "private", "config.toml"), PreferencesPath: filepath.Join(root, "private", "desktop.json"), ClientDataPath: filepath.Join(root, "private", "client-data.json")}
	configuration, preferences, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if configuration.ServerAddress == nil || *configuration.ServerAddress != "vpn.nwafu.edu.cn" {
		t.Fatalf("default server = %v", configuration.ServerAddress)
	}
	if configuration.BrowserMode == nil || !*configuration.BrowserMode {
		t.Fatal("managed browser mode must default to enabled")
	}
	if configuration.RemoteDNSServer == nil || *configuration.RemoteDNSServer != "auto" {
		t.Fatalf("default remote DNS = %v", configuration.RemoteDNSServer)
	}
	username := "student"
	configuration.Username = &username
	preferences.LaunchAtLogin = true
	if err := store.Save(configuration, preferences); err != nil {
		t.Fatal(err)
	}
	loaded, loadedPreferences, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Username == nil || *loaded.Username != username || !loadedPreferences.LaunchAtLogin {
		t.Fatalf("round trip = %#v, %#v", loaded.Username, loadedPreferences)
	}
	for _, path := range []string{store.ConfigPath, store.PreferencesPath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s permissions = %o, want private", path, info.Mode().Perm())
		}
	}
}
