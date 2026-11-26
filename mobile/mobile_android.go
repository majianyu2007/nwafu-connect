//go:build android

// Package mobile provides gomobile-compatible Android VpnService bindings for
// NWAFU Connect's aTrust protocol and TUN stack.
package mobile

import (
	"fmt"
	"strings"
	"sync"
	"time"

	atrustclient "github.com/majianyu2007/nwafu-connect/client/atrust"
	"github.com/majianyu2007/nwafu-connect/client/atrust/auth"
	"github.com/majianyu2007/nwafu-connect/log"
	"github.com/majianyu2007/nwafu-connect/stack/tun"
	"github.com/majianyu2007/nwafu-connect/underlay"
)

const (
	defaultServer      = "vpn.nwafu.edu.cn"
	defaultPort        = 443
	defaultLoginDomain = "LDAP"
)

var (
	vpnClient     *atrustclient.Client
	vpnUnderlay   *underlay.Dialer
	vpnClientData []byte
	lastErr       string
	vpnClientMu   sync.Mutex
	loginMu       sync.Mutex
)

// Login authenticates against an aTrust gateway using username/password (and optional TOTP secret).
// Empty server/port/loginDomain fall back to NWAFU defaults. Returns the assigned virtual IPv4
// address on success, or an empty string on failure (call LastError for details).
func Login(server string, port int, username, password, totpSecret, loginDomain string) string {
	log.Init()
	return login(server, port, username, password, totpSecret, loginDomain, nil)
}

// DebugLogin is like Login with debug logging enabled.
func DebugLogin(server string, port int, username, password, totpSecret, loginDomain string) string {
	log.Init()
	log.EnableDebug()
	return login(server, port, username, password, totpSecret, loginDomain, nil)
}

// LoginWithClientData authenticates while reusing previously saved aTrust client data bytes.
func LoginWithClientData(server string, port int, username, password, totpSecret, loginDomain string, clientData []byte) string {
	log.Init()
	return login(server, port, username, password, totpSecret, loginDomain, clientData)
}

// LastError returns the most recent error message from Login or StartStack.
func LastError() string {
	vpnClientMu.Lock()
	defer vpnClientMu.Unlock()
	return lastErr
}

// ClientData returns the serialized aTrust client state produced by the last successful login.
func ClientData() []byte {
	vpnClientMu.Lock()
	defer vpnClientMu.Unlock()
	return append([]byte(nil), vpnClientData...)
}

// Routes returns comma-separated CIDR prefixes from the active session's IPSet
// so Android VpnService.Builder can install split routes.
func Routes() string {
	vpnClientMu.Lock()
	c := vpnClient
	vpnClientMu.Unlock()
	if c == nil {
		return ""
	}
	ipSet, err := c.IPSet()
	if err != nil || ipSet == nil {
		return ""
	}
	prefixes := ipSet.Prefixes()
	out := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		out = append(out, prefix.String())
	}
	return strings.Join(out, ",")
}

// DNSServers returns comma-separated DNS server IPs provided by the gateway.
func DNSServers() string {
	vpnClientMu.Lock()
	c := vpnClient
	vpnClientMu.Unlock()
	if c == nil {
		return ""
	}
	servers, err := c.DNSServers()
	if err != nil || len(servers) == 0 {
		return ""
	}
	return strings.Join(servers, ",")
}

// Logout closes the active aTrust client and underlay dialer.
func Logout() {
	vpnClientMu.Lock()
	c := vpnClient
	u := vpnUnderlay
	vpnClient = nil
	vpnUnderlay = nil
	vpnClientData = nil
	vpnClientMu.Unlock()

	if c != nil {
		c.Close()
	}
	if u != nil {
		_ = u.Close()
	}
}

func setLastError(err error) {
	vpnClientMu.Lock()
	if err == nil {
		lastErr = ""
	} else {
		lastErr = err.Error()
	}
	vpnClientMu.Unlock()
}

func login(server string, port int, username, password, totpSecret, loginDomain string, savedClientData []byte) string {
	loginMu.Lock()
	defer loginMu.Unlock()

	server = strings.TrimSpace(server)
	if server == "" {
		server = defaultServer
	}
	if port <= 0 || port > 65535 {
		port = defaultPort
	}
	loginDomain = strings.TrimSpace(loginDomain)
	if loginDomain == "" {
		loginDomain = defaultLoginDomain
	}

	newUnderlay, err := underlay.New(underlay.Options{AutoDetect: false})
	if err != nil {
		setLastError(fmt.Errorf("create underlay dialer: %w", err))
		return ""
	}

	loginMethod, err := auth.NewLoginMethod(auth.LoginMethodOptions{
		AuthType: "auth/psw",
		Username: username,
		Password: password,
		Domain:   loginDomain,
	})
	if err != nil {
		_ = newUnderlay.Close()
		setLastError(err)
		return ""
	}

	newClient := atrustclient.NewClient(atrustclient.ClientOptions{
		Session: atrustclient.SessionOptions{
			Username: username,
		},
		UnderlayDialer: newUnderlay,
	})

	// Detach any existing session before bringing up the new one so concurrent
	// StartStack calls never observe a half-initialized client.
	vpnClientMu.Lock()
	oldClient := vpnClient
	oldUnderlay := vpnUnderlay
	vpnClient = nil
	vpnUnderlay = nil
	vpnClientData = nil
	lastErr = ""
	vpnClientMu.Unlock()

	if oldClient != nil {
		oldClient.Close()
	}
	if oldUnderlay != nil {
		_ = oldUnderlay.Close()
	}

	newData, err := newClient.Setup(atrustclient.SetupOptions{
		ServerAddress:            server,
		ServerPort:               port,
		LoginMethod:              loginMethod,
		TOTPSecret:               totpSecret,
		ClientData:               savedClientData,
		BestNodesRefreshInterval: 300 * time.Second,
		SessionRefreshInterval:   1800 * time.Second,
		OnSessionInvalid: func(invalidErr error) {
			setLastError(invalidErr)
		},
	})
	if err != nil {
		newClient.Close()
		_ = newUnderlay.Close()
		setLastError(err)
		return ""
	}

	clientIP, err := newClient.IP()
	if err != nil || clientIP == nil {
		newClient.Close()
		_ = newUnderlay.Close()
		if err == nil {
			err = fmt.Errorf("aTrust gateway did not assign a virtual IP")
		}
		setLastError(err)
		return ""
	}

	vpnClientMu.Lock()
	vpnClient = newClient
	vpnUnderlay = newUnderlay
	vpnClientData = newData
	vpnClientMu.Unlock()

	log.Printf("aTrust mobile client started with IP %s", clientIP.String())
	return clientIP.String()
}

// StartStack attaches the Android VpnService TUN file descriptor to the active aTrust L3 tunnel.
func StartStack(fd int) {
	vpnClientMu.Lock()
	c := vpnClient
	vpnClientMu.Unlock()
	if c == nil {
		setLastError(fmt.Errorf("aTrust client is not logged in"))
		return
	}

	vpnTUNStack, err := tun.NewStack(c, false, false, nil)
	if err != nil {
		setLastError(err)
		return
	}

	vpnTUNStack.SetupTun(fd)
	if err := vpnTUNStack.Run(); err != nil {
		setLastError(err)
		log.Printf("Android TUN stack stopped: %v", err)
	}
}
