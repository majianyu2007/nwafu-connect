package atrust

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

type chunkWriter struct {
	buffer bytes.Buffer
	limit  int
}

func (writer *chunkWriter) Write(payload []byte) (int, error) {
	if len(payload) > writer.limit {
		payload = payload[:writer.limit]
	}
	return writer.buffer.Write(payload)
}

type stalledWriter struct{}

func (stalledWriter) Write([]byte) (int, error) { return 0, nil }

func TestWriteAllCompletesShortWrites(t *testing.T) {
	writer := &chunkWriter{limit: 2}
	payload := []byte("complete tunnel frame")
	if err := writeAll(writer, payload); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(writer.buffer.Bytes(), payload) {
		t.Fatalf("written payload = %q, want %q", writer.buffer.Bytes(), payload)
	}
}

func TestWriteAllRejectsStalledWriter(t *testing.T) {
	if err := writeAll(stalledWriter{}, []byte("frame")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("stalled writer error = %v, want io.ErrShortWrite", err)
	}
}

func TestDialTunnelTLSContextCancelsStalledHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	accepted := make(chan net.Conn, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr == nil {
			accepted <- connection
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	connection, err := dialTunnelTLSContext(ctx, listener.Addr().String())
	if connection != nil {
		connection.Close()
	}
	if err == nil {
		t.Fatal("stalled TLS handshake unexpectedly succeeded")
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("canceled TLS handshake took %v", elapsed)
	}
	select {
	case connection := <-accepted:
		connection.Close()
	case <-time.After(time.Second):
		t.Fatal("test listener did not accept tunnel connection")
	}
}
