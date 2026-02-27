// Package process provides process querying and management services.
// It wraps gopsutil for process enumeration and uses os.FindProcess + Signal
// for process termination.  All mutating operations emit audit events.
package process

import (
	"context"
	"fmt"
	"os"
	"syscall"

	"github.com/shirou/gopsutil/v3/process"

	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// ListOptions controls which processes are returned and in what order.
type ListOptions struct {
	// Limit caps the number of results returned.  0 means no cap.
	Limit int
	// Page is the 1-based page index when Limit > 0.
	Page int
}

// ProcessLister is the interface for enumerating OS processes.
// It can be replaced by a stub in unit tests.
type ProcessLister interface {
	// Pids returns the list of all running process IDs.
	Pids(ctx context.Context) ([]int32, error)
	// Info returns the domain.Process snapshot for the given PID.
	Info(ctx context.Context, pid int32) (*domain.Process, error)
}

// Manager is the shared service for process operations.
type Manager struct {
	lister      ProcessLister
	auditLogger audit.Logger
}

// NewManager creates a production Manager backed by gopsutil.
func NewManager(al audit.Logger) *Manager {
	return &Manager{lister: &gopsutilLister{}, auditLogger: al}
}

// NewManagerWithLister creates a Manager with a custom ProcessLister (for testing).
func NewManagerWithLister(l ProcessLister, al audit.Logger) *Manager {
	return &Manager{lister: l, auditLogger: al}
}

// List returns a page of running processes.
// When opts.Limit == 0 all processes are returned.
// When opts.Limit > 0 results are paginated; opts.Page is 1-based.
func (m *Manager) List(ctx context.Context, opts ListOptions) ([]*domain.Process, int, error) {
	pids, err := m.lister.Pids(ctx)
	if err != nil {
		return nil, 0, apperrors.Wrap(apperrors.ErrInternal, "failed to list processes", err)
	}

	total := len(pids)

	// Apply pagination when Limit is set.
	if opts.Limit > 0 {
		page := opts.Page
		if page < 1 {
			page = 1
		}
		start := (page - 1) * opts.Limit
		if start >= total {
			return []*domain.Process{}, total, nil
		}
		end := start + opts.Limit
		if end > total {
			end = total
		}
		pids = pids[start:end]
	}

	procs := make([]*domain.Process, 0, len(pids))
	for _, pid := range pids {
		p, err := m.lister.Info(ctx, pid)
		if err != nil {
			// Process may have exited between listing and querying; skip it.
			continue
		}
		procs = append(procs, p)
	}

	return procs, total, nil
}

// Get returns the domain.Process for the given PID.
// Returns ErrNotFound when no such process exists.
func (m *Manager) Get(ctx context.Context, pid int32) (*domain.Process, error) {
	p, err := m.lister.Info(ctx, pid)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrNotFound, fmt.Sprintf("process %d not found", pid), err)
	}
	return p, nil
}

// Kill sends SIGTERM to the process with the given PID.
// source identifies the caller ("cli" or a remote address) and is recorded in
// the audit log.  Returns ErrForbidden when the current user lacks permission
// to send signals to the target process.
func (m *Manager) Kill(ctx context.Context, pid int32, source string) error {
	proc, err := os.FindProcess(int(pid))
	if err != nil {
		m.logAudit(source, pid, "failure", apperrors.ErrNotFound)
		return apperrors.Wrap(apperrors.ErrNotFound, fmt.Sprintf("process %d not found", pid), err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		code := apperrors.ErrInternal
		if isPermissionError(err) {
			code = apperrors.ErrForbidden
		}
		m.logAudit(source, pid, "failure", code)
		return apperrors.Wrap(code, fmt.Sprintf("failed to kill process %d", pid), err)
	}

	m.logAudit(source, pid, "success", "")
	return nil
}

// logAudit writes an audit event for a kill operation.
func (m *Manager) logAudit(source string, pid int32, result string, code apperrors.ErrorCode) {
	if m.auditLogger == nil {
		return
	}
	event := audit.AuditEvent{
		Source: source,
		Action: "process.kill",
		Target: fmt.Sprintf("pid:%d", pid),
		Result: result,
	}
	if code != "" {
		event.ErrorCode = string(code)
	}
	_ = m.auditLogger.Log(event)
}

// isPermissionError reports whether err indicates a permission/access denied
// failure when sending a signal.
func isPermissionError(err error) bool {
	return os.IsPermission(err)
}

// ─── gopsutil-backed lister ─────────────────────────────────────────────────

// gopsutilLister is the production ProcessLister backed by gopsutil.
type gopsutilLister struct{}

func (g *gopsutilLister) Pids(ctx context.Context) ([]int32, error) {
	return process.PidsWithContext(ctx)
}

func (g *gopsutilLister) Info(ctx context.Context, pid int32) (*domain.Process, error) {
	p, err := process.NewProcessWithContext(ctx, pid)
	if err != nil {
		return nil, err
	}

	name, _ := p.NameWithContext(ctx)
	statuses, _ := p.StatusWithContext(ctx)
	status := ""
	if len(statuses) > 0 {
		status = statuses[0]
	}
	cpuPct, _ := p.CPUPercentWithContext(ctx)
	memInfo, _ := p.MemoryInfoWithContext(ctx)
	username, _ := p.UsernameWithContext(ctx)
	cmdline, _ := p.CmdlineWithContext(ctx)

	proc := &domain.Process{
		PID:        pid,
		Name:       name,
		Status:     status,
		CPUPercent: cpuPct,
		User:       username,
		Cmdline:    cmdline,
	}
	if memInfo != nil {
		proc.MemRSS = memInfo.RSS
	}
	return proc, nil
}
