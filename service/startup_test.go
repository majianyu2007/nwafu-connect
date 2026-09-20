package service

import (
	"context"
	"net"
	"testing"

	"github.com/majianyu2007/nwafu-connect/dial"
	"github.com/majianyu2007/nwafu-connect/resolve"
	"github.com/miekg/dns"
)

func TestNewDNSServerKeepsOnlyValidAddresses(t *testing.T) {
	server := NewDnsServer(nil, []string{"1.1.1.1", "not-an-ip", "2001:4860:4860::8888"})
	if got := len(server.localDNS); got != 2 {
		t.Fatalf("local DNS address count = %d, want 2", got)
	}
	for _, address := range []string{"1.1.1.1", "2001:4860:4860::8888"} {
		if server.CheckDnsHijack(net.ParseIP(address)) {
			t.Fatalf("CheckDnsHijack(%s) = true, want false for configured DNS", address)
		}
	}
	if !server.CheckDnsHijack(net.ParseIP("192.0.2.53")) {
		t.Fatal("CheckDnsHijack() accepted an unconfigured DNS address")
	}
}

func TestDNSServerReturnsConfiguredTTL(t *testing.T) {
	resolver := resolve.NewResolver(nil, "", "", 3600, nil, map[string][]net.IP{
		"library.example": {net.ParseIP("192.0.2.10")},
	}, false, false)
	defer resolver.Close()
	server := NewDnsServer(resolver, nil, 123)
	request := new(dns.Msg)
	request.SetQuestion("library.example.", dns.TypeA)

	response, err := server.HandleDnsMsg(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if response.Rcode != dns.RcodeSuccess {
		t.Fatalf("DNS response code = %d, want success", response.Rcode)
	}
	if len(response.Answer) != 1 {
		t.Fatalf("DNS answer count = %d, want 1", len(response.Answer))
	}
	if ttl := response.Answer[0].Header().Ttl; ttl != 123 {
		t.Fatalf("DNS answer TTL = %d, want 123", ttl)
	}
}

func TestDNSServerReportsResolutionFailure(t *testing.T) {
	resolver := resolve.NewResolver(nil, "", "", 3600, nil, nil, false, false)
	defer resolver.Close()
	server := NewDnsServer(resolver, nil)
	request := new(dns.Msg)
	request.SetQuestion("unavailable.invalid.", dns.TypeA)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	response, err := server.HandleDnsMsg(ctx, request)
	if err == nil {
		t.Fatal("canceled DNS resolution unexpectedly succeeded")
	}
	if response.Rcode != dns.RcodeServerFailure {
		t.Fatalf("DNS response code = %d, want SERVFAIL", response.Rcode)
	}
}

func TestListenerStartupErrorsAreReturned(t *testing.T) {
	if _, err := StartDNS("not-a-listen-address", DNSServer{}); err == nil {
		t.Fatal("StartDNS() accepted an invalid bind address")
	}

	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	proxyDialer := dial.NewDialer(nil, nil, nil, false, "")
	if _, err := StartSocks5(occupied.Addr().String(), proxyDialer, nil, "", ""); err == nil {
		t.Fatal("StartSocks5() accepted an occupied bind address")
	}
}

func TestStartShadowsocksRejectsMalformedURL(t *testing.T) {
	if _, err := StartShadowsocks(nil, "not-a-shadowsocks-url"); err == nil {
		t.Fatal("StartShadowsocks() accepted a malformed URL")
	}
}
