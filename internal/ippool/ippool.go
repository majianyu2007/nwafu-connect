package ippool

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"sync"
)

var ErrExhausted = errors.New("fake IP range exhausted")

type entry[T any] struct {
	ipUint   uint32
	domain   string
	resource T
}

type IPPool[T any] struct {
	mu         sync.RWMutex
	domainToIP map[string]*entry[T]
	ipToDomain map[uint32]*entry[T]
	minIP      uint32
	maxIP      uint32
	currentIP  uint32
}

func NewIPPool[T any](cidr string) (*IPPool[T], error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}
	ip := ipNet.IP.To4()
	ones, bits := ipNet.Mask.Size()
	if ip == nil || bits != 32 {
		return nil, fmt.Errorf("fake IP pool requires an IPv4 CIDR: %q", cidr)
	}
	if ones > 30 {
		return nil, fmt.Errorf("fake IP pool has no usable addresses: %q", cidr)
	}

	minIP := binary.BigEndian.Uint32(ip)
	addressCount := uint64(1) << uint(bits-ones)
	maxIP := uint32(uint64(minIP) + addressCount - 2)
	return &IPPool[T]{
		domainToIP: make(map[string]*entry[T]),
		ipToDomain: make(map[uint32]*entry[T]),
		minIP:      minIP,
		maxIP:      maxIP,
		currentIP:  minIP + 2,
	}, nil
}

func (p *IPPool[T]) SetIPDomain(ip net.IP, domain string, res T) error {
	ip4 := ip.To4()
	if ip4 == nil {
		return errors.New("only IPv4 is supported")
	}
	ipUint := binary.BigEndian.Uint32(ip4)

	p.mu.Lock()
	defer p.mu.Unlock()

	if previous, ok := p.domainToIP[domain]; ok &&
		previous.ipUint >= p.minIP+2 && previous.ipUint <= p.maxIP {
		previous.resource = res
		return nil
	}
	if previous, ok := p.domainToIP[domain]; ok {
		if mapped := p.ipToDomain[previous.ipUint]; mapped == previous {
			delete(p.ipToDomain, previous.ipUint)
		}
	}
	if previous, ok := p.ipToDomain[ipUint]; ok {
		if mapped := p.domainToIP[previous.domain]; mapped == previous {
			delete(p.domainToIP, previous.domain)
		}
	}
	newEntry := &entry[T]{
		ipUint:   ipUint,
		domain:   domain,
		resource: res,
	}

	p.domainToIP[domain] = newEntry
	p.ipToDomain[ipUint] = newEntry

	return nil
}

func (p *IPPool[T]) GenerateIP(domain string, res T) (net.IP, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if existing, ok := p.domainToIP[domain]; ok {
		if existing.ipUint >= p.minIP+2 && existing.ipUint <= p.maxIP {
			return uint32ToIP(existing.ipUint), nil
		}
		if mapped := p.ipToDomain[existing.ipUint]; mapped == existing {
			delete(p.ipToDomain, existing.ipUint)
		}
		delete(p.domainToIP, domain)
	}
	for p.currentIP <= p.maxIP {
		newIP := p.currentIP
		p.currentIP++
		if _, used := p.ipToDomain[newIP]; used {
			continue
		}
		newEntry := &entry[T]{
			ipUint:   newIP,
			domain:   domain,
			resource: res,
		}
		p.domainToIP[domain] = newEntry
		p.ipToDomain[newIP] = newEntry
		return uint32ToIP(newIP), nil
	}
	return nil, ErrExhausted
}

func (p *IPPool[T]) GetDomain(ip net.IP) (string, T, bool) {
	ip4 := ip.To4()
	if ip4 == nil {
		var zero T
		return "", zero, false
	}
	ipUint := binary.BigEndian.Uint32(ip4)

	p.mu.RLock()
	defer p.mu.RUnlock()

	if e, ok := p.ipToDomain[ipUint]; ok {
		return e.domain, e.resource, true
	}

	var zero T
	return "", zero, false
}

func uint32ToIP(n uint32) net.IP {
	ip := make(net.IP, 4)
	binary.BigEndian.PutUint32(ip, n)
	return ip
}
