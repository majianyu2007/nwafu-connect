package atrust

import (
	"io"
	"net"
	"sync"
)

type L3Conn struct {
	l3Tunnel  *L3Tunnel
	sendLock  sync.Mutex
	recvLock  sync.Mutex
	closed    chan struct{}
	closeOnce sync.Once
}

func (c *L3Conn) Read(p []byte) (int, error) {
	c.recvLock.Lock()
	defer c.recvLock.Unlock()
	select {
	case data := <-c.l3Tunnel.dataChan:
		return copy(p, data), nil
	case <-c.closed:
		return 0, io.EOF
	case <-c.l3Tunnel.closed:
		return 0, io.EOF
	}
}

func (c *L3Conn) Write(p []byte) (int, error) {
	c.sendLock.Lock()
	defer c.sendLock.Unlock()
	select {
	case <-c.closed:
		return 0, net.ErrClosed
	case <-c.l3Tunnel.closed:
		return 0, net.ErrClosed
	default:
	}
	if err := c.l3Tunnel.processIPV4(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *L3Conn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (t *L3Tunnel) NewL3Conn() (io.ReadWriteCloser, error) {
	select {
	case <-t.closed:
		return nil, net.ErrClosed
	default:
	}
	return &L3Conn{
		l3Tunnel: t,
		closed:   make(chan struct{}),
	}, nil
}
