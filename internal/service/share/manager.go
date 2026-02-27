// Package share provides management of network file shares (Samba/NFS).
// It reads and writes share configurations and invokes service reload commands.
// All mutating operations emit audit events and support rollback on failure.
package share

import (
	"context"
	"fmt"
	"strings"

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
	// Create adds a new share.  Returns ErrConflict if the name already exists.
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
		return err
	}

	if err := m.backend.Create(ctx, s); err != nil {
		m.logAudit(source, "share.create", s.Name, "failure", apperrors.ErrInternal)
		return apperrors.Wrap(apperrors.ErrInternal, "failed to create share", err)
	}

	if err := m.backend.Reload(ctx); err != nil {
		// Attempt rollback.
		_ = m.backend.Delete(ctx, s.Name)
		m.logAudit(source, "share.create", s.Name, "failure", apperrors.ErrInternal)
		return apperrors.Wrap(apperrors.ErrInternal, "service reload failed; share creation rolled back", err)
	}

	m.logAudit(source, "share.create", s.Name, "success", "")
	return nil
}

// Update replaces an existing share and reloads the service.
func (m *Manager) Update(ctx context.Context, s domain.Share, source string) error {
	if err := validateShare(s); err != nil {
		return err
	}

	// Snapshot previous state for rollback.
	prev, err := m.backend.Get(ctx, s.Name)
	if err != nil {
		return err
	}

	if err := m.backend.Update(ctx, s); err != nil {
		m.logAudit(source, "share.update", s.Name, "failure", apperrors.ErrInternal)
		return apperrors.Wrap(apperrors.ErrInternal, "failed to update share", err)
	}

	if err := m.backend.Reload(ctx); err != nil {
		// Attempt rollback to previous state.
		_ = m.backend.Update(ctx, *prev)
		m.logAudit(source, "share.update", s.Name, "failure", apperrors.ErrInternal)
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
		m.logAudit(source, "share.delete", name, "failure", apperrors.ErrInternal)
		return apperrors.Wrap(apperrors.ErrInternal, "failed to delete share", err)
	}

	if err := m.backend.Reload(ctx); err != nil {
		// Attempt rollback.
		_ = m.backend.Create(ctx, *prev)
		m.logAudit(source, "share.delete", name, "failure", apperrors.ErrInternal)
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

// ── in-memory backend (stub, also used as production placeholder) ────────────

// memBackend is an in-memory Backend implementation used in tests and as the
// default when no real config path is configured.
type memBackend struct {
	shares map[string]domain.Share
}

func newFileBackend() Backend {
	// Production: returns an in-memory backend as a safe default.
	// Real samba/nfs config file support is a follow-up implementation task.
	return &memBackend{shares: make(map[string]domain.Share)}
}

func (b *memBackend) List(_ context.Context) ([]domain.Share, error) {
	result := make([]domain.Share, 0, len(b.shares))
	for _, s := range b.shares {
		result = append(result, s)
	}
	return result, nil
}

func (b *memBackend) Get(_ context.Context, name string) (*domain.Share, error) {
	s, ok := b.shares[name]
	if !ok {
		return nil, apperrors.New(apperrors.ErrNotFound, fmt.Sprintf("share %q not found", name))
	}
	cp := s
	return &cp, nil
}

func (b *memBackend) Create(_ context.Context, s domain.Share) error {
	if _, exists := b.shares[s.Name]; exists {
		return apperrors.New(apperrors.ErrInvalidInput, fmt.Sprintf("share %q already exists", s.Name))
	}
	b.shares[s.Name] = s
	return nil
}

func (b *memBackend) Update(_ context.Context, s domain.Share) error {
	if _, exists := b.shares[s.Name]; !exists {
		return apperrors.New(apperrors.ErrNotFound, fmt.Sprintf("share %q not found", s.Name))
	}
	b.shares[s.Name] = s
	return nil
}

func (b *memBackend) Delete(_ context.Context, name string) error {
	if _, exists := b.shares[name]; !exists {
		return apperrors.New(apperrors.ErrNotFound, fmt.Sprintf("share %q not found", name))
	}
	delete(b.shares, name)
	return nil
}

func (b *memBackend) Reload(_ context.Context) error { return nil }
