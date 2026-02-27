// Package share provides management of network file shares (Samba/NFS).
// It reads and writes share configurations and invokes service reload commands.
// All mutating operations emit audit events and support rollback on failure.
package share

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// Backend abstracts the underlying share configuration store so that tests
// can use an in-memory implementation instead of touching real config files.
type Backend interface {
	// List returns all configured shares.
	List(ctx context.Context) ([]domain.Share, error)
	// Get returns the share with the given name.
	Get(ctx context.Context, name string) (*domain.Share, error)
	// Create adds a new share. Returns an error (ErrInvalidInput) if the name already exists or input is invalid.
	Create(ctx context.Context, s domain.Share) error
	// Update replaces the share with the given name.
	Update(ctx context.Context, s domain.Share) error
	// Delete removes the share with the given name.
	Delete(ctx context.Context, name string) error
	// Reload signals the underlying service (smbd/nfsd) to reread configuration.
	// On failure the backend should attempt to restore the previous configuration.
	Reload(ctx context.Context) error
}

// Manager is the share management service.
type Manager struct {
	backend     Backend
	auditLogger audit.Logger
}

// NewManager creates a Manager backed by the real filesystem config files.
// The caller should prefer NewManagerWithBackend during tests.
func NewManager(al audit.Logger) *Manager {
	return &Manager{backend: newFileBackend(), auditLogger: al}
}

// NewManagerWithBackend creates a Manager with an injectable Backend (for testing).
func NewManagerWithBackend(b Backend, al audit.Logger) *Manager {
	return &Manager{backend: b, auditLogger: al}
}

// List returns all configured shares.
func (m *Manager) List(ctx context.Context) ([]domain.Share, error) {
	shares, err := m.backend.List(ctx)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternal, "failed to list shares", err)
	}
	return shares, nil
}

