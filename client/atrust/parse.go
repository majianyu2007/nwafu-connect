package atrust

import (
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/majianyu2007/nwafu-connect/client"
	"github.com/majianyu2007/nwafu-connect/internal/ipresource"
	"github.com/majianyu2007/nwafu-connect/log"
	"inet.af/netaddr"
)

type ClientResource struct {
	Data struct {
		AppList struct {
			Data struct {
				AppInfo []struct {
					Apps []resourceApp
				}

				Config struct {
					NodeGroupConf struct {
						MajorNodeGroup struct {
							ID string
						}
						NodeGroupList []struct {
							AddressInfo []struct {
								Address string
								Type    string
							}
							ID string
						}
					}
				}
			}
		}

		SDPPolicy struct {
			Data struct {
				ClientOption struct {
					Tun0RTT struct {
						MaxIdleConnNum    int64 `json:"maxIdleConnNum"`
						MaxIdleLingerTime int64 `json:"maxIdleLingerTime"`
						MinIdleConnNum    int64 `json:"minIdleConnNum"`
						Enable            bool  `json:"enable"`
						PreConnNum        int64 `json:"preConnNum"`
					} `json:"tun0rtt"`

					DNSOption struct {
						FirstDNS  string
						SecondDNS string
					}

					DNSOptionV2 struct {
						FirstDNS  string
						SecondDNS string
					}
				}
			}
		}
	}
}

type resourceApp struct {
	ID              string
	Name            string
	Description     string
	NodeGroupID     string
	AccessModel     string
	EnableTCPPrefL3 bool
	AddrPretend     any `json:"addrPretend"`
	AddressList     []resourceAddress
}

type resourceAddress struct {
	Protocol string
	Port     string
	Host     string
	IP       []string
}

func resourceAddrPretend(value any) bool {
	if value, ok := value.(bool); ok {
		return value
	}
	return true
}

func parseResourcePort(portStr string) (int, int, bool) {
	portStr = strings.TrimSpace(portStr)
	if strings.Contains(portStr, "-") {
		ports := strings.Split(portStr, "-")
		if len(ports) != 2 {
			return 0, 0, false
		}
		portMin, err := strconv.Atoi(strings.TrimSpace(ports[0]))
		if err != nil {
			return 0, 0, false
		}
		portMax, err := strconv.Atoi(strings.TrimSpace(ports[1]))
		if err != nil {
			return 0, 0, false
		}
		if portMin < 1 || portMax > 65535 || portMin > portMax {
			return 0, 0, false
		}
		return portMin, portMax, true
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return 0, 0, false
	}
	return port, port, true
}

func normalizeNodeAddress(raw, serverAddress string) (string, bool) {
	address := strings.TrimSpace(raw)
	if address == "" {
		return "", false
	}
	if strings.Contains(address, "{{sdpcHost}}") {
		if serverAddress == "" {
			return "", false
		}
		address = strings.ReplaceAll(address, "{{sdpcHost}}", serverAddress)
	}
	if host, portText, err := net.SplitHostPort(address); err == nil {
		port, err := strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 || strings.TrimSpace(host) == "" {
			return "", false
		}
		return net.JoinHostPort(strings.TrimSpace(host), strconv.Itoa(port)), true
	}
	if ip := net.ParseIP(strings.Trim(address, "[]")); ip != nil {
		return net.JoinHostPort(ip.String(), "441"), true
	}
	if strings.Contains(address, ":") {
		return "", false
	}
	return net.JoinHostPort(address, "441"), true
}

