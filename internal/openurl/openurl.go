package openurl

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"
)

func Open(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("invalid URL %q", rawURL)
	}
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", rawURL)
	case "windows":
		command = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	case "linux":
		command = exec.Command("xdg-open", rawURL)
	default:
		return fmt.Errorf("open URL is not supported on %s", runtime.GOOS)
	}
	if err := command.Start(); err != nil {
		return fmt.Errorf("open URL: %w", err)
	}
	return nil
}
