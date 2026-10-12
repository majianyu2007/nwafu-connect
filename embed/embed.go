// Package embed runs an NWAFU Connect session inside a host process.
//
// It wires the same path the CLI uses by default (no TUN, no TCP tunnel mode,
// no root): aTrust login -> gVisor userspace stack -> resolver -> dialer ->
// local HTTP proxy. The host then sends campus-bound HTTP(S) traffic through
// the returned proxy address.
//
// Interactive login steps (graph captcha, SMS and TOTP codes) are delegated to
// a Prompter instead of the terminal or a local browser page.
package embed

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	atrustclient "github.com/majianyu2007/nwafu-connect/client/atrust"
	"github.com/majianyu2007/nwafu-connect/client/atrust/auth"
	"github.com/majianyu2007/nwafu-connect/dial"
	"github.com/majianyu2007/nwafu-connect/log"
	"github.com/majianyu2007/nwafu-connect/resolve"
	"github.com/majianyu2007/nwafu-connect/service"
	"github.com/majianyu2007/nwafu-connect/stack/gvisor"
)

const (
	DefaultServer             = "vpn.nwafu.edu.cn"
	DefaultPort               = 443
	DefaultHTTPBind           = "127.0.0.1:0"
	DefaultLoginDomain        = "LDAP"
	DefaultRemoteDNSServer    = "auto"
	DefaultSecondaryDNSServer = "114.114.114.114"
	DefaultDNSTTL             = 3600
	DefaultBestNodesInterval  = 300

	// fallbackDNSServer matches the CLI when the gateway provides none.
	fallbackDNSServer = "10.10.0.21"
)

