//go:build !android

package tun

import (
 "net"
 "sync"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/internal/hook_func"
	"github.com/majianyu2007/nwafu-connect/internal/ippool"
	"github.com/majianyu2007/nwafu-connect/internal/ipresource"
	"github.com/majianyu2007/nwafu-connect/internal/zcdns"
	"github.com/majianyu2007/nwafu-connect/internal/zctcpip"
	"github.com/majianyu2007/nwafu-connect/log"
	"github.com/majianyu2007/nwafu-connect/resolve"
	"github.com/miekg/dns"
	tun "github.com/mythologyli/sing-tun"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	gvisorstack "gvisor.dev/gvisor/pkg/tcpip/stack"
)

const MTU uint32 = 1400

var errTunnelIO = errors.New("VPN tunnel I/O failed")

type Stack struct {
 ipResources []client.IPResource
 resourceIndexOnce sync.Once
 resourceCache *resourceDecisionCache
	endpoint            *Endpoint
	tcpListenerEndpoint *TCPListenerEndpoint
	tcpListenerStack    *gvisorstack.Stack
	l3Conn              io.ReadWriteCloser
	resolve             zcdns.LocalServer
	resourceIndex       *ipresource.Index
	ipPool              *ippool.IPPool[client.DomainResourceSet]
	fakeIP              bool
}

func (s *Stack) setIPResources(resources []client.IPResource) {
	s.ipResources = resources
 s.resourceIndex = ipresource.New(resources)
}

func (s *Stack) SetupResolve(r zcdns.LocalServer) {
	s.resolve = r
}

func (s *Stack) SetupIPPool(ipPool *ippool.IPPool[client.DomainResourceSet]) {
	s.ipPool = ipPool
}

func (s *Stack) Run() error {
	if s.endpoint.client.CanUseTCPTunnel() {
		if err := s.CreateTCPListener(); err != nil {
			return fmt.Errorf("create TCP tunnel listener: %w", err)
		}
		s.StartTCPListener()
	}

	var err error
	s.l3Conn, err = s.endpoint.client.NewL3Conn()
	if err != nil {
		return fmt.Errorf("create L3 tunnel connection: %w", err)
	}
	defer s.l3Conn.Close()

	runErrors := make(chan error, 2)
	go func() {
		for {
			buf := make([]byte, MTU+tun.PacketOffset)
			n, err := s.l3Conn.Read(buf)
			if err != nil {
				runErrors <- fmt.Errorf("read from VPN server: %w", err)
				return
			}
			log.DebugPrintf("Recv: read %d bytes", n)
			log.DebugDumpHex(buf[:n])

			if err := s.endpoint.Write(buf[:n]); err != nil {
				runErrors <- fmt.Errorf("write to TUN interface: %w", err)
				return
			}
		}
	}()

	go func() {
		for {
			buf := make([]byte, MTU+tun.PacketOffset)
			n, err := s.endpoint.Read(buf)
			if err != nil {
				runErrors <- fmt.Errorf("read from TUN interface: %w", err)
				return
			}
			if n < tun.PacketOffset+zctcpip.IPv4PacketMinLength {
				continue
			}

			packet := buf[tun.PacketOffset:n]
			switch ipVersion := packet[0] >> 4; ipVersion {
			case zctcpip.IPv4Version:
				err = s.processIPV4(packet)
			default:
				err = fmt.Errorf("unsupported IP version %d", ipVersion)
			}
			if err == nil {
				continue
			}
			if errors.Is(err, errTunnelIO) {
				runErrors <- err
				return
			}
			log.DebugPrintf("Error occurred while processing IP packet: %v", err)
		}
	}()

	err = <-runErrors
	if hook_func.IsTerminal() {
		return nil
	}
	return err
}

