package hook_func

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"os/user"
	"strings"

	"github.com/majianyu2007/nwafu-connect/configs"
)

// get all services and skip element contains "*"
func ListNetworkServices() ([]string, error) {
	cmd := exec.Command("networksetup", "-listallnetworkservices")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(output), "\n")
	var services []string
	for _, line := range lines[1:] { // Skip the first header line
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "*") {
			services = append(services, line)
		}
	}
	return services, nil
}

func currentDNSServers(service string) ([]string, error) {
	command := exec.Command("networksetup", "-getdnsservers", service)
	output, err := command.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("read DNS servers for %q: %s: %w", service, strings.TrimSpace(string(output)), err)
	}
	return parseDNSServers(string(output)), nil
}

func parseDNSServers(output string) []string {
	text := strings.TrimSpace(output)
	if text == "" || strings.HasPrefix(text, "There aren't any DNS Servers set") {
		return nil
	}
	servers := make([]string, 0, 2)
	for _, line := range strings.Split(text, "\n") {
		server := strings.TrimSpace(line)
		if net.ParseIP(server) != nil {
			servers = append(servers, server)
		}
	}
	return servers
}

func SetDNSServerWithHook(service, dns string) error {
	previousServers, err := currentDNSServers(service)
	if err != nil {
		return err
	}
	command := exec.Command("networksetup", "-setdnsservers", service, dns)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("set DNS server for %q: %s: %w", service, strings.TrimSpace(string(output)), err)
	}

	restoreServers := append([]string(nil), previousServers...)
	if len(restoreServers) == 0 {
		restoreServers = []string{"Empty"}
	}
	RegisterTerminalFunc("RestoreDnsServer_"+service, func(ctx context.Context) error {
		args := append([]string{"-setdnsservers", service}, restoreServers...)
		output, err := exec.Command("networksetup", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("restore DNS servers for %q: %s: %w", service, strings.TrimSpace(string(output)), err)
		}
		return nil
	})
	return nil
}

func init() {
	RegisterInitialFunc("check tun mode cap", func(ctx context.Context, config configs.Config) error {
		if config.TUNMode && !config.BrowserMode {
			current, err := user.Current()
			if err != nil {
				return fmt.Errorf("identify current user: %w", err)
			}
			if current.Uid != "0" {
				return errors.New("run TUN mode using sudo to grant necessary permissions")
			}
		}
		return nil
	})
	// DNS hijacking is applied by stack/tun.NewStack after the TUN interface
	// exists. Applying it here would discard the user's current DNS settings
	// even when later startup steps fail.

}
