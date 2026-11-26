package tun

import (
	"context"
	"net"

	"github.com/majianyu2007/nwafu-connect/resolve"
)

func (s *Stack) DialTCP(ctx context.Context, addr *net.TCPAddr) (net.Conn, error) {
	if s.endpoint.client.CanUseTCPTunnel() && !resolve.TCPPrefersL3(ctx) {
		return s.endpoint.client.DialTCP(ctx, addr)
	}

	dialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP:   s.endpoint.ip,
			Port: 0,
		},
	}
	return dialer.DialContext(ctx, "tcp4", addr.String())
}

func (s *Stack) DialUDP(ctx context.Context, addr *net.UDPAddr) (net.Conn, error) {
	dialer := &net.Dialer{
		LocalAddr: &net.UDPAddr{
			IP:   s.endpoint.ip,
			Port: 0,
		},
	}
	return dialer.DialContext(ctx, "udp4", addr.String())
}