func (s *Stack) processIPV4(packet zctcpip.IPv4Packet) error {
	if !packet.Valid() {
		return fmt.Errorf("invalid IPv4 packet")
	}
	protocol := ""
	port := -1
	switch packet.Protocol() {
	case zctcpip.TCP:
		tcpPacket := zctcpip.TCPPacket(packet.Payload())
		if !tcpPacket.Valid() {
			return fmt.Errorf("invalid TCP packet")
		}
		protocol = "tcp"
		port = int(tcpPacket.DestinationPort())
	case zctcpip.UDP:
		udpPacket := zctcpip.UDPPacket(packet.Payload())
		if !udpPacket.Valid() {
			return fmt.Errorf("invalid UDP packet")
		}
		if s.shouldHijackUDPDns(packet, udpPacket) {
			newPacket := make(zctcpip.IPv4Packet, len(packet))
			copy(newPacket, packet)
			newUdpPacket := zctcpip.UDPPacket(newPacket.Payload())
			// need to be non-blocking
			go s.doHijackUDPDns(newPacket, newUdpPacket)
			return nil
		}

		protocol = "udp"
		port = int(udpPacket.DestinationPort())
	case zctcpip.ICMP:
		if icmpPacket := zctcpip.ICMPPacket(packet.Payload()); !icmpPacket.Valid() {
			return fmt.Errorf("invalid ICMP packet")
		}
		protocol = "icmp"
	default:
		return fmt.Errorf("protocol %d not supported, skip", packet.Protocol())
	}

	domain, resources, ok := s.ipPool.GetDomain(packet.DestinationIP())
	if ok {
		log.DebugPrintf("IP to domain %s", domain)

		if _, matched := client.MatchDomainResource(resources, protocol, port); matched {
			if protocol == "tcp" {
				_, tcpTunnelMatched := client.MatchDomainResourceWhere(resources, protocol, port, func(resource client.DomainResource) bool {
					return !resource.EnableTCPPrefL3
				})
				return s.processIPV4TCP(packet, packet.Payload(), tcpTunnelMatched)
			} else {
				return s.processIPV4UDP(packet, packet.Payload())
			}
		}
	}

	if _, matched := s.matchStaticResource(packet.DestinationIP(), protocol, port); matched {
		if protocol == "icmp" {
			return s.processIPV4ICMP(packet, packet.Payload())
		}
		if protocol == "tcp" {
			_, tcpTunnelMatched := s.resourceIndex.MatchWhere(packet.DestinationIP(), protocol, port, func(resource client.IPResource) bool {
				return !resource.EnableTCPPrefL3
			})
			return s.processIPV4TCP(packet, packet.Payload(), tcpTunnelMatched)
		} else {
			return s.processIPV4UDP(packet, packet.Payload())
		}
	}

	if port != -1 {
		return fmt.Errorf("no VPN resources found for %s:%d, [%s], skip", packet.DestinationIP(), port, protocol)
	} else {
		return fmt.Errorf("no VPN resources found for %s, [%s], skip", packet.DestinationIP(), protocol)
	}
}

func (s *Stack) processIPV4TCP(packet zctcpip.IPv4Packet, tcpPacket zctcpip.TCPPacket, useTCPTunnel bool) error {
	log.DebugPrintf("receive tcp %s:%d -> %s:%d", packet.SourceIP(), tcpPacket.SourcePort(), packet.DestinationIP(), tcpPacket.DestinationPort())

	if !packet.DestinationIP().IsGlobalUnicast() {
		if err := s.endpoint.Write(packet); err != nil {
			return fmt.Errorf("%w: write local TCP packet: %w", errTunnelIO, err)
		}
		return nil
	}

	if useTCPTunnel && s.endpoint.client.CanUseTCPTunnel() {
		pkt := gvisorstack.NewPacketBuffer(gvisorstack.PacketBufferOptions{
			Payload: buffer.MakeWithData(packet),
		})
		s.tcpListenerEndpoint.dispatcher.DeliverNetworkPacket(ipv4.ProtocolNumber, pkt)
		pkt.DecRef()
		return nil
	}

	n, err := s.l3Conn.Write(packet)
	if err != nil {
		if errors.Is(err, client.ErrResourceNotFound) {
			return err
		}
		return fmt.Errorf("%w: write TCP packet: %w", errTunnelIO, err)
	}
	log.DebugPrintf("Send: wrote %d bytes", n)
	log.DebugDumpHex(packet[:n])

	return nil
}

func (s *Stack) processIPV4UDP(packet zctcpip.IPv4Packet, udpPacket zctcpip.UDPPacket) error {
	log.DebugPrintf("receive udp %s:%d -> %s:%d", packet.SourceIP(), udpPacket.SourcePort(), packet.DestinationIP(), udpPacket.DestinationPort())

	if !packet.DestinationIP().IsGlobalUnicast() {
		if err := s.endpoint.Write(packet); err != nil {
			return fmt.Errorf("%w: write local UDP packet: %w", errTunnelIO, err)
		}
		return nil
	}

	n, err := s.l3Conn.Write(packet)
	if err != nil {
		if errors.Is(err, client.ErrResourceNotFound) {
			return err
		}
		return fmt.Errorf("%w: write UDP packet: %w", errTunnelIO, err)
	}
	log.DebugPrintf("Send: wrote %d bytes", n)
	log.DebugDumpHex(packet[:n])

	return nil
}

