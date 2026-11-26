package resolve

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/internal/ippool"
	"github.com/majianyu2007/nwafu-connect/log"
	"github.com/majianyu2007/nwafu-connect/stack"
	"github.com/patrickmn/go-cache"
)

const (
	dnsLookupTimeout          = 10 * time.Second
	remoteDNSTCPFallbackDelay = 300 * time.Millisecond
)

type Resolver struct {
	remoteUDPResolver *net.Resolver
	remoteTCPResolver *net.Resolver
	secondaryResolver *net.Resolver
	isolatedResolver  *dohResolver
	ttl               uint64
	domainIndex       *domainResourceIndex
	dnsResource       map[string][]net.IP
	dnsResourceCursor sync.Map
	useRemoteDNS      bool

	dnsCache *cache.Cache

	IPPool *ippool.IPPool[[]client.DomainResource]

	timer  *time.Timer
	useTCP bool
	// check to use tcp resolver or udp resolver
	tcpLock           sync.RWMutex
	resolutionMu      sync.Mutex
	resolutions       map[string]*sharedResolution
	resolutionClosed  bool
	activeResolutions atomic.Int64

	closeOnce sync.Once
}

type lookupIPFunc func(context.Context, string, string) ([]net.IP, error)

type dnsLookupResult struct {
	ips []net.IP
	err error
}

type sharedResolution struct {
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	waiters int
	ip      net.IP
	err     error
}

func (r *Resolver) coordinationEntryCount() int {
	return int(r.activeResolutions.Load())
}

type contextKey string

var (
	ContextKeyFakeIP          = contextKey("FAKE_IP")
	ContextKeyResolveHost     = contextKey("RESOLVE_HOST")
	ContextKeyDomainResource  = contextKey("DOMAIN_RESOURCE")
	ContextKeyIPResource      = contextKey("IP_RESOURCE")
	contextKeyIgnoreTCPPrefL3 = contextKey("IGNORE_TCP_PREF_L3")
)

func WithIgnoreTCPPrefL3(ctx context.Context) context.Context {
	return context.WithValue(ctx, contextKeyIgnoreTCPPrefL3, true)
}

func IgnoreTCPPrefL3(ctx context.Context) bool {
	ignore, _ := ctx.Value(contextKeyIgnoreTCPPrefL3).(bool)
	return ignore
}

func TCPPrefersL3(ctx context.Context) bool {
	if resource, ok := ctx.Value(ContextKeyDomainResource).(client.DomainResource); ok {
		return resource.EnableTCPPrefL3
	}
	if resource, ok := ctx.Value(ContextKeyIPResource).(client.IPResource); ok {
		return resource.EnableTCPPrefL3
	}
	return false
}

// Resolve resolves host and records matching aTrust domain resources in the returned context.
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

	matchedDomain, domainResources, domainResourceFound := r.domainIndex.Match(host)
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
		if domainResourceFound && r.IPPool != nil {
			if err := r.IPPool.SetIPDomain(cachedIP, host, domainResources); err != nil {
				log.DebugPrintf("Set cached IP mapping err: %s", err)
			}
		}
		log.Printf("%s -> %s", host, cachedIP)
		return resCtx, cachedIP, nil
	}
	if r.dnsResource != nil {
		if ips, found := r.dnsResource[host]; found && len(ips) > 0 {
			cursorValue, _ := r.dnsResourceCursor.LoadOrStore(host, &atomic.Uint64{})
			cursor := cursorValue.(*atomic.Uint64)
			ip := ips[(cursor.Add(1)-1)%uint64(len(ips))]
			if domainResourceFound && r.IPPool != nil {
				if err := r.IPPool.SetIPDomain(ip, host, domainResources); err != nil {
					log.DebugPrintf("Set IP err: %s", err)
				}
			}
			log.Printf("%s -> %s", host, ip)
			return resCtx, ip, nil
		}
	}

	ip, err := r.resolveCoordinated(resCtx, host, func(lookupCtx context.Context) (net.IP, error) {
		if cachedIP, found := r.getDNSCache(host); found {
			return cachedIP, nil
		}
		boundedCtx, cancel := context.WithTimeout(lookupCtx, dnsLookupTimeout)
		defer cancel()
		return r.lookupHost(boundedCtx, host)
	})
	if err != nil {
		return resCtx, nil, err
	}
	if ip == nil {
		return resCtx, nil, errors.New("DNS lookup returned no IP address")
	}
	if domainResourceFound && r.IPPool != nil {
		if err := r.IPPool.SetIPDomain(ip, host, domainResources); err != nil {
			log.DebugPrintf("Set IP err: %s", err)
		}
	}
	return resCtx, ip, nil
}

