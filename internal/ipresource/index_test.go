package ipresource

import (
	"net"
	"testing"

	"github.com/majianyu2007/nwafu-connect/client"
)

func TestIndexPreservesFirstMatchingResource(t *testing.T) {
	index := New([]client.IPResource{
		{IPMin: net.IPv4(10, 0, 0, 0), IPMax: net.IPv4(10, 0, 0, 255), PortMin: 1, PortMax: 65535, Protocol: "all", AppID: "first"},
		{IPMin: net.IPv4(10, 0, 0, 42), IPMax: net.IPv4(10, 0, 0, 42), PortMin: 443, PortMax: 443, Protocol: "tcp", AppID: "more-specific"},
	})
	resource, ok := index.Match(net.IPv4(10, 0, 0, 42), "tcp", 443)
	if !ok {
		t.Fatal("overlapping resource did not match")
	}
	if resource.AppID != "first" {
		t.Fatalf("matched AppID = %q, want first rule", resource.AppID)
	}
}

func TestIndexMatchesProtocolAndPort(t *testing.T) {
	index := New([]client.IPResource{
		{IPMin: net.IPv4(192, 0, 2, 1), IPMax: net.IPv4(192, 0, 2, 10), PortMin: 443, PortMax: 443, Protocol: "tcp", AppID: "web"},
	})
	for _, test := range []struct {
		name     string
		ip       net.IP
		protocol string
		port     int
		want     bool
	}{
		{name: "exact match", ip: net.IPv4(192, 0, 2, 5), protocol: "tcp", port: 443, want: true},
		{name: "wrong protocol", ip: net.IPv4(192, 0, 2, 5), protocol: "udp", port: 443},
		{name: "wrong port", ip: net.IPv4(192, 0, 2, 5), protocol: "tcp", port: 80},
		{name: "outside range", ip: net.IPv4(192, 0, 2, 11), protocol: "tcp", port: 443},
		{name: "IPv6", ip: net.ParseIP("2001:db8::1"), protocol: "tcp", port: 443},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, got := index.Match(test.ip, test.protocol, test.port)
			if got != test.want {
				t.Fatalf("Match() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestIndexMatchesICMPWithoutPortRange(t *testing.T) {
	index := New([]client.IPResource{
		{IPMin: net.IPv4(198, 51, 100, 1), IPMax: net.IPv4(198, 51, 100, 1), Protocol: "all", AppID: "icmp"},
	})
	if _, ok := index.Match(net.IPv4(198, 51, 100, 1), "icmp", -1); !ok {
		t.Fatal("ICMP resource did not match")
	}
}

func TestNilIndexDoesNotMatch(t *testing.T) {
	var index *Index
	if _, ok := index.Match(net.IPv4(10, 0, 0, 1), "tcp", 443); ok {
		t.Fatal("nil index unexpectedly matched a resource")
	}
}
