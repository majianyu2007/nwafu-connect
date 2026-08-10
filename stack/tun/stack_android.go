package tun

import (
	"fmt"
	"io"
	"net"
	"os"
	"syscall"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/log"
	"golang.org/x/net/ipv4"
)

const MTU uint32 = 1400

type Stack struct {
	endpoint *Endpoint
	l3Conn   io.ReadWriteCloser
}

func (s *Stack) Run() error {
	var err error
	s.l3Conn, err = s.endpoint.client.NewL3Conn()
	if err != nil {
		return fmt.Errorf("create L3 tunnel connection: %w", err)
	}
	defer s.l3Conn.Close()

	runErrors := make(chan error, 2)
	go func() {
		for {
			buf := make([]byte, MTU)
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
			buf := make([]byte, MTU)
			n, err := s.endpoint.Read(buf)
			if err != nil {
				runErrors <- fmt.Errorf("read from TUN interface: %w", err)
				return
			}

			header, err := ipv4.ParseHeader(buf[:n])
			if err != nil {
				continue
			}
			if header.Protocol != syscall.IPPROTO_TCP && header.Protocol != syscall.IPPROTO_UDP {
				continue
			}

			n, err = s.l3Conn.Write(buf[:n])
			if err != nil {
				runErrors <- fmt.Errorf("write to VPN server: %w", err)
				return
			}
			log.DebugPrintf("Send: wrote %d bytes", n)
			log.DebugDumpHex(buf[:n])
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

func NewStack(client client.Client) (*Stack, error) {
	s := &Stack{}

	s.endpoint = &Endpoint{
		client: client,
	}

	var err error
	s.endpoint.ip, err = client.IP()
	if err != nil {
		return nil, err
	}

	// We need this dialer to bind to device otherwise packets will not be sent via TUN
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
