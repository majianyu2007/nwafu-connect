package managedbrowser

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type Options struct {
	Executable   string
	ProxyAddress string
	StartURL     string
	ProfileDir   string
}

type Process struct {
	cmd           *exec.Cmd
	ctx           context.Context
	profileDir    string
	executable    string
	removeProfile bool
}

func Start(ctx context.Context, options Options) (*Process, error) {
	if _, _, err := net.SplitHostPort(options.ProxyAddress); err != nil {
		return nil, fmt.Errorf("invalid browser proxy address %q: %w", options.ProxyAddress, err)
	}
	startURL, err := url.Parse(options.StartURL)
	if err != nil || startURL.Host == "" || (startURL.Scheme != "http" && startURL.Scheme != "https") {
		return nil, fmt.Errorf("invalid browser start URL %q", options.StartURL)
	}
	executable, err := FindExecutable(options.Executable)
	if err != nil {
		return nil, err
	}
	profileDir := options.ProfileDir
	removeProfile := false
	if profileDir == "" {
		profileDir, err = os.MkdirTemp("", "nwafu-connect-browser-")
		removeProfile = true
	} else {
		err = os.MkdirAll(profileDir, 0o700)
	}
	if err != nil {
		return nil, fmt.Errorf("create browser profile: %w", err)
	}

	args := browserArgs(profileDir, options.ProxyAddress, startURL.String())
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		if removeProfile {
			_ = os.RemoveAll(profileDir)
		}
		return nil, fmt.Errorf("start managed browser: %w", err)
	}
	return &Process{
		cmd:           cmd,
		ctx:           ctx,
		profileDir:    profileDir,
		executable:    executable,
		removeProfile: removeProfile,
	}, nil
}

func (p *Process) Executable() string {
	return p.executable
}

func (p *Process) Wait() error {
	err := p.cmd.Wait()
	if p.removeProfile {
		_ = os.RemoveAll(p.profileDir)
	}
	if p.ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return fmt.Errorf("managed browser exited: %w", err)
	}
	return nil
}

func browserArgs(profileDir, proxyAddress, startURL string) []string {
	return []string{
		"--user-data-dir=" + profileDir,
		"--proxy-server=http://" + proxyAddress,
		"--proxy-bypass-list=127.0.0.1;localhost",
		"--homepage=" + startURL,
		"--show-home-button",
		"--disable-quic",
		"--disable-background-mode",
		"--force-webrtc-ip-handling-policy=disable_non_proxied_udp",
		"--password-store=basic",
		"--use-mock-keychain",
		"--no-first-run",
		"--no-default-browser-check",
		"--new-window",
		startURL,
	}
}

func FindExecutable(configured string) (string, error) {
	if configured != "" {
		if path, err := exec.LookPath(configured); err == nil {
			return path, nil
		}
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured, nil
		}
		return "", fmt.Errorf("configured browser executable not found: %s", configured)
	}

	for _, candidate := range browserCandidates() {
		if candidate == "" {
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", errors.New("no Chromium-based browser found; set browser_path or -browser-path")
}

func browserCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"google-chrome",
			"chromium",
		}
	case "windows":
		candidates := []string{"msedge.exe", "chrome.exe", "brave.exe"}
		for _, root := range []string{
			os.Getenv("PROGRAMFILES"),
			os.Getenv("PROGRAMFILES(X86)"),
			os.Getenv("LOCALAPPDATA"),
		} {
			if root == "" {
				continue
			}
			candidates = append(candidates,
				filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"),
				filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(root, "BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
			)
		}
		return candidates
	case "linux":
		return []string{
			"google-chrome",
			"google-chrome-stable",
			"chromium",
			"chromium-browser",
			"microsoft-edge",
			"microsoft-edge-stable",
			"brave-browser",
		}
	default:
		return nil
	}
}
