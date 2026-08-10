package ippool

import (
	"errors"
	"net"
	"testing"
)

func TestGenerateIPReturnsExhaustionInsteadOfPanicking(t *testing.T) {
	pool, err := NewIPPool[string]("198.18.0.0/30")
	if err != nil {
		t.Fatal(err)
	}
	first, err := pool.GenerateIP("first.example", "first")
	if err != nil {
		t.Fatal(err)
	}
	if want := "198.18.0.2"; first.String() != want {
		t.Fatalf("first fake IP = %s, want %s", first, want)
	}
	if _, err := pool.GenerateIP("second.example", "second"); !errors.Is(err, ErrExhausted) {
		t.Fatalf("second GenerateIP() error = %v, want ErrExhausted", err)
	}
	repeated, err := pool.GenerateIP("first.example", "updated")
	if err != nil || !repeated.Equal(first) {
		t.Fatalf("existing allocation = %v, %v; want %s, nil", repeated, err, first)
	}
}

func TestGenerateIPSkipsPinnedAddress(t *testing.T) {
	pool, err := NewIPPool[string]("198.18.0.0/29")
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.SetIPDomain(net.ParseIP("198.18.0.2"), "pinned.example", "pinned"); err != nil {
		t.Fatal(err)
	}
	generated, err := pool.GenerateIP("generated.example", "generated")
	if err != nil {
		t.Fatal(err)
	}
	if want := "198.18.0.3"; generated.String() != want {
		t.Fatalf("generated fake IP = %s, want %s", generated, want)
	}
}

func TestGenerateIPReplacesExternalDomainMapping(t *testing.T) {
	pool, err := NewIPPool[string]("198.18.0.0/29")
	if err != nil {
		t.Fatal(err)
	}
	realIP := net.ParseIP("203.0.113.10")
	if err := pool.SetIPDomain(realIP, "resource.example", "real"); err != nil {
		t.Fatal(err)
	}
	fakeIP, err := pool.GenerateIP("resource.example", "fake")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fakeIP.String(), "198.18.0.2"; got != want {
		t.Fatalf("fake IP = %s, want %s", got, want)
	}
	if domain, _, found := pool.GetDomain(realIP); found {
		t.Fatalf("external reverse mapping remains for %q", domain)
	}
	if domain, resource, found := pool.GetDomain(fakeIP); !found || domain != "resource.example" || resource != "fake" {
		t.Fatalf("fake reverse mapping = %q, %q, %v", domain, resource, found)
	}
}

func TestSetIPDomainPreservesExistingFakeAllocation(t *testing.T) {
	pool, err := NewIPPool[string]("198.18.0.0/29")
	if err != nil {
		t.Fatal(err)
	}
	fakeIP, err := pool.GenerateIP("resource.example", "fake")
	if err != nil {
		t.Fatal(err)
	}
	realIP := net.ParseIP("203.0.113.10")
	if err := pool.SetIPDomain(realIP, "resource.example", "updated"); err != nil {
		t.Fatal(err)
	}
	repeated, err := pool.GenerateIP("resource.example", "ignored")
	if err != nil || !repeated.Equal(fakeIP) {
		t.Fatalf("fake allocation after real lookup = %v, %v; want %v, nil", repeated, err, fakeIP)
	}
	if domain, resource, found := pool.GetDomain(fakeIP); !found || domain != "resource.example" || resource != "updated" {
		t.Fatalf("preserved fake mapping = %q, %q, %v", domain, resource, found)
	}
	if domain, _, found := pool.GetDomain(realIP); found {
		t.Fatalf("real IP unexpectedly replaced fake mapping for %q", domain)
	}
}

func TestSetIPDomainRemovesStaleReverseMappings(t *testing.T) {
	pool, err := NewIPPool[string]("198.18.0.0/29")
	if err != nil {
		t.Fatal(err)
	}
	oldIP := net.ParseIP("203.0.113.10")
	newIP := net.ParseIP("203.0.113.11")
	if err := pool.SetIPDomain(oldIP, "resource.example", "old"); err != nil {
		t.Fatal(err)
	}
	if err := pool.SetIPDomain(newIP, "resource.example", "new"); err != nil {
		t.Fatal(err)
	}
	if domain, _, ok := pool.GetDomain(oldIP); ok {
		t.Fatalf("old reverse mapping remains for %q", domain)
	}
	if domain, resource, ok := pool.GetDomain(newIP); !ok || domain != "resource.example" || resource != "new" {
		t.Fatalf("new reverse mapping = %q, %q, %v", domain, resource, ok)
	}
}

func TestNewIPPoolRejectsUnsupportedRanges(t *testing.T) {
	for _, cidr := range []string{"2001:db8::/64", "198.18.0.0/31", "198.18.0.1/32"} {
		if _, err := NewIPPool[struct{}](cidr); err == nil {
			t.Fatalf("NewIPPool(%q) succeeded", cidr)
		}
	}
	pool, err := NewIPPool[struct{}]("0.0.0.0/0")
	if err != nil {
		t.Fatalf("NewIPPool(/0) error = %v", err)
	}
	if pool.maxIP != uint32(0xfffffffe) {
		t.Fatalf("/0 max IP = %#x, want 0xfffffffe", pool.maxIP)
	}
}
