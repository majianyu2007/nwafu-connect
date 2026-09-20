package dial

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/internal/ipresource"
	"github.com/majianyu2007/nwafu-connect/log"
	"github.com/majianyu2007/nwafu-connect/resolve"
	"github.com/majianyu2007/nwafu-connect/stack"
)

import (
	"context"
	"errors"
)

// ErrACLDenied is returned when a caller forces VPN routing for a destination
// outside the resources authorized by the aTrust gateway.
var ErrACLDenied = errors.New("destination not in aTrust resources")

func aclDenied(network, address string) error {
	log.Printf("ACL: refusing %s/%s because it is not authorized by the aTrust gateway", address, network)
	return fmt.Errorf("%w: %s/%s", ErrACLDenied, address, network)
}

type Dialer struct {
	stack                stack.Stack
	resolver             *resolve.Resolver
	resourceIndex        *ipresource.Index
	alwaysUseVPN         bool
	dialDirectHTTPProxy  string // format: "ip:port"
	dialDirectSocksProxy string // WORKING IN PROCESS
}

// dialDirectIP need have a `hostAddr` parameter, which will be passed to PROXY. But `hostAddr` maybe empty, ipAddr never be empty.
func (d *Dialer) dialDirectIP(ctx context.Context, network, ipAddr string, hostAddr string) (net.Conn, error) {
	// only support http proxy now and tcp network type
	if d.dialDirectHTTPProxy != "" && network == "tcp" {
		usedAddr := ipAddr
		if hostAddr != "" {
			usedAddr = hostAddr
		}
		return d.dialDirectWithHTTPProxy(ctx, usedAddr)
		// only support tcp for socks proxy
	} else if d.dialDirectSocksProxy != "" && network == "tcp" {
		if hostAddr != "" {
			return d.dialDirectWithSocksProxy(ctx, network, hostAddr, false)
		} else {
			return d.dialDirectWithSocksProxy(ctx, network, ipAddr, true)
		}
	} else {
		return d.dialDirectWithoutProxy(ctx, network, ipAddr)
	}
}

func (d *Dialer) dialDirectHost(ctx context.Context, network, hostAddr string) (net.Conn, error) {
	// only support http proxy now and tcp network type
	if d.dialDirectHTTPProxy != "" && network == "tcp" {
		return d.dialDirectWithHTTPProxy(ctx, hostAddr)
		// only support tcp for socks proxy
	} else if d.dialDirectSocksProxy != "" && network == "tcp" {
		return d.dialDirectWithSocksProxy(ctx, network, hostAddr, false)
	} else {
		return d.dialDirectWithoutProxy(ctx, network, hostAddr)
	}
}

func (d *Dialer) DialIPPort(ctx context.Context, network, ipAddr string) (net.Conn, error) {
	ipString, portString, err := net.SplitHostPort(ipAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid address %q: %w", ipAddr, err)
	}
	ip := net.ParseIP(ipString)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address %q", ipString)
	}
	port, err := strconv.Atoi(portString)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid port in address %q", ipAddr)
	}

	hostAddr := ""
	if resolvedHost, ok := ctx.Value(resolve.ContextKeyResolveHost).(string); ok && resolvedHost != "" {
		hostAddr = net.JoinHostPort(resolvedHost, portString)
	}
	if ip.To4() == nil {
		if d.alwaysUseVPN {
			return nil, aclDenied(network, ipAddr)
		}
		return d.dialDirectIP(ctx, network, ipAddr, hostAddr)
	}

	resourceNetwork := network
	switch {
	case strings.HasPrefix(network, "tcp"):
		resourceNetwork = "tcp"
	case strings.HasPrefix(network, "udp"):
		resourceNetwork = "udp"
	}

	matchedResource := false
	if res := ctx.Value(resolve.ContextKeyDomainResource); res != nil {
		if resources, ok := res.(client.DomainResourceSet); ok {
			resource, matched := matchDomainResourceForTunnel(resources, resourceNetwork, port)
			matchedResource = matched
			if matched {
				ctx = context.WithValue(ctx, resolve.ContextKeyDomainResource, resource)
			}
		}
	}
	if !matchedResource {
		resource, matched := matchIPResourceForTunnel(d.resourceIndex, ip, resourceNetwork, port)
		matchedResource = matched
		if matched {
			ctx = context.WithValue(ctx, resolve.ContextKeyIPResource, resource)
		}
	}

	if d.alwaysUseVPN && !matchedResource {
		return nil, aclDenied(network, ipAddr)
	}
	if !matchedResource {
		return d.dialDirectIP(ctx, network, ipAddr, hostAddr)
	}

	target := &net.IPAddr{IP: ip}
	switch resourceNetwork {
	case "tcp":
		log.Printf("%s -> VPN", ipAddr)
		return d.stack.DialTCP(ctx, &net.TCPAddr{IP: target.IP, Port: port})
	case "udp":
		log.Printf("%s -> VPN", ipAddr)
		return d.stack.DialUDP(ctx, &net.UDPAddr{IP: target.IP, Port: port})
	default:
		if d.alwaysUseVPN {
			return nil, fmt.Errorf("VPN does not support network %q", network)
		}
		log.Printf("VPN does not support %s; using direct connection for %s", network, ipAddr)
		return d.dialDirectIP(ctx, network, ipAddr, hostAddr)
	}
}

