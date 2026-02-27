package network

import (
	"context"
	"errors"
	"testing"

	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// ─── stubs ────────────────────────────────────────────────────────────────────

type stubReader struct {
	ifaces    []domain.NetworkInterface
	ifacesErr error
	routes    []domain.Route
	routesErr error
}

func (s *stubReader) Interfaces() ([]domain.NetworkInterface, error) {
	return s.ifaces, s.ifacesErr
}

func (s *stubReader) Routes(_ context.Context) ([]domain.Route, error) {
	return s.routes, s.routesErr
}

type stubCommander struct {
	called bool
	err    error
}

func (s *stubCommander) SetInterfaceState(_ context.Context, _ string, _ bool) error {
	s.called = true
	return s.err
}

type mockAuditLogger struct {
	events []audit.AuditEvent
}

func (m *mockAuditLogger) Log(e audit.AuditEvent) error {
	m.events = append(m.events, e)
	return nil
}

func (m *mockAuditLogger) Close() error { return nil }

// ─── Interfaces tests ─────────────────────────────────────────────────────────

func TestManager_Interfaces(t *testing.T) {
	tests := []struct {
		name      string
		reader    *stubReader
		wantCount int
		wantErr   bool
	}{
		{
			name: "returns_interfaces",
			reader: &stubReader{
				ifaces: []domain.NetworkInterface{
					{Name: "eth0", IsUp: true},
					{Name: "lo", IsUp: true},
				},
			},
			wantCount: 2,
		},
		{
			name:    "reader_error_wrapped",
			reader:  &stubReader{ifacesErr: errors.New("no network")},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mgr := NewManagerWithDeps(tc.reader, &stubCommander{}, nil)
			ifaces, err := mgr.Interfaces()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var ae *apperrors.AppError
				if !errors.As(err, &ae) || ae.Code != apperrors.ErrInternal {
					t.Errorf("expected ErrInternal, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(ifaces) != tc.wantCount {
				t.Errorf("len(ifaces): got %d, want %d", len(ifaces), tc.wantCount)
			}
		})
	}
}

// ─── Routes tests ─────────────────────────────────────────────────────────────

func TestManager_Routes(t *testing.T) {
	tests := []struct {
		name      string
		reader    *stubReader
		wantCount int
		wantErr   bool
	}{
		{
			name: "returns_routes",
			reader: &stubReader{
				routes: []domain.Route{
					{Destination: "default", Gateway: "192.168.1.1", Interface: "eth0"},
					{Destination: "10.0.0.0/8", Interface: "eth0"},
				},
			},
			wantCount: 2,
		},
		{
			name:    "reader_error_wrapped",
			reader:  &stubReader{routesErr: errors.New("ip not found")},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mgr := NewManagerWithDeps(tc.reader, &stubCommander{}, nil)
			routes, err := mgr.Routes(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var ae *apperrors.AppError
				if !errors.As(err, &ae) || ae.Code != apperrors.ErrInternal {
					t.Errorf("expected ErrInternal, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(routes) != tc.wantCount {
				t.Errorf("len(routes): got %d, want %d", len(routes), tc.wantCount)
			}
		})
	}
}

// ─── SetInterfaceState tests ──────────────────────────────────────────────────

func TestManager_SetInterfaceState(t *testing.T) {
	t.Run("success_emits_audit_event", func(t *testing.T) {
		logger := &mockAuditLogger{}
		cmd := &stubCommander{}
		mgr := NewManagerWithDeps(&stubReader{}, cmd, logger)

		if err := mgr.SetInterfaceState(context.Background(), "eth0", true, "test"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cmd.called {
			t.Error("expected commander to be called")
		}
		if len(logger.events) == 0 {
			t.Fatal("expected audit event")
		}
		event := logger.events[0]
		if event.Action != "network.interface.up" {
			t.Errorf("Action: got %q, want %q", event.Action, "network.interface.up")
		}
		if event.Target != "eth0" {
			t.Errorf("Target: got %q, want %q", event.Target, "eth0")
		}
		if event.Result != "success" {
			t.Errorf("Result: got %q, want %q", event.Result, "success")
		}
	})

	t.Run("down_action_name", func(t *testing.T) {
		logger := &mockAuditLogger{}
		cmd := &stubCommander{}
		mgr := NewManagerWithDeps(&stubReader{}, cmd, logger)

		if err := mgr.SetInterfaceState(context.Background(), "eth0", false, "test"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(logger.events) == 0 {
			t.Fatal("expected audit event")
		}
		if logger.events[0].Action != "network.interface.down" {
			t.Errorf("Action: got %q, want %q", logger.events[0].Action, "network.interface.down")
		}
	})

	t.Run("commander_error_emits_failure_audit", func(t *testing.T) {
		logger := &mockAuditLogger{}
		cmd := &stubCommander{err: errors.New("operation not permitted")}
		mgr := NewManagerWithDeps(&stubReader{}, cmd, logger)

		err := mgr.SetInterfaceState(context.Background(), "eth0", true, "test")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var ae *apperrors.AppError
		if !errors.As(err, &ae) || ae.Code != apperrors.ErrInternal {
			t.Errorf("expected ErrInternal, got %v", err)
		}
		if len(logger.events) == 0 {
			t.Fatal("expected audit event on failure")
		}
		if logger.events[0].Result != "failure" {
			t.Errorf("Result: got %q, want %q", logger.events[0].Result, "failure")
		}
	})
}

// ─── parseIPRoutes tests ──────────────────────────────────────────────────────

func TestParseIPRoutes(t *testing.T) {
	input := `default via 192.168.1.1 dev eth0 proto dhcp metric 100
10.0.0.0/8 dev eth0 proto kernel scope link src 10.0.0.1
192.168.1.0/24 dev eth0 proto kernel scope link src 192.168.1.100`

	routes := parseIPRoutes(input)
	if len(routes) != 3 {
		t.Fatalf("len(routes): got %d, want 3", len(routes))
	}

	if routes[0].Destination != "default" {
		t.Errorf("routes[0].Destination: got %q, want %q", routes[0].Destination, "default")
	}
	if routes[0].Gateway != "192.168.1.1" {
		t.Errorf("routes[0].Gateway: got %q, want %q", routes[0].Gateway, "192.168.1.1")
	}
	if routes[0].Interface != "eth0" {
		t.Errorf("routes[0].Interface: got %q, want %q", routes[0].Interface, "eth0")
	}
	if routes[0].Metric != "100" {
		t.Errorf("routes[0].Metric: got %q, want %q", routes[0].Metric, "100")
	}

	if routes[1].Destination != "10.0.0.0/8" {
		t.Errorf("routes[1].Destination: got %q, want %q", routes[1].Destination, "10.0.0.0/8")
	}
	if routes[1].Gateway != "" {
		t.Errorf("routes[1].Gateway: got %q, want empty", routes[1].Gateway)
	}
}
