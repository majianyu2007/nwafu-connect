package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/majianyu2007/nwafu-connect/dial"
	"github.com/majianyu2007/nwafu-connect/internal/hook_func"
	"github.com/majianyu2007/nwafu-connect/log"
)

const (
	httpProxyMaxIdleConns        = 100
	httpProxyMaxIdleConnsPerHost = 10
	httpProxyMaxConnsPerHost     = 50
	httpProxyMaxTunnels          = 256
	httpProxyIdleConnTimeout     = 90 * time.Second
	httpProxyResponseTimeout     = 30 * time.Second
	httpProxyReadHeaderTimeout   = 10 * time.Second
	httpProxyServerIdleTimeout   = 90 * time.Second
)

// The MIT License (MIT)
//
// Copyright (c) 2016 Ian Denhardt <ian@zenhack.net>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

type httpTunnel struct {
	client net.Conn
	target net.Conn
}

type httpProxy struct {
	dialContext func(context.Context, string, string) (net.Conn, error)
	client      *http.Client
	ctx         context.Context
	cancel      context.CancelFunc

	tunnelsMu sync.Mutex
	tunnels   map[*httpTunnel]struct{}
	closed    bool
	slots     chan struct{}
	closeOnce sync.Once
}

func newHTTPProxy(dialer *dial.Dialer) *httpProxy {
	ctx, cancel := context.WithCancel(context.Background())
	proxy := &httpProxy{
		dialContext: dialer.Dial,
		ctx:         ctx,
		cancel:      cancel,
		tunnels:     make(map[*httpTunnel]struct{}),
		slots:       make(chan struct{}, httpProxyMaxTunnels),
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return proxy.dialContext(ctx, network, address)
	}
	transport.MaxIdleConns = httpProxyMaxIdleConns
	transport.MaxIdleConnsPerHost = httpProxyMaxIdleConnsPerHost
	transport.MaxConnsPerHost = httpProxyMaxConnsPerHost
	transport.IdleConnTimeout = httpProxyIdleConnTimeout
	transport.ResponseHeaderTimeout = httpProxyResponseTimeout
	proxy.client = &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return proxy
}

func newHTTPProxyHandler(dialer *dial.Dialer) http.Handler {
	return newHTTPProxy(dialer)
}

func (p *httpProxy) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method == http.MethodConnect {
		p.handleConnect(writer, request)
		return
	}
	requestContext, cancelRequest := p.requestContext(request.Context())
	defer cancelRequest()
	request = request.WithContext(requestContext)

	request.RequestURI = ""
	removeHopByHopHeaders(request.Header)
	response, err := p.client.Do(request)
	if err != nil {
		writeProxyError(writer, err)
		return
	}
	defer response.Body.Close()

	removeHopByHopHeaders(response.Header)
	for key, values := range response.Header {
		for _, value := range values {
			writer.Header().Add(key, value)
		}
	}
	writer.WriteHeader(response.StatusCode)
	_, _ = io.Copy(writer, response.Body)
}

func (p *httpProxy) handleConnect(writer http.ResponseWriter, request *http.Request) {
	if !p.acquireTunnel() {
		http.Error(writer, "too many active proxy tunnels", http.StatusServiceUnavailable)
		return
	}
	defer p.releaseTunnel()

	requestContext, cancelRequest := p.requestContext(request.Context())
	defer cancelRequest()
	targetConnection, err := p.dialContext(requestContext, "tcp", request.Host)
	if err != nil {
		writeProxyError(writer, err)
		return
	}

	hijacker, ok := writer.(http.Hijacker)
	if !ok {
		_ = targetConnection.Close()
		http.Error(writer, "HTTP connection hijacking is unavailable", http.StatusInternalServerError)
		return
	}
	clientConnection, buffered, err := hijacker.Hijack()
	if err != nil {
		_ = targetConnection.Close()
		return
	}
	tunnel := &httpTunnel{client: clientConnection, target: targetConnection}
	if !p.registerTunnel(tunnel) {
		_ = targetConnection.Close()
		_ = clientConnection.Close()
		return
	}
	defer p.unregisterTunnel(tunnel)
	defer clientConnection.Close()
	defer targetConnection.Close()

	if _, err := buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if err := buffered.Flush(); err != nil {
		return
	}

	relayDone := make(chan struct{}, 2)
	go relayHTTPConnect(targetConnection, buffered, relayDone)
	go relayHTTPConnect(clientConnection, targetConnection, relayDone)
	<-relayDone
	_ = clientConnection.Close()
	_ = targetConnection.Close()
	<-relayDone
}

