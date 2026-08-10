package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/internal/ippool"
	"github.com/majianyu2007/nwafu-connect/internal/zcdns"
)

type forwardingTestStack struct {
	dialTCP func(context.Context, *net.TCPAddr) (net.Conn, error)
	dialUDP func(context.Context, *net.UDPAddr) (net.Conn, error)
}

func (s *forwardingTestStack) Run() error { return nil }

func (s *forwardingTestStack) SetupResolve(zcdns.LocalServer) {}

func (s *forwardingTestStack) SetupIPPool(*ippool.IPPool[client.DomainResourceSet]) {}

func (s *forwardingTestStack) DialTCP(ctx context.Context, address *net.TCPAddr) (net.Conn, error) {
	return s.dialTCP(ctx, address)
}

func (s *forwardingTestStack) DialUDP(ctx context.Context, address *net.UDPAddr) (net.Conn, error) {
	return s.dialUDP(ctx, address)
}

func TestForwardingRejectsMalformedDestinations(t *testing.T) {
	if _, err := StartTCPForwarding(nil, "127.0.0.1:0", "missing-port"); err == nil {
		t.Fatal("TCP forwarding accepted a malformed destination")
	}
	if _, err := StartUDPForwarding(nil, "127.0.0.1:0", "missing-port"); err == nil {
		t.Fatal("UDP forwarding accepted a malformed destination")
	}
}

func TestTCPForwardingDialFailureClosesClientWithoutPanicking(t *testing.T) {
	vpnStack := &forwardingTestStack{dialTCP: func(context.Context, *net.TCPAddr) (net.Conn, error) {
		return nil, errors.New("dial failed")
	}}
	clientConnection, forwardingConnection := net.Pipe()
	done := make(chan struct{})
	go func() {
		handleTCPForwardingRequest(vpnStack, forwardingConnection, &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 443})
		close(done)
	}()
	if err := clientConnection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := clientConnection.Read(make([]byte, 1)); err == nil {
		t.Fatal("client connection remained open after forwarding dial failure")
	}
	clientConnection.Close()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("TCP forwarding handler did not return after dial failure")
	}
}

func TestUDPForwardingDialFailureDrainsQueuedPackets(t *testing.T) {
	dialStarted := make(chan struct{})
	releaseDial := make(chan struct{})
	vpnStack := &forwardingTestStack{dialUDP: func(context.Context, *net.UDPAddr) (net.Conn, error) {
		close(dialStarted)
		<-releaseDial
		return nil, errors.New("dial failed")
	}}
	ctx, cancel := context.WithCancel(context.Background())
	forwarder := &UDPForward{
		dest:               &net.UDPAddr{IP: net.ParseIP("192.0.2.20"), Port: 53},
		stack:              vpnStack,
		connections:        make(map[netip.AddrPort]*UDPConnection),
		connectCallback:    func(string) {},
		disconnectCallback: func(string) {},
		ctx:                ctx,
		cancel:             cancel,
		maxConnections:     8,
		maxQueuedBytes:     2 * BufferSize,
	}
	forwarder.bufferPool.New = func() any { return new(udpBuffer) }
	t.Cleanup(func() { _ = forwarder.Close() })

	clientAddress := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321}
	forwarder.handle([]byte("first"), clientAddress)
	select {
	case <-dialStarted:
	case <-time.After(time.Second):
		t.Fatal("UDP forwarding dial did not start")
	}
	forwarder.handle([]byte("second"), clientAddress)
	close(releaseDial)

	deadline := time.Now().Add(time.Second)
	for {
		forwarder.connectionsMutex.RLock()
		remaining := len(forwarder.connections)
		forwarder.connectionsMutex.RUnlock()
		if remaining == 0 && forwarder.queuedBytes.Load() == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("UDP dial failure retained %d connections and %d queued bytes", remaining, forwarder.queuedBytes.Load())
		}
		time.Sleep(time.Millisecond)
	}
}

