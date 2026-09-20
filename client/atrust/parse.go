package atrust

import (
	"bytes"
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
					Apps []struct {
						ID          string
						NodeGroupID string
 EnableTCPPrefL3 bool
 AddrPretend bool
						Name        string
						Description string
						AddressList []struct {
							Protocol string
							Port     string
							Host     string
							IP       []string
						}
					}
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

func parseResourcePort(raw string) (int, int, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, "-")
	if len(parts) < 1 || len(parts) > 2 {
		return 0, 0, fmt.Errorf("invalid port range %q", raw)
	}
	minimum, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("invalid port %q", raw)
	}
	maximum := minimum
	if len(parts) == 2 {
		maximum, err = strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return 0, 0, fmt.Errorf("invalid port range %q", raw)
		}
	}
	if minimum < 1 || maximum > 65535 || minimum > maximum {
		return 0, 0, fmt.Errorf("port range out of bounds %q", raw)
	}
	return minimum, maximum, nil
}

func normalizeNodeAddress(rawAddress, serverAddress string) (string, bool) {
	address := strings.TrimSpace(rawAddress)
	if address == "{{sdpcHost}}" {
		address = strings.TrimSpace(serverAddress)
	}
	if address == "" {
		return "", false
	}
	if host, portText, err := net.SplitHostPort(address); err == nil {
		port, portErr := strconv.Atoi(portText)
		if host == "" || portErr != nil || port < 1 || port > 65535 {
			return "", false
		}
		return net.JoinHostPort(host, portText), true
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
	err := json.Unmarshal(resource, &clientResource)
	if err != nil {
		return err
	}

	ipSetBuilder := netaddr.IPSetBuilder{}
	c.ipResources = make([]client.IPResource, 0)
	c.domainResources = make(map[string]client.DomainResourceSet)
	c.dnsResource = make(map[string][]net.IP)
	c.resources = make([]client.Resource, 0)

	for _, app := range clientResource.Data.AppList.Data.AppInfo {
		for _, appItem := range app.Apps {
			parsedResource := client.Resource{Name: appItem.Name, Description: appItem.Description}
			for _, address := range appItem.AddressList {
				protocol := strings.ToLower(strings.TrimSpace(address.Protocol))
				if protocol != "tcp" && protocol != "udp" && protocol != "all" {
					continue
				}
				hostStr := strings.TrimSpace(address.Host)
				if hostStr == "" {
					log.DebugPrintln("resource host is empty, skipping")
					continue
				}
				portMin, portMax, portErr := parseResourcePort(address.Port)
				if portErr != nil {
					log.DebugPrintf("%v", portErr)
					continue
				}
				var rangeMin, rangeMax net.IP
				isRange := false
				if ipParts := strings.Split(hostStr, "-"); len(ipParts) == 2 {
					candidateMin := net.ParseIP(strings.TrimSpace(ipParts[0]))
					candidateMax := net.ParseIP(strings.TrimSpace(ipParts[1]))
					if candidateMin != nil && candidateMax != nil {
						if candidateMin.To4() == nil || candidateMax.To4() == nil {
							log.DebugPrintf("IPv6 or mixed address range found: %s, skipping", hostStr)
							continue
						}
						if bytes.Compare(candidateMin.To4(), candidateMax.To4()) > 0 {
							log.DebugPrintf("invalid reversed IP range: %s", hostStr)
							continue
						}
						rangeMin, rangeMax, isRange = candidateMin, candidateMax, true
					}
				}

				hostIP := net.ParseIP(hostStr)
				_, ipNet, cidrErr := net.ParseCIDR(hostStr)
				if hostIP != nil && hostIP.To4() == nil {
					log.DebugPrintf("IPv6 address found: %s, skipping", hostStr)
					continue
				}
				if cidrErr == nil && ipNet.IP.To4() == nil {
					log.DebugPrintf("IPv6 CIDR found: %s, skipping", hostStr)
					continue
				}
				if strings.Contains(hostStr, ":") {
					log.DebugPrintf("unsupported resource host: %s", hostStr)
					continue
				}

				isDomain := hostIP == nil && cidrErr != nil && !isRange
				domainKey := ""
				if isDomain {
					domainKey = strings.ToLower(strings.TrimSuffix(hostStr, "."))
					if strings.HasPrefix(domainKey, "*.") {
						domainKey = strings.TrimPrefix(domainKey, "*")
					}
					if domainKey == "" || strings.Contains(domainKey, "*") {
						log.DebugPrintf("unsupported wildcard domain: %s", hostStr)
						continue
					}
				}

				parsedResource.Addresses = append(parsedResource.Addresses, client.ResourceAddress{
					Host:     hostStr,
					PortMin:  portMin,
					PortMax:  portMax,
					Protocol: protocol,
				})
				switch {
				case hostIP != nil:
					ipSetBuilder.Add(netaddr.MustParseIP(hostIP.String()))
					c.ipResources = append(c.ipResources, client.IPResource{
						IPMin:       hostIP,
						IPMax:       hostIP,
						PortMin:     portMin,
						PortMax:     portMax,
						Protocol:    protocol,
						AppID:       appItem.ID,
						NodeGroupID: appItem.NodeGroupID,
 EnableTCPPrefL3: appItem.EnableTCPPrefL3,
					})
					log.DebugPrintf("Add IP: %s, Port range: %d ~ %d, [%s]", hostIP, portMin, portMax, protocol)
				case cidrErr == nil:
					ip4 := ipNet.IP.To4()
					ipMax4 := make(net.IP, len(ip4))
					for i := range ip4 {
						ipMax4[i] = ip4[i] | ^ipNet.Mask[i]
					}
					ipSetBuilder.AddPrefix(netaddr.MustParseIPPrefix(hostStr))
					c.ipResources = append(c.ipResources, client.IPResource{
						IPMin:       ip4.To16(),
						IPMax:       ipMax4.To16(),
						PortMin:     portMin,
						PortMax:     portMax,
						Protocol:    protocol,
						AppID:       appItem.ID,
						NodeGroupID: appItem.NodeGroupID,
 EnableTCPPrefL3: appItem.EnableTCPPrefL3,
					})
					log.DebugPrintf("Add CIDR: %s (%s ~ %s), Port range: %d ~ %d, [%s]", hostStr, ip4, ipMax4, portMin, portMax, protocol)
				case isRange:
					ipSetBuilder.AddRange(netaddr.IPRangeFrom(netaddr.MustParseIP(rangeMin.String()), netaddr.MustParseIP(rangeMax.String())))
					c.ipResources = append(c.ipResources, client.IPResource{
						IPMin:       rangeMin,
						IPMax:       rangeMax,
						PortMin:     portMin,
						PortMax:     portMax,
						Protocol:    protocol,
						AppID:       appItem.ID,
						NodeGroupID: appItem.NodeGroupID,
 EnableTCPPrefL3: appItem.EnableTCPPrefL3,
					})
					log.DebugPrintf("Add IP range: %s ~ %s, Port range: %d ~ %d, [%s]", rangeMin, rangeMax, portMin, portMax, protocol)
				default:
					c.domainResources[domainKey] = append(c.domainResources[domainKey], client.DomainResource{
						PortMin:     portMin,
						PortMax:     portMax,
						Protocol:    protocol,
						AppID:       appItem.ID,
						NodeGroupID: appItem.NodeGroupID,
 EnableTCPPrefL3: appItem.EnableTCPPrefL3,
 AddrPretend: appItem.AddrPretend,
					})
					log.DebugPrintf("Add domain: %s, Port range: %d ~ %d, [%s]", hostStr, portMin, portMax, protocol)
				}

				// Handle DNS rules
				if address.IP != nil {
					if !isDomain {
						log.DebugPrintln("IP address found, but no domain name, skipping")
						continue
					}

					seenIPs := make(map[string]struct{}, len(address.IP))
					for _, ipStr := range address.IP {
						ip := net.ParseIP(strings.TrimSpace(ipStr))
						if ip == nil {
							log.DebugPrintf("Invalid IP: %s", ipStr)
							continue
						}
						ip4 := ip.To4()
						if ip4 == nil {
							log.DebugPrintf("IPv6 address found: %s, skipping", ip)
							continue
						}
						key := ip4.String()
						if _, exists := seenIPs[key]; exists {
							continue
						}
						seenIPs[key] = struct{}{}
						ipSetBuilder.Add(netaddr.MustParseIP(key))
						c.ipResources = append(c.ipResources, client.IPResource{
							IPMin:       ip4.To16(),
							IPMax:       ip4.To16(),
							PortMin:     portMin,
							PortMax:     portMax,
							Protocol:    protocol,
							AppID:       appItem.ID,
							NodeGroupID: appItem.NodeGroupID,
 EnableTCPPrefL3: appItem.EnableTCPPrefL3,
						})
						{
							c.dnsResource[domainKey] = append(c.dnsResource[domainKey], append(net.IP(nil), ip4...))
						}
						log.DebugPrintf("Add DNS rule: %s -> %s", hostStr, ip4)
					}
				}
			}
			if len(parsedResource.Addresses) > 0 {
				c.resources = append(c.resources, parsedResource)
			}
		}
	}

	if clientResource.Data.SDPPolicy.Data.ClientOption.DNSOption.FirstDNS != "" {
		c.dnsServer = clientResource.Data.SDPPolicy.Data.ClientOption.DNSOption.FirstDNS
		log.DebugPrintf("Set DNS server: %s", c.dnsServer)
	} else if clientResource.Data.SDPPolicy.Data.ClientOption.DNSOptionV2.FirstDNS != "" {
		c.dnsServer = clientResource.Data.SDPPolicy.Data.ClientOption.DNSOptionV2.FirstDNS
		log.DebugPrintf("Set DNS server: %s", c.dnsServer)
	} else {
		log.DebugPrintf("No DNS server found")
	}

	c.dnsServers = nil
 for _, server := range []string{clientResource.Data.SDPPolicy.Data.ClientOption.DNSOption.FirstDNS, clientResource.Data.SDPPolicy.Data.ClientOption.DNSOption.SecondDNS, clientResource.Data.SDPPolicy.Data.ClientOption.DNSOptionV2.FirstDNS, clientResource.Data.SDPPolicy.Data.ClientOption.DNSOptionV2.SecondDNS} {
  if server != "" { found := false; for _, existing := range c.dnsServers { if existing == server { found = true } }; if !found { c.dnsServers = append(c.dnsServers, server) } }
 }
	c.MajorNodeGroup = clientResource.Data.AppList.Data.Config.NodeGroupConf.MajorNodeGroup.ID
	c.NodeGroups = make(map[string]NodeGroup)
	for _, nodeGroup := range clientResource.Data.AppList.Data.Config.NodeGroupConf.NodeGroupList {
		addressList := NodeGroup{}
		for _, addressInfo := range nodeGroup.AddressInfo {
			address, ok := normalizeNodeAddress(addressInfo.Address, c.serverAddress)
			if !ok {
				log.Printf("Ignore invalid node address in group %s: %q", nodeGroup.ID, addressInfo.Address)
				continue
			}
			if addressInfo.Type == "lan" { addressList.LAN = append(addressList.LAN, address) } else { addressList.WAN = append(addressList.WAN, address) }

			// Remove ip from ipSetBuilder to prevent circular routing
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				continue
			}
			ip := net.ParseIP(host)
			if ip != nil && ip.To4() != nil {
				ipSetBuilder.Remove(netaddr.MustParseIP(ip.String()))
				log.DebugPrintf("Remove IP from IP set to prevent circular routing: %s", ip)
			}
		}
		c.NodeGroups[nodeGroup.ID] = addressList
		log.DebugPrintf("Node Group ID: %s, Addresses: %v", nodeGroup.ID, addressList)
	}

	c.ipSet, err = ipSetBuilder.IPSet()
	if err != nil {
		return fmt.Errorf("build resource IP set: %w", err)
	}
	c.resourceIndex = ipresource.New(c.ipResources)

	return nil
}
