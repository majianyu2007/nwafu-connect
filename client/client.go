package client

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"

	"inet.af/netaddr"
)

var ErrResourceNotFound = errors.New("resource not found")

type IPResource struct {
	IPMin       net.IP
	IPMax       net.IP
	PortMin     int
	PortMax     int
	Protocol    string
	AppID       string
	NodeGroupID string
}

type DomainResource struct {
	PortMin     int
	PortMax     int
	Protocol    string
	AppID       string
	NodeGroupID string
}

type DomainResourceSet []DomainResource

func (resources DomainResourceSet) Match(port int, protocol string) (DomainResource, bool) {
	for _, resource := range resources {
		if resource.PortMin <= port && port <= resource.PortMax &&
			(strings.EqualFold(resource.Protocol, protocol) || strings.EqualFold(resource.Protocol, "all")) {
			return resource, true
		}
	}
	return DomainResource{}, false
}

type ResourceAddress struct {
	Host     string
	PortMin  int
	PortMax  int
	Protocol string
}

type Resource struct {
	Name        string
	Description string
	Addresses   []ResourceAddress
}

type Client interface {
	IP() (net.IP, error)
	IPSet() (*netaddr.IPSet, error)
	IPResources() ([]IPResource, error)
	DomainResources() (map[string]DomainResourceSet, error)
	Resources() ([]Resource, error)
	DNSResource() (map[string]net.IP, error)
	DNSServer() (string, error)

	CanUseTCPTunnel() bool
	DialTCP(ctx context.Context, addr *net.TCPAddr) (net.Conn, error)
	NewL3Conn() (io.ReadWriteCloser, error)
}