func (r *Resolver) resolveCoordinated(ctx context.Context, host string, lookup func(context.Context) (net.IP, error)) (net.IP, error) {
	r.resolutionMu.Lock()
	if r.resolutionClosed {
		r.resolutionMu.Unlock()
		return nil, net.ErrClosed
	}
	call := r.resolutions[host]
	if call == nil {
		lookupCtx, cancel := context.WithCancel(context.Background())
		call = &sharedResolution{ctx: lookupCtx, cancel: cancel, done: make(chan struct{})}
		if r.resolutions == nil {
			r.resolutions = make(map[string]*sharedResolution)
		}
		r.resolutions[host] = call
		r.activeResolutions.Add(1)
		go r.runResolution(host, call, lookup)
	}
	call.waiters++
	r.resolutionMu.Unlock()

	select {
	case <-ctx.Done():
		r.releaseResolutionWaiter(host, call)
		return nil, ctx.Err()
	case <-call.done:
		return call.ip, call.err
	}
}

func (r *Resolver) runResolution(host string, call *sharedResolution, lookup func(context.Context) (net.IP, error)) {
	call.ip, call.err = lookup(call.ctx)
	r.resolutionMu.Lock()
	if r.resolutions[host] == call {
		delete(r.resolutions, host)
	}
	r.resolutionMu.Unlock()
	call.cancel()
	r.activeResolutions.Add(-1)
	close(call.done)
}

func (r *Resolver) releaseResolutionWaiter(host string, call *sharedResolution) {
	r.resolutionMu.Lock()
	if r.resolutions[host] == call {
		call.waiters--
		if call.waiters == 0 {
			delete(r.resolutions, host)
			call.cancel()
		}
	}
	r.resolutionMu.Unlock()
}

