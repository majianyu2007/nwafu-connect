//go:build android

package tun

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"syscall"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/internal/ippool"
	"github.com/majianyu2007/nwafu-connect/internal/zcdns"
	"github.com/majianyu2007/nwafu-connect/log"
	"golang.org/x/net/ipv4"
)

const MTU uint32 = 1400
const maxInboundPacketSize = 1500

type Stack struct {
	endpoint *Endpoint
	l3Conn   io.ReadWriteCloser
}

func (s *Stack) Run() error {
	var connErr error
	s.l3Conn, connErr = s.endpoint.client.NewL3Conn()
	if connErr != nil {
		return fmt.Errorf("create L3 tunnel connection: %w", connErr)
	}
	defer s.l3Conn.Close()

	runErrors := make(chan error, 2)
	// Read from VPN server and send to TUN stack
	go func() {
		buf := make([]byte, maxInboundPacketSize)
		for {
			n, err := s.l3Conn.Read(buf)
			if err != nil {
				runErrors <- fmt.Errorf("read from VPN server: %w", err)
				return
			}
			log.DebugPrintf("Recv: read %d bytes", n)
			log.DebugDumpHex(buf[:n])

			if err := s.endpoint.Write(buf[:n]); err != nil {
				runErrors <- fmt.Errorf("write to TUN stack: %w", err)
				return
			}
		}
	}()

	// Read from TUN stack and send to VPN server
	go func() {
		buf := make([]byte, maxInboundPacketSize)
		for {
			n, err := s.endpoint.Read(buf)
			if err != nil {
				runErrors <- fmt.Errorf("read from TUN stack: %w", err)
				return
			}

			header, err := ipv4.ParseHeader(buf[:n])
			if err != nil {
				continue
			}

			// Filter out non-TCP/UDP packets otherwise error may occur
			if header.Protocol != syscall.IPPROTO_TCP && header.Protocol != syscall.IPPROTO_UDP {
				continue
			}

			written, err := s.l3Conn.Write(buf[:n])
			if err != nil {
				runErrors <- fmt.Errorf("write to VPN server: %w", err)
				return
			}
			log.DebugPrintf("Send: wrote %d bytes", written)
			log.DebugDumpHex(buf[:written])
		}
	}()

	return <-runErrors
}

type Endpoint struct {
	client client.Client

	readWriteCloser io.ReadWriteCloser
	ip              net.IP

	tcpDialer *net.Dialer
	udpDialer *net.Dialer
	configMu  sync.RWMutex
}

func (ep *Endpoint) Write(buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	_, err := ep.readWriteCloser.Write(buf)
	return err
}

func (ep *Endpoint) Read(buf []byte) (int, error) {
	return ep.readWriteCloser.Read(buf)
}

func (s *Stack) AddRoute(target string) error {
	return nil
}

func (s *Stack) SetupResolve(zcdns.LocalServer) {}

func (s *Stack) SetupIPPool(*ippool.IPPool[[]client.DomainResource]) {}

func NewStack(vpnClient client.Client, _ bool, _ bool, _ []client.IPResource) (*Stack, error) {
	s := &Stack{}

	s.endpoint = &Endpoint{
		client: vpnClient,
	}

	var err error
	s.endpoint.ip, err = vpnClient.IP()
	if err != nil {
		return nil, err
	}

	s.endpoint.tcpDialer = &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP:   s.endpoint.ip,
			Port: 0,
		},
	}

	s.endpoint.udpDialer = &net.Dialer{
		LocalAddr: &net.UDPAddr{
			IP:   s.endpoint.ip,
			Port: 0,
		},
	}

	return s, nil
}

func (s *Stack) SetupTun(fd int) {
	s.endpoint.readWriteCloser = os.NewFile(uintptr(fd), "tun")
}