// Config describes one session. Zero values take the CLI defaults.
type Config struct {
	Server     string `json:"server"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	TOTPSecret string `json:"totpSecret"`
	// LoginDomain is the aTrust login domain for auth/psw (default "LDAP").
	LoginDomain string `json:"loginDomain"`
	// ClientDataPath, if set, stores the device ID and gateway cookies
	// (mode 0600) so later sessions can reuse them and may skip the captcha.
	ClientDataPath string `json:"clientDataPath"`
	// HTTPBind is the local proxy listen address (default 127.0.0.1:0).
	HTTPBind string `json:"httpBind"`

	RemoteDNSServer         string `json:"remoteDnsServer"`
	SecondaryDNSServer      string `json:"secondaryDnsServer"`
	DNSTTL                  uint64 `json:"dnsTtl"`
	DisableRemoteDNS        bool   `json:"disableRemoteDns"`
	DisableKeepAlive        bool   `json:"disableKeepAlive"`
	KeepAliveURL            string `json:"keepAliveUrl"`
	UpdateBestNodesInterval int    `json:"updateBestNodesInterval"`
}

func (c Config) withDefaults() Config {
	if c.Server == "" {
		c.Server = DefaultServer
	}
	if c.Port == 0 {
		c.Port = DefaultPort
	}
	if c.LoginDomain == "" {
		c.LoginDomain = DefaultLoginDomain
	}
	if c.HTTPBind == "" {
		c.HTTPBind = DefaultHTTPBind
	}
	if c.RemoteDNSServer == "" {
		c.RemoteDNSServer = DefaultRemoteDNSServer
	}
	if c.SecondaryDNSServer == "" {
		c.SecondaryDNSServer = DefaultSecondaryDNSServer
	}
	if c.DNSTTL == 0 {
		c.DNSTTL = DefaultDNSTTL
	}
	if c.UpdateBestNodesInterval == 0 {
		c.UpdateBestNodesInterval = DefaultBestNodesInterval
	}
	return c
}

// Validate reports configuration errors that do not need the network.
func (c Config) Validate() error {
	c = c.withDefaults()
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid port %d", c.Port)
	}
	if c.Username == "" || c.Password == "" {
		return errors.New("username and password are required")
	}
	if _, _, err := net.SplitHostPort(c.HTTPBind); err != nil {
		return fmt.Errorf("invalid httpBind %q: %w", c.HTTPBind, err)
	}
	if c.UpdateBestNodesInterval < 0 {
		return errors.New("updateBestNodesInterval must not be negative")
	}
	return nil
}

// Prompt kinds passed to Prompter.Code.
const (
	KindCaptcha = "captcha"
	KindSMS     = "sms"
	KindTOTP    = "totp"
)

// Prompter answers interactive login steps. Calls block the login until
// they return; returning an error aborts Start.
type Prompter interface {
	// Captcha shows the gateway's click captcha image (PNG/JPEG bytes) and
	// returns the clicked points, e.g. `[[x,y],[x,y]]` in image pixels, or
	// `{"coordinates":[[x,y]],"width":w,"height":h}`.
	Captcha(ctx context.Context, image []byte) (string, error)
	// Code asks for a one-time code; kind is KindSMS or KindTOTP.
	Code(ctx context.Context, kind, hint string) (string, error)
}

type promptAdapter struct {
	ctx context.Context
	p   Prompter
}

func (a promptAdapter) GraphCheckCode(image []byte) (string, error) {
	if err := a.ctx.Err(); err != nil {
		return "", err
	}
	return a.p.Captcha(a.ctx, image)
}

func (a promptAdapter) VerificationCode(kind, _, hint string, _ bool) (string, error) {
	if err := a.ctx.Err(); err != nil {
		return "", err
	}
	return a.p.Code(a.ctx, kind, hint)
}

// errPrompter fails any interactive step; used when the host passes nil.
type errPrompter struct{}

func (errPrompter) Captcha(context.Context, []byte) (string, error) {
	return "", errors.New("captcha required but no prompter configured")
}

func (errPrompter) Code(_ context.Context, kind, _ string) (string, error) {
	return "", fmt.Errorf("%s code required but no prompter configured", kind)
}

// State is a coarse session lifecycle state.
type State string

const (
	StateRunning State = "running"
	StateFailed  State = "failed"
	StateStopped State = "stopped"
)

// Status is a snapshot of a session.
type Status struct {
	State     State  `json:"state"`
	ProxyAddr string `json:"proxy,omitempty"`
	ClientIP  string `json:"clientIp,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Session is a running tunnel with its local HTTP proxy.
type Session struct {
	proxyAddr string
	clientIP  string

	vpnClient  *atrustclient.Client
	vpnStack   *gvisor.Stack
	resolver   *resolve.Resolver
	closeProxy func(context.Context) error
	cancelBg   context.CancelFunc

	mu      sync.Mutex
	state   State
	failure error

	closeOnce sync.Once
	closeErr  error
}

// Start logs in and brings up the tunnel and the local HTTP proxy. It
// returns once the proxy is listening. ctx bounds the login (it is passed
// to the Prompter); cancelling it later does not stop a started session —
// call Close for that.
func Start(ctx context.Context, cfg Config, prompt Prompter) (_ *Session, returnErr error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	cfg = cfg.withDefaults()
	if prompt == nil {
		prompt = errPrompter{}
	}

	s := &Session{state: StateRunning}
	defer func() {
		if returnErr != nil {
			_ = s.Close()
		}
	}()

	var clientData []byte
	if cfg.ClientDataPath != "" {
		data, err := os.ReadFile(cfg.ClientDataPath)
		if err == nil {
			clientData = data
		} else if !errors.Is(err, os.ErrNotExist) {
			log.Printf("Read client data file error: %s", err)
		}
	}

	s.vpnClient = atrustclient.NewClient(cfg.Username, "", "", "")
	s.vpnClient.Prompter = promptAdapter{ctx: ctx, p: prompt}

	newClientData, err := s.vpnClient.Setup(
		cfg.Server, cfg.Port,
		cfg.Username, cfg.Password, cfg.TOTPSecret,
		"", cfg.LoginDomain, "auth/psw",
		"", "", false, false,
		clientData, nil,
		cfg.UpdateBestNodesInterval,
	)
	if err != nil && clientData != nil && ctx.Err() == nil {
		// Stale or corrupt saved state must not lock the user out: retry
		// once with a clean device, as a first login would.
		log.Printf("Login with saved client data failed (%v); retrying fresh", err)
		s.vpnClient.Close()
		s.vpnClient = atrustclient.NewClient(cfg.Username, "", "", "")
		s.vpnClient.Prompter = promptAdapter{ctx: ctx, p: prompt}
		newClientData, err = s.vpnClient.Setup(
			cfg.Server, cfg.Port,
			cfg.Username, cfg.Password, cfg.TOTPSecret,
			"", cfg.LoginDomain, "auth/psw",
			"", "", false, false,
			nil, nil,
			cfg.UpdateBestNodesInterval,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("login: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.ClientDataPath != "" && newClientData != nil {
		if err := writePrivateFile(cfg.ClientDataPath, newClientData); err != nil {
			log.Printf("Write client data file error: %s", err)
		}
	}

	if ip, err := s.vpnClient.IP(); err == nil {
		s.clientIP = ip.String()
	}
	ipResources, err := s.vpnClient.IPResources()
	if err != nil {
		log.Println("No IP resources")
	}
	domainResources, err := s.vpnClient.DomainResources()
	if err != nil {
		log.Println("No domain resources")
	}
	dnsResource, err := s.vpnClient.DNSResource()
	if err != nil {
		log.Println("No DNS resource")
	}

	s.vpnStack, err = gvisor.NewStack(s.vpnClient)
	if err != nil {
		return nil, fmt.Errorf("gVisor stack setup: %w", err)
	}

	useRemoteDNS := !cfg.DisableRemoteDNS
	remoteDNSServer := cfg.RemoteDNSServer
	if useRemoteDNS && remoteDNSServer == "auto" {
		remoteDNSServer, err = s.vpnClient.DNSServer()
		if err != nil {
			useRemoteDNS = false
			remoteDNSServer = fallbackDNSServer
			log.Println("No DNS server provided by server. Disable remote DNS")
		}
	}

	s.resolver = resolve.NewResolver(
		s.vpnStack,
		remoteDNSServer,
		cfg.SecondaryDNSServer,
		cfg.DNSTTL,
		domainResources,
		dnsResource,
		useRemoteDNS,
		false,
	)
	localResolver := service.NewDnsServer(s.resolver, []string{remoteDNSServer, cfg.SecondaryDNSServer}, cfg.DNSTTL)
	s.vpnStack.SetupResolve(localResolver)
	s.vpnStack.SetupIPPool(s.resolver.IPPool)

	bgCtx, cancelBg := context.WithCancel(context.Background())
	s.cancelBg = cancelBg

	go func() {
		runErr := s.vpnStack.Run()
		if bgCtx.Err() != nil {
			return // closed on purpose
		}
		if runErr == nil {
			runErr = errors.New("VPN network stack stopped unexpectedly")
		}
		log.Printf("VPN network stack stopped: %v", runErr)
		s.fail(runErr)
	}()

	vpnDialer := dial.NewDialer(s.vpnStack, s.resolver, ipResources, false, "")

	s.proxyAddr, s.closeProxy, err = service.ServeHTTPProxy(cfg.HTTPBind, vpnDialer)
	if err != nil {
		return nil, err
	}

	if !cfg.DisableKeepAlive && (cfg.KeepAliveURL != "" || useRemoteDNS) {
		go service.KeepAlive(bgCtx, s.resolver, vpnDialer, cfg.KeepAliveURL)
	}

	log.Printf("Embedded session ready, HTTP proxy at %s", s.proxyAddr)
	return s, nil
}

func (s *Session) fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state == StateRunning {
		s.state = StateFailed
		s.failure = err
	}
}

// ProxyAddr is the bound host:port of the local HTTP proxy.
func (s *Session) ProxyAddr() string { return s.proxyAddr }

// Status returns the current session state.
func (s *Session) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{State: s.state, ProxyAddr: s.proxyAddr, ClientIP: s.clientIP}
	if s.failure != nil {
		st.Error = s.failure.Error()
	}
	if s.state == StateStopped {
		st.ProxyAddr = ""
	}
	return st
}

// Close stops the proxy, keep-alive, resolver, stack and aTrust client. It
// is safe to call more than once.
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		if s.state == StateRunning {
			s.state = StateStopped
		}
		s.mu.Unlock()

		var errs []error
		if s.cancelBg != nil {
			s.cancelBg()
		}
		if s.closeProxy != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			errs = append(errs, s.closeProxy(ctx))
			cancel()
		}
		if s.resolver != nil {
			s.resolver.Close()
		}
		if s.vpnStack != nil {
			errs = append(errs, s.vpnStack.Close())
		}
		if s.vpnClient != nil {
			s.vpnClient.Close()
		}
		s.closeErr = errors.Join(errs...)
	})
	return s.closeErr
}

// compile-time check that the adapter satisfies the auth hook.
var _ auth.Prompter = promptAdapter{}

func writePrivateFile(path string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".nwafu-connect-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}
