// Package network provides network interface management services.
// Read-only queries use the standard net package; mutating operations
// (bring interface up/down, refresh DHCP lease) invoke system commands
// via an injectable Commander so they can be replaced by stubs in tests.
//
// Mutating operations require admin role at the API layer and emit audit events.
package network

import (
	"context"
	"fmt"
	"net"
	"strings"

	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// Commander is the interface for running system commands with context support.
// It mirrors the Commander interface used in the disk package so that the same
// stub implementations can be reused in tests.
type Commander interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Manager provides network interface query and management operations.
type Manager struct {
	commander Commander
	logger    audit.Logger
}

// NewManager creates a production Manager backed by real system commands.
func NewManager(al audit.Logger) *Manager {
	return &Manager{commander: &osCommander{}, logger: al}
}

// NewManagerWithCommander creates a Manager with injected dependencies (for testing).
func NewManagerWithCommander(c Commander, al audit.Logger) *Manager {
	return &Manager{commander: c, logger: al}
}

// ── Read-only queries ─────────────────────────────────────────────────────────

// ListInterfaces returns all network interfaces on the host.
func (m *Manager) ListInterfaces(_ context.Context) ([]domain.NetworkInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternal, "failed to list network interfaces", err)
	}

	result := make([]domain.NetworkInterface, 0, len(ifaces))
	for _, iface := range ifaces {
		ni := domain.NetworkInterface{
			Name:         iface.Name,
			Index:        iface.Index,
			MTU:          iface.MTU,
			HardwareAddr: iface.HardwareAddr.String(),
			Flags:        flagStrings(iface.Flags),
		}

		addrs, err := iface.Addrs()
		if err == nil {
			for _, addr := range addrs {
				na := parseNetAddr(addr)
				if na != nil {
					ni.Addresses = append(ni.Addresses, *na)
				}
			}
		}
		if ni.Addresses == nil {
			ni.Addresses = []domain.NetworkAddress{}
		}
		result = append(result, ni)
	}
	return result, nil
}

// GetInterface returns the named network interface.
func (m *Manager) GetInterface(ctx context.Context, name string) (*domain.NetworkInterface, error) {
	ifaces, err := m.ListInterfaces(ctx)
	if err != nil {
		return nil, err
	}
	for i := range ifaces {
		if ifaces[i].Name == name {
			return &ifaces[i], nil
		}
	}
	return nil, apperrors.New(apperrors.ErrNotFound, fmt.Sprintf("interface %q not found", name))
}

// ── Mutating operations (require admin role) ──────────────────────────────────

// SetLinkUp brings the named network interface up.
// Requires admin role; records an audit event.
func (m *Manager) SetLinkUp(ctx context.Context, name, source string) error {
	_, err := m.commander.Run(ctx, "ip", "link", "set", name, "up")
	if err != nil {
		m.logAudit(source, "network.link.up", name, "failure", apperrors.ErrInternal)
		return apperrors.Wrap(apperrors.ErrInternal, fmt.Sprintf("failed to bring interface %q up", name), err)
	}
	m.logAudit(source, "network.link.up", name, "success", "")
	return nil
}

// SetLinkDown brings the named network interface down.
// Requires admin role; records an audit event.
func (m *Manager) SetLinkDown(ctx context.Context, name, source string) error {
	_, err := m.commander.Run(ctx, "ip", "link", "set", name, "down")
	if err != nil {
		m.logAudit(source, "network.link.down", name, "failure", apperrors.ErrInternal)
		return apperrors.Wrap(apperrors.ErrInternal, fmt.Sprintf("failed to bring interface %q down", name), err)
	}
	m.logAudit(source, "network.link.down", name, "success", "")
	return nil
}

// RenewDHCP requests a DHCP lease renewal for the named interface.
// It tries dhclient first, falling back to dhcpcd.
// Requires admin role; records an audit event.
func (m *Manager) RenewDHCP(ctx context.Context, name, source string) error {
	_, err := m.commander.Run(ctx, "dhclient", name)
	if err != nil {
		// Try dhcpcd as a fallback (common on Arch/Alpine-based systems).
		_, err2 := m.commander.Run(ctx, "dhcpcd", name)
		if err2 != nil {
			m.logAudit(source, "network.dhcp.renew", name, "failure", apperrors.ErrInternal)
			return apperrors.Wrap(apperrors.ErrInternal,
				fmt.Sprintf("failed to renew DHCP on %q: dhclient: %v; dhcpcd: %v", name, err, err2), err)
		}
	}
	m.logAudit(source, "network.dhcp.renew", name, "success", "")
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func flagStrings(f net.Flags) []string {
	var result []string
	known := []struct {
		flag net.Flags
		name string
	}{
		{net.FlagUp, "UP"},
		{net.FlagBroadcast, "BROADCAST"},
		{net.FlagLoopback, "LOOPBACK"},
		{net.FlagPointToPoint, "POINTTOPOINT"},
		{net.FlagMulticast, "MULTICAST"},
	}
	for _, k := range known {
		if f&k.flag != 0 {
			result = append(result, k.name)
		}
	}
	if result == nil {
		result = []string{}
	}
	return result
}

func parseNetAddr(addr net.Addr) *domain.NetworkAddress {
	var ipStr string
	var prefix int
	var family string

	switch v := addr.(type) {
	case *net.IPNet:
		ipStr = v.IP.String()
		ones, _ := v.Mask.Size()
		prefix = ones
	case *net.IPAddr:
		ipStr = v.IP.String()
		if v.IP.To4() != nil {
			prefix = 32
		} else {
			prefix = 128
		}
	default:
		return nil
	}

	if strings.Contains(ipStr, ":") {
		family = "ipv6"
	} else {
		family = "ipv4"
	}

	return &domain.NetworkAddress{
		IP:     ipStr,
		Prefix: prefix,
		Family: family,
	}
}

func (m *Manager) logAudit(source, action, target, result string, code apperrors.ErrorCode) {
	if m.logger == nil {
		return
	}
	event := audit.AuditEvent{
		Source: source,
		Action: action,
		Target: target,
		Result: result,
	}
	if code != "" {
		event.ErrorCode = string(code)
	}
	_ = m.logger.Log(event)
}

// ── osCommander ───────────────────────────────────────────────────────────────

// osCommander is the production Commander backed by exec.CommandContext.
type osCommander struct{}

func (c *osCommander) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return runCmd(ctx, name, args...)
}
