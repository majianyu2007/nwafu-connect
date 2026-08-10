package service

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/majianyu2007/nwafu-connect/dial"
	"github.com/majianyu2007/nwafu-connect/internal/hook_func"
	"github.com/majianyu2007/nwafu-connect/log"
	"github.com/majianyu2007/nwafu-connect/resolve"
	"github.com/things-go/go-socks5"
)

func StartSocks5(bindAddr string, dialer *dial.Dialer, resolver *resolve.Resolver, user, password string) (string, error) {
	var authMethods []socks5.Authenticator
	if user != "" && password != "" {
		authMethods = append(authMethods, socks5.UserPassAuthenticator{
			Credentials: socks5.StaticCredentials{user: password},
		})

		log.Println("Neither traffic nor credentials are encrypted in the SOCKS5 protocol!")
		log.Println("DO NOT deploy it to the public network. All consequences and responsibilities have nothing to do with the developer")
	} else {
		authMethods = append(authMethods, socks5.NoAuthAuthenticator{})
	}

	server := socks5.NewServer(
		socks5.WithAuthMethods(authMethods),
		socks5.WithResolver(resolver),
		socks5.WithDial(dialer.DialIPPort),
		socks5.WithLogger(socks5.NewLogger(log.NewLogger("[SOCKS5] "))),
	)
	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return "", fmt.Errorf("start SOCKS5 listener: %w", err)
	}
	actualAddress := listener.Addr().String()
	log.Printf("SOCKS5 server listening on %s", actualAddress)

	hook_func.RegisterTerminalFunc("CloseSocks5Listener", func(ctx context.Context) error {
		log.Println("Closing SOCKS5 listener...")
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("close SOCKS5 listener failed: %w", err)
		}
		return nil
	})

	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, net.ErrClosed) {
			log.Printf("SOCKS5 server failed: %v", err)
		} else {
			log.Println("SOCKS5 server closed")
		}
	}()
	return actualAddress, nil
}
