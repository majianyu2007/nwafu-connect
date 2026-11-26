package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	maxConnectStatusLineBytes = 4096
	maxConnectHeaderBytes     = 64 * 1024
)

func main() {
	proxyAddress := flag.String("proxy", "", "NWAFU Connect private HTTP proxy address")
	targetAddress := flag.String("target", "", "target host and port")
	flag.Parse()
	if *proxyAddress == "" || *targetAddress == "" {
		fmt.Fprintln(os.Stderr, "usage: nwafu-connect-proxy --proxy HOST:PORT --target HOST:PORT")
		os.Exit(2)
	}
	if err := proxyStream(*proxyAddress, *targetAddress, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func proxyStream(proxyAddress, targetAddress string, input io.Reader, output io.Writer) error {
	if _, _, err := net.SplitHostPort(proxyAddress); err != nil {
		return fmt.Errorf("invalid proxy address: %w", err)
	}
	if _, _, err := net.SplitHostPort(targetAddress); err != nil {
		return fmt.Errorf("invalid target address: %w", err)
	}
	connection, err := net.DialTimeout("tcp", proxyAddress, 10*time.Second)
	if err != nil {
		return fmt.Errorf("connect to NWAFU Connect: %w", err)
	}
	defer connection.Close()
	if err := connection.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return fmt.Errorf("set proxy handshake deadline: %w", err)
	}
	if _, err := fmt.Fprintf(connection, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", targetAddress, targetAddress); err != nil {
		return fmt.Errorf("send CONNECT request: %w", err)
	}
	reader := bufio.NewReader(connection)
	status, statusCode, err := readConnectResponse(reader)
	if err != nil {
		return fmt.Errorf("read CONNECT response: %w", err)
	}
	if statusCode != 200 {
		return fmt.Errorf("NWAFU Connect rejected target: %s", status)
	}
	if err := connection.SetDeadline(time.Time{}); err != nil {
		return fmt.Errorf("clear proxy handshake deadline: %w", err)
	}
	writeDone := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(connection, input)
		if tcpConnection, ok := connection.(*net.TCPConn); ok {
			_ = tcpConnection.CloseWrite()
		}
		writeDone <- copyErr
	}()
	if _, err := io.Copy(output, reader); err != nil {
		return fmt.Errorf("read tunneled connection: %w", err)
	}
	select {
	case writeErr := <-writeDone:
		if writeErr != nil {
			return fmt.Errorf("write tunneled connection: %w", writeErr)
		}
	default:
	}
	return nil
}

func readBoundedLine(reader *bufio.Reader, maxBytes int, tooLongErr error) (string, error) {
	var builder strings.Builder
	for {
		fragment, err := reader.ReadSlice('\n')
		if builder.Len()+len(fragment) > maxBytes {
			return "", tooLongErr
		}
		builder.Write(fragment)
		if err == nil {
			return builder.String(), nil
		}
		if !errors.Is(err, bufio.ErrBufferFull) {
			return "", err
		}
	}
}

func readConnectResponse(reader *bufio.Reader) (string, int, error) {
	statusLine, err := readBoundedLine(reader, maxConnectStatusLineBytes, errors.New("proxy status line is too long"))
	if err != nil {
		return "", 0, err
	}
	status := strings.TrimRight(statusLine, "\r\n")
	parts := strings.Fields(status)
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "HTTP/") {
		return "", 0, fmt.Errorf("invalid proxy status line %q", status)
	}
	statusCode, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", 0, fmt.Errorf("invalid proxy status code %q", parts[1])
	}
	headerBytes := 0
	for {
		remaining := maxConnectHeaderBytes - headerBytes
		line, err := readBoundedLine(reader, remaining, errors.New("proxy response headers are too large"))
		if err != nil {
			return "", 0, err
		}
		headerBytes += len(line)
		if strings.TrimRight(line, "\r\n") == "" {
			return status, statusCode, nil
		}
	}
}