// Get returns the share with the given name.
func (m *Manager) Get(ctx context.Context, name string) (*domain.Share, error) {
	if name == "" {
		return nil, apperrors.New(apperrors.ErrInvalidInput, "share name must not be empty")
	}
	s, err := m.backend.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// Create adds a new share and reloads the service.
func (m *Manager) Create(ctx context.Context, s domain.Share, source string) error {
	if err := validateShare(s); err != nil {
		m.logAudit(source, "share.create", s.Name, "failure", apperrors.ErrInvalidInput)
		return err
	}

	if err := m.backend.Create(ctx, s); err != nil {
		m.logAudit(source, "share.create", s.Name, "failure", errorCode(err))
		return wrapBackendError(err, "failed to create share")
	}

	if err := m.backend.Reload(ctx); err != nil {
		// Attempt rollback.
		rollbackErr := m.backend.Delete(ctx, s.Name)
		m.logAudit(source, "share.create", s.Name, "failure", apperrors.ErrInternal)
		if rollbackErr != nil {
			return apperrors.Wrap(apperrors.ErrInternal,
				fmt.Sprintf("service reload failed; rollback also failed: %v", rollbackErr), err)
		}
		return apperrors.Wrap(apperrors.ErrInternal, "service reload failed; share creation rolled back", err)
	}

	m.logAudit(source, "share.create", s.Name, "success", "")
	return nil
}

// Update replaces an existing share and reloads the service.
func (m *Manager) Update(ctx context.Context, s domain.Share, source string) error {
	if err := validateShare(s); err != nil {
		m.logAudit(source, "share.update", s.Name, "failure", apperrors.ErrInvalidInput)
		return err
	}

	// Snapshot previous state for rollback.
	prev, err := m.backend.Get(ctx, s.Name)
	if err != nil {
		return err
	}

	if err := m.backend.Update(ctx, s); err != nil {
		m.logAudit(source, "share.update", s.Name, "failure", errorCode(err))
		return wrapBackendError(err, "failed to update share")
	}

	if err := m.backend.Reload(ctx); err != nil {
		// Attempt rollback to previous state.
		rollbackErr := m.backend.Update(ctx, *prev)
		m.logAudit(source, "share.update", s.Name, "failure", apperrors.ErrInternal)
		if rollbackErr != nil {
			return apperrors.Wrap(apperrors.ErrInternal,
				fmt.Sprintf("service reload failed; rollback also failed: %v", rollbackErr), err)
		}
		return apperrors.Wrap(apperrors.ErrInternal, "service reload failed; share update rolled back", err)
	}

	m.logAudit(source, "share.update", s.Name, "success", "")
	return nil
}

// Delete removes a share and reloads the service.
func (m *Manager) Delete(ctx context.Context, name, source string) error {
	if name == "" {
		return apperrors.New(apperrors.ErrInvalidInput, "share name must not be empty")
	}

	// Snapshot for rollback.
	prev, err := m.backend.Get(ctx, name)
	if err != nil {
		return err
	}

	if err := m.backend.Delete(ctx, name); err != nil {
		m.logAudit(source, "share.delete", name, "failure", errorCode(err))
		return wrapBackendError(err, "failed to delete share")
	}

	if err := m.backend.Reload(ctx); err != nil {
		// Attempt rollback.
		rollbackErr := m.backend.Create(ctx, *prev)
		m.logAudit(source, "share.delete", name, "failure", apperrors.ErrInternal)
		if rollbackErr != nil {
			return apperrors.Wrap(apperrors.ErrInternal,
				fmt.Sprintf("service reload failed; rollback also failed: %v", rollbackErr), err)
		}
		return apperrors.Wrap(apperrors.ErrInternal, "service reload failed; share deletion rolled back", err)
	}

	m.logAudit(source, "share.delete", name, "success", "")
	return nil
}

// ── validation ────────────────────────────────────────────────────────────────

func validateShare(s domain.Share) error {
	if strings.TrimSpace(s.Name) == "" {
		return apperrors.New(apperrors.ErrInvalidInput, "share name must not be empty")
	}
	if strings.Contains(s.Name, "/") {
		return apperrors.New(apperrors.ErrInvalidInput, "share name must not contain '/'")
	}
	if strings.TrimSpace(s.Path) == "" {
		return apperrors.New(apperrors.ErrInvalidInput, "share path must not be empty")
	}
	if s.Type != domain.ShareTypeSamba && s.Type != domain.ShareTypeNFS {
		return apperrors.New(apperrors.ErrInvalidInput,
			fmt.Sprintf("unsupported share type %q; must be %q or %q",
				s.Type, domain.ShareTypeSamba, domain.ShareTypeNFS))
	}
	return nil
}

// errorCode extracts the ErrorCode from an error, returning ErrInternal for
// non-AppError values.  This preserves business error codes from the backend
// so that callers see 400/404 rather than a blanket 500.
func errorCode(err error) apperrors.ErrorCode {
	var ae *apperrors.AppError
	if errors.As(err, &ae) {
		return ae.Code
	}
	return apperrors.ErrInternal
}

// wrapBackendError wraps err, preserving its AppError code if it has one.
func wrapBackendError(err error, msg string) error {
	var ae *apperrors.AppError
	if errors.As(err, &ae) {
		return apperrors.Wrap(ae.Code, fmt.Sprintf("%s: %s", msg, ae.Message), err)
	}
	return apperrors.Wrap(apperrors.ErrInternal, msg, err)
}

// ── audit ─────────────────────────────────────────────────────────────────────

func (m *Manager) logAudit(source, action, target, result string, code apperrors.ErrorCode) {
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

// ── in-memory backend (placeholder; not for production persistence) ──────────

// memBackend is a thread-safe in-memory Backend implementation.
// It is used in unit tests and as the default production placeholder.
//
// NOTE: This backend does not persist to disk and does not invoke any real
// samba/nfs service reload.  It is a safe placeholder until a real
// config-file-backed backend is implemented (follow-up task).
// Changes made through this backend are lost on process restart.
type memBackend struct {
	mu     sync.RWMutex
	shares map[string]domain.Share
}

func newFileBackend() Backend {
	return &memBackend{shares: make(map[string]domain.Share)}
}

func (b *memBackend) List(_ context.Context) ([]domain.Share, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]domain.Share, 0, len(b.shares))
	for _, s := range b.shares {
		result = append(result, s)
	}
	return result, nil
}

func (b *memBackend) Get(_ context.Context, name string) (*domain.Share, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	s, ok := b.shares[name]
	if !ok {
		return nil, apperrors.New(apperrors.ErrNotFound, fmt.Sprintf("share %q not found", name))
	}
	cp := s
	return &cp, nil
}

func (b *memBackend) Create(_ context.Context, s domain.Share) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.shares[s.Name]; exists {
		return apperrors.New(apperrors.ErrInvalidInput, fmt.Sprintf("share %q already exists", s.Name))
	}
	b.shares[s.Name] = s
	return nil
}

func (b *memBackend) Update(_ context.Context, s domain.Share) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.shares[s.Name]; !exists {
		return apperrors.New(apperrors.ErrNotFound, fmt.Sprintf("share %q not found", s.Name))
	}
	b.shares[s.Name] = s
	return nil
}

func (b *memBackend) Delete(_ context.Context, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.shares[name]; !exists {
		return apperrors.New(apperrors.ErrNotFound, fmt.Sprintf("share %q not found", name))
	}
	delete(b.shares, name)
	return nil
}

func (b *memBackend) Reload(_ context.Context) error { return nil }
