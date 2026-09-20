package atrust

import "testing"

func TestParseResourceSkipsEmptyHosts(t *testing.T) {
	vpnClient := &Client{}
	payload := []byte(`{"Data":{"AppList":{"Data":{"AppInfo":[{"Apps":[{"ID":"empty-host","Name":"broken","AddressList":[{"Protocol":"tcp","Port":"443","Host":""}]}]}]}}}}`)
	if err := vpnClient.parseResource(payload); err != nil {
		t.Fatal(err)
	}
	if len(vpnClient.domainResources) != 0 {
		t.Fatalf("empty host created domain rules: %#v", vpnClient.domainResources)
	}
	if len(vpnClient.resources) != 0 {
		t.Fatalf("empty host created portal resources: %#v", vpnClient.resources)
	}
}

func TestParseResourceRejectsReversedPortRanges(t *testing.T) {
	vpnClient := &Client{}
	payload := []byte(`{"Data":{"AppList":{"Data":{"AppInfo":[{"Apps":[{"ID":"bad-port","Name":"broken","AddressList":[{"Protocol":"tcp","Port":"443-80","Host":"library.example.com"}]}]}]}}}}`)
	if err := vpnClient.parseResource(payload); err != nil {
		t.Fatal(err)
	}
	if len(vpnClient.domainResources) != 0 || len(vpnClient.resources) != 0 {
		t.Fatalf("reversed port range was accepted: domains=%#v resources=%#v", vpnClient.domainResources, vpnClient.resources)
	}
}

func TestParseResourceOmitsUnsupportedHostsFromPortal(t *testing.T) {
	vpnClient := &Client{}
	payload := []byte(`{"Data":{"AppList":{"Data":{"AppInfo":[{"Apps":[{"ID":"hosts","Name":"hosts","AddressList":[{"Protocol":"tcp","Port":"443","Host":"2001:db8::1"},{"Protocol":"tcp","Port":"443","Host":"2001:db8::/64"},{"Protocol":"tcp","Port":"443","Host":"10.0.0.1-2001:db8::1"},{"Protocol":"tcp","Port":"443","Host":"bad.*.example.com"},{"Protocol":"tcp","Port":"443","Host":"library.example.com"}]}]}]}}}}`)
	if err := vpnClient.parseResource(payload); err != nil {
		t.Fatal(err)
	}
	if got := len(vpnClient.resources); got != 1 {
		t.Fatalf("portal resource count = %d, want 1", got)
	}
	addresses := vpnClient.resources[0].Addresses
	if len(addresses) != 1 || addresses[0].Host != "library.example.com" {
		t.Fatalf("portal addresses = %#v, want only supported domain", addresses)
	}
}

func TestParseResourceAcceptsWhitespaceAroundIPRange(t *testing.T) {
	vpnClient := &Client{}
	payload := []byte(`{"Data":{"AppList":{"Data":{"AppInfo":[{"Apps":[{"ID":"range","Name":"range","AddressList":[{"Protocol":"tcp","Port":"443","Host":"10.0.0.1 - 10.0.0.3"}]}]}]}}}}`)
	if err := vpnClient.parseResource(payload); err != nil {
		t.Fatal(err)
	}
	if got := len(vpnClient.ipResources); got != 1 {
		t.Fatalf("IP resource count = %d, want 1", got)
	}
	if got := vpnClient.ipResources[0]; got.IPMin.String() != "10.0.0.1" || got.IPMax.String() != "10.0.0.3" {
		t.Fatalf("parsed range = %s-%s", got.IPMin, got.IPMax)
	}
}

func TestParseResourceRoutesEveryServerIssuedDomainIP(t *testing.T) {
	vpnClient := &Client{}
	payload := []byte(`{"Data":{"AppList":{"Data":{"AppInfo":[{"Apps":[{"ID":"library","NodeGroupID":"campus","Name":"library","AddressList":[{"Protocol":"tcp","Port":"443","Host":"library.example.com","IP":["10.0.0.10","10.0.0.11","10.0.0.10"]}]}]}]}}}}`)
	if err := vpnClient.parseResource(payload); err != nil {
		t.Fatal(err)
	}
	if got := len(vpnClient.ipResources); got != 2 {
		t.Fatalf("IP resource count = %d, want 2 unique server-issued addresses", got)
	}
	for index, want := range []string{"10.0.0.10", "10.0.0.11"} {
		resource := vpnClient.ipResources[index]
		if got := resource.IPMin.String(); got != want || resource.IPMax.String() != want {
			t.Fatalf("IP resource %d = %s-%s, want %s", index, resource.IPMin, resource.IPMax, want)
		}
		if resource.AppID != "library" || resource.NodeGroupID != "campus" || resource.PortMin != 443 || resource.PortMax != 443 {
			t.Fatalf("IP resource %d lost routing metadata: %#v", index, resource)
		}
	}
	if got := vpnClient.dnsResource["library.example.com"].String(); got != "10.0.0.10" {
		t.Fatalf("preferred DNS resource = %q, want first valid address", got)
	}
}

func TestParseResourcePreservesEveryRuleForSharedDomain(t *testing.T) {
	vpnClient := &Client{}
	payload := []byte(`{"Data":{"AppList":{"Data":{"AppInfo":[{"Apps":[{"ID":"web","NodeGroupID":"web-nodes","Name":"web","AddressList":[{"Protocol":"tcp","Port":"443","Host":"shared.example.com"}]},{"ID":"ssh","NodeGroupID":"ssh-nodes","Name":"ssh","AddressList":[{"Protocol":"tcp","Port":"22","Host":"shared.example.com"}]}]}]}}}}`)
	if err := vpnClient.parseResource(payload); err != nil {
		t.Fatal(err)
	}
	resources := vpnClient.domainResources["shared.example.com"]
	if got := len(resources); got != 2 {
		t.Fatalf("shared domain resource count = %d, want 2: %#v", got, resources)
	}
	web, webFound := client.MatchDomainResource(resources, "tcp", 443)
	ssh, sshFound := client.MatchDomainResource(resources, "tcp", 22)
	if !webFound || web.AppID != "web" || web.NodeGroupID != "web-nodes" {
		t.Fatalf("web routing metadata = %#v, found %v", web, webFound)
	}
	if !sshFound || ssh.AppID != "ssh" || ssh.NodeGroupID != "ssh-nodes" {
		t.Fatalf("SSH routing metadata = %#v, found %v", ssh, sshFound)
	}
}

func TestNormalizeNodeAddress(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		server string
		want   string
		ok     bool
	}{
		{name: "domain default port", raw: "node.example.com", want: "node.example.com:441", ok: true},
		{name: "explicit port", raw: "node.example.com:8443", want: "node.example.com:8443", ok: true},
		{name: "IPv6 default port", raw: "2001:db8::1", want: "[2001:db8::1]:441", ok: true},
		{name: "gateway placeholder", raw: "{{sdpcHost}}", server: "vpn.example.com", want: "vpn.example.com:441", ok: true},
		{name: "invalid port", raw: "node.example.com:invalid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := normalizeNodeAddress(test.raw, test.server)
			if got != test.want || ok != test.ok {
				t.Fatalf("normalizeNodeAddress() = %q, %v; want %q, %v", got, ok, test.want, test.ok)
			}
		})
	}
}
