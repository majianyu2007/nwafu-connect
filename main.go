//go:build !tun

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	"github.com/containers/winquit/pkg/winquit"
	"github.com/majianyu2007/nwafu-connect/client"
	atrustclient "github.com/majianyu2007/nwafu-connect/client/atrust"
	"github.com/majianyu2007/nwafu-connect/configs"
	"github.com/majianyu2007/nwafu-connect/dial"
	"github.com/majianyu2007/nwafu-connect/internal/hook_func"
	"github.com/majianyu2007/nwafu-connect/internal/managedbrowser"
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
	initializeConfig()

	if CommitID != "" {
		log.Println("Start " + applicationName + " v" + nwafuConnectVersion + "-" + CommitID)
	} else {
		log.Println("Start " + applicationName + " v" + nwafuConnectVersion)
	}
	if conf.DebugDump {
		log.EnableDebug()
	}
	if conf.BrowserMode {
		browserPath, err := managedbrowser.FindExecutable(conf.BrowserPath)
		if err != nil {
			log.Fatalf("Managed browser setup error: %s", err)
		}
		conf.BrowserPath = browserPath
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
	if closer, ok := vpnClient.(interface{ Close() }); ok {
		hook_func.RegisterTerminalFunc("CloseVPNClient", func(ctx context.Context) error {
			closer.Close()
			return nil
		})
	}

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
		fatalWithCleanup("VPN client setup error: %s", err)
	}

	if conf.ClientDataFile != "" {
		if err := writePrivateFile(conf.ClientDataFile, clientData); err != nil {
			fatalWithCleanup("Write client data file error: %s", err)
		}
		log.Printf("Client data saved to %s", conf.ClientDataFile)
	}

	log.Printf("VPN client started")

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

	resources, err := vpnClient.Resources()
	if err != nil {
		log.Println("No resource metadata")
	}

	dnsResource, err := vpnClient.DNSResource()
	if err != nil {
		log.Println("No DNS resource")
	}

	var vpnStack stack.Stack
	if conf.TCPTunnelMode {
		vpnStack, err = tcptunnel.NewStack(vpnClient)
		if err != nil {
			fatalWithCleanup("TCP Tunnel stack setup error: %s", err)
		}
	} else if conf.TUNMode && !conf.BrowserMode {
		vpnTUNStack, err := tun.NewStack(vpnClient, conf.DNSHijack, conf.FakeIP, ipResources)
		if err != nil {
			fatalWithCleanup("Tun stack setup error, make sure you are root user: %s", err)
		}

		if conf.AddRoute && ipSet != nil {
			for _, prefix := range ipSet.Prefixes() {
				log.Printf("Add route to %s", prefix.String())
				if routeErr := vpnTUNStack.AddRoute(prefix.String()); routeErr != nil {
					log.Printf("Add route to %s failed: %v", prefix.String(), routeErr)
				}
			}
		}

		if conf.FakeIP {
			if routeErr := vpnTUNStack.AddRoute("198.18.0.0/16"); routeErr != nil {
				log.Printf("Add fake-IP route failed: %v", routeErr)
			}
		}
	} else {
		vpnStack, err = gvisor.NewStack(vpnClient)
		if err != nil {
			fatalWithCleanup("gVisor stack setup error: %s", err)
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
		conf.BrowserMode,
	)
	hook_func.RegisterTerminalFunc("CloseResolver", func(ctx context.Context) error {
		vpnResolver.Close()
		return nil
	})

	for _, customDns := range conf.CustomDNSList {
		ipAddr := net.ParseIP(customDns.IP)
		if ipAddr == nil {
			log.Printf("Custom DNS for host name %s is invalid, SKIP", customDns.HostName)
			continue
		}
		vpnResolver.SetPermanentDNS(customDns.HostName, ipAddr)
		log.Printf("Add custom DNS: %s -> %s\n", customDns.HostName, customDns.IP)
	}
	localResolver := service.NewDnsServer(vpnResolver, []string{remoteDNSServer, conf.SecondaryDNSServer}, conf.DNSTTL)
	vpnStack.SetupResolve(localResolver)
	vpnStack.SetupIPPool(vpnResolver.IPPool)

	stackDone := make(chan error, 1)
	go func() {
		runErr := vpnStack.Run()
		if hook_func.IsTerminal() {
			return
		}
		if runErr == nil {
			runErr = errors.New("VPN network stack stopped unexpectedly")
		}
		stackDone <- runErr
	}()

	vpnDialer := dial.NewDialer(vpnStack, vpnResolver, ipResources, conf.BrowserMode, conf.DialDirectProxy)

	var browserDone <-chan struct{}
	var browserErr error
	if conf.BrowserMode {
		proxyAddress, err := service.StartHTTP("127.0.0.1:0", vpnDialer)
		if err != nil {
			fatalWithCleanup("Managed browser proxy setup error: %s", err)
			return
		}
		startURL := conf.BrowserURL
		if startURL == "" {
			startURL, err = service.StartBrowserHome(resources, proxyAddress)
			if err != nil {
				fatalWithCleanup("Managed browser home page setup error: %s", err)
				return
			}
		}
		browserContext, closeBrowser := context.WithCancel(context.Background())
		browserProcess, err := managedbrowser.Start(browserContext, managedbrowser.Options{
			Executable:   conf.BrowserPath,
			ProxyAddress: proxyAddress,
			StartURL:     startURL,
			ProfileDir:   conf.BrowserProfileDir,
		})
		if err != nil {
			closeBrowser()
			fatalWithCleanup("Managed browser setup error: %s", err)
			return
		}
		done := make(chan struct{})
		go func() {
			browserErr = browserProcess.Wait()
			close(done)
		}()
		browserDone = done
		hook_func.RegisterTerminalFunc("CloseManagedBrowser", func(ctx context.Context) error {
			closeBrowser()
			select {
			case <-done:
				return nil
			case <-ctx.Done():
				return fmt.Errorf("wait for managed browser to close: %w", ctx.Err())
			}
		})
		if conf.BrowserStateFile != "" {
			if err := managedbrowser.WriteState(conf.BrowserStateFile, managedbrowser.State{
				ProxyAddress: proxyAddress,
				StartURL:     startURL,
				Executable:   browserProcess.Executable(),
				ProfileDir:   conf.BrowserProfileDir,
			}); err != nil {
				closeBrowser()
				fatalWithCleanup("Managed browser state setup error: %s", err)
				return
			}
			hook_func.RegisterTerminalFunc("RemoveBrowserState", func(ctx context.Context) error {
				return managedbrowser.RemoveState(conf.BrowserStateFile)
			})
		}
		log.Printf("Managed browser started with %s", browserProcess.Executable())
	} else {
		if conf.DNSServerBind != "" {
			if _, err := service.StartDNS(conf.DNSServerBind, localResolver); err != nil {
				fatalWithCleanup("DNS server setup error: %s", err)
				return
			}
		}
		if conf.TUNMode {
			clientIP, err := vpnClient.IP()
			if err != nil {
				fatalWithCleanup("TUN DNS server setup error: %s", err)
				return
			}
			if _, err := service.StartDNS(net.JoinHostPort(clientIP.String(), "53"), localResolver); err != nil {
				fatalWithCleanup("TUN DNS server setup error: %s", err)
				return
			}
		}
		if conf.SocksBind != "" {
			if _, err := service.StartSocks5(conf.SocksBind, vpnDialer, vpnResolver, conf.SocksUser, conf.SocksPasswd); err != nil {
				fatalWithCleanup("SOCKS5 server setup error: %s", err)
				return
			}
		}
		if conf.HTTPBind != "" {
			if _, err := service.StartHTTP(conf.HTTPBind, vpnDialer); err != nil {
				fatalWithCleanup("HTTP server setup error: %s", err)
				return
			}
		}
		if conf.ShadowsocksURL != "" {
			if _, err := service.StartShadowsocks(vpnDialer, conf.ShadowsocksURL); err != nil {
				fatalWithCleanup("Shadowsocks server setup error: %s", err)
				return
			}
		}
		for _, portForwarding := range conf.PortForwardingList {
			var forwardingErr error
			switch portForwarding.NetworkType {
			case "tcp":
				_, forwardingErr = service.StartTCPForwarding(vpnStack, portForwarding.BindAddress, portForwarding.RemoteAddress)
			case "udp":
				_, forwardingErr = service.StartUDPForwarding(vpnStack, portForwarding.BindAddress, portForwarding.RemoteAddress)
			default:
				log.Printf("Port forwarding: unknown network type %s; skipping", portForwarding.NetworkType)
				continue
			}
			if forwardingErr != nil {
				log.Printf("Port forwarding %s -> %s skipped: %v", portForwarding.BindAddress, portForwarding.RemoteAddress, forwardingErr)
			}
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

	quit := make(chan os.Signal, 1)
	if runtime.GOOS == "windows" {
		signal.Notify(quit, syscall.SIGINT)
		winquit.SimulateSigTermOnQuit(quit)
	} else {
		signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	}
	if browserDone != nil && conf.BrowserStayRunning {
		detachedBrowserDone := browserDone
		browserDone = nil
		go func() {
			<-detachedBrowserDone
			if browserErr != nil {
				log.Printf("Managed browser error: %s", browserErr)
			} else {
				log.Println("Managed browser closed; VPN session remains available from the tray")
			}
		}()
	}
	var runtimeErr error
	select {
	case <-quit:
	case err := <-stackDone:
		runtimeErr = fmt.Errorf("VPN network stack stopped: %w", err)
	case <-browserDone:
		if browserErr != nil {
			runtimeErr = browserErr
		} else {
			log.Println("Managed browser closed")
		}
	}
	signal.Stop(quit)
	log.Printf("Shutdown %s ......", applicationName)
	cleanupErr := errors.Join(hook_func.ExecTerminalFunc(context.Background())...)
	if runtimeErr != nil || cleanupErr != nil {
		log.Fatalf("Shutdown %s failed: %v", applicationName, errors.Join(runtimeErr, cleanupErr))
	}
	log.Printf("Shutdown %s success, Bye~", applicationName)
}
func writePrivateFile(path string, payload []byte) (returnErr error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return fmt.Errorf("create private file directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".nwafu-connect-*")
	if err != nil {
		return fmt.Errorf("create temporary private file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	defer func() {
		if temporary != nil {
			returnErr = errors.Join(returnErr, temporary.Close())
		}
	}()
	if err := temporary.Chmod(0600); err != nil {
		return fmt.Errorf("protect temporary private file: %w", err)
	}
	written, err := temporary.Write(payload)
	if err != nil {
		return fmt.Errorf("write temporary private file: %w", err)
	}
	if written != len(payload) {
		return fmt.Errorf("write temporary private file: wrote %d of %d bytes", written, len(payload))
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync temporary private file: %w", err)
	}
	closeErr := temporary.Close()
	temporary = nil
	if closeErr != nil {
		return fmt.Errorf("close temporary private file: %w", closeErr)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		if _, statErr := os.Stat(path); statErr != nil {
			return fmt.Errorf("publish private file: %w", err)
		}
		if removeErr := os.Remove(path); removeErr != nil {
			return fmt.Errorf("replace private file: %w", removeErr)
		}
		if renameErr := os.Rename(temporaryPath, path); renameErr != nil {
			return fmt.Errorf("publish replacement private file: %w", renameErr)
		}
	}
	if err := os.Chmod(path, 0600); err != nil {
		return fmt.Errorf("protect published private file: %w", err)
	}
	return nil
}

func fatalWithCleanup(format string, args ...any) {
	for _, cleanupErr := range hook_func.ExecTerminalFunc(context.Background()) {
		log.Printf("Cleanup after startup failure: %s", cleanupErr)
	}
	log.Fatalf(format, args...)
}
