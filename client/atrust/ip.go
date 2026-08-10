package atrust

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/majianyu2007/nwafu-connect/log"
)

const tunnelDialTimeout = 10 * time.Second

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		written, err := writer.Write(data)
		if written < 0 || written > len(data) {
			return io.ErrShortWrite
		}
		data = data[written:]
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func dialTunnelTLS(address string) (*tls.Conn, error) {
	return dialTunnelTLSContext(context.Background(), address)
}

func dialTunnelTLSContext(ctx context.Context, address string) (*tls.Conn, error) {
	handshakeContext, cancel := context.WithTimeout(ctx, tunnelDialTimeout)
	defer cancel()

	rawConnection, err := (&net.Dialer{}).DialContext(handshakeContext, "tcp", address)
	if err != nil {
		return nil, err
	}
	// aTrust tunnel nodes commonly use gateway-private certificates whose
	// hostnames do not match their server-issued IP addresses.
	connection := tls.Client(rawConnection, &tls.Config{InsecureSkipVerify: true})
	if err := connection.HandshakeContext(handshakeContext); err != nil {
		_ = rawConnection.Close()
		return nil, err
	}
	return connection, nil
}

func (c *Client) getIP() error {
	addr := c.BestNodes[c.MajorNodeGroup]
	if addr == "" {
		for _, node := range c.BestNodes {
			addr = node
			break
		}
	}
	if addr == "" {
		return fmt.Errorf("no reachable node for ip request")
	}

	conn, err := dialTunnelTLS(addr)
	if err != nil {
		return fmt.Errorf("connect to node %s for IP request: %w", addr, err)
	}
	defer func(conn *tls.Conn) {
		_ = conn.Close()
	}(conn)
	if err := conn.SetDeadline(time.Now().Add(tunnelDialTimeout)); err != nil {
		return fmt.Errorf("set IP request deadline: %w", err)
	}

	msg := []byte{0x05, 0x01, 0xd0, 0x53, 0x00, 0x00, 0x53}
	msg = append(msg, []byte(fmt.Sprintf(`{"sid":"%s"}`, c.SID))...)
	if err := writeAll(conn, msg); err != nil {
		return fmt.Errorf("send IP authentication request: %w", err)
	}

	msg = []byte{0x05, 0x04, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if err := writeAll(conn, msg); err != nil {
		return fmt.Errorf("send IP allocation request: %w", err)
	}

	for {
		err = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
		if err != nil {
			return err
		}

		header := make([]byte, 2)
		_, err = io.ReadFull(conn, header)
		if err != nil {
			return err
		}
		if header[0] == 0x53 && header[1] == 0x00 {
			lengthBytes := make([]byte, 2)
			_, err = io.ReadFull(conn, lengthBytes)
			if err != nil {
				return err
			}
			length := binary.BigEndian.Uint16(lengthBytes)
			data := make([]byte, length)
			_, err = io.ReadFull(conn, data)
			if err != nil {
				return err
			}

			if !strings.Contains(string(data), "OK") {
				return fmt.Errorf("failed to connect to the server: %s", string(data))
			}
		} else if header[0] == 0x05 && header[1] == 0x00 {
			data := make([]byte, 6)
			_, err = io.ReadFull(conn, data)
			if err != nil {
				return err
			}
			if data[0] != 0x00 || data[1] != 0x01 {
				return fmt.Errorf("unexpected response: %x", data)
			}

			c.ip = net.IPv4(data[2], data[3], data[4], data[5])
			log.Printf("Received IP: %s", c.ip.String())
			return nil
		}
	}
}