func relayHTTPConnect(destination net.Conn, source io.Reader, done chan<- struct{}) {
	_, _ = io.Copy(destination, source)
	if connection, ok := destination.(interface{ CloseWrite() error }); ok {
		_ = connection.CloseWrite()
	}
	if connection, ok := source.(interface{ CloseRead() error }); ok {
		_ = connection.CloseRead()
	}
	done <- struct{}{}
}

func (p *httpProxy) requestContext(parent context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(p.ctx, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

func (p *httpProxy) acquireTunnel() bool {
	p.tunnelsMu.Lock()
	defer p.tunnelsMu.Unlock()
	if p.closed {
		return false
	}
	select {
	case p.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (p *httpProxy) releaseTunnel() {
	<-p.slots
}

func (p *httpProxy) registerTunnel(tunnel *httpTunnel) bool {
	p.tunnelsMu.Lock()
	defer p.tunnelsMu.Unlock()
	if p.closed {
		return false
	}
	p.tunnels[tunnel] = struct{}{}
	return true
}

func (p *httpProxy) unregisterTunnel(tunnel *httpTunnel) {
	p.tunnelsMu.Lock()
	delete(p.tunnels, tunnel)
	p.tunnelsMu.Unlock()
}

func (p *httpProxy) close() {
	p.closeOnce.Do(func() {
		p.tunnelsMu.Lock()
		p.closed = true
		tunnels := make([]*httpTunnel, 0, len(p.tunnels))
		for tunnel := range p.tunnels {
			tunnels = append(tunnels, tunnel)
		}
		p.tunnelsMu.Unlock()

		p.cancel()
		for _, tunnel := range tunnels {
			_ = tunnel.client.Close()
			_ = tunnel.target.Close()
		}
		p.client.CloseIdleConnections()
	})
}

func newHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: httpProxyReadHeaderTimeout,
		IdleTimeout:       httpProxyServerIdleTimeout,
	}
}

func StartHTTP(bindAddr string, dialer *dial.Dialer) (string, error) {
	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		return "", fmt.Errorf("start HTTP listener: %w", err)
	}
	actualAddr := listener.Addr().String()
	proxy := newHTTPProxy(dialer)
	server := newHTTPServer(proxy)
	log.Printf("HTTP server listening on %s", actualAddr)

	hook_func.RegisterTerminalFunc("CloseHTTPListener", func(ctx context.Context) error {
		log.Println("Closing HTTP listener...")
		shutdownContext, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		proxy.close()
		if err := server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("close HTTP listener failed: %w", err)
		}
		return nil
	})

	go func() {
		if err := server.Serve(listener); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				log.Println("HTTP server closed")
			} else {
				log.Println("HTTP listen failed: " + err.Error())
			}
		}
	}()
	return actualAddr, nil
}

func writeProxyError(writer http.ResponseWriter, err error) {
	if errors.Is(err, dial.ErrACLDenied) {
		http.Error(writer, "destination is not authorized by the aTrust gateway", http.StatusForbidden)
		return
	}
	http.Error(writer, "upstream connection failed", http.StatusBadGateway)
}

func removeHopByHopHeaders(header http.Header) {
	for _, connectionValue := range header.Values("Connection") {
		for _, name := range strings.Split(connectionValue, ",") {
			header.Del(strings.TrimSpace(name))
		}
	}
	for _, name := range []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Proxy-Connection",
		"Te",
		"Trailer",
		"Transfer-Encoding",
		"Upgrade",
	} {
		header.Del(name)
	}
}
