//go:build !tun

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWritePrivateFileReplacesContentAndProtectsPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "client-data.json")
	if err := writePrivateFile(path, []byte("a much longer previous payload")); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateFile(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "new" {
		t.Fatalf("private file payload = %q, want %q", payload, "new")
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if permissions := info.Mode().Perm(); permissions != 0600 {
			t.Fatalf("private file permissions = %04o, want 0600", permissions)
		}
	}
}
