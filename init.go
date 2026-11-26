package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"

	"github.com/knadh/koanf/parsers/toml/v2"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/posflag"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	"github.com/majianyu2007/nwafu-connect/client/atrust"
	"github.com/majianyu2007/nwafu-connect/configs"
	"github.com/spf13/pflag"
)

const (
	applicationName = "NWAFU Connect"
	envPrefix       = "NWAFU_CONNECT_"
)

var (
	nwafuConnectVersion = "1.5.0"
	CommitID            string
)

type startupOptions struct {
	Config        configs.Config
	ShowVersion   bool
	AuthInfo      bool
	TrustDevice   bool
	UntrustDevice bool
}

type collectionKey string

const (
	collectionTCPPortForwarding collectionKey = "tcp_port_forwarding"
	collectionUDPPortForwarding collectionKey = "udp_port_forwarding"
	collectionCustomDNS         collectionKey = "custom_dns"
)

type collectionSpec struct {
	Key      collectionKey
	FlagName string
	EnvName  string
	Help     string
}

var collectionSpecs = []collectionSpec{
	{collectionTCPPortForwarding, "tcp-port-forwarding", "TCP_PORT_FORWARDING", "TCP port forwarding (e.g. 127.0.0.1:9898-10.10.98.98:80)"},
	{collectionUDPPortForwarding, "udp-port-forwarding", "UDP_PORT_FORWARDING", "UDP port forwarding (e.g. 127.0.0.1:53-10.10.0.21:53)"},
	{collectionCustomDNS, "custom-dns", "CUSTOM_DNS", "Custom DNS records (e.g. library.nwafu.edu.cn:10.0.0.10)"},
}

func nwafuConnectVersionString() string {
	version := strings.TrimPrefix(strings.TrimSpace(nwafuConnectVersion), "v")
	if version == "" {
		version = "dev"
	}
	if CommitID != "" {
		return version + "-" + CommitID
	}
	return version
}