func (r *Resolver) lookupHost(ctx context.Context, host string) (net.IP, error) {
	if !r.useRemoteDNS {
		ip, err := r.lookupSecondary(ctx, host)
		if err != nil {
			return nil, err
		}
		r.setDNSCache(host, ip)
		return ip, nil
	}

	ip, err := r.resolveRemote(ctx, host)
	if err == nil {
		log.Printf("%s -> %s", host, ip)
		return ip, nil
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	log.Printf("Resolve IPv4 addr failed using remote DNS: %s, using secondary DNS instead", host)
	ip, err = r.lookupSecondary(ctx, host)
	if err != nil {
		return nil, err
	}
	r.setDNSCache(host, ip)
	return ip, nil
}

func (r *Resolver) resolveRemote(ctx context.Context, host string) (net.IP, error) {
	r.tcpLock.RLock()
	useTCP := r.useTCP
	r.tcpLock.RUnlock()

	if useTCP {
		ips, err := r.remoteTCPResolver.LookupIP(ctx, "ip4", host)
		return r.cacheFirstIP(host, ips, err)
	}

	ips, preferTCP, err := lookupIPWithTCPFallback(
		ctx,
		host,
		r.remoteUDPResolver.LookupIP,
		r.remoteTCPResolver.LookupIP,
		remoteDNSTCPFallbackDelay,
	)
	if err == nil && preferTCP {
		r.preferTCPTemporarily()
	}
	return r.cacheFirstIP(host, ips, err)
}

func lookupIPWithTCPFallback(ctx context.Context, host string, udpLookup, tcpLookup lookupIPFunc, fallbackDelay time.Duration) ([]net.IP, bool, error) {
	lookupCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	udpResult := make(chan dnsLookupResult, 1)
	go func() {
		ips, err := udpLookup(lookupCtx, "ip4", host)
		udpResult <- dnsLookupResult{ips: ips, err: err}
	}()

	timer := time.NewTimer(fallbackDelay)
	defer timer.Stop()
	var tcpResult chan dnsLookupResult
	var udpErr, tcpErr error
	udpDone := false
	tcpDone := false

	startTCP := func() {
		if tcpResult != nil {
			return
		}
		tcpResult = make(chan dnsLookupResult, 1)
		go func() {
			ips, err := tcpLookup(lookupCtx, "ip4", host)
			tcpResult <- dnsLookupResult{ips: ips, err: err}
		}()
	}

	for {
		select {
		case result := <-udpResult:
			udpDone = true
			if result.err == nil && len(result.ips) > 0 {
				return result.ips, false, nil
			}
			udpErr = result.err
			if udpErr == nil {
				udpErr = errors.New("UDP DNS returned no addresses")
			}
			startTCP()
			if tcpDone {
				return nil, true, errors.Join(udpErr, tcpErr)
			}
		case result := <-tcpResult:
			tcpDone = true
			if result.err == nil && len(result.ips) > 0 {
				return result.ips, udpDone, nil
			}
			tcpErr = result.err
			if tcpErr == nil {
				tcpErr = errors.New("TCP DNS returned no addresses")
			}
			if udpDone {
				return nil, true, errors.Join(udpErr, tcpErr)
			}
		case <-timer.C:
			startTCP()
		case <-ctx.Done():
			return nil, false, ctx.Err()
		}
	}
}

func (r *Resolver) preferTCPTemporarily() {
	r.tcpLock.Lock()
	defer r.tcpLock.Unlock()
	r.useTCP = true
	if r.timer == nil {
		r.timer = time.AfterFunc(10*time.Minute, func() {
			r.tcpLock.Lock()
			r.useTCP = false
			r.timer = nil
			r.tcpLock.Unlock()
		})
	}
}

func (r *Resolver) cacheFirstIP(host string, ips []net.IP, err error) (net.IP, error) {
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 {
		return nil, errors.New("DNS lookup returned no addresses")
	}
	r.setDNSCache(host, ips[0])
	return ips[0], nil
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
	}
	return nil, errors.New("remote UDP resolver is nil")
}

func (r *Resolver) RemoteTCPResolver() (*net.Resolver, error) {
	if r.remoteTCPResolver != nil {
		return r.remoteTCPResolver, nil
	}
	return nil, errors.New("remote TCP resolver is nil")
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
		r.resolutionMu.Lock()
		r.resolutionClosed = true
		for host, call := range r.resolutions {
			delete(r.resolutions, host)
			call.cancel()
		}
		r.resolutionMu.Unlock()
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

func NewResolver(stack stack.Stack, remoteDNSServer, secondaryDNSServer string, ttl uint64, domainResources client.DomainResources, dnsResource map[string][]net.IP, useRemoteDNS, isolatedDNS bool) *Resolver {
	ttl = min(ttl, uint64(math.MaxUint32))
	normalizedDNS := make(map[string][]net.IP, len(dnsResource))
	for host, ips := range dnsResource {
		host = normalizeHostname(host)
		if host == "" {
			continue
		}
		for _, ip := range ips {
			if ip != nil {
				normalizedDNS[host] = append(normalizedDNS[host], ip)
			}
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
	resolver.IPPool, err = ippool.NewIPPool[[]client.DomainResource]("198.18.0.0/16")
	if err != nil {
		log.Fatalf("Create Fake IP Pool failed: %v", err)
	}
	return resolver
}
