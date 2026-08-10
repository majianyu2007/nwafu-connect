package atrust

import (
	"context"
	"net"
	"strconv"
	"time"

	"github.com/majianyu2007/nwafu-connect/internal/ping"
	"github.com/majianyu2007/nwafu-connect/log"
)

const pingNum = 3

type nodeCandidate struct {
	address string
	host    string
	port    int
}

type nodeGroupResult struct {
	group   string
	address string
}

func getBestNodes(nodeGroups map[string][]string) map[string]string {
	bestNodes := make(map[string]string, len(nodeGroups))
	results := make(chan nodeGroupResult, len(nodeGroups))
	for group, nodes := range nodeGroups {
		go func(group string, nodes []string) {
			results <- nodeGroupResult{group: group, address: getBestNode(group, nodes)}
		}(group, nodes)
	}
	for range nodeGroups {
		result := <-results
		if result.address != "" {
			bestNodes[result.group] = result.address
		}
	}
	return bestNodes
}

func getBestNode(group string, nodes []string) string {
	candidates := make([]nodeCandidate, 0, len(nodes))
	for _, node := range nodes {
		host, portString, err := net.SplitHostPort(node)
		if err != nil || host == "" {
			log.Printf("Ignore invalid node address in group %s: %q", group, node)
			continue
		}
		port, err := strconv.Atoi(portString)
		if err != nil || port < 1 || port > 65535 {
			log.Printf("Ignore invalid node address in group %s: %q", group, node)
			continue
		}
		candidates = append(candidates, nodeCandidate{address: node, host: host, port: port})
	}
	if len(candidates) == 0 {
		log.Printf("No valid node in group %s", group)
		return ""
	}
	if len(candidates) == 1 {
		return candidates[0].address
	}

	pingList := make([]*ping.TCPing, 0, len(candidates))
	doneList := make([]<-chan struct{}, 0, len(candidates))
	for _, candidate := range candidates {
		tcping := ping.NewTCPing()
		tcping.SetTarget(&ping.Target{
			Protocol: ping.TCP,
			Host:     candidate.host,
			Port:     candidate.port,
			Counter:  pingNum,
			Interval: 500 * time.Millisecond,
			Timeout:  time.Second,
		})
		pingList = append(pingList, tcping)
		doneList = append(doneList, tcping.Start())
	}
	for _, done := range doneList {
		<-done
	}

	bestIndex := -1
	bestSuccesses := 0
	var bestLatency time.Duration
	for index, tcping := range pingList {
		result := tcping.Result()
		if result.SuccessCounter == 0 {
			continue
		}
		latency := result.Avg()
		if result.SuccessCounter > bestSuccesses ||
			(result.SuccessCounter == bestSuccesses && (bestIndex < 0 || latency < bestLatency)) {
			bestIndex = index
			bestSuccesses = result.SuccessCounter
			bestLatency = latency
		}
	}
	if bestIndex < 0 {
		log.Printf("No reachable node in group %s, using the first valid node", group)
		return candidates[0].address
	}
	bestNode := candidates[bestIndex].address
	log.Printf("Best node in group %s: %s with %d/%d probes and latency %d ms", group, bestNode, bestSuccesses, pingNum, bestLatency.Milliseconds())
	return bestNode
}

func (c *Client) updateBestNodes(ctx context.Context, updateBestNodesInterval int) {
	const maxIntervalSeconds = int64((1<<63 - 1) / int64(time.Second))
	if updateBestNodesInterval <= 0 || int64(updateBestNodesInterval) > maxIntervalSeconds {
		log.Printf("Ignore invalid best-node update interval: %d seconds", updateBestNodesInterval)
		return
	}
	ticker := time.NewTicker(time.Duration(updateBestNodesInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		bestNodes := getBestNodes(c.NodeGroups)
		c.BestNodesRWMutex.Lock()
		c.BestNodes = bestNodes
		c.BestNodesRWMutex.Unlock()
	}
}
