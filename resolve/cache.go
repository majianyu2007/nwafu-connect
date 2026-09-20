package resolve

import (
	"net"
	"strings"

	"github.com/patrickmn/go-cache"
)

func (r *Resolver) getDNSCache(host string) (net.IP, bool) {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	item, found := r.dnsCache.Get(host)
	if !found {
		return nil, false
	}
	ip, ok := item.(net.IP)
	return ip, ok && ip != nil
}

func (r *Resolver) setDNSCache(host string, ip net.IP) {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if r.ttl == 0 || host == "" || ip == nil {
		return
	}
	r.dnsCache.Set(host, ip, cache.DefaultExpiration)
}

func (r *Resolver) SetPermanentDNS(host string, ip net.IP) {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" || ip == nil {
		return
	}
	r.dnsCache.Set(host, ip, cache.NoExpiration)
}
