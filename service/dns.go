package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"time"

	"github.com/majianyu2007/nwafu-connect/internal/hook_func"
	"github.com/majianyu2007/nwafu-connect/log"
	"github.com/majianyu2007/nwafu-connect/resolve"
	"github.com/miekg/dns"
)

type DNSServer struct {
	resolver *resolve.Resolver
	localDNS []net.IP
	ttl      uint32
}

func (d DNSServer) serveDNSRequest(w dns.ResponseWriter, r *dns.Msg) {
	m := new(dns.Msg)
	m.SetReply(r)
	m.Compress = false
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.handleSingleDNSResolve(ctx, r, m); err != nil {
		m.Rcode = dns.RcodeServerFailure
		log.DebugPrintf("DNS request failed: %v", err)
	}
	_ = w.WriteMsg(m)
}

func (d DNSServer) HandleDnsMsg(ctx context.Context, requestMsg *dns.Msg) (*dns.Msg, error) {
	resMsg := new(dns.Msg)
	resMsg.SetReply(requestMsg)
	resMsg.Compress = false

	err := d.handleSingleDNSResolve(ctx, requestMsg, resMsg)
	if err != nil {
		resMsg.Rcode = dns.RcodeServerFailure
	}
	return resMsg, err
}

func (d DNSServer) CheckDnsHijack(dstIP net.IP) bool {
	for _, ip := range d.localDNS {
		if ip.Equal(dstIP) {
			return false
		}
	}
	return true
}

func (d DNSServer) handleSingleDNSResolve(ctx context.Context, requestMsg *dns.Msg, resMsg *dns.Msg) error {
	if requestMsg.Opcode != dns.OpcodeQuery {
		return nil
	}
	for _, question := range requestMsg.Question {
		if question.Qclass != dns.ClassINET {
			continue
		}
		name := strings.TrimSuffix(question.Name, ".")
		switch question.Qtype {
		case dns.TypeA:
			_, ip, err := d.resolver.Resolve(ctx, name)
			if err != nil {
				return fmt.Errorf("resolve A record for %s: %w", name, err)
			}
			if ip4 := ip.To4(); ip4 != nil {
				resMsg.Answer = append(resMsg.Answer, &dns.A{
					Hdr: dns.RR_Header{Name: question.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: d.ttl},
					A:   ip4,
				})
			}
		case dns.TypeAAAA:
			_, ip, err := d.resolver.Resolve(ctx, name)
			if err != nil {
				return fmt.Errorf("resolve AAAA record for %s: %w", name, err)
			}
			if ip.To4() == nil {
				resMsg.Answer = append(resMsg.Answer, &dns.AAAA{
					Hdr:  dns.RR_Header{Name: question.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: d.ttl},
					AAAA: ip.To16(),
				})
			}
		}
	}
	return nil
}

func NewDnsServer(resolver *resolve.Resolver, dnsServers []string, ttl ...uint64) DNSServer {
	netIPs := make([]net.IP, 0, len(dnsServers))
	for _, dnsServer := range dnsServers {
		if ip := net.ParseIP(dnsServer); ip != nil {
			netIPs = append(netIPs, ip)
		}
	}
	responseTTL := uint32(60)
	if len(ttl) > 0 {
		responseTTL = uint32(min(ttl[0], uint64(math.MaxUint32)))
	}
	return DNSServer{resolver: resolver, localDNS: netIPs, ttl: responseTTL}
}

func StartDNS(bindAddr string, dnsServer DNSServer) (string, error) {
	packetConn, err := net.ListenPacket("udp", bindAddr)
	if err != nil {
		return "", fmt.Errorf("start DNS listener: %w", err)
	}
	server := &dns.Server{
		PacketConn: packetConn,
		Handler:    dns.HandlerFunc(dnsServer.serveDNSRequest),
	}
	actualAddress := packetConn.LocalAddr().String()
	log.Printf("Starting DNS server at %s", actualAddress)

	hook_func.RegisterTerminalFunc("CloseDNSListener", func(ctx context.Context) error {
		log.Println("Closing DNS listener...")
		if err := server.ShutdownContext(ctx); err != nil && !errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("close DNS listener failed: %w", err)
		}
		return nil
	})

	go func() {
		if err := server.ActivateAndServe(); err != nil && !errors.Is(err, net.ErrClosed) && !hook_func.IsTerminal() {
			log.Printf("DNS server failed: %v", err)
		} else {
			log.Println("DNS server closed")
		}
	}()
	return actualAddress, nil
}
