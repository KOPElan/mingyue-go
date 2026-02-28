package share

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// ── mock audit logger ─────────────────────────────────────────────────────────

type mockAuditLogger struct {
	events []audit.AuditEvent
}

func (m *mockAuditLogger) Log(e audit.AuditEvent) error {
	m.events = append(m.events, e)
	return nil
}
func (m *mockAuditLogger) Close() error { return nil }

// ── in-memory backend (test helper) ──────────────────────────────────────────

// memBackend is a thread-safe in-memory Backend used only in tests.
type memBackend struct {
	mu     sync.RWMutex
	shares map[string]domain.Share
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
		return apperrors.New(apperrors.ErrConflict, fmt.Sprintf("share %q already exists", s.Name))
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

// ── failing backend ───────────────────────────────────────────────────────────

// failReloadBackend wraps memBackend but always fails Reload.
type failReloadBackend struct {
	*memBackend
}

func (f *failReloadBackend) Reload(_ context.Context) error {
	return fmt.Errorf("smbd reload failed")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func makeShare(name string) domain.Share {
	return domain.Share{
		Name:     name,
		Type:     domain.ShareTypeSamba,
		Path:     "/srv/" + name,
		ReadOnly: false,
		Enabled:  true,
	}
}

func newTestManager() (*Manager, *mockAuditLogger) {
	al := &mockAuditLogger{}
	mgr := NewManagerWithBackend(&memBackend{shares: make(map[string]domain.Share)}, al)
	return mgr, al
}

// ── List tests ────────────────────────────────────────────────────────────────

func TestManager_List(t *testing.T) {
	mgr, _ := newTestManager()
	_ = mgr.backend.Create(context.Background(), makeShare("data"))
	_ = mgr.backend.Create(context.Background(), makeShare("media"))

	shares, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(shares) != 2 {
		t.Errorf("len: got %d, want 2", len(shares))
	}
}

// ── Get tests ─────────────────────────────────────────────────────────────────

func TestManager_Get_Found(t *testing.T) {
	mgr, _ := newTestManager()
	_ = mgr.backend.Create(context.Background(), makeShare("docs"))

	s, err := mgr.Get(context.Background(), "docs")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "docs" {
		t.Errorf("Name: got %q, want %q", s.Name, "docs")
	}
}

func TestManager_Get_NotFound(t *testing.T) {
	mgr, _ := newTestManager()

	_, err := mgr.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != apperrors.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestManager_Get_EmptyName(t *testing.T) {
	mgr, _ := newTestManager()

	_, err := mgr.Get(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != apperrors.ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

// ── Create tests ──────────────────────────────────────────────────────────────

func TestManager_Create(t *testing.T) {
	mgr, al := newTestManager()

	err := mgr.Create(context.Background(), makeShare("backup"), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(al.events) == 0 {
		t.Error("expected audit event")
	}
	if al.events[0].Action != "share.create" || al.events[0].Result != "success" {
		t.Errorf("audit event: %+v", al.events[0])
	}
}

func TestManager_Create_NameWithSlash(t *testing.T) {
	mgr, al := newTestManager()

	bad := domain.Share{Name: "a/b", Type: domain.ShareTypeSamba, Path: "/srv/x"}
	err := mgr.Create(context.Background(), bad, "test")
	if err == nil {
		t.Fatal("expected validation error for name with '/'")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != apperrors.ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
	// Validation failure must also emit an audit event.
	if len(al.events) == 0 || al.events[0].Result != "failure" {
		t.Error("expected failure audit event on validation error")
	}
}

func TestManager_Create_ValidationFails(t *testing.T) {
	mgr, _ := newTestManager()

	bad := domain.Share{Name: "", Type: domain.ShareTypeSamba, Path: "/srv/x"}
	err := mgr.Create(context.Background(), bad, "test")
	if err == nil {
		t.Fatal("expected validation error")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != apperrors.ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput, got %v", err)
	}
}

func TestManager_Create_InvalidType(t *testing.T) {
	mgr, _ := newTestManager()

	bad := domain.Share{Name: "x", Type: "ftp", Path: "/srv/x"}
	err := mgr.Create(context.Background(), bad, "test")
	if err == nil {
		t.Fatal("expected validation error for invalid type")
	}
}

func TestManager_Create_ReloadFailure_Rollback(t *testing.T) {
	al := &mockAuditLogger{}
	b := &failReloadBackend{memBackend: &memBackend{shares: make(map[string]domain.Share)}}
	mgr := NewManagerWithBackend(b, al)

	err := mgr.Create(context.Background(), makeShare("willrollback"), "test")
	if err == nil {
		t.Fatal("expected reload error")
	}

	// After rollback the share must not exist.
	_, getErr := b.Get(context.Background(), "willrollback")
	if getErr == nil {
		t.Error("share should have been rolled back")
	}
	// Audit event must be failure.
	if len(al.events) == 0 || al.events[0].Result != "failure" {
		t.Error("expected failure audit event")
	}
}

// ── Update tests ──────────────────────────────────────────────────────────────

func TestManager_Update(t *testing.T) {
	mgr, al := newTestManager()
	_ = mgr.backend.Create(context.Background(), makeShare("shared"))

	updated := makeShare("shared")
	updated.Comment = "updated comment"

	err := mgr.Update(context.Background(), updated, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := mgr.Get(context.Background(), "shared")
	if got.Comment != "updated comment" {
		t.Errorf("Comment: got %q, want %q", got.Comment, "updated comment")
	}
	if al.events[0].Action != "share.update" || al.events[0].Result != "success" {
		t.Errorf("audit event: %+v", al.events[0])
	}
}

func TestManager_Update_ReloadFailure_Rollback(t *testing.T) {
	b := &failReloadBackend{memBackend: &memBackend{shares: make(map[string]domain.Share)}}
	original := makeShare("orig")
	original.Comment = "original"
	_ = b.Create(context.Background(), original)

	mgr := NewManagerWithBackend(b, nil)

	modified := makeShare("orig")
	modified.Comment = "modified"

	err := mgr.Update(context.Background(), modified, "test")
	if err == nil {
		t.Fatal("expected reload failure")
	}

	// State should be rolled back to original.
	got, _ := b.Get(context.Background(), "orig")
	if got.Comment != "original" {
		t.Errorf("Comment after rollback: got %q, want %q", got.Comment, "original")
	}
}

// ── Delete tests ──────────────────────────────────────────────────────────────

func TestManager_Delete(t *testing.T) {
	mgr, al := newTestManager()
	_ = mgr.backend.Create(context.Background(), makeShare("todelete"))

	err := mgr.Delete(context.Background(), "todelete", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if al.events[0].Action != "share.delete" || al.events[0].Result != "success" {
		t.Errorf("audit event: %+v", al.events[0])
	}
}

func TestManager_Delete_NotFound(t *testing.T) {
	mgr, _ := newTestManager()

	err := mgr.Delete(context.Background(), "doesnotexist", "test")
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != apperrors.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestManager_Delete_ReloadFailure_Rollback(t *testing.T) {
	b := &failReloadBackend{memBackend: &memBackend{shares: make(map[string]domain.Share)}}
	_ = b.Create(context.Background(), makeShare("persistent"))
	mgr := NewManagerWithBackend(b, nil)

	err := mgr.Delete(context.Background(), "persistent", "test")
	if err == nil {
		t.Fatal("expected reload failure")
	}

	// Share should be restored after failed reload.
	_, getErr := b.Get(context.Background(), "persistent")
	if getErr != nil {
		t.Error("share should have been restored after rollback")
	}
}
