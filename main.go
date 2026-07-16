//go:build !tun

package main

import (
	"context"
	"net"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/containers/winquit/pkg/winquit"
	"github.com/majianyu2007/nwafu-connect/client"
	atrustclient "github.com/majianyu2007/nwafu-connect/client/atrust"
	"github.com/majianyu2007/nwafu-connect/configs"
	"github.com/majianyu2007/nwafu-connect/dial"
	"github.com/majianyu2007/nwafu-connect/internal/hook_func"
	"github.com/majianyu2007/nwafu-connect/log"
	"github.com/majianyu2007/nwafu-connect/resolve"
	"github.com/majianyu2007/nwafu-connect/service"
	"github.com/majianyu2007/nwafu-connect/stack"
	"github.com/majianyu2007/nwafu-connect/stack/gvisor"
	"github.com/majianyu2007/nwafu-connect/stack/tcptunnel"
	"github.com/majianyu2007/nwafu-connect/stack/tun"
)

var conf configs.Config

func main() {
	log.Init()

	if CommitID != "" {
		log.Println("Start " + applicationName + " v" + nwafuConnectVersion + "-" + CommitID)
	} else {
		log.Println("Start " + applicationName + " v" + nwafuConnectVersion)
	}
	if conf.DebugDump {
		log.EnableDebug()
	}

	if errs := hook_func.ExecInitialFunc(context.Background(), conf); errs != nil {
		for _, err := range errs {
			log.Printf("Initial %s failed: %s", applicationName, err)
		}
		os.Exit(1)
	}

	var vpnClient client.Client
	var err error
	var resourceData []byte

	if conf.ResourceFile != "" {
		resourceData, err = os.ReadFile(conf.ResourceFile)
		if err != nil {
			log.Fatalf("Read resource file error: %s", err)
		}
	}

	var clientData []byte
	if conf.ClientDataFile != "" {
		clientData, err = os.ReadFile(conf.ClientDataFile)
		if err != nil {
			log.Printf("Read client data file error: %s", err)
			log.Println("Will create a new client data file if log in successfully")
		}
	}

	vpnClient = atrustclient.NewClient(conf.Username, conf.SID, conf.DeviceID, conf.SignKey)

	log.Println("VPN protocol: aTrust")
	clientData, err = vpnClient.(*atrustclient.Client).Setup(
		conf.ServerAddress,
		conf.ServerPort,
		conf.Username,
		conf.Password,
		conf.TOTPSecret,
		conf.Phone,
		conf.LoginDomain,
		conf.AuthType,
		conf.GraphCodeFile,
		conf.QYWechatQRCodeFile,
		conf.QYWechatQRCodeTerminal,
		conf.QYWechatQRCodeBrowser,
		clientData,
		resourceData,
		conf.UpdateBestNodesInterval,
	)
	if err != nil {
		log.Fatalf("VPN client setup error: %s", err)
	}

	if conf.ClientDataFile != "" {
		err = os.WriteFile(conf.ClientDataFile, clientData, 0600)
		if err != nil {
			log.Fatalf("Write client data file error: %s", err)
		}
		if err := os.Chmod(conf.ClientDataFile, 0600); err != nil {
			log.Fatalf("Secure client data file error: %s", err)
		}
		log.Printf("Client data saved to %s", conf.ClientDataFile)
	}

	log.Printf("VPN client started")
	if closer, ok := vpnClient.(interface{ Close() }); ok {
		hook_func.RegisterTerminalFunc("CloseVPNClient", func(ctx context.Context) error {
			closer.Close()
			return nil
		})
	}

	ipResources, err := vpnClient.IPResources()
	if err != nil {
		log.Println("No IP resources")
	}

	ipSet, err := vpnClient.IPSet()
	if err != nil {
		log.Println("No IP set")
	}

	domainResources, err := vpnClient.DomainResources()
	if err != nil {
		log.Println("No domain resources")
	}

	dnsResource, err := vpnClient.DNSResource()
	if err != nil {
		log.Println("No DNS resource")
	}

	var vpnStack stack.Stack
	if conf.TCPTunnelMode {
		vpnStack, err = tcptunnel.NewStack(vpnClient)
		if err != nil {
			log.Fatalf("TCP Tunnel stack setup error: %s", err)
		}
	} else if conf.TUNMode {
		vpnTUNStack, err := tun.NewStack(vpnClient, conf.DNSHijack, conf.FakeIP, ipResources)
		if err != nil {
			log.Fatalf("Tun stack setup error, make sure you are root user : %s", err)
		}

		if conf.AddRoute && ipSet != nil {
			for _, prefix := range ipSet.Prefixes() {
				log.Printf("Add route to %s", prefix.String())
				_ = vpnTUNStack.AddRoute(prefix.String())
			}
		}

		if conf.FakeIP {
			_ = vpnTUNStack.AddRoute("198.18.0.0/16")
		}

		vpnStack = vpnTUNStack
	} else {
		vpnStack, err = gvisor.NewStack(vpnClient)
		if err != nil {
			log.Fatalf("gVisor stack setup error: %s", err)
		}
	}

	useRemoteDNS := !conf.DisableRemoteDNS
	remoteDNSServer := conf.RemoteDNSServer
	if useRemoteDNS && remoteDNSServer == "auto" {
		remoteDNSServer, err = vpnClient.DNSServer()
		if err != nil {
			useRemoteDNS = false
			remoteDNSServer = "10.10.0.21"
			log.Println("No DNS server provided by server. Disable remote DNS")
		} else {
			log.Printf("Use DNS server %s provided by server", remoteDNSServer)
		}
	}

	vpnResolver := resolve.NewResolver(
		vpnStack,
		remoteDNSServer,
		conf.SecondaryDNSServer,
		conf.DNSTTL,
		domainResources,
		dnsResource,
		useRemoteDNS,
	)
	hook_func.RegisterTerminalFunc("CloseResolver", func(ctx context.Context) error {
		vpnResolver.Close()
		return nil
	})

	for _, customDns := range conf.CustomDNSList {
		ipAddr := net.ParseIP(customDns.IP)
		if ipAddr == nil {
			log.Printf("Custom DNS for host name %s is invalid, SKIP", customDns.HostName)
		}
		vpnResolver.SetPermanentDNS(customDns.HostName, ipAddr)
		log.Printf("Add custom DNS: %s -> %s\n", customDns.HostName, customDns.IP)
	}
	localResolver := service.NewDnsServer(vpnResolver, []string{remoteDNSServer, conf.SecondaryDNSServer})
	vpnStack.SetupResolve(localResolver)
	vpnStack.SetupIPPool(vpnResolver.IPPool)

	go vpnStack.Run()

	vpnDialer := dial.NewDialer(vpnStack, vpnResolver, ipResources, false, conf.DialDirectProxy)

	if conf.DNSServerBind != "" {
		go service.ServeDNS(conf.DNSServerBind, localResolver)
	}
	if conf.TUNMode {
		clientIP, _ := vpnClient.IP()
		go service.ServeDNS(clientIP.String()+":53", localResolver)
	}

	if conf.SocksBind != "" {
		go service.ServeSocks5(conf.SocksBind, vpnDialer, vpnResolver, conf.SocksUser, conf.SocksPasswd)
	}

	if conf.HTTPBind != "" {
		go service.ServeHTTP(conf.HTTPBind, vpnDialer)
	}

	if conf.ShadowsocksURL != "" {
		go service.ServeShadowsocks(vpnDialer, conf.ShadowsocksURL)
	}

	for _, portForwarding := range conf.PortForwardingList {
		switch portForwarding.NetworkType {
		case "tcp":
			go service.ServeTCPForwarding(vpnStack, portForwarding.BindAddress, portForwarding.RemoteAddress)
		case "udp":
			go service.ServeUDPForwarding(vpnStack, portForwarding.BindAddress, portForwarding.RemoteAddress)
		default:
			log.Printf("Port forwarding: unknown network type %s. Aborting", portForwarding.NetworkType)
		}
	}

	if !conf.DisableKeepAlive {
		if conf.KeepAliveURL == "" && !useRemoteDNS {
			log.Println("Keep alive is disabled because remote DNS is disabled, and no KeepAliveURL is provided")
		} else {
			keepAliveCtx, keepAliveCancel := context.WithCancel(context.Background())
			hook_func.RegisterTerminalFunc("CloseKeepAlive", func(ctx context.Context) error {
				keepAliveCancel()
				return nil
			})
			go service.KeepAlive(keepAliveCtx, vpnResolver, vpnDialer, conf.KeepAliveURL)
		}
	}

	if runtime.GOOS == "windows" {
		done := make(chan os.Signal, 1)
		signal.Notify(done, syscall.SIGINT)
		winquit.SimulateSigTermOnQuit(done)
		<-done
	} else {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
		<-quit
	}
	log.Printf("Shutdown %s ......", applicationName)
	if errs := hook_func.ExecTerminalFunc(context.Background()); errs != nil {
		for _, err := range errs {
			log.Printf("Shutdown %s failed: %s", applicationName, err)
		}
	} else {
		log.Printf("Shutdown %s success, Bye~", applicationName)
	}
}
