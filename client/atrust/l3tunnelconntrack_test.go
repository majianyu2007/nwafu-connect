package atrust

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestConntrackMarkAuthIsSafeForDuplicateResponses(t *testing.T) {
	manager := newConntrackMgr()
	entry := manager.getOrCreate("flow", "app", "group")

	var waitGroup sync.WaitGroup
	for index := range 32 {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			manager.markAuth(entry.authID, fmt.Sprintf("token-%d", index), nil)
		}()
	}
	waitGroup.Wait()

	select {
	case <-entry.authCh:
	default:
		t.Fatal("authentication completion channel was not closed")
	}
	if entry.connectToken == "" {
		t.Fatal("authentication token was not recorded")
	}
}

func TestConntrackManagerPrunesIdleFlows(t *testing.T) {
	manager := newConntrackMgr()
	idle := manager.getOrCreate("idle", "app", "group")
	manager.mu.Lock()
	idle.expiresAt = time.Now().Add(-time.Second)

	manager.mu.Unlock()

	manager.getOrCreate("active", "app", "group")
	if got := manager.getByKey("idle"); got != nil {
		t.Fatalf("idle conntrack was retained: %#v", got)
	}
	if got := manager.getByID(idle.authID); got != nil {
		t.Fatalf("idle auth ID was retained: %#v", got)
	}
	select {
	case <-idle.authCh:
		if !errors.Is(idle.authErr, errConntrackEvicted) {
			t.Fatalf("idle conntrack error = %v, want expiration", idle.authErr)
		}
	default:
		t.Fatal("idle conntrack waiters were not released")
	}
}

func TestConntrackExpireAllowsFreshAuthentication(t *testing.T) {
	manager := newConntrackMgr()
	expired := manager.getOrCreate("flow", "app", "group")
	manager.remove(expired.key)
	replacement := manager.getOrCreate("flow", "app", "group")
	if replacement == expired || replacement.authID == expired.authID {
		t.Fatalf("expired conntrack was reused: old=%#v new=%#v", expired, replacement)
	}
}
