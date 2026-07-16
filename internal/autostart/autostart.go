package autostart

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const applicationID = "com.nwafu.connect.desktop"

func Set(enabled bool, executable string) error {
	absolute, err := filepath.Abs(executable)
	if err != nil {
		return fmt.Errorf("resolve desktop executable: %w", err)
	}
	switch runtime.GOOS {
	case "darwin":
		return setDarwin(enabled, absolute)
	case "windows":
		return setWindows(enabled, absolute)
	case "linux":
		return setLinux(enabled, absolute)
	default:
		return fmt.Errorf("launch at login is not supported on %s", runtime.GOOS)
	}
}

func setDarwin(enabled bool, executable string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(home, "Library", "LaunchAgents", applicationID+".plist")
	if !enabled {
		return remove(path)
	}
	payload := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>%s</string>
<key>ProgramArguments</key><array><string>%s</string></array>
<key>RunAtLoad</key><true/>
<key>KeepAlive</key><false/>
</dict></plist>
`, applicationID, xmlEscape(executable))
	return write(path, []byte(payload))
}

func setWindows(enabled bool, executable string) error {
	arguments := []string{"add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "NWAFU Connect", "/t", "REG_SZ", "/d", `"` + executable + `"`, "/f"}
	if !enabled {
		arguments = []string{"delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, "/v", "NWAFU Connect", "/f"}
	}
	if output, err := exec.Command("reg.exe", arguments...).CombinedOutput(); err != nil {
		return fmt.Errorf("update Windows launch at login: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func setLinux(enabled bool, executable string) error {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	path := filepath.Join(configRoot, "autostart", applicationID+".desktop")
	if !enabled {
		return remove(path)
	}
	payload := fmt.Sprintf("[Desktop Entry]\nType=Application\nName=NWAFU Connect\nExec=%s\nTerminal=false\nX-GNOME-Autostart-enabled=true\n", desktopQuote(executable))
	return write(path, []byte(payload))
}

func write(path string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o600)
}

func remove(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return replacer.Replace(value)
}

func desktopQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
}
