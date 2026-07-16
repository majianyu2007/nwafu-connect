package resolve

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/miekg/dns"
)

const isolatedDoHURL = "https://dns.alidns.com/dns-query"

var isolatedDoHEndpoints = [...]string{"223.5.5.5:443", "223.6.6.6:443"}

type dohResolver struct {
	client *http.Client
}

func newDoHResolver() *dohResolver {
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var lastErr error
			for _, endpoint := range isolatedDoHEndpoints {
				connection, err := dialer.DialContext(ctx, network, endpoint)
				if err == nil {
					return connection, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12, ServerName: "dns.alidns.com"},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
	}
	return &dohResolver{client: &http.Client{Transport: transport, Timeout: 8 * time.Second}}
}

func (r *dohResolver) LookupIPv4(ctx context.Context, host string) ([]net.IP, error) {
	query := new(dns.Msg)
	query.SetQuestion(dns.Fqdn(host), dns.TypeA)
	payload, err := query.Pack()
	if err != nil {
		return nil, fmt.Errorf("pack DoH query: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, isolatedDoHURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create DoH request: %w", err)
	}
	request.Header.Set("Accept", "application/dns-message")
	request.Header.Set("Content-Type", "application/dns-message")
	response, err := r.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("query isolated DoH: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("query isolated DoH: HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("read DoH response: %w", err)
	}
	return parseDoHIPv4(body)
}

func parseDoHIPv4(payload []byte) ([]net.IP, error) {
	response := new(dns.Msg)
	if err := response.Unpack(payload); err != nil {
		return nil, fmt.Errorf("unpack DoH response: %w", err)
	}
	if response.Rcode != dns.RcodeSuccess {
		return nil, fmt.Errorf("DoH response code: %s", dns.RcodeToString[response.Rcode])
	}
	addresses := make([]net.IP, 0, len(response.Answer))
	for _, answer := range response.Answer {
		if record, ok := answer.(*dns.A); ok {
			addresses = append(addresses, record.A)
		}
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("DoH returned no IPv4 address")
	}
	return addresses, nil
}
