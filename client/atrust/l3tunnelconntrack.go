package atrust

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

type conntrack struct {
	key          string
	authID       uint64
	connectToken string
	appID        string
	nodeGroupID  string
	authCh       chan struct{}
	authErr      error
	authStarted  uint32
	lastUsed     time.Time
}

type conntrackMgr struct {
	mu         sync.Mutex
	nextAuthID uint64
	byKey      map[string]*conntrack
	byID       map[uint64]*conntrack
	lastSweep  time.Time
}

const (
	conntrackIdleTTL       = 30 * time.Minute
	conntrackSweepInterval = time.Minute
)

var errConntrackExpired = errors.New("L3 tunnel conntrack expired")

func newConntrackMgr() *conntrackMgr {
	return &conntrackMgr{
		byKey: make(map[string]*conntrack),
		byID:  make(map[uint64]*conntrack),
	}
}

func (m *conntrackMgr) getByKey(key string) *conntrack {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.byKey[key]
}

func (m *conntrackMgr) getByID(authID uint64) *conntrack {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.byID[authID]
}

func (m *conntrackMgr) getOrCreate(key, appID, nodeGroupID string) *conntrack {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastSweep.IsZero() || now.Sub(m.lastSweep) >= conntrackSweepInterval {
		m.pruneIdleLocked(now)
		m.lastSweep = now
	}
	if ct := m.byKey[key]; ct != nil {
		ct.lastUsed = now
		return ct
	}
	authID := atomic.AddUint64(&m.nextAuthID, 1)
	ct := &conntrack{
		key:         key,
		authID:      authID,
		appID:       appID,
		nodeGroupID: nodeGroupID,
		authCh:      make(chan struct{}),
		lastUsed:    now,
	}
	m.byKey[key] = ct
	m.byID[authID] = ct
	return ct
}

func (m *conntrackMgr) pruneIdleLocked(now time.Time) {
	for key, ct := range m.byKey {
		if now.Sub(ct.lastUsed) < conntrackIdleTTL {
			continue
		}
		delete(m.byKey, key)
		delete(m.byID, ct.authID)
		select {
		case <-ct.authCh:
		default:
			ct.authErr = errConntrackExpired
			close(ct.authCh)
		}
	}
}

func (m *conntrackMgr) expire(ct *conntrack, err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	select {
	case <-ct.authCh:
		return ct.authErr
	default:
	}
	if m.byKey[ct.key] == ct {
		delete(m.byKey, ct.key)
	}
	if m.byID[ct.authID] == ct {
		delete(m.byID, ct.authID)
	}
	ct.authErr = err
	close(ct.authCh)
	return err
}

func (m *conntrackMgr) markAuth(authID uint64, token string, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ct := m.byID[authID]
	if ct == nil {
		return
	}
	select {
	case <-ct.authCh:
		return
	default:
	}
	if token != "" {
		ct.connectToken = token
	}
	ct.authErr = err
	close(ct.authCh)
}
