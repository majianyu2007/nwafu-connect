package dial

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/majianyu2007/nwafu-connect/log"
	"github.com/things-go/go-socks5/statute"
)

const proxyHandshakeTimeout = 10 * time.Second

func beginProxyHandshake(ctx context.Context, connection net.Conn) (func() bool, error) {
	deadline := time.Now().Add(proxyHandshakeTimeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := connection.SetDeadline(deadline); err != nil {
		return nil, err
	}
	return context.AfterFunc(ctx, func() {
		_ = connection.SetDeadline(time.Now())
	}), nil
}

func writeProxyBytes(writer io.Writer, payload []byte) error {
	for len(payload) > 0 {
		written, err := writer.Write(payload)
		if written < 0 || written > len(payload) {
			return io.ErrShortWrite
		}
		payload = payload[written:]
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func (d *Dialer) dialDirectWithoutProxy(ctx context.Context, network, addr string) (net.Conn, error) {
	goDialer := &net.Dialer{}
	goDial := goDialer.DialContext
	log.Printf("%s -> DIRECT", addr)
	return goDial(ctx, network, addr)
}

// usedAddr maybe ip:port or hostname:port, it doesn't matter
func (d *Dialer) dialDirectWithHTTPProxy(ctx context.Context, usedAddr string) (net.Conn, error) {
	if _, _, err := net.SplitHostPort(usedAddr); err != nil {
		return nil, fmt.Errorf("invalid HTTP proxy destination %q: %w", usedAddr, err)
	}
	log.Printf("%s -> PROXY[%s]", usedAddr, d.dialDirectHTTPProxy)
	connection, err := (&net.Dialer{Timeout: proxyHandshakeTimeout}).DialContext(ctx, "tcp", d.dialDirectHTTPProxy)
	if err != nil {
		return nil, fmt.Errorf("connect to HTTP proxy: %w", err)
	}
	keepConnection := false
	defer func() {
		if !keepConnection {
			_ = connection.Close()
		}
	}()

	stopCancellation, err := beginProxyHandshake(ctx, connection)
	if err != nil {
		return nil, fmt.Errorf("set HTTP proxy handshake deadline: %w", err)
	}
	defer stopCancellation()

	request := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: usedAddr},
		Host:   usedAddr,
		Header: make(http.Header),
	}
	if err := request.Write(connection); err != nil {
		return nil, fmt.Errorf("write HTTP proxy CONNECT request: %w", err)
	}
	bufferedConn, err := readHTTPProxyConnectResponse(connection)
	if err != nil {
		return nil, err
	}

	if !stopCancellation() {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("clear HTTP proxy deadline: %w", err)
	}

	keepConnection = true
	return bufferedConn, nil
}

type bufferedProxyConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedProxyConn) Read(buffer []byte) (int, error) {
	return c.reader.Read(buffer)
}

func (d *Dialer) dialDirectWithSocksProxy(ctx context.Context, network, usedAddr string, isIP bool) (net.Conn, error) {
	log.Printf("%s -> PROXY[%s]", usedAddr, d.dialDirectSocksProxy)
	connection, err := (&net.Dialer{Timeout: proxyHandshakeTimeout}).DialContext(ctx, "tcp", d.dialDirectSocksProxy)
	if err != nil {
		return nil, fmt.Errorf("connect to SOCKS5 proxy: %w", err)
	}
	keepConnection := false
	defer func() {
		if !keepConnection {
			_ = connection.Close()
		}
	}()

	stopCancellation, err := beginProxyHandshake(ctx, connection)
	if err != nil {
		return nil, fmt.Errorf("set SOCKS5 proxy handshake deadline: %w", err)
	}
	defer stopCancellation()

	methodRequest := statute.NewMethodRequest(statute.VersionSocks5, []byte{statute.MethodNoAuth}).Bytes()
	if err := writeProxyBytes(connection, methodRequest); err != nil {
		return nil, fmt.Errorf("write SOCKS5 method request: %w", err)
	}
	methodReply, err := statute.ParseMethodReply(connection)
	if err != nil {
		return nil, fmt.Errorf("read SOCKS5 method response: %w", err)
	}
	if methodReply.Method != statute.MethodNoAuth || methodReply.Ver != statute.VersionSocks5 {
		return nil, errors.New("SOCKS5 proxy rejected no-authentication method")
	}

	host, portText, err := net.SplitHostPort(usedAddr)
	if err != nil {
		return nil, fmt.Errorf("invalid SOCKS5 destination %q: %w", usedAddr, err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid SOCKS5 destination port in %q", usedAddr)
	}
	destination := statute.AddrSpec{Port: port}
	if isIP {
		destination.IP = net.ParseIP(host)
		if destination.IP == nil {
			return nil, fmt.Errorf("invalid IP address for SOCKS5 proxy: %s", host)
		}
		if destination.IP.To4() == nil {
			destination.AddrType = statute.ATYPIPv6
		} else {
			destination.AddrType = statute.ATYPIPv4
		}
	} else {
		if host == "" || len(host) > 255 {
			return nil, fmt.Errorf("invalid domain for SOCKS5 proxy: %q", host)
		}
		destination.AddrType = statute.ATYPDomain
		destination.FQDN = host
	}
	if network != "tcp" {
		return nil, fmt.Errorf("SOCKS5 proxy does not support network %q", network)
	}
	request := statute.Request{
		Version:  statute.VersionSocks5,
		Command:  statute.CommandConnect,
		Reserved: 0,
		DstAddr:  destination,
	}
	if err := writeProxyBytes(connection, request.Bytes()); err != nil {
		return nil, fmt.Errorf("write SOCKS5 connect request: %w", err)
	}
	reply, err := statute.ParseReply(connection)
	if err != nil {
		return nil, fmt.Errorf("read SOCKS5 connect response: %w", err)
	}
	if reply.Version != statute.VersionSocks5 || reply.Response != statute.RepSuccess {
		return nil, fmt.Errorf("SOCKS5 proxy rejected connection with status %d", reply.Response)
	}
	if !stopCancellation() {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("clear SOCKS5 proxy deadline: %w", err)
	}
	keepConnection = true
	return connection, nil
}

const maxHTTPProxyResponseHeader = 64 << 10

func readHTTPProxyConnectResponse(conn net.Conn) (net.Conn, error) {
	reader := bufio.NewReader(conn)
	var header bytes.Buffer
	for {
		fragment, err := reader.ReadSlice('\n')
		if header.Len()+len(fragment) > maxHTTPProxyResponseHeader {
			return nil, fmt.Errorf("HTTP proxy response header exceeds %d bytes", maxHTTPProxyResponseHeader)
		}
		_, _ = header.Write(fragment)
		if bytes.HasSuffix(header.Bytes(), []byte("\r\n\r\n")) {
			break
		}
		if err != nil && !errors.Is(err, bufio.ErrBufferFull) {
			return nil, fmt.Errorf("read HTTP proxy response: %w", err)
		}
	}

	request := &http.Request{Method: http.MethodConnect}
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(header.Bytes())), request)
	if err != nil {
		return nil, fmt.Errorf("parse HTTP proxy response: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP proxy CONNECT failed: %s", response.Status)
	}

	return &bufferedProxyConn{Conn: conn, reader: reader}, nil
}
