package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
		{
			name:     "protocol matching is case insensitive",
			host:     "portal.nwafu.edu.cn",
			resource: client.ResourceAddress{PortMin: 443, PortMax: 443, Protocol: "TCP"},
			want:     "https://portal.nwafu.edu.cn/",
		},
		{
			name:     "IPv6 address is display only",
			host:     "2001:db8::1",
			resource: client.ResourceAddress{PortMin: 443, PortMax: 443, Protocol: "tcp"},
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

func TestSSHProxyCommandLineQuotesWindowsHelperPath(t *testing.T) {
	got := sshProxyCommandLine(
		"windows",
		`C:\Program Files\NWAFU Connect\nwafu-connect-proxy.exe`,
		"127.0.0.1:43210",
	)
	want := `$proxyCommand = 'ProxyCommand="C:\Program Files\NWAFU Connect\nwafu-connect-proxy.exe" --proxy 127.0.0.1:43210 --target %h:%p'; ssh -o $proxyCommand USER@HOST`
	if got != want {
		t.Fatalf("Windows SSH command = %q, want %q", got, want)
	}
}

func TestSSHProxyCommandLineQuotesPOSIXHelperPath(t *testing.T) {
	got := sshProxyCommandLine(
		"darwin",
		"/Users/O'Brien/NWAFU Connect/nwafu-connect-proxy",
		"127.0.0.1:43210",
	)
	want := `ssh -o 'ProxyCommand="/Users/O'"'"'Brien/NWAFU Connect/nwafu-connect-proxy" --proxy 127.0.0.1:43210 --target %h:%p' USER@HOST`
	if got != want {
		t.Fatalf("POSIX SSH command = %q, want %q", got, want)
	}
}

func TestResourceMonogramUsesUnicodeInitials(t *testing.T) {
	if got, want := resourceMonogram("École Über"), "ÉÜ"; got != want {
		t.Fatalf("resourceMonogram() = %q, want %q", got, want)
	}
}

func TestInferResourceTypeRecognizesAllPorts(t *testing.T) {
	resource := client.Resource{Addresses: []client.ResourceAddress{{
		Host:     "10.0.0.0/8",
		PortMin:  1,
		PortMax:  65535,
		Protocol: "all",
	}}}
	kind, label := inferResourceType(resource)
	if kind != "all" || label != "全端口" {
		t.Fatalf("inferResourceType() = %q, %q, want all, 全端口", kind, label)
	}
}

func TestInferResourceTypeRecognizesUDP(t *testing.T) {
	resource := client.Resource{Addresses: []client.ResourceAddress{{
		Host:     "service.nwafu.edu.cn",
		PortMin:  53,
		PortMax:  53,
		Protocol: "UDP",
	}}}
	kind, label := inferResourceType(resource)
	if kind != "udp" || label != "UDP" {
		t.Fatalf("inferResourceType() = %q, %q, want udp, UDP", kind, label)
	}
}

func TestBuildBrowserHomeResourcesPrefersActionableAddressAndDefersLargeLists(t *testing.T) {
	sources := make([]client.Resource, browserHomeInitialLimit+1)
	for index := range sources {
		sources[index] = client.Resource{
			Name: fmt.Sprintf("资源 %02d", index),
			Addresses: []client.ResourceAddress{
				{Host: fmt.Sprintf("ssh-%02d.nwafu.edu.cn", index), PortMin: 22, PortMax: 22, Protocol: "tcp"},
				{Host: fmt.Sprintf("web-%02d.nwafu.edu.cn", index), PortMin: 443, PortMax: 443, Protocol: "tcp"},
				{Host: fmt.Sprintf("web-%02d.nwafu.edu.cn", index), PortMin: 443, PortMax: 443, Protocol: "TCP"},
			},
		}
	}

	resources := buildBrowserHomeResources(sources)
	if len(resources) != len(sources) {
		t.Fatalf("resource count = %d, want %d", len(resources), len(sources))
	}
	if got, want := resources[0].Primary.URL, "https://web-00.nwafu.edu.cn/"; got != want {
		t.Fatalf("primary URL = %q, want %q", got, want)
	}
	if got := len(resources[0].Additional); got != 1 {
		t.Fatalf("additional address count = %d, want 1 after deduplication", got)
	}

	initial, deferred := splitBrowserHomeResources(resources)
	if len(initial) != browserHomeInitialLimit || len(deferred) != 1 {
		t.Fatalf("resource split = %d initial, %d deferred", len(initial), len(deferred))
	}
	data := browserHomeData{
		Resources:    initial,
		HasMore:      true,
		InitialLimit: browserHomeInitialLimit,
		Total:        len(resources),
	}
	handler := newBrowserHomeHandler(data, deferred)
	homeRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	homeResponse := httptest.NewRecorder()
	handler.ServeHTTP(homeResponse, homeRequest)
	if got := strings.Count(homeResponse.Body.String(), `class="resource-card"`); got != browserHomeInitialLimit {
		t.Fatalf("initial HTML card count = %d, want %d", got, browserHomeInitialLimit)
	}
	if strings.Contains(homeResponse.Body.String(), "web-18.nwafu.edu.cn") {
		t.Fatal("deferred resource was rendered into the initial HTML")
	}

	deferredRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/resources", nil)
	deferredResponse := httptest.NewRecorder()
	handler.ServeHTTP(deferredResponse, deferredRequest)
	if got := strings.Count(deferredResponse.Body.String(), `class="resource-card"`); got != 1 {
		t.Fatalf("deferred HTML card count = %d, want 1", got)
	}
}

func TestBrowserHomeHandlerRejectsForeignHostAndSetsSecurityHeaders(t *testing.T) {
	handler := newBrowserHomeHandler(browserHomeData{InitialLimit: browserHomeInitialLimit}, nil)

	foreignRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	foreignRequest.Host = "attacker.example"
	foreignResponse := httptest.NewRecorder()
	handler.ServeHTTP(foreignResponse, foreignRequest)
	if foreignResponse.Code != http.StatusForbidden {
		t.Fatalf("foreign host status = %d, want %d", foreignResponse.Code, http.StatusForbidden)
	}

	localRequest := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	localRequest.Host = "127.0.0.1:54321"
	localResponse := httptest.NewRecorder()
	handler.ServeHTTP(localResponse, localRequest)
	if localResponse.Code != http.StatusOK {
		t.Fatalf("loopback host status = %d, want %d", localResponse.Code, http.StatusOK)
	}
	for _, header := range []string{
		"Content-Security-Policy",
		"Cross-Origin-Opener-Policy",
		"Permissions-Policy",
		"Referrer-Policy",
		"X-Content-Type-Options",
		"X-Frame-Options",
	} {
		if strings.TrimSpace(localResponse.Header().Get(header)) == "" {
			t.Errorf("response header %s is missing", header)
		}
	}
}
