package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/majianyu2007/nwafu-connect/client/atrust"
	"github.com/majianyu2007/nwafu-connect/configs"
)

var CommitID string

const (
	applicationName      = "NWAFU Connect"
	nwafuConnectVersion  = "1.2.0"
	defaultServerAddress = "vpn.nwafu.edu.cn"
	defaultAuthType      = "auth/psw"
	defaultLoginDomain   = "LDAP"
)

func getTOMLVal[T int | uint64 | string | bool](valPointer *T, defaultVal T) T {
	if valPointer == nil {
		return defaultVal
	} else {
		return *valPointer
	}
}

func parseTOMLConfig(configFile string, conf *configs.Config) error {
	var confTOML configs.ConfigTOML

	_, err := toml.DecodeFile(configFile, &confTOML)
	if err != nil {
		return fmt.Errorf("%s: error parsing the config file", applicationName)
	}

	conf.ServerAddress = getTOMLVal(confTOML.ServerAddress, defaultServerAddress)
	conf.ServerPort = getTOMLVal(confTOML.ServerPort, 443)
	conf.Username = getTOMLVal(confTOML.Username, "")
	conf.Password = getTOMLVal(confTOML.Password, "")
	conf.TOTPSecret = getTOMLVal(confTOML.TOTPSecret, "")
	conf.DisableRemoteDNS = getTOMLVal(confTOML.DisableRemoteDNS, false)
	conf.SocksBind = getTOMLVal(confTOML.SocksBind, ":1080")
	conf.SocksUser = getTOMLVal(confTOML.SocksUser, "")
	conf.SocksPasswd = getTOMLVal(confTOML.SocksPasswd, "")
	conf.HTTPBind = getTOMLVal(confTOML.HTTPBind, ":1081")
	conf.ShadowsocksURL = getTOMLVal(confTOML.ShadowsocksURL, "")
	conf.DialDirectProxy = getTOMLVal(confTOML.DialDirectProxy, "")
	conf.TCPTunnelMode = getTOMLVal(confTOML.TCPTunnelMode, false)
	conf.TUNMode = getTOMLVal(confTOML.TUNMode, false)
	conf.AddRoute = getTOMLVal(confTOML.AddRoute, false)
	conf.DNSTTL = getTOMLVal(confTOML.DNSTTL, uint64(3600))
	conf.DebugDump = getTOMLVal(confTOML.DebugDump, false)
	conf.DisableKeepAlive = getTOMLVal(confTOML.DisableKeepAlive, false)
	conf.KeepAliveURL = getTOMLVal(confTOML.KeepAliveURL, "")
	conf.RemoteDNSServer = getTOMLVal(confTOML.RemoteDNSServer, "auto")
	conf.SecondaryDNSServer = getTOMLVal(confTOML.SecondaryDNSServer, "114.114.114.114")
	conf.DNSServerBind = getTOMLVal(confTOML.DNSServerBind, "")
	conf.DNSHijack = getTOMLVal(confTOML.DNSHijack, false)
	conf.FakeIP = getTOMLVal(confTOML.FakeIP, false)
	conf.GraphCodeFile = getTOMLVal(confTOML.GraphCodeFile, "")
	conf.AuthType = getTOMLVal(confTOML.AuthType, defaultAuthType)
	conf.Phone = getTOMLVal(confTOML.Phone, "")
	conf.LoginDomain = getTOMLVal(confTOML.LoginDomain, defaultLoginDomain)
	conf.ClientDataFile = getTOMLVal(confTOML.ClientDataFile, "")
	conf.SID = getTOMLVal(confTOML.SID, "")
	conf.DeviceID = getTOMLVal(confTOML.DeviceID, "")
	conf.SignKey = getTOMLVal(confTOML.SignKey, "")
	conf.ResourceFile = getTOMLVal(confTOML.ResourceFile, "")
	conf.UpdateBestNodesInterval = getTOMLVal(confTOML.UpdateBestNodesInterval, 300)

	for _, singlePortForwarding := range confTOML.PortForwarding {
		if singlePortForwarding.NetworkType == nil {
			return fmt.Errorf("%s: network type is not set", applicationName)
		}

		if singlePortForwarding.BindAddress == nil {
			return fmt.Errorf("%s: bind address is not set", applicationName)
		}

		if singlePortForwarding.RemoteAddress == nil {
			return fmt.Errorf("%s: remote address is not set", applicationName)
		}

		conf.PortForwardingList = append(conf.PortForwardingList, configs.SinglePortForwarding{
			NetworkType:   *singlePortForwarding.NetworkType,
			BindAddress:   *singlePortForwarding.BindAddress,
			RemoteAddress: *singlePortForwarding.RemoteAddress,
		})
	}

	for _, singleCustomDns := range confTOML.CustomDNS {
		if singleCustomDns.HostName == nil {
			return fmt.Errorf("%s: host name is not set", applicationName)
		}

		if singleCustomDns.IP == nil {
			return fmt.Errorf("%s: IP is not set", applicationName)
		}

		conf.CustomDNSList = append(conf.CustomDNSList, configs.SingleCustomDNS{
			HostName: *singleCustomDns.HostName,
			IP:       *singleCustomDns.IP,
		})
	}

	return nil
}

