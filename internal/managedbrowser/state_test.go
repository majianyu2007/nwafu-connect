package managedbrowser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStateRoundTripUsesPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "browser-state.json")
	want := State{ProxyAddress: "127.0.0.1:43210", StartURL: "http://127.0.0.1:54321/", Executable: "/browser", ProfileDir: "/profile"}
	if err := WriteState(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := ReadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ReadState() = %#v, want %#v", got, want)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("browser state permissions = %o, want private", info.Mode().Perm())
	}
	if err := RemoveState(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("browser state still exists: %v", err)
	}
}
