package atrust

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/client/atrust/auth"
	"github.com/majianyu2007/nwafu-connect/internal/ipresource"
	"github.com/majianyu2007/nwafu-connect/log"
	"inet.af/netaddr"
)

type Client struct {
	Username     string
	SID          string
	DeviceID     string
	ConnectionID string
	SignKey      string

	serverAddress   string
	ipResources     []client.IPResource
	resourceIndex   *ipresource.Index
	domainResources map[string]client.DomainResourceSet
	resources       []client.Resource
	ipSet           *netaddr.IPSet
	dnsResource     map[string]net.IP
	dnsServer       string

	MajorNodeGroup   string
	NodeGroups       map[string][]string
	BestNodes        map[string]string
	BestNodesRWMutex sync.RWMutex

	ip net.IP // Client IP

	l3Tunnel   *L3Tunnel
	l3TunnelMu sync.Mutex

	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
	closeOnce       sync.Once
}

func NewClient(username, sid, deviceID, signKey string) *Client {
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	return &Client{
		Username:        username,
		SID:             sid,
		DeviceID:        deviceID,
		SignKey:         signKey,
		lifecycleCtx:    lifecycleCtx,
		lifecycleCancel: lifecycleCancel,
	}
}

func (c *Client) Close() {
	c.closeOnce.Do(func() {
		c.lifecycleCancel()
		c.l3TunnelMu.Lock()
		tunnel := c.l3Tunnel
		c.l3TunnelMu.Unlock()
		if tunnel != nil {
			tunnel.Close()
		}
	})
}

func (c *Client) IP() (net.IP, error) {
	if c.ip == nil {
		return nil, errors.New("IP not available")
	}

	return c.ip.To4(), nil
}

func (c *Client) IPSet() (*netaddr.IPSet, error) {
	if c.ipSet == nil {
		return nil, errors.New("IP set not available")
	}

	return c.ipSet, nil
}

func (c *Client) IPResources() ([]client.IPResource, error) {
	if c.ipResources == nil {
		return nil, errors.New("IP resources not available")
	}

	return c.ipResources, nil
}

func (c *Client) DomainResources() (map[string]client.DomainResourceSet, error) {
	if c.domainResources == nil {
		return nil, errors.New("domain resources not available")
	}

	return c.domainResources, nil
}

func (c *Client) Resources() ([]client.Resource, error) {
	if c.resources == nil {
		return nil, errors.New("resources not available")
	}
	return c.resources, nil
}

func (c *Client) DNSResource() (map[string]net.IP, error) {
	if c.dnsResource == nil {
		return nil, errors.New("DNS resource not available")
	}

	return c.dnsResource, nil
}

func (c *Client) DNSServer() (string, error) {
	if c.dnsServer == "" {
		return "", errors.New("DNS server not available")
	}

	return c.dnsServer, nil
}

func randHex(n int) string {
	numBytes := (n + 1) / 2
	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return strings.ToUpper(hex.EncodeToString(b)[:n])
}

func formatServerHost(serverAddress string, serverPort int) string {
	host := strings.Trim(strings.TrimSpace(serverAddress), "[]")
	if serverPort == 443 {
		if ip := net.ParseIP(host); ip != nil && strings.Contains(host, ":") {
			return "[" + host + "]"
		}
		return host
	}
	return net.JoinHostPort(host, strconv.Itoa(serverPort))
}

func GetAuthInfoList(serverAddress string, serverPort int) ([]auth.AuthInfo, error) {
	serverHost := formatServerHost(serverAddress, serverPort)
	sess := auth.NewSession(serverHost)
	return sess.GetAuthInfoList()
}

func (c *Client) CanUseTCPTunnel() bool {
	return true
}

func (c *Client) NewL3Conn() (io.ReadWriteCloser, error) {
	c.l3TunnelMu.Lock()
	tunnel := c.l3Tunnel
	c.l3TunnelMu.Unlock()
	if tunnel == nil {
		return nil, errors.New("L3 tunnel not initialized")
	}
	return tunnel.NewL3Conn()
}

func SetTrusted(serverAddress string, serverPort int, authData []byte, trusted bool) error {
	var clientAuthData auth.ClientAuthData
	if authData != nil {
		err := json.Unmarshal(authData, &clientAuthData)
		if err != nil {
			log.Println("Error parsing client data:", err)
			return err
		}
	}
	log.DebugPrintf("Given auth data: %+v", clientAuthData)

	if clientAuthData.DeviceID == "" {
		clientAuthData.DeviceID = strings.ToLower(randHex(32))
	}

	serverHost := formatServerHost(serverAddress, serverPort)
	sess := auth.NewSession(serverHost)

	if _, err := sess.Login(nil, auth.LoginOptions{
		DeviceID: clientAuthData.DeviceID,
		Cookies:  clientAuthData.Cookies,
	}); err != nil {
		return fmt.Errorf("restore aTrust session for device trust update: %w", err)
	}
	result, err := sess.QueryDevice()
	if err != nil {
		return err
	}

	if trusted {
		if result.DeviceTrusted {
			log.Println("Device already trusted, skipping")
			return nil
		}
		return sess.TrustDevice([]string{result.SelfID})
	} else {
		if !result.DeviceTrusted {
			log.Println("Device already untrusted, skipping")
			return nil
		}
		return sess.UntrustDevice([]string{result.SelfID})
	}
}

