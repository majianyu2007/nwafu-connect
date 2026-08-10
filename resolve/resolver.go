package resolve

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"sync"
	"time"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/internal/ippool"
	"github.com/majianyu2007/nwafu-connect/log"
	"github.com/majianyu2007/nwafu-connect/stack"
	"github.com/patrickmn/go-cache"
	"golang.org/x/sync/singleflight"
)

const dnsLookupTimeout = 10 * time.Second

type Resolver struct {
	remoteUDPResolver *net.Resolver
	remoteTCPResolver *net.Resolver
	secondaryResolver *net.Resolver
	isolatedResolver  *dohResolver
	ttl               uint64
	domainIndex       *domainResourceIndex
	dnsResource       map[string]net.IP
	useRemoteDNS      bool

	dnsCache *cache.Cache

	IPPool *ippool.IPPool[client.DomainResourceSet]

	timer  *time.Timer
	useTCP bool
	// check to use tcp resolver or udp resolver
	tcpLock     sync.RWMutex
	lookupGroup singleflight.Group

	closeOnce sync.Once
}

type contextKey string

var (
	ContextKeyFakeIP         = contextKey("FAKE_IP")
	ContextKeyResolveHost    = contextKey("RESOLVE_HOST")
	ContextKeyDomainResource = contextKey("DOMAIN_RESOURCE")
)

// Resolve resolves host and records the matching aTrust resource in the returned context.
func (r *Resolver) Resolve(ctx context.Context, host string) (resCtx context.Context, resIP net.IP, resErr error) {
	host = normalizeHostname(host)
	if host == "" {
		return ctx, nil, errors.New("host is empty")
	}
	resCtx = ctx
	defer func() {
		if resErr == nil {
			resCtx = context.WithValue(resCtx, ContextKeyResolveHost, host)
		}
	}()

	domainResources, matchedDomain, domainResourceFound := r.domainIndex.Match(host)
	if domainResourceFound {
		resCtx = context.WithValue(resCtx, ContextKeyDomainResource, domainResources)
		log.DebugPrintf("Domain resource found: %s", matchedDomain)
	}
	if fakeIPValue := resCtx.Value(ContextKeyFakeIP); fakeIPValue != nil && domainResourceFound {
		ip, err := r.IPPool.GenerateIP(host, domainResources)
		if err != nil {
			return resCtx, nil, fmt.Errorf("allocate fake IP for %s: %w", host, err)
		}
		log.Printf("%s -> %s (Fake IP)", host, ip)
		return resCtx, ip, nil
	}
	if cachedIP, found := r.getDNSCache(host); found {
		if domainResourceFound {
			if err := r.IPPool.SetIPDomain(cachedIP, host, domainResources); err != nil {
				log.DebugPrintf("Set cached IP mapping err: %s", err)
			}
		}
		log.Printf("%s -> %s", host, cachedIP)
		return resCtx, cachedIP, nil
	}
	if ip, found := r.dnsResource[host]; found && ip != nil {
		if domainResourceFound {
			if err := r.IPPool.SetIPDomain(ip, host, domainResources); err != nil {
				log.DebugPrintf("Set IP err: %s", err)
			}
		}
		log.Printf("%s -> %s", host, ip)
		return resCtx, ip, nil
	}

	resultChannel := r.lookupGroup.DoChan(host, func() (any, error) {
		if cachedIP, found := r.getDNSCache(host); found {
			return cachedIP, nil
		}
		// A singleflight lookup outlives any one caller. Every waiter still
		// observes its own cancellation through the select below.
		lookupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), dnsLookupTimeout)
		defer cancel()
		ip, err := r.lookupHost(lookupContext, host)
		if err != nil {
			return nil, err
		}
		r.setDNSCache(host, ip)
		return ip, nil
	})
	select {
	case <-ctx.Done():
		return resCtx, nil, ctx.Err()
	case result := <-resultChannel:
		if result.Err != nil {
			return resCtx, nil, result.Err
		}
		ip, ok := result.Val.(net.IP)
		if !ok || ip == nil {
			return resCtx, nil, errors.New("DNS lookup returned no IP address")
		}
		if domainResourceFound {
			if err := r.IPPool.SetIPDomain(ip, host, domainResources); err != nil {
				log.DebugPrintf("Set IP err: %s", err)
			}
		}
		return resCtx, ip, nil
	}
}

func (r *Resolver) lookupHost(ctx context.Context, host string) (net.IP, error) {
	if !r.useRemoteDNS {
		return r.lookupSecondary(ctx, host)
	}

	r.tcpLock.RLock()
	useTCP := r.useTCP
	r.tcpLock.RUnlock()
	if !useTCP {
		ips, err := r.remoteUDPResolver.LookupIP(ctx, "ip4", host)
		if err == nil && len(ips) > 0 {
			log.Printf("%s -> %s", host, ips[0])
			return ips[0], nil
		}
		ips, tcpErr := r.remoteTCPResolver.LookupIP(ctx, "ip4", host)
		if tcpErr == nil && len(ips) > 0 {
			r.preferRemoteTCP()
			log.Printf("%s -> %s", host, ips[0])
			return ips[0], nil
		}
		log.Printf("Resolve IPv4 addr failed using remote UDP/TCP DNS: %s, using secondary DNS instead", host)
		return r.lookupSecondary(ctx, host)
	}

	ips, err := r.remoteTCPResolver.LookupIP(ctx, "ip4", host)
	if err == nil && len(ips) > 0 {
		log.Printf("%s -> %s", host, ips[0])
		return ips[0], nil
	}
	log.Printf("Resolve IPv4 addr failed using remote TCP DNS: %s, using secondary DNS instead", host)
	return r.lookupSecondary(ctx, host)
}