func (s *Stack) processIPV4ICMP(packet zctcpip.IPv4Packet, icmpHeader zctcpip.ICMPPacket) error {
	log.DebugPrintf("receive icmp %s -> %s", packet.SourceIP(), packet.DestinationIP())
	if icmpHeader.Code() != 0 {
		return nil
	}

	n, err := s.l3Conn.Write(packet)
	if err != nil {
		if errors.Is(err, client.ErrResourceNotFound) {
			return err
		}
		return fmt.Errorf("%w: write ICMP packet: %w", errTunnelIO, err)
	}
	log.DebugPrintf("Send: wrote %d bytes", n)
	log.DebugDumpHex(packet[:n])

	return nil
}

// only can handle udp dns query!
func (s *Stack) shouldHijackUDPDns(ipHeader zctcpip.IPv4Packet, udpHeader zctcpip.UDPPacket) bool {
	if udpHeader.DestinationPort() != 53 {
		return false
	}
	return s.resolve.CheckDnsHijack(ipHeader.DestinationIP())
}

func (s *Stack) doHijackUDPDns(ipHeader zctcpip.IPv4Packet, udpHeader zctcpip.UDPPacket) {
	log.DebugPrintf("hijack dns %s:%d -> %s:%d", ipHeader.SourceIP(), udpHeader.SourcePort(), ipHeader.DestinationIP(), udpHeader.DestinationPort())
	msg := dns.Msg{}
	if err := msg.Unpack(udpHeader.Payload()); err != nil {
		log.Printf("unpack dns msg error: %v", err)
		return
	}
	ctx := context.Background()
	if s.fakeIP && s.endpoint.client.CanUseTCPTunnel() {
		ctx = context.WithValue(ctx, resolve.ContextKeyFakeIP, true)
	}
	resMsg, err := s.resolve.HandleDnsMsg(ctx, &msg)
	if err != nil {
		log.Printf("hijack dns %s:%d -> %s:%d error: %v", ipHeader.SourceIP(), udpHeader.SourcePort(), ipHeader.DestinationIP(), udpHeader.DestinationPort(), err)
		return
	}

	resByte, err := resMsg.Pack()
	if err != nil {
		log.Printf("pack dns msg error: %v", err)
		return
	}

	totalLen := int(ipHeader.HeaderLen()) + zctcpip.UDPHeaderSize + len(resByte)

	newPacket := make(zctcpip.IPv4Packet, totalLen)
	copy(newPacket, ipHeader[:ipHeader.HeaderLen()])
	newPacket.SetTotalLength(uint16(totalLen))
	newPacket.SetSourceIP(ipHeader.DestinationIP())
	newPacket.SetDestinationIP(ipHeader.SourceIP())

	newUDPHeader := zctcpip.UDPPacket(newPacket.Payload())
	newUDPHeader.SetSourcePort(udpHeader.DestinationPort())
	newUDPHeader.SetDestinationPort(udpHeader.SourcePort())
	newUDPHeader.SetLength(zctcpip.UDPHeaderSize + uint16(len(resByte)))
	copy(newUDPHeader.Payload(), resByte)

	newUDPHeader.ResetChecksum(newPacket.PseudoSum())
	newPacket.ResetChecksum()
	_ = s.endpoint.Write(newPacket)
}

func (s *Stack) matchesStaticResource(destination net.IP, protocol string, port int) bool {
	ip, ok := ipresource.IPv4Uint32(destination)
	if !ok {
		return false
	}
	s.resourceIndexOnce.Do(func() {
		s.resourceIndex = ipresource.New(s.ipResources)
		s.resourceCache = newResourceDecisionCache()
	})
	key := resourceDecisionKey{ip: ip, protocol: protocol, port: port}
	if decision, ok := s.resourceCache.get(key); ok {
		return decision
	}
	_, decision := s.resourceIndex.Match(destination, protocol, port)
	s.resourceCache.set(key, decision)
	return decision
}

func (s *Stack) matchStaticResource(destination net.IP, protocol string, port int) (client.IPResource, bool) {
	s.resourceIndexOnce.Do(func() {
		s.resourceIndex = ipresource.New(s.ipResources)
		s.resourceCache = newResourceDecisionCache()
	})
	return s.resourceIndex.Match(destination, protocol, port)
}