func (c *Client) parseResource(resource []byte) error {
	log.Println("Parsing resource...")

	var clientResource ClientResource
	if err := json.Unmarshal(resource, &clientResource); err != nil {
		return err
	}

	ipSetBuilder := netaddr.IPSetBuilder{}
	c.ipResources = make([]client.IPResource, 0)
	c.domainResources = make(client.DomainResources)
	c.resources = make([]client.Resource, 0)
	c.dnsResource = make(map[string][]net.IP)

	for _, app := range clientResource.Data.AppList.Data.AppInfo {
		for _, appItem := range app.Apps {
			if appItem.AccessModel != "" && !strings.EqualFold(appItem.AccessModel, "L3VPN") {
				log.DebugPrintf("Ignore unsupported aTrust access model %q for app %s", appItem.AccessModel, appItem.ID)
				continue
			}
			addrPretend := resourceAddrPretend(appItem.AddrPretend)
			portalAddresses := make([]client.ResourceAddress, 0, len(appItem.AddressList))

			for _, address := range appItem.AddressList {
				protocol := strings.ToLower(strings.TrimSpace(address.Protocol))
				if protocol != "tcp" && protocol != "udp" && protocol != "all" {
					continue
				}
				portMin, portMax, ok := parseResourcePort(address.Port)
				if !ok {
					log.DebugPrintf("invalid port: %s", address.Port)
					continue
				}

				hostStr := strings.TrimSpace(address.Host)
				if hostStr == "" {
					continue
				}

				isDomain := false
				recordedHost := ""

				if ip := net.ParseIP(hostStr); ip != nil {
					ip4 := ip.To4()
					if ip4 == nil {
						log.DebugPrintf("IPv6 address found: %s, skipping", hostStr)
						continue
					}
					addr, ok := netaddr.FromStdIP(ip4)
					if ok {
						ipSetBuilder.Add(addr)
					}
					c.ipResources = append(c.ipResources, client.IPResource{
						IPMin:           ip4.To16(),
						IPMax:           ip4.To16(),
						PortMin:         portMin,
						PortMax:         portMax,
						Protocol:        protocol,
						AppID:           appItem.ID,
						NodeGroupID:     appItem.NodeGroupID,
						EnableTCPPrefL3: appItem.EnableTCPPrefL3,
					})
					recordedHost = ip4.String()
					log.DebugPrintf("Add IP: %s, Port range: %d ~ %d, [%s]", ip4, portMin, portMax, protocol)
				} else if _, ipNet, cidrErr := net.ParseCIDR(hostStr); cidrErr == nil {
					ip4 := ipNet.IP.To4()
					if ip4 == nil {
						log.DebugPrintf("IPv6 CIDR found: %s, skipping", hostStr)
						continue
					}
					ipMax4 := make(net.IP, len(ip4))
					for i := range ip4 {
						ipMax4[i] = ip4[i] | ^ipNet.Mask[i]
					}
					if prefix, parseErr := netaddr.ParseIPPrefix(hostStr); parseErr == nil {
						ipSetBuilder.AddPrefix(prefix)
					}
					c.ipResources = append(c.ipResources, client.IPResource{
						IPMin:           ip4.To16(),
						IPMax:           ipMax4.To16(),
						PortMin:         portMin,
						PortMax:         portMax,
						Protocol:        protocol,
						AppID:           appItem.ID,
						NodeGroupID:     appItem.NodeGroupID,
						EnableTCPPrefL3: appItem.EnableTCPPrefL3,
					})
					recordedHost = hostStr
					log.DebugPrintf("Add CIDR: %s (%s ~ %s), Port range: %d ~ %d, [%s]", hostStr, ip4, ipMax4, portMin, portMax, protocol)
				} else if ipParts := strings.Split(hostStr, "-"); len(ipParts) == 2 &&
					net.ParseIP(strings.TrimSpace(ipParts[0])) != nil &&
					net.ParseIP(strings.TrimSpace(ipParts[1])) != nil {
					ipMin := net.ParseIP(strings.TrimSpace(ipParts[0])).To4()
					ipMax := net.ParseIP(strings.TrimSpace(ipParts[1])).To4()
					if ipMin == nil || ipMax == nil {
						log.DebugPrintf("IPv6 range found: %s, skipping", hostStr)
						continue
					}
					start, startOK := ipresource.IPv4Uint32(ipMin)
					end, endOK := ipresource.IPv4Uint32(ipMax)
					if !startOK || !endOK || start > end {
						log.DebugPrintf("invalid IP range: %s", hostStr)
						continue
					}
					fromAddr, ok1 := netaddr.FromStdIP(ipMin)
					toAddr, ok2 := netaddr.FromStdIP(ipMax)
					if ok1 && ok2 {
						ipSetBuilder.AddRange(netaddr.IPRangeFrom(fromAddr, toAddr))
					}
					c.ipResources = append(c.ipResources, client.IPResource{
						IPMin:           ipMin.To16(),
						IPMax:           ipMax.To16(),
						PortMin:         portMin,
						PortMax:         portMax,
						Protocol:        protocol,
						AppID:           appItem.ID,
						NodeGroupID:     appItem.NodeGroupID,
						EnableTCPPrefL3: appItem.EnableTCPPrefL3,
					})
					recordedHost = ipMin.String() + "-" + ipMax.String()
					log.DebugPrintf("Add IP range: %s ~ %s, Port range: %d ~ %d, [%s]", ipMin, ipMax, portMin, portMax, protocol)
				} else {
					if strings.ContainsAny(hostStr, ":/ \t\r\n") {
						log.DebugPrintf("invalid domain host: %s", hostStr)
						continue
					}
					domainKey := normalizePolicyDomain(hostStr)
					if domainKey == "" {
						log.DebugPrintf("invalid wildcard domain: %s", hostStr)
						continue
					}
					isDomain = true
					c.domainResources[domainKey] = append(c.domainResources[domainKey], client.DomainResource{
						PortMin:         portMin,
						PortMax:         portMax,
						Protocol:        protocol,
						AppID:           appItem.ID,
						NodeGroupID:     appItem.NodeGroupID,
						EnableTCPPrefL3: appItem.EnableTCPPrefL3,
						AddrPretend:     addrPretend,
					})
					recordedHost = strings.TrimSuffix(strings.ToLower(hostStr), ".")
					log.DebugPrintf("Add domain: %s, Port range: %d ~ %d, [%s]", domainKey, portMin, portMax, protocol)
				}

				if recordedHost != "" {
					portalAddresses = append(portalAddresses, client.ResourceAddress{
						Host:     recordedHost,
						PortMin:  portMin,
						PortMax:  portMax,
						Protocol: protocol,
					})
				}

				if isDomain && len(address.IP) > 0 {
					domainKey := normalizePolicyDomain(hostStr)
					if domainKey == "" || strings.HasPrefix(domainKey, ".") {
						continue
					}
					seenIPs := make(map[string]struct{}, len(address.IP))
					for _, existing := range c.dnsResource[domainKey] {
						seenIPs[existing.String()] = struct{}{}
					}
					for _, ipStr := range address.IP {
						parsedIP := net.ParseIP(strings.TrimSpace(ipStr))
						if parsedIP == nil {
							continue
						}
						ip4 := parsedIP.To4()
						if ip4 == nil {
							continue
						}
						ipKey := ip4.String()
						if _, dup := seenIPs[ipKey]; dup {
							continue
						}
						seenIPs[ipKey] = struct{}{}
						c.dnsResource[domainKey] = append(c.dnsResource[domainKey], ip4)
						if addr, ok := netaddr.FromStdIP(ip4); ok {
							ipSetBuilder.Add(addr)
						}
						c.ipResources = append(c.ipResources, client.IPResource{
							IPMin:           ip4.To16(),
							IPMax:           ip4.To16(),
							PortMin:         portMin,
							PortMax:         portMax,
							Protocol:        protocol,
							AppID:           appItem.ID,
							NodeGroupID:     appItem.NodeGroupID,
							EnableTCPPrefL3: appItem.EnableTCPPrefL3,
						})
						log.DebugPrintf("Add DNS: %s -> %s", domainKey, ip4)
					}
				}
			}

			if len(portalAddresses) > 0 {
				c.resources = append(c.resources, client.Resource{
					Name:        strings.TrimSpace(appItem.Name),
					Description: strings.TrimSpace(appItem.Description),
					Addresses:   portalAddresses,
				})
			}
		}
	}

	// DNS option
	c.dnsServer = ""
	c.dnsServers = nil
	dnsOption := clientResource.Data.SDPPolicy.Data.ClientOption.DNSOptionV2
	if strings.TrimSpace(dnsOption.FirstDNS) == "" && strings.TrimSpace(dnsOption.SecondDNS) == "" {
		dnsOption = clientResource.Data.SDPPolicy.Data.ClientOption.DNSOption
	}
	for _, candidate := range []string{dnsOption.FirstDNS, dnsOption.SecondDNS} {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		if net.ParseIP(candidate) == nil {
			continue
		}
		if c.dnsServer == "" {
			c.dnsServer = candidate
		}
		c.dnsServers = append(c.dnsServers, candidate)
	}
	if len(c.dnsServers) > 0 {
		log.DebugPrintf("DNS servers: %v", c.dnsServers)
	}

	// Node groups
	c.MajorNodeGroup = clientResource.Data.AppList.Data.Config.NodeGroupConf.MajorNodeGroup.ID
	log.DebugPrintf("Major Node Group ID: %s", c.MajorNodeGroup)
	c.NodeGroups = make(map[string]NodeGroup)
	for _, nodeGroup := range clientResource.Data.AppList.Data.Config.NodeGroupConf.NodeGroupList {
		var addresses NodeGroup
		for _, addressInfo := range nodeGroup.AddressInfo {
			address, ok := normalizeNodeAddress(addressInfo.Address, c.serverAddress)
			if !ok {
				log.DebugPrintf("Ignore malformed node address %q", addressInfo.Address)
				continue
			}

			switch strings.ToLower(strings.TrimSpace(addressInfo.Type)) {
			case "wan", "":
				addresses.WAN = append(addresses.WAN, address)
			case "lan":
				addresses.LAN = append(addresses.LAN, address)
			default:
				log.DebugPrintf("Ignore node with unsupported type %q: %s", addressInfo.Type, address)
				continue
			}

			// Remove ip from ipSetBuilder to prevent circular routing
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				continue
			}
			if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
				if addr, ok := netaddr.FromStdIP(ip.To4()); ok {
					ipSetBuilder.Remove(addr)
					log.DebugPrintf("Remove IP from IP set to prevent circular routing: %s", ip)
				}
			}
		}
		c.NodeGroups[nodeGroup.ID] = addresses
		log.DebugPrintf(
			"Node Group ID: %s, WAN Addresses: %v, LAN Addresses: %v",
			nodeGroup.ID,
			addresses.WAN,
			addresses.LAN,
		)
	}

	ipSet, err := ipSetBuilder.IPSet()
	if err != nil {
		return fmt.Errorf("build IP set: %w", err)
	}
	c.ipSet = ipSet
	c.resourceIndex = ipresource.New(c.ipResources)

	return nil
}

func normalizePolicyDomain(domain string) string {
	domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if domain == "" {
		return ""
	}
	if strings.HasPrefix(domain, "*.") {
		rest := domain[2:]
		if rest == "" || strings.Contains(rest, "*") {
			return ""
		}
		return "." + rest
	}
	if strings.Contains(domain, "*") {
		return ""
	}
	return domain
}
