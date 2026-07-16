package appdata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveUsesPrivateOverride(t *testing.T) {
	root := filepath.Join(t.TempDir(), "desktop-data")
	t.Setenv("NWAFU_CONNECT_DATA_DIR", root)
	paths, err := Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if paths.Root != root || paths.Config != filepath.Join(root, "config.toml") || paths.BrowserProfile != filepath.Join(root, "browser-profile") {
		t.Fatalf("resolved paths = %#v", paths)
	}
	for _, directory := range []string{paths.Root, paths.BrowserProfile} {
		info, err := os.Stat(directory)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s permissions = %o, want private", directory, info.Mode().Perm())
		}
	}
}
