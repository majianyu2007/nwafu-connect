package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/internal/hook_func"
	"github.com/majianyu2007/nwafu-connect/log"
)

func StartTCPForwarding(dialContext client.DialContextFunc, bindAddress, remoteAddress string) (string, error) {
	if _, _, err := net.SplitHostPort(remoteAddress); err != nil {
		return "", fmt.Errorf("invalid TCP forwarding destination %q: %w", remoteAddress, err)
	}

	listener, err := net.Listen("tcp", bindAddress)
	if err != nil {
		return "", fmt.Errorf("start TCP forwarding listener: %w", err)
	}
	actualAddress := listener.Addr().String()
	log.Printf("TCP port forwarding: %s -> %s", actualAddress, remoteAddress)
	hook_func.RegisterTerminalFunc("CloseTCPForwardingPort", func(ctx context.Context) error {
		log.Println("Closing TCP forwarding port...")
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("close TCP forwarding listener failed: %w", err)
		}
		return nil
	})

	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				if !errors.Is(err, net.ErrClosed) {
					log.Printf("TCP forwarding listener failed: %v", err)
				}
				return
			}
			go handleTCPForwardingRequest(dialContext, connection, remoteAddress)
		}
	}()
	return actualAddress, nil
}

func handleTCPForwardingRequest(dialContext client.DialContextFunc, connection net.Conn, destination string) {
	log.Printf("Port forwarding (TCP): %s -> %s -> %s", connection.RemoteAddr(), connection.LocalAddr(), destination)
	proxy, err := dialContext(context.Background(), "tcp", destination)
	if err != nil {
		log.Printf("TCP forwarding dial failed for %s: %v", destination, err)
		_ = connection.Close()
		return
	}
	proxyBidirectionalTCP(connection, proxy)
}

func proxyBidirectionalTCP(left, right net.Conn) {
	completed := make(chan struct{}, 2)
	copyHalf := func(destination, source net.Conn) {
		_, _ = io.Copy(destination, source)
		if closeWriter, ok := destination.(interface{ CloseWrite() error }); ok {
			_ = closeWriter.CloseWrite()
		} else {
			_ = destination.Close()
		}
		completed <- struct{}{}
	}
	go copyHalf(left, right)
	go copyHalf(right, left)
	<-completed
	<-completed
	_ = left.Close()
	_ = right.Close()
}
