package resolve

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/miekg/dns"
	"github.com/patrickmn/go-cache"
)

func TestResolveDoesNotMatchPartialDomainSuffix(t *testing.T) {
	resource := client.DomainResource{PortMin: 443, PortMax: 443, Protocol: "tcp", AppID: "campus"}
	resolver := NewResolver(
		nil,
		"",
		"",
		60,
		map[string]client.DomainResourceSet{"example.com": {resource}},
		map[string]net.IP{"malicious-example.com": net.ParseIP("192.0.2.10")},
		false,
		false,
	)
	defer resolver.Close()

	resolvedContext, _, err := resolver.Resolve(context.Background(), "malicious-example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got := resolvedContext.Value(ContextKeyDomainResource); got != nil {
		t.Fatalf("partial suffix inherited campus resource: %#v", got)
	}
}

func TestResolveMatchesDomainAtLabelBoundary(t *testing.T) {
	resource := client.DomainResource{PortMin: 443, PortMax: 443, Protocol: "tcp", AppID: "campus"}
	resolver := NewResolver(
		nil,
		"",
		"",
		60,
		map[string]client.DomainResourceSet{"example.com": {resource}},
		map[string]net.IP{"library.example.com": net.ParseIP("192.0.2.11")},
		false,
		false,
	)
	defer resolver.Close()

	resolvedContext, _, err := resolver.Resolve(context.Background(), "library.example.com")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := resolvedContext.Value(ContextKeyDomainResource).(client.DomainResourceSet)
	if !ok || len(got) != 1 || got[0].AppID != resource.AppID {
		t.Fatalf("label suffix resources = %#v, want %#v", got, resource)
	}
}

func TestResolveCoalescesAndCachesSecondaryDNS(t *testing.T) {
	packetConnection, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var queryCount atomic.Int32
	dnsServer := &dns.Server{
		PacketConn: packetConnection,
		Handler: dns.HandlerFunc(func(writer dns.ResponseWriter, request *dns.Msg) {
			queryCount.Add(1)
			response := new(dns.Msg)
			response.SetReply(request)
			response.Answer = []dns.RR{&dns.A{
				Hdr: dns.RR_Header{Name: request.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP("192.0.2.25"),
			}}
			_ = writer.WriteMsg(response)
		}),
	}
	go func() { _ = dnsServer.ActivateAndServe() }()
	defer dnsServer.Shutdown()

	resolver := &Resolver{
		secondaryResolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "udp", packetConnection.LocalAddr().String())
			},
		},
		ttl:      60,
		dnsCache: cache.New(time.Minute, time.Minute),
	}

	const callers = 8
	start := make(chan struct{})
	var waitGroup sync.WaitGroup
	waitGroup.Add(callers)
	for range callers {
		go func() {
			defer waitGroup.Done()
			<-start
			_, ip, resolveErr := resolver.Resolve(context.Background(), "library.example.com")
			if resolveErr != nil {
				t.Errorf("Resolve() error = %v", resolveErr)
				return
			}
			if got, want := ip.String(), "192.0.2.25"; got != want {
				t.Errorf("Resolve() IP = %q, want %q", got, want)
			}
		}()
	}
	close(start)
	waitGroup.Wait()
	if _, _, err := resolver.Resolve(context.Background(), "library.example.com"); err != nil {
		t.Fatal(err)
	}
	if got := queryCount.Load(); got != 1 {
		t.Fatalf("secondary DNS queries = %d, want 1 coalesced and cached lookup", got)
	}
}

