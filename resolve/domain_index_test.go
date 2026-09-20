package resolve

import (
	"testing"

	"github.com/majianyu2007/nwafu-connect/client"
)

func TestDomainResourceIndexRespectsLabelBoundaries(t *testing.T) {
	resource := client.DomainResourceSet{{AppID: "campus"}}
	index := newDomainResourceIndex(map[string]client.DomainResourceSet{"example.com": resource})
	for _, host := range []string{"example.com", "library.example.com"} {
		domain, matched, ok := index.Match(host)
		if !ok || domain != "example.com" || len(matched) != 1 || matched[0].AppID != "campus" {
			t.Fatalf("Match(%q) = (%#v, %q, %t), want campus example.com", host, matched, domain, ok)
		}
	}
	if _, _, ok := index.Match("malicious-example.com"); ok {
		t.Fatal("partial label suffix unexpectedly matched")
	}
}

func TestDomainResourceIndexWildcardRequiresSubdomain(t *testing.T) {
	index := newDomainResourceIndex(map[string]client.DomainResourceSet{
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

func TestDomainResourceIndexPrefersLongestSuffix(t *testing.T) {
	index := newDomainResourceIndex(map[string]client.DomainResourceSet{
		"example.com":         {{AppID: "parent"}},
		"library.example.com": {{AppID: "library"}},
	})
	domain, resources, ok := index.Match("catalog.library.example.com")
	if !ok || domain != "library.example.com" || resources[0].AppID != "library" {
		t.Fatalf("Match() = (%#v, %q, %t), want most-specific library resource", resources, domain, ok)
	}
}

func TestNilDomainResourceIndexDoesNotMatch(t *testing.T) {
	var index *domainResourceIndex
	if _, _, ok := index.Match("example.com"); ok {
		t.Fatal("nil domain index unexpectedly matched")
	}
}