func (c *Client) Setup(serverAddress string, serverPort int, username, password, totpSecret, phone, loginDomain, authType, graphCodeFile, qyWechatQRCodeFile string, qyWechatQRCodeTerminal, qyWechatQRCodeBrowser bool, authData, resourceData []byte, updateBestNodesInterval int) ([]byte, error) {
	c.serverAddress = serverAddress

	if c.SID != "" && c.DeviceID != "" && resourceData != nil {
		log.Println("Skipping login")

		c.ConnectionID = buildConnectionID(c.DeviceID)
		if c.SignKey == "" {
			c.SignKey = randHex(64)
		}
	} else {
		var clientAuthData auth.ClientAuthData
		if authData != nil {
			err := json.Unmarshal(authData, &clientAuthData)
			if err != nil {
				log.Println("Error parsing client data:", err)
				return nil, err
			}
		}
		log.DebugPrintf("Given auth data: %+v", clientAuthData)

		if clientAuthData.DeviceID == "" {
			clientAuthData.DeviceID = strings.ToLower(randHex(32))
		}
		c.DeviceID = clientAuthData.DeviceID
		c.ConnectionID = buildConnectionID(c.DeviceID)
		c.SignKey = randHex(64)

		serverHost := formatServerHost(serverAddress, serverPort)
		sess := auth.NewSession(serverHost)

		var err error
		var loginMethod auth.LoginMethod
		switch authType {
		case "auth/psw":
			loginMethod = auth.PasswordLogin{
				Username:      username,
				Password:      password,
				Domain:        loginDomain,
				GraphCodeFile: graphCodeFile,
			}
		case "auth/smsCheckCode":
			loginMethod = auth.SMSLogin{
				Phone:         phone,
				Domain:        loginDomain,
				GraphCodeFile: graphCodeFile,
			}
		case "auth/qywechat":
			loginMethod = auth.QYWechatLogin{
				Domain:      loginDomain,
				QRCodeFile:  qyWechatQRCodeFile,
				PrintQRCode: qyWechatQRCodeTerminal,
				OpenBrowser: qyWechatQRCodeBrowser,
			}
		case "":
			log.Println("No auth type specified, trying to skip auth")
		default:
			return nil, fmt.Errorf("unsupported auth type: %s", authType)
		}

		loginResult, err := sess.Login(loginMethod, auth.LoginOptions{
			DeviceID:   c.DeviceID,
			Cookies:    clientAuthData.Cookies,
			TOTPSecret: totpSecret,
		})
		if err != nil {
			log.Println("Login error:", err)
			return nil, err
		}
		c.Username = loginResult.Username
		c.SID = loginResult.SID
		if c.SID == "" {
			return nil, errors.New("login succeeded without an aTrust session ID")
		}
		clientAuthData.Cookies = loginResult.Cookies

		resourceData, err = sess.ClientResource()
		if err != nil {
			log.Println("Error fetching client resource:", err)
			return nil, err
		}

		authData, err = json.Marshal(clientAuthData)
		if err != nil {
			return nil, fmt.Errorf("encode client authentication data: %w", err)
		}
	}

	err := c.parseResource(resourceData)
	if err != nil {
		return nil, err
	}

	log.DebugPrintf("SID: %s, DeviceID: %s, ConnectionID: %s, SignKey: %s", c.SID, c.DeviceID, c.ConnectionID, c.SignKey)

	c.BestNodes = getBestNodes(c.NodeGroups)

	err = c.getIP()
	if err != nil {
		return nil, err
	}

	l3Tunnel, err := NewL3Tunnel(c)
	if err != nil {
		return nil, fmt.Errorf("failed to create L3 tunnel: %w", err)
	}
	c.l3TunnelMu.Lock()
	c.l3Tunnel = l3Tunnel
	c.l3TunnelMu.Unlock()

	if updateBestNodesInterval > 0 {
		go c.updateBestNodes(c.lifecycleCtx, updateBestNodesInterval)
	}

	return authData, nil
}

func buildConnectionID(deviceID string) string {
	sum := md5.Sum([]byte(deviceID))
	return fmt.Sprintf("%X-%d", sum, time.Now().UnixMicro())
}
