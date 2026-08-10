package tun

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/internal/hook_func"
	"github.com/majianyu2007/nwafu-connect/log"
	tun "github.com/mythologyli/sing-tun"
	"golang.org/x/sys/unix"
	"inet.af/netaddr"
)

type Endpoint struct {
	client client.Client

	ifce      tun.Tun
	ifceName  string
	ifceIndex int
	readLock  sync.Mutex
	writeLock sync.Mutex
	ip        net.IP

	ipSetBuilder netaddr.IPSetBuilder
	ipSet        *netaddr.IPSet

	tcpDialer *net.Dialer
	udpDialer *net.Dialer
}

func (ep *Endpoint) Write(buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	ep.writeLock.Lock()
	defer ep.writeLock.Unlock()
	_, err := ep.ifce.Write(buf)
	return err
}

func (ep *Endpoint) Read(buf []byte) (int, error) {
	ep.readLock.Lock()
	defer ep.readLock.Unlock()
	return ep.ifce.Read(buf)
}

func (s *Stack) AddRoute(target string) error {
	prefix, err := netaddr.ParseIPPrefix(target)
	if err != nil {
		return fmt.Errorf("parse route %q: %w", target, err)
	}
	command := exec.Command("route", "-n", "add", "-net", prefix.String(), "-interface", s.endpoint.ifceName)
	if err := command.Run(); err != nil {
		return err
	}
	hook_func.RegisterTerminalFunc("Delete route "+prefix.String(), func(ctx context.Context) error {
		return exec.Command("route", "-n", "delete", "-net", prefix.String(), "-interface", s.endpoint.ifceName).Run()
	})

	s.endpoint.ipSetBuilder.AddPrefix(prefix)
	s.endpoint.ipSet, _ = s.endpoint.ipSetBuilder.IPSet()
	return nil
}

func (s *Stack) AddDnsServer(dnsServer string, targetHost string) error {
	fileName := fmt.Sprintf("/etc/resolver/%s", targetHost)
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	file.WriteString(fmt.Sprintf("nameserver %s\n", dnsServer))

	hook_func.RegisterTerminalFunc("DelDnsServer_"+targetHost, func(ctx context.Context) error {
		delCommand := exec.Command("rm", fmt.Sprintf("/etc/resolver/%s", targetHost))
		delErr := delCommand.Run()
		if delErr != nil {
			return delErr
		}
		return nil
	})
	return nil
}

func NewStack(client client.Client, dnsHijack, fakeIP bool, ipResources []client.IPResource) (*Stack, error) {
	var err error
	s := &Stack{}
	s.setIPResources(ipResources)
	s.fakeIP = fakeIP
	s.endpoint = &Endpoint{
		client: client,
	}
	s.endpoint.ipSetBuilder = netaddr.IPSetBuilder{}

	s.endpoint.ip, err = client.IP()
	if err != nil {
		return nil, err
	}
	ipPrefix, _ := netip.ParsePrefix(s.endpoint.ip.String() + "/32")
	tunName := "utun0"
	tunName = tun.CalculateInterfaceName(tunName)
	tunOptions := tun.Options{
		Name: tunName,
		MTU:  MTU,
		Inet4Address: []netip.Prefix{
			ipPrefix,
		},
		// Inet4Address and Inet4RouteAddress must be set concurrently if we want to enable AutoRoute
		// otherwise the sing-tun will add weird router
		AutoRoute: false,
	}

	ifce, err := tun.New(tunOptions)
	if err != nil {
		return nil, err
	}
	var closeOnce sync.Once
	var closeErr error
	closeTun := func() error {
		closeOnce.Do(func() {
			closeErr = ifce.Close()
		})
		return closeErr
	}
	hook_func.RegisterTerminalFunc("Close Tun Device", func(ctx context.Context) error {
		return closeTun()
	})
	s.endpoint.ifce = ifce
	s.endpoint.ifceName = tunName
	netIfce, err := net.InterfaceByName(tunName)
	if err != nil {
		_ = closeTun()
		return nil, err
	}

	s.endpoint.ifceIndex = netIfce.Index
	log.Printf("Interface Name: %s, index %d\n", tunName, netIfce.Index)

	// We need this dialer to bind to device otherwise packets will not be sent via TUN
	// Doesn't work on macOS. See  https://github.com/Mythologyli/zju-connect/pull/44#issuecomment-1784050022
	s.endpoint.tcpDialer = &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP:   s.endpoint.ip,
			Port: 0,
		},
		Control: func(network, address string, c syscall.RawConn) error { // By ChenXuzheng
			return c.Control(func(fd uintptr) {
				if bindErr := unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_RECVIF, s.endpoint.ifceIndex); bindErr != nil {
					log.Println("Warning: failed to bind to interface", s.endpoint.ifceName)
				}
			})
		},
	}

	// Doesn't work on macOS. See  https://github.com/Mythologyli/zju-connect/pull/44#issuecomment-1784050022
	s.endpoint.udpDialer = &net.Dialer{
		LocalAddr: &net.UDPAddr{
			IP:   s.endpoint.ip,
			Port: 0,
		},
		Control: func(network, address string, c syscall.RawConn) error { // By ChenXuzheng
			return c.Control(func(fd uintptr) {
				if bindErr := unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, unix.IP_RECVIF, s.endpoint.ifceIndex); bindErr != nil {
					log.Println("Warning: failed to bind to interface", s.endpoint.ifceName)
				}
			})
		},
	}
	if dnsHijack {
		dnsServers, err := hook_func.ListNetworkServices()
		if err != nil {
			_ = closeTun()
			return nil, err
		}
		for _, dnsServer := range dnsServers {
			if hook_func.SetDNSServerWithHook(dnsServer, s.endpoint.ip.String()) != nil {
				log.Println("Warning: failed to set DNS server", s.endpoint.ifceName)
			}
		}
	}
	return s, nil
}
