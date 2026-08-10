package service

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/majianyu2007/nwafu-connect/internal/hook_func"
	"github.com/majianyu2007/nwafu-connect/log"
	"github.com/majianyu2007/nwafu-connect/stack"
)

const BufferSize = 40960
const DefaultTimeout = 5 * time.Minute
const udpForwardQueueSize = 64
const defaultUDPForwardMaxConnections = 1024
const defaultUDPForwardMaxQueuedBytes int64 = 64 << 20

type UDPForward struct {
	src          *net.UDPAddr
	dest         *net.UDPAddr
	stack        stack.Stack
	listenerConn *net.UDPConn

	connections      map[netip.AddrPort]*UDPConnection
	connectionsMutex sync.RWMutex
	connectionLRU    list.List

	connectCallback    func(addr string)
	disconnectCallback func(addr string)

	timeout time.Duration
	ctx     context.Context
	cancel  context.CancelFunc

	bufferPool     sync.Pool
	maxConnections int
	maxQueuedBytes int64
	queuedBytes    atomic.Int64
	wg             sync.WaitGroup
	closeOnce      sync.Once
	closeErr       error
}

type UDPConnection struct {
	ctx    context.Context
	cancel context.CancelFunc
	send   chan udpDatagram

	sendMu sync.RWMutex
	closed bool

	udpMu sync.Mutex
	udp   net.Conn

	lastActive atomic.Int64
	lruElement *list.Element
	closeOnce  sync.Once
}

type udpBuffer [BufferSize]byte

type udpDatagram struct {
	data           []byte
	buffer         *udpBuffer
	accountedBytes int64
}

