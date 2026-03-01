// Package discovery implements LAN-based agent discovery using UDP multicast.
//
// When an agent starts it calls Advertise to broadcast its address on the local
// network.  A web frontend or the CLI can call Browse to enumerate running
// agents without needing to know their IP addresses in advance.
//
// Protocol
//   - Multicast group : 224.0.0.251 (shared with mDNS, but on a different port)
//   - Port            : 7071
//   - Payload         : JSON-encoded AgentInfo
//   - Advertisement   : sent every 3 s while the agent runs
//   - Browse timeout  : caller-supplied; typically 3–5 s
package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

const (
	// MulticastGroup is the IPv4 multicast address used for agent discovery.
	MulticastGroup = "224.0.0.251"
	// DiscoveryPort is the UDP port on which agents advertise and clients listen.
	DiscoveryPort = 7071

	advertiseInterval = 3 * time.Second
)

// AgentInfo is the payload broadcast by a running agent and collected by Browse.
type AgentInfo struct {
	// Hostname is the system hostname of the host running the agent.
	Hostname string `json:"hostname"`
	// Addr is the HTTP listen address of the agent (host:port or :port).
	Addr string `json:"addr"`
	// Version is the agent software version string.
	Version string `json:"version"`
}

// Advertise continuously multicast-sends info on the LAN until ctx is
// cancelled.  It is intended to be run in a dedicated goroutine.
//
//	go discovery.Advertise(ctx, info)
func Advertise(ctx context.Context, info AgentInfo) error {
	dst := &net.UDPAddr{
		IP:   net.ParseIP(MulticastGroup),
		Port: DiscoveryPort,
	}
	conn, err := net.DialUDP("udp4", nil, dst)
	if err != nil {
		return fmt.Errorf("discovery advertise: dial multicast: %w", err)
	}
	defer conn.Close()

	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("discovery advertise: marshal: %w", err)
	}

	// Send an initial packet immediately so the agent appears right away.
	// Write errors are intentionally ignored: UDP multicast advertisement is
	// fire-and-forget; transient failures will be retried on the next tick.
	_, _ = conn.Write(data)

	ticker := time.NewTicker(advertiseInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			_, _ = conn.Write(data) // best-effort; see note above
		}
	}
}

// Browse listens on the multicast group for agent advertisements and returns
// all unique agents heard within timeout.  Duplicate entries (same Hostname
// and Addr) are deduplicated.
func Browse(timeout time.Duration) ([]AgentInfo, error) {
	addr := &net.UDPAddr{
		IP:   net.ParseIP(MulticastGroup),
		Port: DiscoveryPort,
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		return nil, fmt.Errorf("discovery browse: listen multicast: %w", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, fmt.Errorf("discovery browse: set deadline: %w", err)
	}

	seen := map[string]bool{}
	var results []AgentInfo
	buf := make([]byte, 2048)

	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			// Deadline exceeded or connection closed — stop collecting.
			break
		}
		var info AgentInfo
		if err := json.Unmarshal(buf[:n], &info); err != nil {
			continue
		}
		key := info.Hostname + "|" + info.Addr
		if !seen[key] {
			seen[key] = true
			results = append(results, info)
		}
	}

	return results, nil
}
