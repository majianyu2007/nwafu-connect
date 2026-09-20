package atrust

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func testL3Tunnel() *L3Tunnel {
	return &L3Tunnel{
		conns:    make(map[string]*l3TunnelConn),
		dataChan: make(chan []byte, 1),
		closeCh:   make(chan struct{}),
	}
}

func TestCampusL3ConnCloseUnblocksRead(t *testing.T) {
	tunnel := testL3Tunnel()
	connection, err := tunnel.NewL3Conn()
	if err != nil {
		t.Fatal(err)
	}
	readDone := make(chan error, 1)
	go func() {
		_, err := connection.Read(make([]byte, 1))
		readDone <- err
	}()
	if err := connection.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-readDone:
		if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Read() error = %v, want io.EOF", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Read() remained blocked after L3 connection close")
	}
}

func TestL3TunnelCloseUnblocksReadAndRejectsNewConnections(t *testing.T) {
	tunnel := testL3Tunnel()
	connection, err := tunnel.NewL3Conn()
	if err != nil {
		t.Fatal(err)
	}
	readDone := make(chan error, 1)
	go func() {
		_, err := connection.Read(make([]byte, 1))
		readDone <- err
	}()
	tunnel.Close()

	select {
	case err := <-readDone:
		if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Read() error = %v, want io.EOF", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Read() remained blocked after L3 tunnel close")
	}
	if _, err := tunnel.NewL3Conn(); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("NewL3Conn() error = %v, want net.ErrClosed", err)
	}
}