func newUDPForward(vpnStack stack.Stack, src, dest string) (*UDPForward, error) {
	source, err := net.ResolveUDPAddr("udp", src)
	if err != nil {
		return nil, fmt.Errorf("invalid UDP forwarding bind address %q: %w", src, err)
	}
	host, portText, err := net.SplitHostPort(dest)
	if err != nil {
		return nil, fmt.Errorf("invalid UDP forwarding destination %q: %w", dest, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid UDP forwarding destination port in %q", dest)
	}
	destinationIP := net.ParseIP(host)
	if destinationIP == nil {
		return nil, fmt.Errorf("invalid UDP forwarding destination IP %q", host)
	}
	listener, err := net.ListenUDP("udp", source)
	if err != nil {
		return nil, fmt.Errorf("start UDP forwarding listener: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	forwarder := &UDPForward{
		src:                listener.LocalAddr().(*net.UDPAddr),
		dest:               &net.UDPAddr{IP: destinationIP, Port: port},
		stack:              vpnStack,
		listenerConn:       listener,
		connections:        make(map[netip.AddrPort]*UDPConnection),
		connectCallback:    func(string) {},
		disconnectCallback: func(string) {},
		timeout:            DefaultTimeout,
		ctx:                ctx,
		cancel:             cancel,
		maxConnections:     defaultUDPForwardMaxConnections,
		maxQueuedBytes:     defaultUDPForwardMaxQueuedBytes,
	}
	forwarder.bufferPool.New = func() any { return new(udpBuffer) }
	return forwarder, nil
}

func StartUDPForwarding(vpnStack stack.Stack, bindAddress, remoteAddress string) (string, error) {
	forwarder, err := newUDPForward(vpnStack, bindAddress, remoteAddress)
	if err != nil {
		return "", err
	}
	log.Printf("UDP port forwarding: %s -> %s", forwarder.src, forwarder.dest)
	hook_func.RegisterTerminalFunc("CloseUDPForwardingPort", func(ctx context.Context) error {
		log.Println("Closing UDP forwarding port...")
		return forwarder.Close()
	})
	go forwarder.start()
	return forwarder.src.String(), nil
}

func (u *UDPForward) start() {
	u.wg.Add(1)
	go func() {
		defer u.wg.Done()
		u.janitor()
	}()
	defer func() {
		_ = u.Close()
		u.wg.Wait()
	}()

	for {
		buffer := u.getBuffer()
		n, address, err := u.listenerConn.ReadFromUDP(buffer[:])
		if err != nil {
			u.putBuffer(buffer)
			if u.ctx.Err() == nil && !errors.Is(err, net.ErrClosed) {
				log.Printf("UDP forwarding listener failed: %v", err)
			}
			return
		}
		log.DebugPrintf("Port forwarding (UDP): %s -> %s -> %s", address, u.src, u.dest)
		u.handleOwned(udpDatagram{data: buffer[:n], buffer: buffer}, address)
	}
}

func (u *UDPForward) handle(data []byte, address *net.UDPAddr) {
	buffer := u.getBuffer()
	if len(data) > len(buffer) {
		u.putBuffer(buffer)
		u.handleOwned(udpDatagram{data: append([]byte(nil), data...)}, address)
		return
	}
	copy(buffer[:], data)
	u.handleOwned(udpDatagram{data: buffer[:len(data)], buffer: buffer}, address)
}

func (u *UDPForward) handleOwned(data udpDatagram, address *net.UDPAddr) {
	if !u.reserveDatagram(&data) {
		u.releaseDatagram(data)
		log.DebugPrintf("UDP forwarding queue budget exceeded for %s; dropping packet", address)
		return
	}
	connection := u.getOrCreateConnection(address)
	if !connection.enqueue(data) {
		u.releaseDatagram(data)
		log.DebugPrintf("UDP forwarding send queue full for %s; dropping packet", address)
	}
}
func (u *UDPForward) getOrCreateConnection(address *net.UDPAddr) *UDPConnection {
	key := normalizedUDPAddrPort(address)
	u.connectionsMutex.Lock()
	if connection := u.connections[key]; connection != nil {
		u.markConnectionActiveLocked(key, connection)
		u.connectionsMutex.Unlock()
		return connection
	}
	evicted := u.makeRoomForConnectionLocked()
	ctx, cancel := context.WithCancel(u.ctx)
	connection := &UDPConnection{
		ctx:    ctx,
		cancel: cancel,
		send:   make(chan udpDatagram, udpForwardQueueSize),
	}
	u.connections[key] = connection
	u.markConnectionActiveLocked(key, connection)
	u.wg.Add(1)
	go func() {
		defer u.wg.Done()
		u.runConnection(key, cloneUDPAddr(address), connection)
	}()
	u.connectionsMutex.Unlock()
	if evicted != nil {
		evicted.close()
	}
	return connection
}

func (u *UDPForward) makeRoomForConnectionLocked() *UDPConnection {
	if u.maxConnections <= 0 || len(u.connections) < u.maxConnections {
		return nil
	}
	oldestElement := u.connectionLRU.Front()
	if oldestElement == nil {
		return nil
	}
	oldestKey := oldestElement.Value.(netip.AddrPort)
	oldest := u.connections[oldestKey]
	delete(u.connections, oldestKey)
	u.connectionLRU.Remove(oldestElement)
	oldest.lruElement = nil
	return oldest
}

func (u *UDPForward) runConnection(key netip.AddrPort, clientAddress *net.UDPAddr, connection *UDPConnection) {
	connected := false
	defer func() {
		connection.close()
		u.removeConnection(key, connection)
		for {
			select {
			case data := <-connection.send:
				u.releaseDatagram(data)
			default:
				if connected {
					u.disconnectCallback(key.String())
				}
				return
			}
		}
	}()

	proxy, err := u.stack.DialUDP(connection.ctx, cloneUDPAddr(u.dest))
	if err != nil {
		if connection.ctx.Err() == nil {
			log.Printf("UDP forwarding dial failed for %s: %v", u.dest, err)
		}
		return
	}
	if !connection.setUDP(proxy) {
		return
	}
	connected = true
	u.connectCallback(key.String())

	readDone := make(chan error, 1)
	go func() {
		readDone <- u.forwardResponses(key, connection, proxy, clientAddress)
	}()
	readFinished := false
	defer func() {
		connection.close()
		if !readFinished {
			<-readDone
		}
	}()

	for {
		select {
		case data := <-connection.send:
			_, err := proxy.Write(data.data)
			u.releaseDatagram(data)
			if err != nil {
				if connection.ctx.Err() == nil {
					log.Printf("UDP forwarding write failed for %s: %v", key, err)
				}
				return
			}
		case err := <-readDone:
			readFinished = true
			if err != nil && connection.ctx.Err() == nil {
				log.Printf("UDP forwarding read closed for %s: %v", key, err)
			}
			return
		case <-connection.ctx.Done():
			return
		}
	}
}

func (u *UDPForward) forwardResponses(key netip.AddrPort, connection *UDPConnection, proxy net.Conn, clientAddress *net.UDPAddr) error {
	buffer := u.getBuffer()
	defer u.putBuffer(buffer)
	for {
		n, err := proxy.Read(buffer[:])
		if err != nil {
			return err
		}
		u.markConnectionActive(key, connection)
		if _, err := u.listenerConn.WriteToUDP(buffer[:n], clientAddress); err != nil {
			return err
		}
	}
}

func (u *UDPForward) removeConnection(key netip.AddrPort, connection *UDPConnection) {
	u.connectionsMutex.Lock()
	if u.connections[key] == connection {
		delete(u.connections, key)
		if connection.lruElement != nil {
			u.connectionLRU.Remove(connection.lruElement)
			connection.lruElement = nil
		}
	}
	u.connectionsMutex.Unlock()
}

func (u *UDPForward) markConnectionActive(key netip.AddrPort, connection *UDPConnection) {
	u.connectionsMutex.Lock()
	if u.connections[key] == connection {
		u.markConnectionActiveLocked(key, connection)
	}
	u.connectionsMutex.Unlock()
}

func (u *UDPForward) markConnectionActiveLocked(key netip.AddrPort, connection *UDPConnection) {
	connection.touch()
	if connection.lruElement == nil {
		connection.lruElement = u.connectionLRU.PushBack(key)
		return
	}
	u.connectionLRU.MoveToBack(connection.lruElement)
}

func (u *UDPForward) janitor() {
	interval := u.timeout / 4
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-u.ctx.Done():
			return
		case now := <-ticker.C:
			cutoff := now.Add(-u.timeout).UnixNano()
			u.connectionsMutex.RLock()
			stale := make([]*UDPConnection, 0)
			for _, connection := range u.connections {
				if connection.lastActive.Load() < cutoff {
					stale = append(stale, connection)
				}
			}
			u.connectionsMutex.RUnlock()
			for _, connection := range stale {
				connection.close()
			}
		}
	}
}

