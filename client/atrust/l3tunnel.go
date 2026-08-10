package atrust

import (
	"fmt"
	"net"
	"sync"

	"github.com/majianyu2007/nwafu-connect/internal/ipresource"
)

type L3Tunnel struct {
	client *Client

	ip net.IP

	resourceIndex *ipresource.Index

	conns   map[string]*l3TunnelConn
	connsMu sync.Mutex

	vipMu   sync.Mutex
	vipList []net.IP

	dataChan  chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

func NewL3Tunnel(aTrustClient *Client) (*L3Tunnel, error) {
	t := &L3Tunnel{
		client:   aTrustClient,
		conns:    make(map[string]*l3TunnelConn),
		dataChan: make(chan []byte, 4096),
		closed:   make(chan struct{}),
	}

	t.resourceIndex = aTrustClient.resourceIndex
	if t.resourceIndex == nil {
		ipResources, err := aTrustClient.IPResources()
		if err != nil {
			return nil, fmt.Errorf("failed to get IP resources: %w", err)
		}
		t.resourceIndex = ipresource.New(ipResources)
	}

	ip, err := aTrustClient.IP()
	if err != nil {
		return nil, fmt.Errorf("failed to get client IP: %w", err)
	}
	t.ip = ip

	return t, nil
}

func (t *L3Tunnel) updateVIP(ips []net.IP) {
	t.vipMu.Lock()
	defer t.vipMu.Unlock()
	t.vipList = ips
}

func (t *L3Tunnel) Close() {
	t.closeOnce.Do(func() {
		close(t.closed)
		t.connsMu.Lock()
		conns := make([]*l3TunnelConn, 0, len(t.conns))
		for _, conn := range t.conns {
			conns = append(conns, conn)
		}
		t.conns = make(map[string]*l3TunnelConn)
		t.connsMu.Unlock()

		for _, conn := range conns {
			_ = conn.Close()
		}
	})
}

func (t *L3Tunnel) getConn(nodeGroupID string) (*l3TunnelConn, error) {
	select {
	case <-t.closed:
		return nil, net.ErrClosed
	default:
	}
	t.connsMu.Lock()
	if conn := t.conns[nodeGroupID]; conn != nil {
		t.connsMu.Unlock()
		return conn, nil
	}
	t.connsMu.Unlock()

	t.client.BestNodesRWMutex.RLock()
	addr := t.client.BestNodes[nodeGroupID]
	if addr == "" {
		addr = t.client.BestNodes[t.client.MajorNodeGroup]
	}
	t.client.BestNodesRWMutex.RUnlock()
	if addr == "" {
		return nil, fmt.Errorf("no available node for group %s", nodeGroupID)
	}

	info := clientInfo{
		sid:          t.client.SID,
		deviceID:     t.client.DeviceID,
		connectionID: t.client.ConnectionID,
		username:     t.client.Username,
	}
	conn, err := newL3TunnelConn(addr, info, t.client.SignKey, t.updateVIP)
	if err != nil {
		return nil, err
	}

	t.connsMu.Lock()
	select {
	case <-t.closed:
		t.connsMu.Unlock()
		_ = conn.Close()
		return nil, net.ErrClosed
	default:
	}
	if existing := t.conns[nodeGroupID]; existing != nil {
		t.connsMu.Unlock()
		_ = conn.Close()
		return existing, nil
	}
	t.conns[nodeGroupID] = conn
	t.connsMu.Unlock()

	go t.forwardFromConn(nodeGroupID, conn)

	return conn, nil
}

func (t *L3Tunnel) evictConn(nodeGroupID string, conn *l3TunnelConn) {
	removed := false
	t.connsMu.Lock()
	if existing := t.conns[nodeGroupID]; existing == conn {
		delete(t.conns, nodeGroupID)
		removed = true
	}
	t.connsMu.Unlock()
	if removed {
		_ = conn.Close()
	}
}

func (t *L3Tunnel) forwardFromConn(nodeGroupID string, conn *l3TunnelConn) {
	for {
		pkt, err := conn.ReadPacket()
		if err != nil {
			t.evictConn(nodeGroupID, conn)
			return
		}
		logPacket("recv", pkt)
		select {
		case t.dataChan <- pkt:
		case <-t.closed:
			return
		}
	}
}
