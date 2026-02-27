// Package disk provides mount management and SMART health query services.
package disk

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// PowerService manages disk power states using hdparm.
type PowerService struct {
	commander Commander
	logger    audit.Logger
}

// NewPowerService creates a production PowerService backed by the real hdparm binary.
func NewPowerService(al audit.Logger) *PowerService {
	return &PowerService{commander: &osCommander{}, logger: al}
}

// NewPowerServiceWithCommander creates a PowerService with injected dependencies (for testing).
func NewPowerServiceWithCommander(c Commander, al audit.Logger) *PowerService {
	return &PowerService{commander: c, logger: al}
}

// GetStatus queries the current power mode of a block device using hdparm -C.
//
// device may be a short name like "sda" (auto-expanded to "/dev/sda") or a
// full path like "/dev/sda".
//
// Error semantics:
//   - ErrNotFound: hdparm binary is not installed (install hdparm).
//   - ErrForbidden: insufficient permissions to query the device.
//   - ErrInternal: all other failures.
func (s *PowerService) GetStatus(ctx context.Context, device string) (*domain.DiskPower, error) {
	device = normalizeDevice(device)
	output, runErr := s.commander.Run(ctx, "hdparm", "-C", device)

	if len(output) > 0 {
		if mode, ok := parseHdparmPowerMode(string(output)); ok {
			return &domain.DiskPower{Device: device, PowerMode: mode}, nil
		}
	}

	if runErr == nil {
		return nil, apperrors.New(apperrors.ErrInternal,
			fmt.Sprintf("hdparm returned no parseable output for device %s", device))
	}

	if errors.Is(runErr, exec.ErrNotFound) {
		return nil, apperrors.Wrap(apperrors.ErrNotFound,
			"hdparm not found: install the hdparm package to manage disk power", runErr)
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 1 {
		return nil, apperrors.Wrap(apperrors.ErrForbidden,
			fmt.Sprintf("cannot access device %s: check that the process runs as root", device), runErr)
	}

	return nil, apperrors.Wrap(apperrors.ErrInternal,
		fmt.Sprintf("hdparm failed for device %s", device), runErr)
}

// SetMode forces a block device into the specified power mode.
// action must be "standby" (hdparm -y) or "sleep" (hdparm -Y).
//
// Requires root privileges.
//
// Error semantics:
//   - ErrInvalidInput: unsupported action.
//   - ErrNotFound: hdparm binary is not installed.
//   - ErrForbidden: insufficient permissions.
//   - ErrInternal: all other failures.
func (s *PowerService) SetMode(ctx context.Context, device, action, source string) error {
	device = normalizeDevice(device)

	var flag string
	switch strings.ToLower(action) {
	case "standby":
		flag = "-y"
	case "sleep":
		flag = "-Y"
	default:
		return apperrors.New(apperrors.ErrInvalidInput,
			fmt.Sprintf("unsupported power action %q: use 'standby' or 'sleep'", action))
	}

	_, runErr := s.commander.Run(ctx, "hdparm", flag, device)
	if runErr != nil {
		if errors.Is(runErr, exec.ErrNotFound) {
			s.logPowerAudit(source, device, action, "failure", apperrors.ErrNotFound)
			return apperrors.Wrap(apperrors.ErrNotFound,
				"hdparm not found: install the hdparm package to manage disk power", runErr)
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) && exitErr.ExitCode() == 1 {
			s.logPowerAudit(source, device, action, "failure", apperrors.ErrForbidden)
			return apperrors.Wrap(apperrors.ErrForbidden,
				fmt.Sprintf("cannot access device %s: check that the process runs as root", device), runErr)
		}
		s.logPowerAudit(source, device, action, "failure", apperrors.ErrInternal)
		return apperrors.Wrap(apperrors.ErrInternal,
			fmt.Sprintf("hdparm %s failed for device %s", action, device), runErr)
	}

	s.logPowerAudit(source, device, action, "success", "")
	return nil
}

// logPowerAudit writes an audit event for a power operation.
func (s *PowerService) logPowerAudit(source, device, action, result string, code apperrors.ErrorCode) {
	if s.logger == nil {
		return
	}
	event := audit.AuditEvent{
		Source: source,
		Action: "disk.power." + action,
		Target: device,
		Result: result,
	}
	if code != "" {
		event.ErrorCode = string(code)
	}
	_ = s.logger.Log(event)
}

// ─── helpers ──────────────────────────────────────────────────────────────────

// normalizeDevice expands short device names (e.g. "sda" → "/dev/sda").
func normalizeDevice(device string) string {
	if !strings.HasPrefix(device, "/") {
		return "/dev/" + device
	}
	return device
}

// parseHdparmPowerMode extracts the drive state string from hdparm -C output.
// Returns the canonical mode and true on success; "", false if unparseable.
func parseHdparmPowerMode(output string) (string, bool) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "drive state is:") {
			state := strings.TrimSpace(strings.TrimPrefix(line, "drive state is:"))
			return canonicalPowerMode(state), true
		}
	}
	return "", false
}

// canonicalPowerMode maps hdparm state strings to canonical power mode names.
func canonicalPowerMode(state string) string {
	switch state {
	case "active/idle":
		return "active"
	case "standby":
		return "standby"
	case "sleeping":
		return "sleeping"
	default:
		if state != "" {
			return state
		}
		return "unknown"
	}
}