func (u *UDPForward) getBuffer() *udpBuffer {
	return u.bufferPool.Get().(*udpBuffer)
}

func (u *UDPForward) putBuffer(buffer *udpBuffer) {
	u.bufferPool.Put(buffer)
}

func (u *UDPForward) releaseDatagram(data udpDatagram) {
	if data.accountedBytes > 0 {
		u.queuedBytes.Add(-data.accountedBytes)
	}
	if data.buffer != nil {
		u.putBuffer(data.buffer)
	}
}

func (u *UDPForward) reserveDatagram(data *udpDatagram) bool {
	if u.maxQueuedBytes <= 0 {
		return true
	}
	bytes := int64(len(data.data))
	if data.buffer != nil {
		bytes = BufferSize
	}
	for {
		used := u.queuedBytes.Load()
		if bytes > u.maxQueuedBytes-used {
			return false
		}
		if u.queuedBytes.CompareAndSwap(used, used+bytes) {
			data.accountedBytes = bytes
			return true
		}
	}
}

func (u *UDPForward) Close() error {
	u.closeOnce.Do(func() {
		if u.cancel != nil {
			u.cancel()
		}
		if u.listenerConn != nil {
			if err := u.listenerConn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				u.closeErr = fmt.Errorf("close UDP forwarding listener failed: %w", err)
			}
		}
		u.connectionsMutex.RLock()
		connections := make([]*UDPConnection, 0, len(u.connections))
		for _, connection := range u.connections {
			connections = append(connections, connection)
		}
		u.connectionsMutex.RUnlock()
		for _, connection := range connections {
			connection.close()
		}
	})
	return u.closeErr
}

func (c *UDPConnection) enqueue(data udpDatagram) bool {
	c.sendMu.RLock()
	defer c.sendMu.RUnlock()
	if c.closed {
		return false
	}
	select {
	case c.send <- data:
		c.touch()
		return true
	default:
		return false
	}
}

func (c *UDPConnection) setUDP(connection net.Conn) bool {
	c.udpMu.Lock()
	defer c.udpMu.Unlock()
	if c.ctx.Err() != nil {
		_ = connection.Close()
		return false
	}
	c.udp = connection
	return true
}

func (c *UDPConnection) touch() {
	c.lastActive.Store(time.Now().UnixNano())
}

func (c *UDPConnection) close() {
	c.closeOnce.Do(func() {
		c.sendMu.Lock()
		c.closed = true
		c.cancel()
		c.sendMu.Unlock()
		c.udpMu.Lock()
		if c.udp != nil {
			_ = c.udp.Close()
		}
		c.udpMu.Unlock()
	})
}

func normalizedUDPAddrPort(address *net.UDPAddr) netip.AddrPort {
	key := address.AddrPort()
	return netip.AddrPortFrom(key.Addr().Unmap(), key.Port())
}

func cloneUDPAddr(address *net.UDPAddr) *net.UDPAddr {
	clone := *address
	clone.IP = append(net.IP(nil), address.IP...)
	return &clone
}
