package configs

type (
	Config struct {
		// Common fields
		ServerAddress      string
		ServerPort         int
		Username           string
		Password           string
		TOTPSecret         string
		SocksBind          string
		SocksUser          string
		SocksPasswd        string
		HTTPBind           string
		PortForwardingList []SinglePortForwarding
		ShadowsocksURL     string
		DialDirectProxy    string
		DisableRemoteDNS   bool
		DNSTTL             uint64
		RemoteDNSServer    string
		SecondaryDNSServer string
		DNSServerBind      string
		CustomDNSList      []SingleCustomDNS
		DisableKeepAlive   bool
		KeepAliveURL       string
		TCPTunnelMode      bool
		TUNMode            bool
		AddRoute           bool
		DNSHijack          bool
		FakeIP             bool
		GraphCodeFile      string
		DebugDump          bool

		// aTrust fields
		AuthType                string
		Phone                   string
		LoginDomain             string
		ClientDataFile          string
		SID                     string
		DeviceID                string
		SignKey                 string
		ResourceFile            string
		UpdateBestNodesInterval int
	}

	SinglePortForwarding struct {
		NetworkType   string
		BindAddress   string
		RemoteAddress string
	}

	SingleCustomDNS struct {
		HostName string `toml:"host_name"`
		IP       string `toml:"ip"`
	}
)

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
		DNSHijack               *bool                      `toml:"dns_hijack"`
		FakeIP                  *bool                      `toml:"fake_ip"`
		GraphCodeFile           *string                    `toml:"graph_code_file"`
		DebugDump               *bool                      `toml:"debug_dump"`
		PortForwarding          []SinglePortForwardingTOML `toml:"port_forwarding"`
		CustomDNS               []SingleCustomDNSTOML      `toml:"custom_dns"`
		AuthType                *string                    `toml:"auth_type"`
		Phone                   *string                    `toml:"phone"`
		LoginDomain             *string                    `toml:"login_domain"`
		ClientDataFile          *string                    `toml:"client_data_file"`
		SID                     *string                    `toml:"sid"`
		DeviceID                *string                    `toml:"device_id"`
		SignKey                 *string                    `toml:"sign_key"`
		ResourceFile            *string                    `toml:"resource_file"`
		UpdateBestNodesInterval *int                       `toml:"update_best_nodes_interval"`
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
