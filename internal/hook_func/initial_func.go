package hook_func

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/majianyu2007/nwafu-connect/configs"
	"github.com/majianyu2007/nwafu-connect/log"
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
	if config.BrowserMode {
		return nil
	}
	type binding struct {
		name    string
		network string
		address string
	}
	bindings := []binding{
		{name: "HTTP proxy", network: "tcp", address: config.HTTPBind},
		{name: "SOCKS5 proxy", network: "tcp", address: config.SocksBind},
		{name: "DNS server", network: "udp", address: config.DNSServerBind},
	}
	listenConfig := net.ListenConfig{}
	closers := make([]io.Closer, 0, len(bindings))
	defer func() {
		for _, closer := range closers {
			_ = closer.Close()
		}
	}()
	for _, binding := range bindings {
		if binding.address == "" {
			continue
		}
		switch binding.network {
		case "tcp":
			address, err := net.ResolveTCPAddr("tcp", binding.address)
			if err != nil || address.Port == 0 {
				return fmt.Errorf("invalid %s bind address %q", binding.name, binding.address)
			}
			listener, err := listenConfig.Listen(ctx, "tcp", binding.address)
			if err != nil {
				return fmt.Errorf("%s bind address %q is unavailable: %w", binding.name, binding.address, err)
			}
			closers = append(closers, listener)
		case "udp":
			address, err := net.ResolveUDPAddr("udp", binding.address)
			if err != nil || address.Port == 0 {
				return fmt.Errorf("invalid %s bind address %q", binding.name, binding.address)
			}
			connection, err := listenConfig.ListenPacket(ctx, "udp", binding.address)
			if err != nil {
				return fmt.Errorf("%s bind address %q is unavailable: %w", binding.name, binding.address, err)
			}
			closers = append(closers, connection)
		}
	}
	return nil
}

func init() {
	RegisterInitialFunc("check listener addresses", checkBindPortLegal)
}
