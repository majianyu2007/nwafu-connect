package atrust

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/log"
	"github.com/majianyu2007/nwafu-connect/resolve"
)

type tcpTunnelConn struct {
	tlsConn   *tls.Conn
	reader    *bufio.Reader
	readBuf   []byte
	readMu    sync.Mutex
	writeMu   sync.Mutex
	closeOnce sync.Once
	closeErr  error
}

type tcpTunnelAuthRequest struct {
	SID           string    `json:"sid"`
	AppID         string    `json:"appId"`
	URL           string    `json:"url"`
	DeviceID      string    `json:"deviceId"`
	ConnectionID  string    `json:"connectionId"`
	ProcHash      string    `json:"procHash"`
	Username      string    `json:"userName"`
	RCAppliedInfo int       `json:"rcAppliedInfo"`
	Lang          string    `json:"lang"`
	DestAddr      string    `json:"destAddr"`
	Env           *trustEnv `json:"env"`
	XRequestSig   string    `json:"xRequestSig"`
}

func readTCPProtocolResponse(reader *bufio.Reader) (string, error) {
	var lengthBytes [2]byte
	if _, err := io.ReadFull(reader, lengthBytes[:]); err != nil {
		return "", err
	}
	data := make([]byte, int(binary.BigEndian.Uint16(lengthBytes[:])))
	if _, err := io.ReadFull(reader, data); err != nil {
		return "", err
	}
	return string(data), nil
}

func waitForTCPConnect(ctx context.Context, connection net.Conn, reader *bufio.Reader) (returnErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	cancelDone := make(chan struct{})
	stopCancellation := context.AfterFunc(ctx, func() {
		defer close(cancelDone)
		_ = connection.Close()
	})
	defer func() {
		if !stopCancellation() {
			<-cancelDone
		}
		if err := ctx.Err(); err != nil {
			returnErr = err
		}
	}()

	for {
		var header [2]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return fmt.Errorf("read TCP tunnel response: %w", err)
		}
		log.DebugPrintf("Received TCP tunnel setup header: %02X %02X", header[0], header[1])
		if header == [2]byte{0x05, 0x81} {
			continue
		}
		if header != [2]byte{0x53, 0x00} {
			return fmt.Errorf("unexpected TCP tunnel setup response: %02X %02X", header[0], header[1])
		}
		response, err := readTCPProtocolResponse(reader)
		if err != nil {
			return fmt.Errorf("read TCP tunnel protocol response: %w", err)
		}
		log.DebugPrintf("Received TCP tunnel protocol response: %s", response)
		if !strings.Contains(response, "OK") {
			return fmt.Errorf("TCP tunnel setup failed: %s", strings.TrimSpace(response))
		}
		break
	}

	probe := []byte{0x01, 0x00, 0x00, 0x00}
	if err := writeAll(connection, probe); err != nil {
		return fmt.Errorf("send TCP tunnel connect probe: %w", err)
	}
	var status [2]byte
	if _, err := io.ReadFull(reader, status[:]); err != nil {
		return fmt.Errorf("read TCP tunnel connect status: %w", err)
	}
	if status[0] != 0x05 {
		return fmt.Errorf("unexpected TCP tunnel connect status: %02X %02X", status[0], status[1])
	}
	switch status[1] {
	case 0x00:
		return nil
	case 0x01:
		return errors.New("TCP tunnel server failure")
	case 0x02:
		return errors.New("TCP tunnel connection not allowed")
	case 0x03:
		return errors.New("TCP tunnel network is unreachable")
	case 0x04:
		return errors.New("TCP tunnel host is unreachable")
	case 0x05:
		return errors.New("TCP tunnel connection refused")
	case 0x06:
		return errors.New("TCP tunnel TTL expired")
	case 0x07:
		return errors.New("TCP tunnel command not supported")
	case 0x08:
		return errors.New("TCP tunnel address type not supported")
	default:
		return fmt.Errorf("TCP tunnel connect failed with status 0x%02X", status[1])
	}
}

