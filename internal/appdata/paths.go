package appdata

import (
	"fmt"
	"os"
	"path/filepath"
)

type Paths struct {
	Root           string
	Config         string
	ClientData     string
	BrowserProfile string
	BrowserState   string
	Log            string
}

func Resolve() (Paths, error) {
	root := os.Getenv("NWAFU_CONNECT_DATA_DIR")
	if root == "" {
		configRoot, err := os.UserConfigDir()
		if err != nil {
			return Paths{}, fmt.Errorf("resolve user config directory: %w", err)
		}
		root = filepath.Join(configRoot, "NWAFU Connect")
	} else {
		var err error
		root, err = filepath.Abs(root)
		if err != nil {
			return Paths{}, fmt.Errorf("resolve custom private data directory: %w", err)
		}
	}
	paths := Paths{
		Root:           root,
		Config:         filepath.Join(root, "config.toml"),
		ClientData:     filepath.Join(root, "client-data.json"),
		BrowserProfile: filepath.Join(root, "browser-profile"),
		BrowserState:   filepath.Join(root, "browser-state.json"),
		Log:            filepath.Join(root, "nwafu-connect.log"),
	}
	for _, directory := range []string{paths.Root, paths.BrowserProfile} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return Paths{}, fmt.Errorf("create private data directory %s: %w", directory, err)
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return Paths{}, fmt.Errorf("protect private data directory %s: %w", directory, err)
		}
	}
	return paths, nil
}