func newFlagSet(defaults configs.Config) *pflag.FlagSet {
	flags := pflag.NewFlagSet("nwafu-connect", pflag.ContinueOnError)
	flags.String("server", defaults.ServerAddress, "aTrust server address")
	flags.Int("port", defaults.ServerPort, "aTrust server port")
	flags.String("username", defaults.Username, "Your username")
	flags.String("password", defaults.Password, "Your password")
	flags.String("totp-secret", defaults.TOTPSecret, "TOTP secret")
	flags.Bool("disable-remote-dns", defaults.DisableRemoteDNS, "Use local DNS instead of remote DNS")
	flags.Bool("proxy-all", defaults.ProxyAll, "Route all traffic through the VPN tunnel")
	flags.String("socks-bind", defaults.SocksBind, "The address SOCKS5 server listens on (e.g. 127.0.0.1:1080)")
	flags.String("socks-user", defaults.SocksUser, "SOCKS5 username, default is no auth")
	flags.String("socks-passwd", defaults.SocksPasswd, "SOCKS5 password, default is no auth")
	flags.String("http-bind", defaults.HTTPBind, "The address HTTP server listens on (e.g. 127.0.0.1:1081)")
	flags.Bool("browser-mode", defaults.BrowserMode, "Launch a dedicated browser through a private aTrust proxy")
	flags.String("browser-path", defaults.BrowserPath, "Chromium-based browser executable; auto-detected when empty")
	flags.String("browser-url", defaults.BrowserURL, "Initial URL for browser mode; empty shows server-issued resources")
	flags.String("browser-profile-dir", defaults.BrowserProfileDir, "Persistent managed browser profile directory; empty uses a temporary profile")
	flags.Bool("browser-stay-running", defaults.BrowserStayRunning, "Keep the VPN session running after the managed browser closes")
	flags.String("browser-state-file", defaults.BrowserStateFile, "Write managed browser connection state to this private JSON file")
	flags.String("shadowsocks-url", defaults.ShadowsocksURL, "The address Shadowsocks server listens on (e.g. ss://method:password@host:port)")
	flags.String("dial-direct-proxy", defaults.DialDirectProxy, "Dial with proxy when a connection doesn't match VPN rules (e.g. http://127.0.0.1:7890)")
	flags.Bool("tcp-tunnel-mode", defaults.TCPTunnelMode, "Use the aTrust TCP tunnel only and disable the L3 tunnel")
	flags.Bool("tun-mode", defaults.TUNMode, "Enable TUN mode (experimental)")
	flags.Bool("add-route", defaults.AddRoute, "Add route from rules for TUN interface")
	flags.Uint64("dns-ttl", defaults.DNSTTL, "DNS record time to live in seconds")
	flags.Bool("debug-dump", defaults.DebugDump, "Enable traffic debug dump (only for debug usage)")
	flags.String("debug-pcap-file", defaults.DebugPCAPFile, "Save reconstructed VPN traffic to a PCAP file")
	flags.String("debug-tls-log-file", defaults.DebugTLSLogFile, "Save TLS session secrets in NSS key log format")
	flags.Bool("disable-keep-alive", defaults.DisableKeepAlive, "Disable keep alive")
	flags.String("keep-alive-url", defaults.KeepAliveURL, "Keep alive URL, default is empty (use DNS keep alive)")
	flags.String("remote-dns-server", defaults.RemoteDNSServer, "Remote DNS server address ('auto' uses server-provided DNS)")
	flags.String("secondary-dns-server", defaults.SecondaryDNSServer, "Secondary DNS server address ('auto' uses secondary server DNS or 114.114.114.114; empty uses system DNS)")
	flags.String("dns-server-bind", defaults.DNSServerBind, "The address DNS server listens on (e.g. 127.0.0.1:53)")
	flags.String("local-dns-server", defaults.LocalDNSServer, "DNS server used to resolve the VPN server hostname")
	flags.Bool("dns-hijack", defaults.DNSHijack, "Hijack all DNS queries to NWAFU Connect")
	flags.Bool("fake-ip", defaults.FakeIP, "Enable Fake IP for DNS hijack")
	flags.String("graph-code-file", defaults.GraphCodeFile, "Graph Check Code File")
	flags.String("bind-interface", defaults.BindInterface, "Bind VPN underlay connections to this network interface")
	flags.Bool("auto-detect-interface", defaults.AutoDetectInterface, "Automatically detect and bind the VPN underlay interface")
	flags.String("auth-type", defaults.AuthType, "NWAFU authentication type (auth/psw, auth/smsCheckCode, auth/qywechat, auth/cas, or auth/httpsOauth2)")
	flags.String("phone", defaults.Phone, "Phone number with country code for aTrust SMS login (e.g. 86-13800138000)")
	flags.String("login-domain", defaults.LoginDomain, "aTrust login domain")
	flags.String("client-data-file", defaults.ClientDataFile, "aTrust Client Data File")
	flags.String("qywechat-qrcode-file", defaults.QYWechatQRCodeFile, "Path for the enterprise WeChat QR code PNG; empty disables saving")
	flags.Bool("qywechat-qrcode-terminal", defaults.QYWechatQRCodeTerminal, "Render the enterprise WeChat QR code in the terminal")
	flags.Bool("qywechat-qrcode-browser", defaults.QYWechatQRCodeBrowser, "Open the enterprise WeChat QR code in a local browser page")
	flags.String("cas-ticket", defaults.CasTicket, "aTrust CAS Ticket")
	flags.String("oauth2-code", defaults.OAuth2Code, "aTrust OAuth2 code")
	flags.String("sid", defaults.SID, "aTrust SID (mostly for debug usage)")
	flags.String("device-id", defaults.DeviceID, "aTrust Device ID (mostly for debug usage)")
	flags.String("sign-key", defaults.SignKey, "aTrust Sign Key (mostly for debug usage)")
	flags.String("resource-file", defaults.ResourceFile, "aTrust Resource File (mostly for debug usage)")
	flags.Int("update-best-nodes-interval", defaults.UpdateBestNodesInterval, "Interval to update best nodes in seconds (0 disables)")
	flags.Int("session-refresh-interval", defaults.SessionRefreshInterval, "aTrust session refresh interval in seconds (0 disables)")

	for _, spec := range collectionSpecs {
		flags.String(spec.FlagName, "", spec.Help)
	}

	flags.String("config", "", "Config file (can also be set with NWAFU_CONNECT_CONFIG)")
	flags.Bool("version", false, "Show version")
	flags.Bool("auth-info", false, "Fetch aTrust authentication information, but do not login")
	flags.Bool("trust-device", false, "Trust the current device for aTrust with client data, but do not connect")
	flags.Bool("untrust-device", false, "Untrust the current device for aTrust with client data, but do not connect")
	return flags
}