func (r *Resolver) preferRemoteTCP() {
	r.tcpLock.Lock()
	defer r.tcpLock.Unlock()
	r.useTCP = true
	if r.timer != nil {
		return
	}
	r.timer = time.AfterFunc(10*time.Minute, func() {
		r.tcpLock.Lock()
		r.useTCP = false
		r.timer = nil
		r.tcpLock.Unlock()
	})
}

func (r *Resolver) lookupSecondary(ctx context.Context, host string) (net.IP, error) {
	if r.isolatedResolver != nil {
		targets, err := r.isolatedResolver.LookupIPv4(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve IPv4 using isolated DoH for %s: %w", host, err)
		}
		if len(targets) == 0 {
			return nil, fmt.Errorf("isolated DoH returned no IPv4 address for %s", host)
		}
		log.Printf("%s -> %s (isolated DoH)", host, targets[0])
		return targets[0], nil
	}

	targets, err := r.secondaryResolver.LookupIP(ctx, "ip4", host)
	if err == nil && len(targets) > 0 {
		log.Printf("%s -> %s", host, targets[0])
		return targets[0], nil
	}
	log.Printf("Resolve IPv4 addr failed using secondary DNS: %s. Try IPv6 addr", host)
	targets, ipv6Err := r.secondaryResolver.LookupIP(ctx, "ip6", host)
	if ipv6Err != nil {
		return nil, fmt.Errorf("resolve %s using secondary DNS: %w", host, errors.Join(err, ipv6Err))
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("secondary DNS returned no address for %s", host)
	}
	log.Printf("%s -> %s", host, targets[0])
	return targets[0], nil
}

func (r *Resolver) RemoteUDPResolver() (*net.Resolver, error) {
	if r.remoteUDPResolver != nil {
		return r.remoteUDPResolver, nil
	} else {
		return nil, errors.New("remote UDP resolver is nil")
	}
}

func (r *Resolver) RemoteTCPResolver() (*net.Resolver, error) {
	if r.remoteTCPResolver != nil {
		return r.remoteTCPResolver, nil
	} else {
		return nil, errors.New("remote TCP resolver is nil")
	}
}

func (r *Resolver) ResolveWithSecondaryDNS(ctx context.Context, host string) (context.Context, net.IP, error) {
	host = normalizeHostname(host)
	ip, err := r.lookupSecondary(ctx, host)
	if err != nil {
		return ctx, nil, err
	}
	r.setDNSCache(host, ip)
	return ctx, ip, nil
}

func (r *Resolver) Close() {
	r.closeOnce.Do(func() {
		r.tcpLock.Lock()
		if r.timer != nil {
			r.timer.Stop()
			r.timer = nil
		}
		if r.isolatedResolver != nil {
			r.isolatedResolver.client.CloseIdleConnections()
		}
		r.tcpLock.Unlock()
	})
}

func NewResolver(stack stack.Stack, remoteDNSServer, secondaryDNSServer string, ttl uint64, domainResources map[string]client.DomainResourceSet, dnsResource map[string]net.IP, useRemoteDNS, isolatedDNS bool) *Resolver {
	ttl = min(ttl, uint64(math.MaxUint32))
	normalizedDNS := make(map[string]net.IP, len(dnsResource))
	for host, ip := range dnsResource {
		host = normalizeHostname(host)
		if host != "" && ip != nil {
			normalizedDNS[host] = ip
		}
	}

	remoteDNSIP := net.ParseIP(remoteDNSServer)
	resolver := &Resolver{
		remoteUDPResolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				if stack == nil || remoteDNSIP == nil {
					return nil, errors.New("remote DNS stack or address is unavailable")
				}
				return stack.DialUDP(ctx, &net.UDPAddr{IP: remoteDNSIP, Port: 53})
			},
		},
		remoteTCPResolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				if stack == nil || remoteDNSIP == nil {
					return nil, errors.New("remote DNS stack or address is unavailable")
				}
				return stack.DialTCP(ctx, &net.TCPAddr{IP: remoteDNSIP, Port: 53})
			},
		},
		ttl:          ttl,
		domainIndex:  newDomainResourceIndex(domainResources),
		dnsResource:  normalizedDNS,
		dnsCache:     cache.New(time.Duration(ttl)*time.Second, time.Duration(ttl)*2*time.Second),
		useRemoteDNS: useRemoteDNS,
	}
	if isolatedDNS {
		resolver.isolatedResolver = newDoHResolver()
	}

	if secondaryDNSServer != "" {
		secondaryAddress := net.JoinHostPort(secondaryDNSServer, "53")
		resolver.secondaryResolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, secondaryAddress)
			},
		}
	} else {
		resolver.secondaryResolver = &net.Resolver{PreferGo: true}
	}
	var err error
	resolver.IPPool, err = ippool.NewIPPool[client.DomainResourceSet]("198.18.0.0/16")
	if err != nil {
		log.Fatalf("Create Fake IP Pool failed: %v", err)
	}
	return resolver
}