func TestResolveCallerCancellationDoesNotPoisonSharedLookup(t *testing.T) {
	packetConnection, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	queried := make(chan struct{})
	release := make(chan struct{})
	var queryOnce sync.Once
	dnsServer := &dns.Server{
		PacketConn: packetConnection,
		Handler: dns.HandlerFunc(func(writer dns.ResponseWriter, request *dns.Msg) {
			queryOnce.Do(func() { close(queried) })
			<-release
			response := new(dns.Msg)
			response.SetReply(request)
			response.Answer = []dns.RR{&dns.A{
				Hdr: dns.RR_Header{Name: request.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.ParseIP("192.0.2.26"),
			}}
			_ = writer.WriteMsg(response)
		}),
	}
	go func() { _ = dnsServer.ActivateAndServe() }()
	defer dnsServer.Shutdown()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()

	resolver := &Resolver{
		secondaryResolver: &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "udp", packetConnection.LocalAddr().String())
			},
		},
		ttl:      60,
		dnsCache: cache.New(time.Minute, time.Minute),
	}

	firstContext, cancelFirst := context.WithCancel(context.Background())
	firstResult := make(chan error, 1)
	go func() {
		_, _, resolveErr := resolver.Resolve(firstContext, "library.example.com")
		firstResult <- resolveErr
	}()
	select {
	case <-queried:
	case <-time.After(time.Second):
		t.Fatal("shared DNS lookup did not start")
	}

	firstCancellation := make(chan error, 1)
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancelFirst()
		firstCancellation <- <-firstResult
		close(release)
	}()

	secondContext, cancelSecond := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancelSecond()
	_, ip, err := resolver.Resolve(secondContext, "library.example.com")
	if err != nil {
		t.Fatalf("second Resolve() inherited first caller cancellation: %v", err)
	}
	if got, want := ip.String(), "192.0.2.26"; got != want {
		t.Fatalf("second Resolve() IP = %q, want %q", got, want)
	}
	if err := <-firstCancellation; !errors.Is(err, context.Canceled) {
		t.Fatalf("first Resolve() error = %v, want context.Canceled", err)
	}
}

func TestResolveRestoresDomainMappingForCachedIP(t *testing.T) {
	resource := client.DomainResource{PortMin: 443, PortMax: 443, Protocol: "tcp", AppID: "library"}
	resolver := NewResolver(
		nil,
		"",
		"",
		60,
		map[string]client.DomainResourceSet{"library.example.com": {resource}},
		nil,
		false,
		false,
	)
	defer resolver.Close()

	ip := net.ParseIP("192.0.2.30")
	resolver.setDNSCache("library.example.com", ip)
	if err := resolver.IPPool.SetIPDomain(ip, "other.example.com", client.DomainResourceSet{{AppID: "other"}}); err != nil {
		t.Fatal(err)
	}

	if _, gotIP, err := resolver.Resolve(context.Background(), "library.example.com"); err != nil {
		t.Fatal(err)
	} else if !gotIP.Equal(ip) {
		t.Fatalf("Resolve() IP = %v, want %v", gotIP, ip)
	}
	domain, gotResource, found := resolver.IPPool.GetDomain(ip)
	if !found || domain != "library.example.com" || len(gotResource) != 1 || gotResource[0].AppID != resource.AppID {
		t.Fatalf("cached IP mapping = (%q, %#v, %v), want library resource", domain, gotResource, found)
	}
}

func TestResolveFakeIPOverridesCachedAndServerIssuedAddresses(t *testing.T) {
	resource := client.DomainResource{PortMin: 443, PortMax: 443, Protocol: "tcp", AppID: "library"}
	resources := client.DomainResourceSet{resource}
	resolver := NewResolver(
		nil,
		"",
		"",
		60,
		map[string]client.DomainResourceSet{"library.example.com": resources},
		map[string]net.IP{"library.example.com": net.ParseIP("203.0.113.10")},
		false,
		false,
	)
	defer resolver.Close()

	realIP := net.ParseIP("203.0.113.10")
	resolver.setDNSCache("library.example.com", realIP)
	if err := resolver.IPPool.SetIPDomain(realIP, "library.example.com", resources); err != nil {
		t.Fatal(err)
	}
	fakeContext := context.WithValue(context.Background(), ContextKeyFakeIP, true)
	_, fakeIP, err := resolver.Resolve(fakeContext, "library.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fakeIP.String(), "198.18.0.2"; got != want {
		t.Fatalf("Resolve() fake IP = %s, want %s", got, want)
	}
	domain, mappedResources, found := resolver.IPPool.GetDomain(fakeIP)
	if !found || domain != "library.example.com" || len(mappedResources) != 1 || mappedResources[0].AppID != resource.AppID {
		t.Fatalf("fake IP mapping = (%q, %#v, %v)", domain, mappedResources, found)
	}
}

func TestZeroTTLDisablesTransientDNSCaching(t *testing.T) {
	resolver := NewResolver(nil, "", "", 0, nil, nil, false, false)
	defer resolver.Close()
	ip := net.ParseIP("192.0.2.44")

	resolver.setDNSCache("transient.example.com", ip)
	if _, found := resolver.getDNSCache("transient.example.com"); found {
		t.Fatal("zero-TTL transient DNS result was cached")
	}
	resolver.SetPermanentDNS("permanent.example.com", ip)
	if got, found := resolver.getDNSCache("permanent.example.com"); !found || !got.Equal(ip) {
		t.Fatalf("permanent DNS result = %v, %v; want %v, true", got, found, ip)
	}
}