func loadStartupOptions(args []string, environ func() []string) (startupOptions, *pflag.FlagSet, error) {
	defaults := configs.Default()
	flags := newFlagSet(defaults)
	if err := flags.Parse(normalizeLegacyArgs(flags, args)); err != nil {
		return startupOptions{}, flags, err
	}

	showVersion, _ := flags.GetBool("version")
	authInfo, _ := flags.GetBool("auth-info")
	trustDevice, _ := flags.GetBool("trust-device")
	untrustDevice, _ := flags.GetBool("untrust-device")
	configFile, _ := flags.GetString("config")
	options := startupOptions{
		ShowVersion:   showVersion,
		AuthInfo:      authInfo,
		TrustDevice:   trustDevice,
		UntrustDevice: untrustDevice,
	}
	if options.ShowVersion {
		return options, flags, nil
	}

	envValues := environ()
	if !flags.Lookup("config").Changed {
		if path, ok := lookupEnvironment(envValues, envPrefix+"CONFIG"); ok {
			configFile = path
		}
	}

	k := koanf.New(".")
	if err := k.Load(structs.Provider(defaults, "koanf"), nil); err != nil {
		return startupOptions{}, flags, fmt.Errorf("load config defaults: %w", err)
	}

	allowedKeys := make(map[string]struct{}, len(k.Keys()))
	for _, key := range k.Keys() {
		allowedKeys[key] = struct{}{}
	}

	if configFile != "" {
		if err := k.Load(file.Provider(configFile), toml.Parser()); err != nil {
			return startupOptions{}, flags, fmt.Errorf("%s: parse config file %q: %w", applicationName, configFile, err)
		}
		if err := rejectUnknownKeys(k, allowedKeys); err != nil {
			return startupOptions{}, flags, fmt.Errorf("%s: parse config file %q: %w", applicationName, configFile, err)
		}
	}

	envProvider := env.Provider(".", env.Opt{
		Prefix:      envPrefix,
		EnvironFunc: func() []string { return envValues },
		TransformFunc: func(key, value string) (string, any) {
			key = strings.ToLower(strings.TrimPrefix(key, envPrefix))
			if key == "config" || isCollectionEnvKey(key) {
				return "", nil
			}
			return key, value
		},
	})
	if err := k.Load(envProvider, nil); err != nil {
		return startupOptions{}, flags, fmt.Errorf("load environment: %w", err)
	}
	if err := rejectUnknownKeys(k, allowedKeys); err != nil {
		return startupOptions{}, flags, fmt.Errorf("load environment: %w", err)
	}
	if err := applyCollectionValues(k, collectionValuesFromEnvironment(envValues)); err != nil {
		return startupOptions{}, flags, fmt.Errorf("load environment: %w", err)
	}

	cliProvider := posflag.ProviderWithFlag(flags, ".", k, func(flag *pflag.Flag) (string, any) {
		key, ok := configKeyForFlag(flag.Name)
		if !ok {
			return "", nil
		}
		return key, posflag.FlagVal(flags, flag)
	})
	if err := k.Load(cliProvider, nil); err != nil {
		return startupOptions{}, flags, fmt.Errorf("load command line: %w", err)
	}

	if err := applyCollectionValues(k, collectionValuesFromFlags(flags)); err != nil {
		return startupOptions{}, flags, err
	}

	var cfg configs.Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return startupOptions{}, flags, fmt.Errorf("decode merged configuration: %w", err)
	}
	normalizeConfig(&cfg)
	if err := validateConfig(cfg); err != nil {
		return startupOptions{}, flags, err
	}

	options.Config = cfg
	return options, flags, nil
}