func TestUDPForwardingBoundsQueuedBytes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	address := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 54321}
	key := normalizedUDPAddrPort(address)
	connectionCtx, connectionCancel := context.WithCancel(ctx)
	connection := &UDPConnection{
		ctx:    connectionCtx,
		cancel: connectionCancel,
		send:   make(chan udpDatagram, udpForwardQueueSize),
	}
	forwarder := &UDPForward{
		connections:    map[netip.AddrPort]*UDPConnection{key: connection},
		ctx:            ctx,
		cancel:         cancel,
		maxQueuedBytes: BufferSize,
	}
	forwarder.bufferPool.New = func() any { return new(udpBuffer) }
	forwarder.markConnectionActiveLocked(key, connection)

	forwarder.handle([]byte("first"), address)
	forwarder.handle([]byte("second"), address)
	if got := len(connection.send); got != 1 {
		t.Fatalf("queued datagrams = %d, want 1", got)
	}
	if got := forwarder.queuedBytes.Load(); got != BufferSize {
		t.Fatalf("accounted queued bytes = %d, want %d", got, BufferSize)
	}
	forwarder.releaseDatagram(<-connection.send)
	if got := forwarder.queuedBytes.Load(); got != 0 {
		t.Fatalf("accounted queued bytes after release = %d, want 0", got)
	}
	connection.close()
}

func TestUDPForwardingEvictsLeastRecentlyUsedConnection(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	firstContext, firstCancel := context.WithCancel(ctx)
	secondContext, secondCancel := context.WithCancel(ctx)
	first := &UDPConnection{ctx: firstContext, cancel: firstCancel, send: make(chan udpDatagram, 1)}
	second := &UDPConnection{ctx: secondContext, cancel: secondCancel, send: make(chan udpDatagram, 1)}
	firstKey := netip.MustParseAddrPort("127.0.0.1:10001")
	secondKey := netip.MustParseAddrPort("127.0.0.1:10002")
	forwarder := &UDPForward{
		connections:    map[netip.AddrPort]*UDPConnection{firstKey: first, secondKey: second},
		maxConnections: 2,
	}
	forwarder.markConnectionActiveLocked(firstKey, first)
	forwarder.markConnectionActiveLocked(secondKey, second)
	forwarder.markConnectionActiveLocked(firstKey, first)

	evicted := forwarder.makeRoomForConnectionLocked()
	if evicted != second {
		t.Fatalf("evicted connection = %p, want least recently used %p", evicted, second)
	}
	if _, found := forwarder.connections[secondKey]; found {
		t.Fatal("least recently used connection remained in the map")
	}
	if _, found := forwarder.connections[firstKey]; !found {
		t.Fatal("recently active connection was removed")
	}
	first.close()
	second.close()
}

func TestUDPForwardingRelaysDatagrams(t *testing.T) {
	proxyConnection, targetConnection := net.Pipe()
	defer targetConnection.Close()
	vpnStack := &forwardingTestStack{dialUDP: func(context.Context, *net.UDPAddr) (net.Conn, error) {
		return proxyConnection, nil
	}}
	forwarder, err := newUDPForward(vpnStack, "127.0.0.1:0", "192.0.2.20:53")
	if err != nil {
		t.Fatal(err)
	}
	runDone := make(chan struct{})
	go func() {
		forwarder.start()
		close(runDone)
	}()
	t.Cleanup(func() {
		_ = forwarder.Close()
		select {
		case <-runDone:
		case <-time.After(time.Second):
			t.Error("UDP forwarding loop did not stop")
		}
	})

	targetDone := make(chan error, 1)
	go func() {
		payload := make([]byte, 4)
		if _, err := targetConnection.Read(payload); err != nil {
			targetDone <- err
			return
		}
		if string(payload) != "ping" {
			targetDone <- fmt.Errorf("target received %q, want ping", payload)
			return
		}
		_, err := targetConnection.Write([]byte("pong"))
		targetDone <- err
	}()

	clientConnection, err := net.DialUDP("udp", nil, forwarder.src)
	if err != nil {
		t.Fatal(err)
	}
	defer clientConnection.Close()
	if err := clientConnection.SetDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := clientConnection.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 4)
	if _, err := clientConnection.Read(response); err != nil {
		t.Fatal(err)
	}
	if string(response) != "pong" {
		t.Fatalf("forwarded response = %q, want pong", response)
	}
	if err := <-targetDone; err != nil {
		t.Fatalf("UDP target fixture: %v", err)
	}
}