func (c *tcpTunnelConn) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	c.readMu.Lock()
	defer c.readMu.Unlock()
	if len(c.readBuf) > 0 {
		read := copy(buffer, c.readBuf)
		c.readBuf = c.readBuf[read:]
		return read, nil
	}

	for {
		var header [2]byte
		if _, err := io.ReadFull(c.reader, header[:]); err != nil {
			return 0, err
		}
		log.DebugPrintf("Received TCP tunnel header: %02X %02X", header[0], header[1])
		switch header {
		case [2]byte{0x01, 0x00}:
			var lengthBytes [2]byte
			if _, err := io.ReadFull(c.reader, lengthBytes[:]); err != nil {
				return 0, err
			}
			length := int(binary.BigEndian.Uint16(lengthBytes[:]))
			if length == 0 {
				continue
			}
			if length <= len(buffer) {
				if _, err := io.ReadFull(c.reader, buffer[:length]); err != nil {
					return 0, err
				}
				log.DebugPrintf("Received TCP tunnel application data, length: %d", length)
				log.DebugDumpHex(buffer[:length])
				return length, nil
			}
			data := make([]byte, length)
			if _, err := io.ReadFull(c.reader, data); err != nil {
				return 0, err
			}
			log.DebugPrintf("Received TCP tunnel application data, length: %d", length)
			log.DebugDumpHex(data)
			read := copy(buffer, data)
			c.readBuf = data[read:]
			return read, nil
		case [2]byte{0x01, 0x01}:
			var closeStatus [2]byte
			if _, err := io.ReadFull(c.reader, closeStatus[:]); err != nil {
				return 0, err
			}
			if closeStatus != [2]byte{0x30, 0x30} {
				return 0, fmt.Errorf("unexpected TCP tunnel close status: %02X %02X", closeStatus[0], closeStatus[1])
			}
			log.DebugPrint("Received TCP tunnel close message")
			_ = c.tlsConn.Close()
			return 0, io.EOF
		case [2]byte{0x53, 0x00}:
			response, err := readTCPProtocolResponse(c.reader)
			if err != nil {
				return 0, fmt.Errorf("read TCP tunnel protocol response: %w", err)
			}
			log.DebugPrintf("Received TCP tunnel protocol response: %s", response)
			if !strings.Contains(response, "OK") {
				message := strings.TrimSpace(response)
				if message == "" {
					message = "empty response"
				}
				_ = c.tlsConn.Close()
				return 0, fmt.Errorf("TCP tunnel connection rejected: %s", message)
			}
		default:
			return 0, fmt.Errorf("unexpected TCP tunnel response header: %02X %02X", header[0], header[1])
		}
	}
}

func (c *tcpTunnelConn) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	if len(b) > 0xFFFF {
		return 0, fmt.Errorf("data too large")
	}
	frame := make([]byte, 4+len(b))
	frame[0], frame[1] = 0x01, 0x00
	binary.BigEndian.PutUint16(frame[2:4], uint16(len(b)))
	copy(frame[4:], b)

	c.writeMu.Lock()
	err := writeAll(c.tlsConn, frame)
	c.writeMu.Unlock()
	log.DebugDumpHex(frame)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

func (c *tcpTunnelConn) Close() error {
	c.closeOnce.Do(func() {
		_ = c.tlsConn.SetWriteDeadline(time.Now().Add(time.Second))
		closeMsg := []byte{0x01, 0x01, 0x00, 0x00}
		_, _ = c.tlsConn.Write(closeMsg)
		log.DebugPrint("Sent close message")
		log.DebugDumpHex(closeMsg)
		c.closeErr = c.tlsConn.Close()
	})
	return c.closeErr
}

func (c *tcpTunnelConn) LocalAddr() net.Addr {
	return c.tlsConn.LocalAddr()
}

func (c *tcpTunnelConn) RemoteAddr() net.Addr {
	return c.tlsConn.RemoteAddr()
}

func (c *tcpTunnelConn) SetDeadline(t time.Time) error {
	return c.tlsConn.SetDeadline(t)
}

func (c *tcpTunnelConn) SetReadDeadline(t time.Time) error {
	return c.tlsConn.SetReadDeadline(t)
}

func (c *tcpTunnelConn) SetWriteDeadline(t time.Time) error {
	return c.tlsConn.SetWriteDeadline(t)
}

func calcXRequestSig(key []byte, data []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	sum := h.Sum(nil)
	return strings.ToUpper(hex.EncodeToString(sum))
}

func buildTCPTunnelAuthRequest(c *Client, appID, destAddr, procName, procPath string) ([]byte, error) {
	signKey, err := hex.DecodeString(c.SignKey)
	if err != nil {
		return nil, fmt.Errorf("invalid sign key: %w", err)
	}
	procHash := fmt.Sprintf("%X", sha256.Sum256([]byte(procPath)))
	var env trustEnv
	env.Application.Runtime.Process.Name = procName
	env.Application.Runtime.Process.DigitalSignature = "TrustAppClosed"
	env.Application.Runtime.Process.Platform = "Linux"
	env.Application.Runtime.Process.Fingerprint = procHash
	env.Application.Runtime.Process.Description = "TrustAppClosed"
	env.Application.Runtime.Process.Path = procPath
	env.Application.Runtime.Process.Version = "TrustAppClosed"
	env.Application.Runtime.Process.SecurityEnv = "normal"
	env.Application.Runtime.ProcessTrusted = "TRUSTED"
	request := tcpTunnelAuthRequest{
		SID:           c.SID,
		AppID:         appID,
		URL:           "tcp://" + destAddr,
		DeviceID:      c.DeviceID,
		ConnectionID:  c.ConnectionID,
		ProcHash:      procHash,
		Username:      c.Username,
		RCAppliedInfo: 0,
		Lang:          "en-US",
		DestAddr:      destAddr,
		Env:           &env,
	}
	unsigned, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode unsigned TCP tunnel request: %w", err)
	}
	request.XRequestSig = calcXRequestSig(signKey, unsigned)
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("encode TCP tunnel request: %w", err)
	}
	return payload, nil
}

