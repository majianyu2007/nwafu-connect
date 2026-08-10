package gvisor

import (
	"errors"
	"fmt"
	"io"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/internal/hook_func"
	"github.com/majianyu2007/nwafu-connect/internal/ippool"
	"github.com/majianyu2007/nwafu-connect/internal/zcdns"
	"github.com/majianyu2007/nwafu-connect/log"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/header"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv4"
	"gvisor.dev/gvisor/pkg/tcpip/stack"
	"gvisor.dev/gvisor/pkg/tcpip/transport/tcp"
	"gvisor.dev/gvisor/pkg/tcpip/transport/udp"
)

type Stack struct {
	gvisorStack *stack.Stack
	resolve     zcdns.LocalServer
	ipPool      *ippool.IPPool[client.DomainResourceSet]

	endpoint *Endpoint
}

const NICID tcpip.NICID = 1
const MTU uint32 = 1400

type Endpoint struct {
	client client.Client

	l3Conn io.ReadWriteCloser
	failed chan error

	dispatcher stack.NetworkDispatcher
}

func (ep *Endpoint) ParseHeader(*stack.PacketBuffer) bool {
	return true
}

func (ep *Endpoint) MTU() uint32 {
	return MTU
}

func (ep *Endpoint) SetMTU(mtu uint32) {
	log.Printf("don't support change MTU from %d to %d", MTU, mtu)
}

func (ep *Endpoint) MaxHeaderLength() uint16 {
	return 0
}

func (ep *Endpoint) LinkAddress() tcpip.LinkAddress {
	return ""
}

func (ep *Endpoint) SetLinkAddress(addr tcpip.LinkAddress) {}

func (ep *Endpoint) Capabilities() stack.LinkEndpointCapabilities {
	return stack.CapabilityNone
}

func (ep *Endpoint) Attach(dispatcher stack.NetworkDispatcher) {
	ep.dispatcher = dispatcher
}

func (ep *Endpoint) IsAttached() bool {
	return ep.dispatcher != nil
}

func (ep *Endpoint) Wait() {}

func (ep *Endpoint) ARPHardwareType() header.ARPHardwareType {
	return header.ARPHardwareNone
}

func (ep *Endpoint) AddHeader(*stack.PacketBuffer) {}

func (ep *Endpoint) Close() {}

func (ep *Endpoint) SetOnCloseAction(func()) {}

// WritePackets is called when get packets from gVisor stack. Then it sends them to VPN server
func (ep *Endpoint) WritePackets(list stack.PacketBufferList) (int, tcpip.Error) {
	written := 0
	for _, packetBuffer := range list.AsSlice() {
		var packet []byte
		for _, slice := range packetBuffer.AsSlices() {
			packet = append(packet, slice...)
		}
		if ep.l3Conn == nil {
			return written, &tcpip.ErrAborted{}
		}

		n, err := ep.l3Conn.Write(packet)
		if err == nil && n != len(packet) {
			err = io.ErrShortWrite
		}
		if err != nil {
			if errors.Is(err, client.ErrResourceNotFound) {
				log.Printf("%v", err)
				return written, &tcpip.ErrHostUnreachable{}
			}
			select {
			case ep.failed <- err:
			default:
			}
			return written, &tcpip.ErrAborted{}
		}
		written++
		log.DebugPrintf("Send: wrote %d bytes", n)
		log.DebugDumpHex(packet[:n])
	}

	return written, nil
}

func NewStack(client client.Client) (*Stack, error) {
	s := &Stack{}

	s.gvisorStack = stack.New(stack.Options{
		NetworkProtocols:   []stack.NetworkProtocolFactory{ipv4.NewProtocol},
		TransportProtocols: []stack.TransportProtocolFactory{tcp.NewProtocol, udp.NewProtocol},
		HandleLocal:        true,
	})

	l3Conn, err := client.NewL3Conn()
	if err != nil {
		return nil, fmt.Errorf("create L3 tunnel connection: %w", err)
	}
	stackReady := false
	defer func() {
		if !stackReady {
			_ = l3Conn.Close()
		}
	}()
	s.endpoint = &Endpoint{
		client: client,
		l3Conn: l3Conn,
		failed: make(chan error, 1),
	}

	tcpipErr := s.gvisorStack.CreateNIC(NICID, s.endpoint)
	if tcpipErr != nil {
		return nil, errors.New(tcpipErr.String())
	}

	ip, err := client.IP()
	if err != nil {
		return nil, err
	}

	addr := tcpip.AddrFromSlice(ip)
	protoAddr := tcpip.ProtocolAddress{
		AddressWithPrefix: tcpip.AddressWithPrefix{
			Address:   addr,
			PrefixLen: 32,
		},
		Protocol: ipv4.ProtocolNumber,
	}

	tcpipErr = s.gvisorStack.AddProtocolAddress(NICID, protoAddr, stack.AddressProperties{})
	if tcpipErr != nil {
		return nil, errors.New(tcpipErr.String())
	}

	sOpt := tcpip.TCPSACKEnabled(true)
	s.gvisorStack.SetTransportProtocolOption(tcp.ProtocolNumber, &sOpt)
	cOpt := tcpip.CongestionControlOption("cubic")
	s.gvisorStack.SetTransportProtocolOption(tcp.ProtocolNumber, &cOpt)
	s.gvisorStack.AddRoute(tcpip.Route{Destination: header.IPv4EmptySubnet, NIC: NICID})

	stackReady = true
	return s, nil
}

func (s *Stack) SetupResolve(r zcdns.LocalServer) {
	s.resolve = r
}

func (s *Stack) SetupIPPool(ipPool *ippool.IPPool[client.DomainResourceSet]) {
	s.ipPool = ipPool
}

func (s *Stack) Run() error {
	readDone := make(chan error, 1)
	go func() {
		buf := make([]byte, MTU)
		for {
			n, err := s.endpoint.l3Conn.Read(buf)
			if err != nil {
				if hook_func.IsTerminal() {
					readDone <- nil
				} else {
					readDone <- fmt.Errorf("read from L3 tunnel: %w", err)
				}
				return
			}
			if n == 0 {
				continue
			}
			log.DebugPrintf("Recv: read %d bytes", n)
			log.DebugDumpHex(buf[:n])

			packetBuffer := stack.NewPacketBuffer(stack.PacketBufferOptions{
				Payload: buffer.MakeWithData(buf[:n]),
			})
			s.endpoint.dispatcher.DeliverNetworkPacket(header.IPv4ProtocolNumber, packetBuffer)
			packetBuffer.DecRef()
		}
	}()

	select {
	case err := <-s.endpoint.failed:
		return fmt.Errorf("write to L3 tunnel: %w", err)
	case err := <-readDone:
		return err
	}
}
