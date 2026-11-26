package configs

type (
	Config struct {
		// Common fields
		ServerAddress       string                 `koanf:"server_address"`
		ServerPort          int                    `koanf:"server_port"`
		Username            string                 `koanf:"username"`
		Password            string                 `koanf:"password"`
		TOTPSecret          string                 `koanf:"totp_secret"`
		SocksBind           string                 `koanf:"socks_bind"`
		SocksUser           string                 `koanf:"socks_user"`
		SocksPasswd         string                 `koanf:"socks_passwd"`
		HTTPBind            string                 `koanf:"http_bind"`
		BrowserMode         bool                   `koanf:"browser_mode"`
		BrowserPath         string                 `koanf:"browser_path"`
		BrowserURL          string                 `koanf:"browser_url"`
		BrowserProfileDir   string                 `koanf:"browser_profile_dir"`
		BrowserStayRunning  bool                   `koanf:"browser_stay_running"`
		BrowserStateFile    string                 `koanf:"browser_state_file"`
		PortForwardingList  []SinglePortForwarding `koanf:"port_forwarding"`
		ShadowsocksURL      string                 `koanf:"shadowsocks_url"`
		DialDirectProxy     string                 `koanf:"dial_direct_proxy"`
		DisableRemoteDNS    bool                   `koanf:"disable_remote_dns"`
		DNSTTL              uint64                 `koanf:"dns_ttl"`
		RemoteDNSServer     string                 `koanf:"remote_dns_server"`
		SecondaryDNSServer  string                 `koanf:"secondary_dns_server"`
		DNSServerBind       string                 `koanf:"dns_server_bind"`
		LocalDNSServer      string                 `koanf:"local_dns_server"`
		CustomDNSList       []SingleCustomDNS      `koanf:"custom_dns"`
		DisableKeepAlive    bool                   `koanf:"disable_keep_alive"`
		KeepAliveURL        string                 `koanf:"keep_alive_url"`
		TCPTunnelMode       bool                   `koanf:"tcp_tunnel_mode"`
		TUNMode             bool                   `koanf:"tun_mode"`
		AddRoute            bool                   `koanf:"add_route"`
		DNSHijack           bool                   `koanf:"dns_hijack"`
		ProxyAll            bool                   `koanf:"proxy_all"`
		FakeIP              bool                   `koanf:"fake_ip"`
		GraphCodeFile       string                 `koanf:"graph_code_file"`
		DebugDump           bool                   `koanf:"debug_dump"`
		DebugPCAPFile       string                 `koanf:"debug_pcap_file"`
		DebugTLSLogFile     string                 `koanf:"debug_tls_log_file"`
		BindInterface       string                 `koanf:"bind_interface"`
		AutoDetectInterface bool                   `koanf:"auto_detect_interface"`

		// aTrust fields
		AuthType                string   `koanf:"auth_type"`
		Phone                   string   `koanf:"phone"`
		LoginDomain             string   `koanf:"login_domain"`
		ClientDataFile          string   `koanf:"client_data_file"`
		QYWechatQRCodeFile      string   `koanf:"qywechat_qrcode_file"`
		QYWechatQRCodeTerminal  bool     `koanf:"qywechat_qrcode_terminal"`
		QYWechatQRCodeBrowser   bool     `koanf:"qywechat_qrcode_browser"`
		CasTicket               string   `koanf:"cas_ticket"`
		OAuth2Code              string   `koanf:"oauth2_code"`
		SID                     string   `koanf:"sid"`
		DeviceID                string   `koanf:"device_id"`
		MACAddresses            []string `koanf:"mac_addresses"`
		SignKey                 string   `koanf:"sign_key"`
		ResourceFile            string   `koanf:"resource_file"`
		UpdateBestNodesInterval int      `koanf:"update_best_nodes_interval"`
		SessionRefreshInterval  int      `koanf:"session_refresh_interval"`
	}

	SinglePortForwarding struct {
		NetworkType   string `koanf:"network_type" toml:"network_type"`
		BindAddress   string `koanf:"bind_address" toml:"bind_address"`
		RemoteAddress string `koanf:"remote_address" toml:"remote_address"`
	}

	SingleCustomDNS struct {
		HostName string `koanf:"host_name" toml:"host_name"`
		IP       string `koanf:"ip" toml:"ip"`
	}
)