func init() {
	configFile, tcpPortForwarding, udpPortForwarding, customDns := "", "", "", ""
	showVersion := false
	atrustAuthInfo := false
	atrustTrustDevice := false
	atrustUntrustDevice := false

	flag.StringVar(&conf.ServerAddress, "server", defaultServerAddress, "aTrust server address")
	flag.IntVar(&conf.ServerPort, "port", 443, "aTrust server port")
	flag.StringVar(&conf.Username, "username", "", "Your username")
	flag.StringVar(&conf.Password, "password", "", "Your password")
	flag.StringVar(&conf.TOTPSecret, "totp-secret", "", "TOTP secret")
	flag.BoolVar(&conf.DisableRemoteDNS, "disable-remote-dns", false, "Use local DNS instead of remote DNS")
	flag.StringVar(&conf.SocksBind, "socks-bind", ":1080", "The address SOCKS5 server listens on (e.g. 127.0.0.1:1080)")
	flag.StringVar(&conf.SocksUser, "socks-user", "", "SOCKS5 username, default is don't use auth")
	flag.StringVar(&conf.SocksPasswd, "socks-passwd", "", "SOCKS5 password, default is don't use auth")
	flag.StringVar(&conf.HTTPBind, "http-bind", ":1081", "The address HTTP server listens on (e.g. 127.0.0.1:1081)")
	flag.StringVar(&conf.ShadowsocksURL, "shadowsocks-url", "", "The address Shadowsocks server listens on (e.g. ss://method:password@host:port)")
	flag.StringVar(&conf.DialDirectProxy, "dial-direct-proxy", "", "Dial with proxy when the connection doesn't match RVPN rules (e.g. http://127.0.0.1:7890)")
	flag.BoolVar(&conf.TCPTunnelMode, "tcp-tunnel-mode", false, "Use the aTrust TCP tunnel only and disable the L3 tunnel")
	flag.BoolVar(&conf.TUNMode, "tun-mode", false, "Enable TUN mode (experimental)")
	flag.BoolVar(&conf.AddRoute, "add-route", false, "Add route from rules for TUN interface")
	flag.Uint64Var(&conf.DNSTTL, "dns-ttl", 3600, "DNS record time to live, unit is second")
	flag.BoolVar(&conf.DebugDump, "debug-dump", false, "Enable traffic debug dump (only for debug usage)")
	flag.BoolVar(&conf.DisableKeepAlive, "disable-keep-alive", false, "Disable keep alive")
	flag.StringVar(&conf.KeepAliveURL, "keep-alive-url", "", "Keep alive URL, default is empty (use DNS keep alive)")
	flag.StringVar(&conf.RemoteDNSServer, "remote-dns-server", "auto", "Remote DNS server address. Set to 'auto' to use remote DNS server provided by server")
	flag.StringVar(&conf.SecondaryDNSServer, "secondary-dns-server", "114.114.114.114", "Secondary DNS server address. Leave empty to use system default DNS server")
	flag.StringVar(&conf.DNSServerBind, "dns-server-bind", "", "The address DNS server listens on (e.g. 127.0.0.1:53)")
	flag.BoolVar(&conf.DNSHijack, "dns-hijack", false, "Hijack all DNS queries to NWAFU Connect. False by default.")
	flag.BoolVar(&conf.FakeIP, "fake-ip", false, "Enable Fake IP for DNS hijack")
	flag.StringVar(&conf.GraphCodeFile, "graph-code-file", "", "Graph Check Code File")
	flag.StringVar(&conf.AuthType, "auth-type", defaultAuthType, "NWAFU authentication type (auth/psw or auth/smsCheckCode)")
	flag.StringVar(&conf.Phone, "phone", "", "Phone number with country code for aTrust SMS check code login (e.g. 86-13800138000)")
	flag.StringVar(&conf.LoginDomain, "login-domain", defaultLoginDomain, "aTrust login domain")
	flag.StringVar(&conf.ClientDataFile, "client-data-file", "", "aTrust Client Data File")
	flag.StringVar(&conf.SID, "sid", "", "aTrust SID (mostly for debug usage)")
	flag.StringVar(&conf.DeviceID, "device-id", "", "aTrust Device ID (mostly for debug usage)")
	flag.StringVar(&conf.SignKey, "sign-key", "", "aTrust Sign Key (mostly for debug usage)")
	flag.StringVar(&conf.ResourceFile, "resource-file", "", "aTrust Resource File (mostly for debug usage)")
	flag.IntVar(&conf.UpdateBestNodesInterval, "update-best-nodes-interval", 300, "Interval to update best nodes in seconds. Set to 0 to disable")
	flag.StringVar(&tcpPortForwarding, "tcp-port-forwarding", "", "TCP port forwarding (e.g. 0.0.0.0:9898-10.10.98.98:80,127.0.0.1:9899-10.10.98.98:80)")
	flag.StringVar(&udpPortForwarding, "udp-port-forwarding", "", "UDP port forwarding (e.g. 127.0.0.1:53-10.10.0.21:53)")
	flag.StringVar(&customDns, "custom-dns", "", "Custom DNS records (e.g. library.nwafu.edu.cn:10.0.0.10)")
	flag.StringVar(&configFile, "config", "", "Config file")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.BoolVar(&atrustAuthInfo, "auth-info", false, "Fetch aTrust authentication information, but not login")
	flag.BoolVar(&atrustTrustDevice, "trust-device", false, "Trust the current device for aTrust with client data, but not connect")
	flag.BoolVar(&atrustUntrustDevice, "untrust-device", false, "Untrust the current device for aTrust with client data, but not connect")

	flag.Parse()

	if showVersion {
		fmt.Printf("%s v%s\n", applicationName, nwafuConnectVersion)
		os.Exit(0)
	}

	if atrustAuthInfo {
		log.SetOutput(io.Discard) // suppress log
		info, err := atrust.GetAuthInfoList(conf.ServerAddress, conf.ServerPort)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Get auth info list error:", err)
			os.Exit(1)
		}
		jsonInfo, err := json.Marshal(info)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error marshaling auth info:", err)
			os.Exit(1)
		}
		fmt.Println(string(jsonInfo))
		os.Exit(0)
	}

	if atrustTrustDevice || atrustUntrustDevice {
		if conf.ClientDataFile == "" {
			fmt.Fprintln(os.Stderr, "Client data file is required for trust/untrust device")
			os.Exit(1)
		}
		clientData, err := os.ReadFile(conf.ClientDataFile)
		if err != nil {
			log.Printf("Read client data file error: %s", err)
			os.Exit(1)
		}

		err = atrust.SetTrusted(conf.ServerAddress, conf.ServerPort, clientData, atrustTrustDevice)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Trust/Untrust device error:", err)
			os.Exit(1)
		}
		if atrustTrustDevice {
			log.Println("Device trusted successfully")
		} else {
			log.Println("Device untrusted successfully")
		}
		os.Exit(0)
	}

	if configFile != "" {
		err := parseTOMLConfig(configFile, &conf)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	} else {
		if tcpPortForwarding != "" {
			forwardingStringList := strings.Split(tcpPortForwarding, ",")
			for _, forwardingString := range forwardingStringList {
				addressStringList := strings.Split(forwardingString, "-")
				if len(addressStringList) != 2 {
					fmt.Fprintf(os.Stderr, "%s: wrong TCP port forwarding format\n", applicationName)
					os.Exit(1)
				}

				conf.PortForwardingList = append(conf.PortForwardingList, configs.SinglePortForwarding{
					NetworkType:   "tcp",
					BindAddress:   addressStringList[0],
					RemoteAddress: addressStringList[1],
				})
			}
		}

		if udpPortForwarding != "" {
			forwardingStringList := strings.Split(udpPortForwarding, ",")
			for _, forwardingString := range forwardingStringList {
				addressStringList := strings.Split(forwardingString, "-")
				if len(addressStringList) != 2 {
					fmt.Fprintf(os.Stderr, "%s: wrong UDP port forwarding format\n", applicationName)
					os.Exit(1)
				}

				conf.PortForwardingList = append(conf.PortForwardingList, configs.SinglePortForwarding{
					NetworkType:   "udp",
					BindAddress:   addressStringList[0],
					RemoteAddress: addressStringList[1],
				})
			}
		}

		if customDns != "" {
			dnsList := strings.Split(customDns, ",")
			for _, dnsString := range dnsList {
				dnsStringSplit := strings.Split(dnsString, ":")
				if len(dnsStringSplit) != 2 {
					fmt.Fprintf(os.Stderr, "%s: wrong custom DNS format\n", applicationName)
					os.Exit(1)
				}

				conf.CustomDNSList = append(conf.CustomDNSList, configs.SingleCustomDNS{
					HostName: dnsStringSplit[0],
					IP:       dnsStringSplit[1],
				})
			}
		}

	}

	missing := conf.ServerAddress == ""
	switch conf.AuthType {
	case "auth/psw":
		missing = missing || conf.Username == "" || conf.Password == ""
	case "auth/smsCheckCode":
		missing = missing || conf.Phone == ""
	}
	if missing {
		missing = conf.SID == "" || conf.DeviceID == "" || conf.ResourceFile == ""
	}
	if missing {
		fmt.Printf("%s: missing required arguments\n", applicationName)
		fmt.Println("Use -auth-info to inspect the server's available aTrust authentication methods.")
		fmt.Println("\nUsage:")
		flag.PrintDefaults()

		os.Exit(1)
	}

}
