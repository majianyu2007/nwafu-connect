package resolve

import (
	"strings"

	"github.com/majianyu2007/nwafu-connect/client"
)

type domainResourceEntry struct {
	domain    string
	resources client.DomainResourceSet
}

type domainResourceNode struct {
	children map[byte]*domainResourceNode
	exact    *domainResourceEntry
	wildcard *domainResourceEntry
}

type domainResourceIndex struct {
	root *domainResourceNode
}

func newDomainResourceIndex(resources map[string]client.DomainResourceSet) *domainResourceIndex {
	index := &domainResourceIndex{root: &domainResourceNode{}}
	for pattern, resourceSet := range resources {
		pattern = normalizeHostname(pattern)
		wildcard := strings.HasPrefix(pattern, "*.") || strings.HasPrefix(pattern, ".")
		domain := strings.TrimPrefix(strings.TrimPrefix(pattern, "*."), ".")
		if domain == "" {
			continue
		}
		node := index.root
		for pos := len(domain) - 1; pos >= 0; pos-- {
			if node.children == nil {
				node.children = make(map[byte]*domainResourceNode)
			}
			child := node.children[domain[pos]]
			if child == nil {
				child = &domainResourceNode{}
				node.children[domain[pos]] = child
			}
			node = child
		}
		entry := &domainResourceEntry{domain: domain, resources: resourceSet}
		if wildcard {
			node.wildcard = entry
		} else {
			node.exact = entry
		}
	}
	return index
}

func (i *domainResourceIndex) Match(host string) (client.DomainResourceSet, string, bool) {
	host = normalizeHostname(host)
	if host == "" || i == nil || i.root == nil {
		return nil, "", false
	}
	node := i.root
	var best *domainResourceEntry
	for pos := len(host) - 1; pos >= 0; pos-- {
		node = node.children[host[pos]]
		if node == nil {
			break
		}
		if pos != 0 && host[pos-1] != '.' {
			continue
		}
		if node.exact != nil {
			best = node.exact
		} else if pos != 0 && node.wildcard != nil {
			best = node.wildcard
		}
	}
	if best == nil {
		return nil, "", false
	}
	return best.resources, best.domain, true
}

func normalizeHostname(host string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
}
