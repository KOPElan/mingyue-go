// Package network provides network interface and routing information services.
// Read-only queries are accessible to viewer-role callers; mutating operations
// (interface up/down) require admin role and emit audit events.
package network

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// Reader is the interface for reading network state.
// It can be replaced by a stub in unit tests.
type Reader interface {
	// Interfaces returns the list of all network interfaces.
	Interfaces() ([]domain.NetworkInterface, error)
	// Routes returns the current kernel routing table.
	Routes(ctx context.Context) ([]domain.Route, error)
}

// Commander is the interface for mutating network state.
// It can be replaced by a stub in unit tests.
type Commander interface {
	// SetInterfaceState brings the named interface up (up=true) or down (up=false).
	SetInterfaceState(ctx context.Context, name string, up bool) error
}

// Manager is the shared service for network operations.
type Manager struct {
	reader      Reader
	commander   Commander
	auditLogger audit.Logger
}

// NewManager creates a production Manager backed by the OS network stack.
func NewManager(al audit.Logger) *Manager {
	return &Manager{
		reader:      &osReader{},
		commander:   &osCommander{},
		auditLogger: al,
	}
}

// NewManagerWithDeps creates a Manager with injected dependencies (for testing).
func NewManagerWithDeps(reader Reader, commander Commander, al audit.Logger) *Manager {
	return &Manager{reader: reader, commander: commander, auditLogger: al}
}

// Interfaces returns the list of network interfaces.
func (m *Manager) Interfaces() ([]domain.NetworkInterface, error) {
	ifaces, err := m.reader.Interfaces()
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternal, "failed to list network interfaces", err)
	}
	return ifaces, nil
}

// Routes returns the current kernel routing table.
func (m *Manager) Routes(ctx context.Context) ([]domain.Route, error) {
	routes, err := m.reader.Routes(ctx)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternal, "failed to list routes", err)
	}
	return routes, nil
}

// SetInterfaceState brings the named interface up or down.
// source identifies the caller and is recorded in the audit log.
// This operation requires admin privileges.
func (m *Manager) SetInterfaceState(ctx context.Context, name string, up bool, source string) error {
	state := "down"
	if up {
		state = "up"
	}
	action := "network.interface." + state

	if err := m.commander.SetInterfaceState(ctx, name, up); err != nil {
		m.logAudit(source, name, action, "failure", apperrors.ErrInternal)
		return apperrors.Wrap(apperrors.ErrInternal,
			fmt.Sprintf("failed to set interface %s %s", name, state), err)
	}

	m.logAudit(source, name, action, "success", "")
	return nil
}

func (m *Manager) logAudit(source, target, action, result string, code apperrors.ErrorCode) {
	if m.auditLogger == nil {
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
	_ = m.auditLogger.Log(event)
}

// ─── OS-backed implementations ────────────────────────────────────────────────

type osReader struct{}

func (o *osReader) Interfaces() ([]domain.NetworkInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	result := make([]domain.NetworkInterface, 0, len(ifaces))
	for _, iface := range ifaces {
		ni := domain.NetworkInterface{
			Name:         iface.Name,
			HardwareAddr: iface.HardwareAddr.String(),
			IsUp:         iface.Flags&net.FlagUp != 0,
			Flags:        parseFlags(iface.Flags),
		}
		addrs, _ := iface.Addrs()
		// Address retrieval is best-effort; a failure (e.g. interface removed
		// between listing and querying) leaves Addrs nil, which is safe to return.
		for _, addr := range addrs {
			ni.Addrs = append(ni.Addrs, addr.String())
		}
		result = append(result, ni)
	}
	return result, nil
}

func (o *osReader) Routes(ctx context.Context) ([]domain.Route, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "ip", "-o", "route").Output()
	if err != nil {
		return nil, fmt.Errorf("ip route: %w", err)
	}
	return parseIPRoutes(string(out)), nil
}

// parseFlags converts net.Flags bitmask to a human-readable slice.
func parseFlags(flags net.Flags) []string {
	var result []string
	if flags&net.FlagUp != 0 {
		result = append(result, "up")
	}
	if flags&net.FlagBroadcast != 0 {
		result = append(result, "broadcast")
	}
	if flags&net.FlagLoopback != 0 {
		result = append(result, "loopback")
	}
	if flags&net.FlagPointToPoint != 0 {
		result = append(result, "pointtopoint")
	}
	if flags&net.FlagMulticast != 0 {
		result = append(result, "multicast")
	}
	return result
}

// parseIPRoutes parses the output of `ip -o route` into Route slices.
func parseIPRoutes(output string) []domain.Route {
	var routes []domain.Route
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		routes = append(routes, parseIPRouteLine(line))
	}
	return routes
}

// parseIPRouteLine parses a single line of `ip -o route` output.
// Example: "10.0.0.0/8 dev eth0 proto kernel scope link src 10.0.0.1"
// Example: "default via 192.168.1.1 dev eth0 proto dhcp metric 100"
func parseIPRouteLine(line string) domain.Route {
	fields := strings.Fields(line)
	route := domain.Route{}
	if len(fields) == 0 {
		return route
	}
	route.Destination = fields[0]
	for i := 1; i < len(fields); i++ {
		switch fields[i] {
		case "via":
			if i+1 < len(fields) {
				route.Gateway = fields[i+1]
				i++
			}
		case "dev":
			if i+1 < len(fields) {
				route.Interface = fields[i+1]
				i++
			}
		case "metric":
			if i+1 < len(fields) {
				route.Metric = fields[i+1]
				i++
			}
		}
	}
	return route
}

type osCommander struct{}

func (o *osCommander) SetInterfaceState(ctx context.Context, name string, up bool) error {
	state := "down"
	if up {
		state = "up"
	}
	out, err := exec.CommandContext(ctx, "ip", "link", "set", name, state).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
