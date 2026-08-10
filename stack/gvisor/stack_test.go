package gvisor

import (
	"errors"
	"io"
	"testing"

	"github.com/majianyu2007/nwafu-connect/client"
	"gvisor.dev/gvisor/pkg/buffer"
	"gvisor.dev/gvisor/pkg/tcpip"
	gvisorstack "gvisor.dev/gvisor/pkg/tcpip/stack"
)

type failingTunnelConnection struct {
	writeErr    error
	writeLength int
	readRelease chan struct{}
}

func (c *failingTunnelConnection) Read([]byte) (int, error) {
	if c.readRelease != nil {
		<-c.readRelease
	}
	return 0, io.EOF
}

func (c *failingTunnelConnection) Write(payload []byte) (int, error) {
	if c.writeLength >= 0 {
		return c.writeLength, c.writeErr
	}
	return len(payload), c.writeErr
}

func (c *failingTunnelConnection) Close() error { return nil }

func onePacketList(payload []byte) (gvisorstack.PacketBufferList, *gvisorstack.PacketBuffer) {
	packet := gvisorstack.NewPacketBuffer(gvisorstack.PacketBufferOptions{
		Payload: buffer.MakeWithData(payload),
	})
	var packets gvisorstack.PacketBufferList
	packets.PushBack(packet)
	return packets, packet
}

func TestWritePacketsReportsFatalTunnelFailureToRun(t *testing.T) {
	tunnelFailure := errors.New("tunnel write failed")
	connection := &failingTunnelConnection{
		writeErr:    tunnelFailure,
		writeLength: -1,
		readRelease: make(chan struct{}),
	}
	defer close(connection.readRelease)
	endpoint := &Endpoint{
		l3Conn: connection,
		failed: make(chan error, 1),
	}
	packets, packet := onePacketList([]byte("packet"))
	defer packet.DecRef()

	written, tcpipErr := endpoint.WritePackets(packets)
	if written != 0 {
		t.Fatalf("WritePackets() wrote %d packets, want 0", written)
	}
	if _, ok := tcpipErr.(*tcpip.ErrAborted); !ok {
		t.Fatalf("WritePackets() error = %T, want *tcpip.ErrAborted", tcpipErr)
	}

	runErr := (&Stack{endpoint: endpoint}).Run()
	if !errors.Is(runErr, tunnelFailure) {
		t.Fatalf("Run() error = %v, want wrapped tunnel failure", runErr)
	}
}

func TestWritePacketsReturnsUnreachableForUnauthorizedResource(t *testing.T) {
	endpoint := &Endpoint{
		l3Conn: &failingTunnelConnection{
			writeErr:    client.ErrResourceNotFound,
			writeLength: -1,
		},
		failed: make(chan error, 1),
	}
	packets, packet := onePacketList([]byte("packet"))
	defer packet.DecRef()

	written, tcpipErr := endpoint.WritePackets(packets)
	if written != 0 {
		t.Fatalf("WritePackets() wrote %d packets, want 0", written)
	}
	if _, ok := tcpipErr.(*tcpip.ErrHostUnreachable); !ok {
		t.Fatalf("WritePackets() error = %T, want *tcpip.ErrHostUnreachable", tcpipErr)
	}
	select {
	case err := <-endpoint.failed:
		t.Fatalf("authorization rejection stopped the network stack: %v", err)
	default:
	}
}

func TestWritePacketsRejectsShortTunnelWrite(t *testing.T) {
	endpoint := &Endpoint{
		l3Conn: &failingTunnelConnection{writeLength: 1},
		failed: make(chan error, 1),
	}
	packets, packet := onePacketList([]byte("packet"))
	defer packet.DecRef()

	written, tcpipErr := endpoint.WritePackets(packets)
	if written != 0 {
		t.Fatalf("WritePackets() wrote %d packets, want 0", written)
	}
	if _, ok := tcpipErr.(*tcpip.ErrAborted); !ok {
		t.Fatalf("WritePackets() error = %T, want *tcpip.ErrAborted", tcpipErr)
	}
	select {
	case err := <-endpoint.failed:
		if !errors.Is(err, io.ErrShortWrite) {
			t.Fatalf("reported error = %v, want io.ErrShortWrite", err)
		}
	default:
		t.Fatal("short tunnel write was not reported")
	}
}