// Default returns the canonical NWAFU Connect runtime defaults.
func Default() Config {
	return Config{
		ServerAddress:           "vpn.nwafu.edu.cn",
		ServerPort:              443,
		SocksBind:               "127.0.0.1:1080",
		HTTPBind:                "127.0.0.1:1081",
		DNSTTL:                  3600,
		RemoteDNSServer:         "auto",
		SecondaryDNSServer:      "auto",
		AuthType:                "auth/psw",
		LoginDomain:             "LDAP",
		QYWechatQRCodeFile:      "qywechat_qrcode.png",
		QYWechatQRCodeTerminal:  true,
		QYWechatQRCodeBrowser:   true,
		UpdateBestNodesInterval: 300,
		SessionRefreshInterval:  1800,
	}
}

type (
	ConfigTOML struct {
		ServerAddress           *string                    `toml:"server_address"`
		ServerPort              *int                       `toml:"server_port"`
		Username                *string                    `toml:"username"`
		Password                *string                    `toml:"password"`
		TOTPSecret              *string                    `toml:"totp_secret"`
		DisableRemoteDNS        *bool                      `toml:"disable_remote_dns"`
		SocksBind               *string                    `toml:"socks_bind"`
		SocksUser               *string                    `toml:"socks_user"`
		SocksPasswd             *string                    `toml:"socks_passwd"`
		HTTPBind                *string                    `toml:"http_bind"`
		BrowserMode             *bool                      `toml:"browser_mode"`
		BrowserPath             *string                    `toml:"browser_path"`
		BrowserURL              *string                    `toml:"browser_url"`
		BrowserProfileDir       *string                    `toml:"browser_profile_dir"`
		BrowserStayRunning      *bool                      `toml:"browser_stay_running"`
		BrowserStateFile        *string                    `toml:"browser_state_file"`
		ShadowsocksURL          *string                    `toml:"shadowsocks_url"`
		DialDirectProxy         *string                    `toml:"dial_direct_proxy"`
		TCPTunnelMode           *bool                      `toml:"tcp_tunnel_mode"`
		TUNMode                 *bool                      `toml:"tun_mode"`
		AddRoute                *bool                      `toml:"add_route"`
		DNSTTL                  *uint64                    `toml:"dns_ttl"`
		DisableKeepAlive        *bool                      `toml:"disable_keep_alive"`
		KeepAliveURL            *string                    `toml:"keep_alive_url"`
		RemoteDNSServer         *string                    `toml:"remote_dns_server"`
		SecondaryDNSServer      *string                    `toml:"secondary_dns_server"`
		DNSServerBind           *string                    `toml:"dns_server_bind"`
		LocalDNSServer          *string                    `toml:"local_dns_server"`
		DNSHijack               *bool                      `toml:"dns_hijack"`
		ProxyAll                *bool                      `toml:"proxy_all"`
		FakeIP                  *bool                      `toml:"fake_ip"`
		GraphCodeFile           *string                    `toml:"graph_code_file"`
		DebugDump               *bool                      `toml:"debug_dump"`
		DebugPCAPFile           *string                    `toml:"debug_pcap_file"`
		DebugTLSLogFile         *string                    `toml:"debug_tls_log_file"`
		BindInterface           *string                    `toml:"bind_interface"`
		AutoDetectInterface     *bool                      `toml:"auto_detect_interface"`
		PortForwarding          []SinglePortForwardingTOML `toml:"port_forwarding"`
		CustomDNS               []SingleCustomDNSTOML      `toml:"custom_dns"`
		AuthType                *string                    `toml:"auth_type"`
		Phone                   *string                    `toml:"phone"`
		LoginDomain             *string                    `toml:"login_domain"`
		ClientDataFile          *string                    `toml:"client_data_file"`
		QYWechatQRCodeFile      *string                    `toml:"qywechat_qrcode_file"`
		QYWechatQRCodeTerminal  *bool                      `toml:"qywechat_qrcode_terminal"`
		QYWechatQRCodeBrowser   *bool                      `toml:"qywechat_qrcode_browser"`
		CasTicket               *string                    `toml:"cas_ticket"`
		OAuth2Code              *string                    `toml:"oauth2_code"`
		SID                     *string                    `toml:"sid"`
		DeviceID                *string                    `toml:"device_id"`
		MACAddresses            []string                   `toml:"mac_addresses,omitempty"`
		SignKey                 *string                    `toml:"sign_key"`
		ResourceFile            *string                    `toml:"resource_file"`
		UpdateBestNodesInterval *int                       `toml:"update_best_nodes_interval"`
		SessionRefreshInterval  *int                       `toml:"session_refresh_interval"`
	}

	SinglePortForwardingTOML struct {
		NetworkType   *string `toml:"network_type"`
		BindAddress   *string `toml:"bind_address"`
		RemoteAddress *string `toml:"remote_address"`
	}

	SingleCustomDNSTOML struct {
		HostName *string `toml:"host_name"`
		IP       *string `toml:"ip"`
	}
)