func (d *Dialer) Dial(ctx context.Context, network string, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if d.alwaysUseVPN {
			return nil, fmt.Errorf("invalid managed-browser destination %q: %w", addr, err)
		}
		return d.dialDirectHost(ctx, network, addr)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		if d.resolver == nil {
			if d.alwaysUseVPN {
				return nil, aclDenied(network, addr)
			}
			return d.dialDirectHost(ctx, network, addr)
		}
		ctx, ip, err = d.resolver.Resolve(ctx, host)
		if err != nil {
			if d.alwaysUseVPN {
				return nil, fmt.Errorf("resolve managed-browser destination %q: %w", host, err)
			}
			return d.dialDirectHost(ctx, network, addr)
		}
	}

	return d.DialIPPort(ctx, network, net.JoinHostPort(ip.String(), port))
}

func NewDialer(stack stack.Stack, resolver *resolve.Resolver, ipResources []client.IPResource, alwaysUseVPN bool, dialDirectProxy string) *Dialer {
	dialHttpProxy := ""
	dialSocksProxy := ""
	if strings.HasPrefix(dialDirectProxy, "http://") {
		dialHttpProxy = strings.TrimPrefix(dialDirectProxy, "http://")
	} else if strings.HasPrefix(dialDirectProxy, "socks://") {
		dialSocksProxy = strings.TrimPrefix(dialDirectProxy, "socks://")
	} else if len(dialDirectProxy) > 0 {
		log.Println("暂不支持除[http/socks]之外的DialDirectProxy，忽略该配置项")
	}
	return &Dialer{
		stack:                stack,
		resolver:             resolver,
		resourceIndex:        ipresource.New(ipResources),
		alwaysUseVPN:         alwaysUseVPN,
		dialDirectHTTPProxy:  dialHttpProxy,
		dialDirectSocksProxy: dialSocksProxy,
	}
}

func matchDomainResourceForTunnel(resources []client.DomainResource, network string, port int) (client.DomainResource, bool) {
	if network == "tcp" {
		if resource, ok := client.MatchDomainResourceWhere(resources, network, port, func(resource client.DomainResource) bool {
			return !resource.EnableTCPPrefL3
		}); ok {
			return resource, true
		}
	}
	return client.MatchDomainResource(resources, network, port)
}

func matchesIPResource(index *ipresource.Index, target net.IP, network string, port int) bool {
	_, ok := matchIPResourceForTunnel(index, target, network, port)
	return ok
}

func matchIPResourceForTunnel(index *ipresource.Index, target net.IP, network string, port int) (client.IPResource, bool) {
	if network == "tcp" {
		if resource, ok := index.MatchWhere(target, network, port, func(resource client.IPResource) bool {
			return !resource.EnableTCPPrefL3
		}); ok {
			return resource, true
		}
	}
	return index.Match(target, network, port)
}
