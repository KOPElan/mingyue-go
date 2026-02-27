package process

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// newSleepCmd returns a command that sleeps long enough to be killed in tests.
func newSleepCmd() *exec.Cmd {
	return exec.Command("sleep", "30")
}

// ─── stub lister ────────────────────────────────────────────────────────────

type stubLister struct {
	pids    []int32
	pidsErr error
	procs   map[int32]*domain.Process
	infoErr map[int32]error
}

func (s *stubLister) Pids(_ context.Context) ([]int32, error) {
	return s.pids, s.pidsErr
}

func (s *stubLister) Info(_ context.Context, pid int32) (*domain.Process, error) {
	if s.infoErr != nil {
		if err, ok := s.infoErr[pid]; ok {
			return nil, err
		}
	}
	if s.procs != nil {
		if p, ok := s.procs[pid]; ok {
			return p, nil
		}
	}
	return nil, fmt.Errorf("process %d not found", pid)
}

// ─── helpers ────────────────────────────────────────────────────────────────

func makeProcs(pids ...int32) map[int32]*domain.Process {
	m := make(map[int32]*domain.Process, len(pids))
	for _, pid := range pids {
		m[pid] = &domain.Process{PID: pid, Name: fmt.Sprintf("proc-%d", pid)}
	}
	return m
}

// ─── List tests ─────────────────────────────────────────────────────────────

func TestManager_List(t *testing.T) {
	allPIDs := []int32{1, 2, 3, 4, 5}
	procs := makeProcs(allPIDs...)

	tests := []struct {
		name      string
		lister    *stubLister
		opts      ListOptions
		wantCount int
		wantTotal int
		wantErr   bool
	}{
		{
			name:      "no_limit_returns_all",
			lister:    &stubLister{pids: allPIDs, procs: procs},
			opts:      ListOptions{},
			wantCount: 5,
			wantTotal: 5,
		},
		{
			name:      "limit_first_page",
			lister:    &stubLister{pids: allPIDs, procs: procs},
			opts:      ListOptions{Limit: 2, Page: 1},
			wantCount: 2,
			wantTotal: 5,
		},
		{
			name:      "limit_second_page",
			lister:    &stubLister{pids: allPIDs, procs: procs},
			opts:      ListOptions{Limit: 2, Page: 2},
			wantCount: 2,
			wantTotal: 5,
		},
		{
			name:      "limit_last_partial_page",
			lister:    &stubLister{pids: allPIDs, procs: procs},
			opts:      ListOptions{Limit: 3, Page: 2},
			wantCount: 2, // only 2 remaining
			wantTotal: 5,
		},
		{
			name:      "page_beyond_end_returns_empty",
			lister:    &stubLister{pids: allPIDs, procs: procs},
			opts:      ListOptions{Limit: 3, Page: 3},
			wantCount: 0,
			wantTotal: 5,
		},
		{
			name:    "pids_error",
			lister:  &stubLister{pidsErr: errors.New("kernel error")},
			opts:    ListOptions{},
			wantErr: true,
		},
		{
			name: "vanished_process_is_skipped",
			lister: &stubLister{
				pids:    []int32{1, 2, 3},
				procs:   makeProcs(1, 3), // 2 is missing from procs map
				infoErr: map[int32]error{2: errors.New("no such process")},
			},
			opts:      ListOptions{},
			wantCount: 2,
			wantTotal: 3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mgr := NewManagerWithLister(tc.lister, nil)
			procs, total, err := mgr.List(context.Background(), tc.opts)

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(procs) != tc.wantCount {
				t.Errorf("len(procs): got %d, want %d", len(procs), tc.wantCount)
			}
			if total != tc.wantTotal {
				t.Errorf("total: got %d, want %d", total, tc.wantTotal)
			}
		})
	}
}

// ─── Get tests ───────────────────────────────────────────────────────────────

func TestManager_Get(t *testing.T) {
	lister := &stubLister{
		pids:  []int32{42},
		procs: makeProcs(42),
	}

	t.Run("found", func(t *testing.T) {
		mgr := NewManagerWithLister(lister, nil)
		p, err := mgr.Get(context.Background(), 42)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.PID != 42 {
			t.Errorf("PID: got %d, want 42", p.PID)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		mgr := NewManagerWithLister(lister, nil)
		_, err := mgr.Get(context.Background(), 9999)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var ae *apperrors.AppError
		if !errors.As(err, &ae) || ae.Code != apperrors.ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})
}

// ─── Kill tests ───────────────────────────────────────────────────────────────

func TestManager_Kill_AuditEvents(t *testing.T) {
	// Use a buffer-backed audit logger to capture events.
	var events []audit.AuditEvent
	mockLogger := &mockAuditLogger{log: func(e audit.AuditEvent) { events = append(events, e) }}

	t.Run("success_emits_audit_event", func(t *testing.T) {
		events = nil
		// Spawn a child process that sleeps so we can safely kill it.
		cmd := newSleepCmd()
		if err := cmd.Start(); err != nil {
			t.Skip("cannot start child process:", err)
		}
		pid := int32(cmd.Process.Pid)
		defer func() { _ = cmd.Wait() }()

		mgr := NewManagerWithLister(&stubLister{}, mockLogger)
		err := mgr.Kill(context.Background(), pid, "test")
		if err != nil {
			t.Fatalf("unexpected kill error: %v", err)
		}
		if len(events) == 0 {
			t.Error("expected at least one audit event")
		}
		got := events[0]
		if got.Action != "process.kill" {
			t.Errorf("Action: got %q, want %q", got.Action, "process.kill")
		}
		if got.Target != fmt.Sprintf("pid:%d", pid) {
			t.Errorf("Target: got %q, want pid:%d", got.Target, pid)
		}
		if got.Result != "success" {
			t.Errorf("Result: got %q, want %q", got.Result, "success")
		}
	})

	t.Run("not_found_emits_audit_event", func(t *testing.T) {
		events = nil
		mgr := NewManagerWithLister(&stubLister{}, mockLogger)
		err := mgr.Kill(context.Background(), 999999, "test")
		if err == nil {
			t.Fatal("expected error killing non-existent PID")
		}
		if len(events) == 0 {
			t.Error("expected audit event on failure")
		}
		if events[0].Result != "failure" {
			t.Errorf("Result: got %q, want %q", events[0].Result, "failure")
		}
	})

	t.Run("forbidden_returns_appError_and_emits_audit", func(t *testing.T) {
		events = nil
		mgr := NewManagerWithLister(&stubLister{}, mockLogger)
		// PID 1 (init/systemd) always returns EPERM for unprivileged callers.
		err := mgr.Kill(context.Background(), 1, "test")
		if err == nil {
			t.Skip("unexpectedly succeeded killing PID 1 (running as root?)")
		}
		var ae *apperrors.AppError
		if !errors.As(err, &ae) {
			t.Fatalf("expected *AppError, got %T: %v", err, err)
		}
		// Must be FORBIDDEN or NOT_FOUND depending on OS behaviour.
		if ae.Code != apperrors.ErrForbidden && ae.Code != apperrors.ErrNotFound {
			t.Errorf("unexpected error code: %v", ae.Code)
		}
		if len(events) == 0 {
			t.Error("expected audit event")
		}
	})
}

// mockAuditLogger is a test double for audit.Logger.
type mockAuditLogger struct {
	log func(audit.AuditEvent)
}

func (m *mockAuditLogger) Log(e audit.AuditEvent) error {
	m.log(e)
	return nil
}

func (m *mockAuditLogger) Close() error { return nil }
