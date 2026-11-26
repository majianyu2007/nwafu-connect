package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/internal/hook_func"
	"github.com/majianyu2007/nwafu-connect/log"
)

const tcpForwardingDialTimeout = 15 * time.Second

func StartTCPForwarding(dialContext client.DialContextFunc, bindAddress, remoteAddress string) (string, error) {
	if dialContext == nil {
		return "", errors.New("TCP forwarding dialer is required")
	}
	host, portText, err := net.SplitHostPort(remoteAddress)
	if err != nil || strings.TrimSpace(host) == "" {
		return "", fmt.Errorf("invalid TCP forwarding destination %q", remoteAddress)
	}
	if _, err := net.LookupPort("tcp", portText); err != nil {
		return "", fmt.Errorf("invalid TCP forwarding destination port %q: %w", portText, err)
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

func handleTCPForwardingRequest(dialContext client.DialContextFunc, connection net.Conn, remoteAddress string) {
	log.Printf("Port forwarding (TCP): %s -> %s -> %s", connection.RemoteAddr(), connection.LocalAddr(), remoteAddress)
	ctx, cancel := context.WithTimeout(context.Background(), tcpForwardingDialTimeout)
	proxy, err := dialContext(ctx, "tcp", remoteAddress)
	cancel()
	if err != nil {
		log.Printf("TCP forwarding dial failed for %s: %v", remoteAddress, err)
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