func normalizeLegacyArgs(flags *pflag.FlagSet, args []string) []string {
	normalized := append([]string(nil), args...)
	expectValue := false
	stopParsing := false
	for i, arg := range normalized {
		if stopParsing {
			continue
		}
		if expectValue {
			expectValue = false
			continue
		}
		if arg == "--" {
			stopParsing = true
			continue
		}
		if arg == "-h" {
			normalized[i] = "--help"
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		hasValue := false
		if index := strings.IndexByte(name, '='); index >= 0 {
			name = name[:index]
			hasValue = true
		}
		flag := flags.Lookup(name)
		if flag != nil && !strings.HasPrefix(arg, "--") {
			normalized[i] = "-" + arg
		}
		if flag != nil && flag.NoOptDefVal == "" && !hasValue {
			expectValue = true
		}
	}
	return normalized
}

func lookupEnvironment(environ []string, name string) (string, bool) {
	prefix := name + "="
	for _, item := range environ {
		if strings.HasPrefix(item, prefix) {
			return strings.TrimPrefix(item, prefix), true
		}
	}
	return "", false
}

func rejectUnknownKeys(k *koanf.Koanf, allowed map[string]struct{}) error {
	for _, key := range k.Keys() {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown configuration key %q", key)
		}
	}
	return nil
}

func isCollectionEnvKey(key string) bool {
	for _, spec := range collectionSpecs {
		if string(spec.Key) == key {
			return true
		}
	}
	return false
}

func configKeyForFlag(name string) (string, bool) {
	switch name {
	case "config", "version", "auth-info", "trust-device", "untrust-device":
		return "", false
	case "server":
		return "server_address", true
	case "port":
		return "server_port", true
	}
	for _, spec := range collectionSpecs {
		if spec.FlagName == name {
			return "", false
		}
	}
	return strings.ReplaceAll(name, "-", "_"), true
}

type collectionValue struct {
	Value string
	Set   bool
}

type collectionValues map[collectionKey]collectionValue

func collectionValuesFromEnvironment(environ []string) collectionValues {
	values := make(collectionValues, len(collectionSpecs))
	for _, spec := range collectionSpecs {
		value, set := lookupEnvironment(environ, envPrefix+spec.EnvName)
		values[spec.Key] = collectionValue{Value: value, Set: set}
	}
	return values
}

func collectionValuesFromFlags(flags *pflag.FlagSet) collectionValues {
	values := make(collectionValues, len(collectionSpecs))
	for _, spec := range collectionSpecs {
		flag := flags.Lookup(spec.FlagName)
		values[spec.Key] = collectionValue{Value: flag.Value.String(), Set: flag.Changed}
	}
	return values
}

func applyCollectionValues(k *koanf.Koanf, values collectionValues) error {
	tcp := values[collectionTCPPortForwarding]
	udp := values[collectionUDPPortForwarding]
	if tcp.Set || udp.Set {
		var existing []configs.SinglePortForwarding
		_ = k.Unmarshal("port_forwarding", &existing)
		entries := make([]configs.SinglePortForwarding, 0, len(existing))
		for _, forwarding := range existing {
			networkType := strings.ToLower(strings.TrimSpace(forwarding.NetworkType))
			if (networkType == "tcp" && tcp.Set) || (networkType == "udp" && udp.Set) {
				continue
			}
			entries = append(entries, forwarding)
		}
		for _, item := range []struct {
			value   collectionValue
			network string
		}{
			{tcp, "tcp"},
			{udp, "udp"},
		} {
			if !item.value.Set {
				continue
			}
			parsed, err := parsePortForwarding(item.network, item.value.Value)
			if err != nil {
				return err
			}
			entries = append(entries, parsed...)
		}
		if err := k.Set("port_forwarding", entries); err != nil {
			return err
		}
	}

	customDNS := values[collectionCustomDNS]
	if customDNS.Set {
		entries, err := parseCustomDNS(customDNS.Value)
		if err != nil {
			return err
		}
		if err := k.Set("custom_dns", entries); err != nil {
			return err
		}
	}

	return nil
}

