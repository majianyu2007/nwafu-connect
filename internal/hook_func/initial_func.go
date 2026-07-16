package hook_func

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/majianyu2007/nwafu-connect/configs"
	"github.com/majianyu2007/nwafu-connect/log"
	netstat "github.com/shirou/gopsutil/v4/net"
)

type InitialFunc func(ctx context.Context, config configs.Config) error
type InitialItem struct {
	f    InitialFunc
	name string
}

var initialFuncList []InitialItem

var initialEnd = false

func RegisterInitialFunc(execName string, fun InitialFunc) {
	initialFuncList = append(initialFuncList, InitialItem{
		f:    fun,
		name: execName,
	})
}

func ExecInitialFunc(ctx context.Context, config configs.Config) []error {
	var errList []error
	for _, item := range initialFuncList {
		log.Println("Exec func on initial:", item.name)
		if err := item.f(ctx, config); err != nil {
			errList = append(errList, err)
			log.Println("Exec func on initial ", item.name, "failed:", err)
		} else {
			log.Println("Exec func on initial ", item.name, "success")
		}
	}
	initialEnd = true
	return errList
}

func IsInitial() bool {
	return initialEnd
}

func checkBindPortLegal(ctx context.Context, config configs.Config) error {
	var checkTCPPorts, checkUDPPorts []uint32
	var checkTCPPortsStr, checkUDPPortsStr []string
	if !config.BrowserMode {
		checkTCPPortsStr = []string{config.HTTPBind, config.SocksBind}
		checkUDPPortsStr = []string{config.DNSServerBind}
	}

	for _, addrStr := range checkTCPPortsStr {
		if len(addrStr) != 0 {
			addr, err := net.ResolveTCPAddr("tcp", addrStr)
			if err != nil || addr.Port == 0 {
				return errors.New(fmt.Sprintf("the value for %s in the config is incorrect. Please refer to the README for the correct format", addr))
			}
			checkTCPPorts = append(checkTCPPorts, uint32(addr.Port))
		}
	}

	for _, addrStr := range checkUDPPortsStr {
		if len(addrStr) != 0 {
			addr, err := net.ResolveUDPAddr("udp", addrStr)
			if err != nil || addr.Port == 0 {
				return errors.New(fmt.Sprintf("the value for %s in the config is incorrect. Please refer to the README for the correct format", addr))
			}
			checkUDPPorts = append(checkUDPPorts, uint32(addr.Port))
		}
	}

	for _, kind := range []string{"tcp", "udp"} {
		var targetCheckPorts []uint32
		if kind == "tcp" {
			targetCheckPorts = checkTCPPorts
		} else {
			targetCheckPorts = checkUDPPorts
		}
		if len(targetCheckPorts) == 0 {
			// Browser mode does not expose local SOCKS/HTTP/DNS listeners, so
			// there is nothing to conflict with. Skipping the netstat scan
			// also avoids the macOS "Local Network" permission prompt that
			// gopsutil would otherwise trigger on first launch.
			continue
		}
		connectionStats, err := netstat.Connections(kind)
		if err != nil {
			// skip this check due to lack of information
			return nil
		}
		for _, conn := range connectionStats {
			for _, checkPort := range targetCheckPorts {
				// darwin "*" means "0.0.0.0"
				if checkPort == conn.Laddr.Port && (conn.Laddr.IP == "::" || conn.Laddr.IP == "*" ||
					conn.Laddr.IP == "0.0.0.0" || conn.Laddr.IP == "127.0.0.1") {
					return errors.New(fmt.Sprintf("%s port %s is already in use by process %d. Please choose a different port or terminate the existing process", kind, conn.Laddr.String(), conn.Pid))
				}
			}
		}
	}
	return nil
}
