package service

import (
	"testing"

	"github.com/majianyu2007/nwafu-connect/client"
)

func TestBrowserAddressURL(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		resource client.ResourceAddress
		want     string
	}{
		{
			name:     "prefer HTTPS",
			host:     "lib.nwafu.edu.cn",
			resource: client.ResourceAddress{PortMin: 80, PortMax: 443, Protocol: "tcp"},
			want:     "https://lib.nwafu.edu.cn/",
		},
		{
			name:     "HTTP only",
			host:     "example.nwafu.edu.cn",
			resource: client.ResourceAddress{PortMin: 80, PortMax: 80, Protocol: "tcp"},
			want:     "http://example.nwafu.edu.cn/",
		},
		{
			name:     "non-web port is display only",
			host:     "service.nwafu.edu.cn",
			resource: client.ResourceAddress{PortMin: 8443, PortMax: 8443, Protocol: "tcp"},
			want:     "",
		},
		{
			name:     "IP range is display only",
			host:     "202.117.179.2-202.117.179.254",
			resource: client.ResourceAddress{PortMin: 1, PortMax: 65535, Protocol: "tcp"},
			want:     "",
		},
		{
			name:     "UDP web port is display only",
			host:     "service.nwafu.edu.cn",
			resource: client.ResourceAddress{PortMin: 443, PortMax: 443, Protocol: "udp"},
			want:     "",
		},
		{
			name:     "wildcard suffix is display only",
			host:     "*.cnki.net",
			resource: client.ResourceAddress{PortMin: 1, PortMax: 65535, Protocol: "tcp"},
			want:     "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := browserAddressURL(test.host, test.resource); got != test.want {
				t.Fatalf("browserAddressURL() = %q, want %q", got, test.want)
			}
		})
	}
}