func splitForwardingAddresses(value string) (string, string, bool) {
	for index, char := range value {
		if char != '-' {
			continue
		}
		bindAddress := strings.TrimSpace(value[:index])
		remoteAddress := strings.TrimSpace(value[index+1:])
		if _, _, err := net.SplitHostPort(bindAddress); err != nil {
			continue
		}
		if _, _, err := net.SplitHostPort(remoteAddress); err != nil {
			continue
		}
		return bindAddress, remoteAddress, true
	}
	return "", "", false
}

func parsePortForwarding(network, value string) ([]configs.SinglePortForwarding, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var entries []configs.SinglePortForwarding
	for _, forwarding := range strings.Split(value, ",") {
		bindAddress, remoteAddress, ok := splitForwardingAddresses(forwarding)
		if !ok {
			return nil, fmt.Errorf("%s: wrong %s port forwarding format", applicationName, strings.ToUpper(network))
		}
		entries = append(entries, configs.SinglePortForwarding{
			NetworkType:   network,
			BindAddress:   bindAddress,
			RemoteAddress: remoteAddress,
		})
	}
	return entries, nil
}

func parseCustomDNS(value string) ([]configs.SingleCustomDNS, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var entries []configs.SingleCustomDNS
	for _, entry := range strings.Split(value, ",") {
		hostName, ip, ok := strings.Cut(entry, ":")
		hostName = strings.TrimSpace(hostName)
		ip = strings.TrimSpace(ip)
		if !ok || hostName == "" || net.ParseIP(ip) == nil {
			return nil, fmt.Errorf("%s: wrong custom DNS format", applicationName)
		}
		entries = append(entries, configs.SingleCustomDNS{HostName: hostName, IP: ip})
	}
	return entries, nil
}

func normalizeConfig(cfg *configs.Config) {
	cfg.ServerAddress = strings.TrimSpace(cfg.ServerAddress)
	for i := range cfg.PortForwardingList {
		cfg.PortForwardingList[i].NetworkType = strings.ToLower(strings.TrimSpace(cfg.PortForwardingList[i].NetworkType))
		cfg.PortForwardingList[i].BindAddress = strings.TrimSpace(cfg.PortForwardingList[i].BindAddress)
		cfg.PortForwardingList[i].RemoteAddress = strings.TrimSpace(cfg.PortForwardingList[i].RemoteAddress)
	}
	for i := range cfg.CustomDNSList {
		cfg.CustomDNSList[i].HostName = strings.TrimSpace(cfg.CustomDNSList[i].HostName)
		cfg.CustomDNSList[i].IP = strings.TrimSpace(cfg.CustomDNSList[i].IP)
	}
}

func validateConfig(cfg configs.Config) error {
	if cfg.ServerPort < 1 || cfg.ServerPort > 65535 {
		return fmt.Errorf("%s: server port must be between 1 and 65535", applicationName)
	}
	if cfg.UpdateBestNodesInterval < 0 {
		return fmt.Errorf("%s: best-node update interval cannot be negative", applicationName)
	}
	if cfg.SessionRefreshInterval < 0 {
		return fmt.Errorf("%s: session refresh interval cannot be negative", applicationName)
	}
	for _, forwarding := range cfg.PortForwardingList {
		if forwarding.NetworkType == "" || forwarding.BindAddress == "" || forwarding.RemoteAddress == "" {
			return fmt.Errorf("%s: every port_forwarding entry requires network_type, bind_address, and remote_address", applicationName)
		}
		if forwarding.NetworkType != "tcp" && forwarding.NetworkType != "udp" {
			return fmt.Errorf("%s: unsupported port forwarding network type %q", applicationName, forwarding.NetworkType)
		}
		if _, _, err := net.SplitHostPort(forwarding.BindAddress); err != nil {
			return fmt.Errorf("%s: invalid port forwarding bind address %q: %w", applicationName, forwarding.BindAddress, err)
		}
		if _, _, err := net.SplitHostPort(forwarding.RemoteAddress); err != nil {
			return fmt.Errorf("%s: invalid port forwarding remote address %q: %w", applicationName, forwarding.RemoteAddress, err)
		}
	}
	for _, entry := range cfg.CustomDNSList {
		if entry.HostName == "" || net.ParseIP(entry.IP) == nil {
			return fmt.Errorf("%s: invalid custom DNS entry %q -> %q", applicationName, entry.HostName, entry.IP)
		}
	}
	return nil
}

