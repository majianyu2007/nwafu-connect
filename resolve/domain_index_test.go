package resolve

import (
	"testing"

	"github.com/majianyu2007/nwafu-connect/client"
)

func TestDomainResourceIndexRespectsLabelBoundaries(t *testing.T) {
	resource := []client.DomainResource{{AppID: "campus"}}
	index := newDomainResourceIndex(client.DomainResources{"example.com": resource})
	for _, host := range []string{"example.com", "library.example.com"} {
		domain, matched, ok := index.Match(host)
		if !ok || domain != "example.com" || len(matched) != 1 || matched[0].AppID != "campus" {
			t.Fatalf("Match(%q) = (%q, %#v, %t), want campus example.com", host, domain, matched, ok)
		}
	}
	if _, _, ok := index.Match("malicious-example.com"); ok {
		t.Fatal("partial label suffix unexpectedly matched")
	}
}

func TestDomainResourceIndexWildcardRequiresSubdomain(t *testing.T) {
	index := newDomainResourceIndex(client.DomainResources{
		"*.example.com": {{AppID: "wildcard"}},
	})
	if _, _, ok := index.Match("example.com"); ok {
		t.Fatal("wildcard unexpectedly matched the apex domain")
	}
	_, resources, ok := index.Match("library.example.com")
	if !ok || resources[0].AppID != "wildcard" {
		t.Fatalf("wildcard resources = %#v, matched = %t", resources, ok)
	}
}

func TestDomainResourceIndexCollectsSpecificAndAncestorRules(t *testing.T) {
	index := newDomainResourceIndex(client.DomainResources{
		"*.example.com":       {{AppID: "parent", PortMin: 80, PortMax: 80, Protocol: "tcp"}},
		"library.example.com": {{AppID: "library", PortMin: 443, PortMax: 443, Protocol: "tcp"}},
	})
	domain, resources, ok := index.Match("catalog.library.example.com")
	if !ok || domain != "library.example.com" || len(resources) != 2 || resources[0].AppID != "library" || resources[1].AppID != "parent" {
		t.Fatalf("Match() = (%q, %#v, %t), want library then parent", domain, resources, ok)
	}
}

func TestNilDomainResourceIndexDoesNotMatch(t *testing.T) {
	var index *domainResourceIndex
	if _, _, ok := index.Match("example.com"); ok {
		t.Fatal("nil domain index unexpectedly matched")
	}
}