func (c *Client) DialTCP(ctx context.Context, addr *net.TCPAddr) (net.Conn, error) {
	appID := ""
	nodeGroupID := ""
	domain := ""
	if res := ctx.Value(resolve.ContextKeyDomainResource); res != nil {
		resources, ok := res.(client.DomainResourceSet)
		if !ok {
			return nil, errors.New("invalid domain resource routing context")
		}
		resource, ok := resources.Match(addr.Port, "tcp")
		if !ok {
			return nil, fmt.Errorf("%s: %w", addr, client.ErrResourceNotFound)
		}
		appID = resource.AppID
		nodeGroupID = resource.NodeGroupID
		if resolvedHost, ok := ctx.Value(resolve.ContextKeyResolveHost).(string); ok {
			domain = resolvedHost
		}
	} else if resource, ok := c.resourceIndex.Match(addr.IP, "tcp", addr.Port); ok {
		appID = resource.AppID
		nodeGroupID = resource.NodeGroupID
	}
	if appID == "" {
		return nil, fmt.Errorf("%s: %w", addr, client.ErrResourceNotFound)
	}

	c.BestNodesRWMutex.RLock()
	nodeAddr := c.BestNodes[nodeGroupID]
	if nodeAddr == "" {
		nodeAddr = c.BestNodes[c.MajorNodeGroup]
	}
	c.BestNodesRWMutex.RUnlock()
	if nodeAddr == "" {
		return nil, fmt.Errorf("no available aTrust node for group %q", nodeGroupID)
	}
	conn, err := dialTunnelTLSContext(ctx, nodeAddr)
	if err != nil {
		return nil, fmt.Errorf("connect to TCP tunnel node %s: %w", nodeAddr, err)
	}
	procName := "google-chrome-stable"
	procPath := "/usr/bin/google-chrome-stable"
	if addr.Port == 22 {
		procName = "ssh"
		procPath = "/usr/bin/ssh"
	}
	destAddr := addr.String()
	if domain != "" {
		destAddr = net.JoinHostPort(domain, fmt.Sprint(addr.Port))
	}

	destIP := addr.IP.To4()
	if destIP == nil {
		_ = conn.Close()
		return nil, fmt.Errorf("invalid IPv4 address")
	}
	destPort := make([]byte, 2)
	binary.BigEndian.PutUint16(destPort, uint16(addr.Port))

	msgBytes, err := buildTCPTunnelAuthRequest(c, appID, destAddr, procName, procPath)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if len(msgBytes) > 0xFFFF {
		_ = conn.Close()
		return nil, fmt.Errorf("TCP tunnel authentication request is too large: %d bytes", len(msgBytes))
	}
	lenBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBytes, uint16(len(msgBytes)))
	initHeader := []byte{0x05, 0x01, 0x81, 0x53, 0x03}
	initMsg := append(initHeader, lenBytes...)
	initMsg = append(initMsg, msgBytes...)
	if err := conn.SetDeadline(time.Now().Add(tunnelDialTimeout)); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("set TCP tunnel initialization deadline: %w", err)
	}
	if err := writeAll(conn, initMsg); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to send init message: %w", err)
	}
	log.DebugDumpHex(initMsg)

	var destMsg []byte
	if domain == "" {
		destHeader := []byte{0x05, 0x01, 0x01, 0x01}
		destMsg = append(destHeader, destIP...)
	} else {
		destHeader := []byte{0x05, 0x01, 0x01, 0x03}
		// For domain, we need to send the length of the domain name
		domainLen := len(domain)
		if domainLen > 255 {
			_ = conn.Close()
			return nil, fmt.Errorf("domain name too long: %s", domain)
		}
		destHeader = append(destHeader, byte(domainLen))
		destMsg = append(destHeader, []byte(domain)...)
	}
	destMsg = append(destMsg, destPort...)
	if err := writeAll(conn, destMsg); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to send dest address: %w", err)
	}
	log.DebugDumpHex(destMsg)
	tunnelConnection := &tcpTunnelConn{
		tlsConn: conn,
		reader:  bufio.NewReader(conn),
	}
	if err := waitForTCPConnect(ctx, conn, tunnelConnection.reader); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("clear TCP tunnel initialization deadline: %w", err)
	}
	return tunnelConnection, nil
}