func validateConnectConfig(cfg configs.Config) error {
	if cfg.ServerAddress == "" {
		return fmt.Errorf("%s: missing required server address", applicationName)
	}
	var primaryCredentialsPresent bool
	switch cfg.AuthType {
	case "auth/psw":
		primaryCredentialsPresent = cfg.Username != "" && cfg.Password != ""
	case "auth/smsCheckCode":
		primaryCredentialsPresent = cfg.Phone != ""
	case "auth/qywechat", "auth/cas", "auth/httpsOauth2", "":
		return nil
	default:
		return fmt.Errorf("%s: unsupported auth type %q", applicationName, cfg.AuthType)
	}
	debugCredentialsPresent := cfg.SID != "" && cfg.DeviceID != "" && cfg.ResourceFile != ""
	if !primaryCredentialsPresent && !debugCredentialsPresent {
		return fmt.Errorf("%s: missing required arguments", applicationName)
	}
	return nil
}

func initialize(args []string) int {
	options, flags, err := loadStartupOptions(args, os.Environ)
	if err != nil {
		if errors.Is(err, pflag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if options.ShowVersion {
		fmt.Printf("%s v%s\n", applicationName, nwafuConnectVersionString())
		return 0
	}

	conf = options.Config
	if options.AuthInfo {
		if conf.ServerAddress == "" {
			fmt.Fprintf(os.Stderr, "%s: server address is required for -auth-info\n", applicationName)
			return 1
		}
		log.SetOutput(io.Discard)
		info, err := atrust.GetAuthInfoList(conf.ServerAddress, conf.ServerPort, conf.BindInterface, conf.AutoDetectInterface, conf.LocalDNSServer, conf.DebugTLSLogFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Get auth info list error:", err)
			return 1
		}
		jsonInfo, err := json.Marshal(info)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error marshaling auth info:", err)
			return 1
		}
		fmt.Println(string(jsonInfo))
		return 0
	}

	if options.TrustDevice || options.UntrustDevice {
		if conf.ServerAddress == "" {
			fmt.Fprintf(os.Stderr, "%s: server address is required for trust/untrust device\n", applicationName)
			return 1
		}
		if conf.ClientDataFile == "" {
			fmt.Fprintln(os.Stderr, "Client data file is required for trust/untrust device")
			return 1
		}
		clientData, err := os.ReadFile(conf.ClientDataFile)
		if err != nil {
			log.Printf("Read client data file error: %s", err)
			return 1
		}
		if err := atrust.SetTrusted(conf.ServerAddress, conf.ServerPort, clientData, options.TrustDevice, conf.BindInterface, conf.AutoDetectInterface, conf.LocalDNSServer, conf.DebugTLSLogFile); err != nil {
			fmt.Fprintln(os.Stderr, "Trust/Untrust device error:", err)
			return 1
		}
		if options.TrustDevice {
			log.Println("Device trusted successfully")
		} else {
			log.Println("Device untrusted successfully")
		}
		return 0
	}

	if err := validateConnectConfig(conf); err != nil {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "Use -auth-info to inspect the server's available aTrust authentication methods.")
		fmt.Fprintln(os.Stderr, "\nUsage:")
		flags.PrintDefaults()
		return 1
	}
	return -1
}
