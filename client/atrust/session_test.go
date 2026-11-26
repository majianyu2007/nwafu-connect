package atrust

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/majianyu2007/nwafu-connect/client/atrust/auth"
)

func TestSessionRefreshPublishesAndPersists(t *testing.T) {
	c := NewClient(ClientOptions{Session: SessionOptions{SID: "old"}})
	defer c.Close()
	persisted := make(chan []byte, 1)
	retried := make(chan struct{})
	calls := 0
	c.startSessionRefresh(func(ctx context.Context) (auth.LoginResult, error) {
		calls++
		if calls == 1 {
			return auth.LoginResult{SID: "new", Cookies: []auth.Cookie{{Name: "sid", Value: "new"}}}, nil
		}
		close(retried)
		<-ctx.Done()
		return auth.LoginResult{}, ctx.Err()
	}, auth.ClientAuthData{DeviceID: "device", ServerVersionInfo: json.RawMessage(`{"code":0}`)}, func(data []byte) error {
		if sid, err := c.sessionSID(); sid != "new" || err != nil {
			t.Errorf("published SID = %q, %v", sid, err)
		}
		persisted <- data
		return nil
	}, time.Millisecond)
	select {
	case <-retried:
	case <-c.refreshDone:
		t.Fatal("refresh stopped unexpectedly")
	case <-time.After(time.Second):
		t.Fatal("next refresh did not start")
	}
	if calls != 2 {
		t.Fatalf("refresh calls = %d", calls)
	}
	var saved auth.ClientAuthData
	select {
	case data := <-persisted:
		if err := json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("cookies not persisted")
	}
	if saved.DeviceID != "device" || len(saved.ServerVersionInfo) == 0 || saved.Cookies[0].Value != "new" {
		t.Fatalf("saved = %+v", saved)
	}
	if sid, err := c.sessionSID(); sid != "new" || err != nil {
		t.Fatalf("retained SID = %q, %v", sid, err)
	}
	if sid, err := (clientInfo{sidProvider: c.sessionSID}).currentSID(); sid != "new" || err != nil {
		t.Fatalf("L3 SID = %q, %v", sid, err)
	}
}

func TestSessionRefreshPreservesSIDOnTransientFailureAndCancels(t *testing.T) {
	c := NewClient(ClientOptions{Session: SessionOptions{SID: "old"}})
	defer c.Close()
	second := make(chan struct{})
	calls := 0
	c.startSessionRefresh(func(ctx context.Context) (auth.LoginResult, error) {
		calls++
		if calls == 1 {
			return auth.LoginResult{}, errors.New("temporary network error")
		}
		close(second)
		<-ctx.Done()
		return auth.LoginResult{}, ctx.Err()
	}, auth.ClientAuthData{}, nil, time.Millisecond)
	select {
	case <-second:
	case <-time.After(time.Second):
		t.Fatal("no retry")
	}
	if sid, err := c.sessionSID(); sid != "old" || err != nil {
		t.Fatalf("SID = %q, %v", sid, err)
	}
	done := make(chan struct{})
	go func() { c.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel refresh")
	}
}

func TestExistingL3InfoUsesCurrentSIDAndSignsIt(t *testing.T) {
	c := NewClient(ClientOptions{Session: SessionOptions{SID: "old"}})
	defer c.Close()
	info := clientInfo{sid: "old", sidProvider: c.sessionSID}
	meta := packetMeta{atype: 4, proto: 17, srcIP: net.IPv4(192, 0, 2, 1), dstIP: net.IPv4(198, 51, 100, 2), srcPort: 1234, dstPort: 53}
	for _, sid := range []string{"first", "second"} {
		c.setSessionSID(sid, nil)
		data, err := buildAuthRequest(info, []byte("key"), meta, &conntrack{appID: "app", authID: 1})
		if err != nil {
			t.Fatal(err)
		}
		var req authRequestIP
		if err := json.Unmarshal(data, &req); err != nil {
			t.Fatal(err)
		}
		if req.Sid != sid {
			t.Fatalf("wire SID = %q, want %q", req.Sid, sid)
		}
		signature := req.XRequestSig
		req.XRequestSig = ""
		unsigned, _ := json.Marshal(req)
		if signature != calcXRequestSig([]byte("key"), unsigned) {
			t.Fatal("signature did not cover current SID")
		}
	}
	c.setSessionSID("", auth.ErrSessionInvalid)
	if _, err := buildAuthRequest(info, []byte("key"), meta, &conntrack{}); !errors.Is(err, auth.ErrSessionInvalid) {
		t.Fatalf("auth error = %v", err)
	}
}

func TestConcurrentSessionSIDReaders(t *testing.T) {
	c := NewClient(ClientOptions{})
	defer c.Close()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				c.sessionSID()
			}
		}()
	}
	for i := 0; i < 1000; i++ {
		c.setSessionSID("new", nil)
	}
	wg.Wait()
}

func TestSessionInvalidNotifiesWithoutExiting(t *testing.T) {
	t.Run("refresh", func(t *testing.T) {
		c := NewClient(ClientOptions{Session: SessionOptions{SID: "old"}})
		defer c.Close()
		invalidCh := make(chan error, 1)
		c.onSessionInvalid = func(err error) { invalidCh <- err }
		c.startSessionRefresh(func(context.Context) (auth.LoginResult, error) {
			return auth.LoginResult{}, fmt.Errorf("refresh rejected: %w", auth.ErrSessionInvalid)
		}, auth.ClientAuthData{}, nil, time.Millisecond)
		select {
		case err := <-invalidCh:
			if !errors.Is(err, auth.ErrSessionInvalid) {
				t.Fatalf("error = %v, want ErrSessionInvalid", err)
			}
		case <-time.After(time.Second):
			t.Fatal("OnSessionInvalid was not invoked")
		}
		<-c.refreshDone
	})

	for _, code := range []string{"10000004", "75500002"} {
		payload := fmt.Sprintf(`{"code":%s,"message":"session rejected"}`, code)
		t.Run("tcp:"+code, func(t *testing.T) {
			if err := parseTCPTunnelAuthResponse(payload); !errors.Is(err, auth.ErrSessionInvalid) {
				t.Fatalf("error = %v, want ErrSessionInvalid", err)
			}
		})
		t.Run("ip:"+code, func(t *testing.T) {
			if err := parseIPAuthResponse([]byte(payload)); !errors.Is(err, auth.ErrSessionInvalid) {
				t.Fatalf("error = %v, want ErrSessionInvalid", err)
			}
		})
		t.Run("l3:"+code, func(t *testing.T) {
			var notified error
			conn := &l3TunnelConn{
				info: clientInfo{
					onSessionInvalid: func(err error) { notified = err },
				},
				closeCh: make(chan struct{}),
			}
			conn.handleAuthResp(authImmediateRetryStatus, []byte(payload))
			if !errors.Is(notified, auth.ErrSessionInvalid) {
				t.Fatalf("notified = %v, want ErrSessionInvalid", notified)
			}
		})
	}
}
