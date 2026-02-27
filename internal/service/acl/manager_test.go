package acl

import (
	"context"
	"errors"
	"os"
	"testing"

	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// ─── stubs ────────────────────────────────────────────────────────────────────

type stubReader struct {
	acl *domain.FileACL
	err error
}

func (s *stubReader) GetACL(_ context.Context, _ string) (*domain.FileACL, error) {
	return s.acl, s.err
}

type stubWriter struct {
	modeErr  error
	ownerErr error
}

func (s *stubWriter) SetMode(_ context.Context, _ string, _ os.FileMode) error {
	return s.modeErr
}

func (s *stubWriter) SetOwner(_ context.Context, _, _, _ string) error {
	return s.ownerErr
}

type mockAuditLogger struct {
	events []audit.AuditEvent
}

func (m *mockAuditLogger) Log(e audit.AuditEvent) error {
	m.events = append(m.events, e)
	return nil
}

func (m *mockAuditLogger) Close() error { return nil }

// ─── validatePath tests ───────────────────────────────────────────────────────

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr apperrors.ErrorCode
	}{
		{name: "valid_absolute", path: "/var/data", wantErr: ""},
		{name: "empty_path", path: "", wantErr: apperrors.ErrInvalidInput},
		{name: "relative_path", path: "relative/path", wantErr: apperrors.ErrForbidden},
		{name: "dot_dot_relative", path: "../etc/passwd", wantErr: apperrors.ErrForbidden},
		{name: "absolute_with_traversal", path: "/var/data/../../etc/passwd", wantErr: apperrors.ErrForbidden},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePath(tc.path)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			var ae *apperrors.AppError
			if !errors.As(err, &ae) {
				t.Fatalf("expected *AppError, got %T", err)
			}
			if ae.Code != tc.wantErr {
				t.Errorf("Code: got %q, want %q", ae.Code, tc.wantErr)
			}
		})
	}
}

// ─── Get tests ────────────────────────────────────────────────────────────────

func TestManager_Get(t *testing.T) {
	t.Run("invalid_path_rejected", func(t *testing.T) {
		mgr := NewManagerWithDeps(&stubReader{}, &stubWriter{}, nil)
		_, err := mgr.Get(context.Background(), "../etc/passwd")
		if err == nil {
			t.Fatal("expected error for traversal path")
		}
		var ae *apperrors.AppError
		if !errors.As(err, &ae) || ae.Code != apperrors.ErrForbidden {
			t.Errorf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("reader_error_propagated", func(t *testing.T) {
		mgr := NewManagerWithDeps(
			&stubReader{err: apperrors.New(apperrors.ErrNotFound, "not found")},
			&stubWriter{},
			nil,
		)
		_, err := mgr.Get(context.Background(), "/some/path")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("success_returns_acl", func(t *testing.T) {
		want := &domain.FileACL{Path: "/some/path", Mode: "0644", Owner: "root", Group: "root"}
		mgr := NewManagerWithDeps(&stubReader{acl: want}, &stubWriter{}, nil)
		got, err := mgr.Get(context.Background(), "/some/path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Path != want.Path || got.Mode != want.Mode {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
}

// ─── Set tests ────────────────────────────────────────────────────────────────

func TestManager_Set(t *testing.T) {
	t.Run("invalid_path_rejected", func(t *testing.T) {
		mgr := NewManagerWithDeps(&stubReader{}, &stubWriter{}, nil)
		err := mgr.Set(context.Background(), "relative/path", SetRequest{Owner: "root"}, "test")
		if err == nil {
			t.Fatal("expected error for relative path")
		}
	})

	t.Run("mode_writer_error_emits_failure_audit", func(t *testing.T) {
		logger := &mockAuditLogger{}
		w := &stubWriter{modeErr: errors.New("permission denied")}
		mgr := NewManagerWithDeps(&stubReader{}, w, logger)

		err := mgr.Set(context.Background(), "/some/path", SetRequest{Mode: 0o644}, "test")
		if err == nil {
			t.Fatal("expected error")
		}
		if len(logger.events) == 0 {
			t.Fatal("expected audit event on failure")
		}
		if logger.events[0].Result != "failure" {
			t.Errorf("Result: got %q, want %q", logger.events[0].Result, "failure")
		}
	})

	t.Run("success_emits_audit_event", func(t *testing.T) {
		logger := &mockAuditLogger{}
		w := &stubWriter{}
		mgr := NewManagerWithDeps(&stubReader{}, w, logger)

		// Create a real temp file so os.Chmod succeeds when using osWriter.
		// With stubWriter, no file is needed since SetMode is stubbed.
		if err := mgr.Set(context.Background(), "/some/path", SetRequest{Mode: 0o644}, "test"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(logger.events) == 0 {
			t.Fatal("expected audit event")
		}
		event := logger.events[0]
		if event.Action != "acl.set" {
			t.Errorf("Action: got %q, want %q", event.Action, "acl.set")
		}
		if event.Result != "success" {
			t.Errorf("Result: got %q, want %q", event.Result, "success")
		}
		if event.Target != "/some/path" {
			t.Errorf("Target: got %q, want %q", event.Target, "/some/path")
		}
	})

	t.Run("no_changes_still_succeeds", func(t *testing.T) {
		logger := &mockAuditLogger{}
		w := &stubWriter{}
		mgr := NewManagerWithDeps(&stubReader{}, w, logger)

		// Empty SetRequest — nothing to change.
		if err := mgr.Set(context.Background(), "/some/path", SetRequest{}, "test"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(logger.events) == 0 {
			t.Fatal("expected audit event even for no-op set")
		}
	})
}

// ─── parseGetfaclOutput tests ─────────────────────────────────────────────────

func TestParseGetfaclOutput(t *testing.T) {
	input := []byte("user::rwx\nuser:alice:r-x\ngroup::r-x\nmask::r-x\nother::r--\n")
	entries := parseGetfaclOutput(input)
	if len(entries) != 5 {
		t.Fatalf("len(entries): got %d, want 5", len(entries))
	}
	if entries[0].Type != "user" || entries[0].Name != "" || entries[0].Permissions != "rwx" {
		t.Errorf("entries[0]: %+v", entries[0])
	}
	if entries[1].Type != "user" || entries[1].Name != "alice" || entries[1].Permissions != "r-x" {
		t.Errorf("entries[1]: %+v", entries[1])
	}
	if entries[2].Type != "group" {
		t.Errorf("entries[2].Type: got %q, want %q", entries[2].Type, "group")
	}
}
