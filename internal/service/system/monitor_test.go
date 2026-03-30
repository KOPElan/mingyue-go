package system

import (
	"context"
	"errors"
	"testing"

	"github.com/shirou/gopsutil/v3/mem"

	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// stubCollector is a test double for Collector.
type stubCollector struct {
	cpuPercent float64
	cpuErr     error

	vmStat *mem.VirtualMemoryStat
	vmErr  error

	uptime    uint64
	uptimeErr error
}

func (s *stubCollector) CPUPercent(_ context.Context) (float64, error) {
	return s.cpuPercent, s.cpuErr
}

func (s *stubCollector) VirtualMemory(_ context.Context) (*mem.VirtualMemoryStat, error) {
	return s.vmStat, s.vmErr
}

func (s *stubCollector) Uptime(_ context.Context) (uint64, error) {
	return s.uptime, s.uptimeErr
}

func TestMonitor_Snapshot(t *testing.T) {
	okVM := &mem.VirtualMemoryStat{
		Total:       8 * 1024 * 1024 * 1024, // 8 GB
		Used:        4 * 1024 * 1024 * 1024, // 4 GB
		UsedPercent: 50.0,
	}

	tests := []struct {
		name      string
		collector *stubCollector
		wantErr   bool
		wantCode  apperrors.ErrorCode
		check     func(t *testing.T, s *domain.HostSnapshot)
	}{
		{
			name: "success",
			collector: &stubCollector{
				cpuPercent: 25.5,
				vmStat:     okVM,
				uptime:     3600,
			},
			check: func(t *testing.T, s *domain.HostSnapshot) {
				if s.CPUPercent != 25.5 {
					t.Errorf("CPUPercent: got %v, want 25.5", s.CPUPercent)
				}
				if s.MemTotal != okVM.Total {
					t.Errorf("MemTotal: got %v, want %v", s.MemTotal, okVM.Total)
				}
				if s.MemUsed != okVM.Used {
					t.Errorf("MemUsed: got %v, want %v", s.MemUsed, okVM.Used)
				}
				if s.MemPercent != 50.0 {
					t.Errorf("MemPercent: got %v, want 50.0", s.MemPercent)
				}
				if s.Uptime != 3600 {
					t.Errorf("Uptime: got %v, want 3600", s.Uptime)
				}
				if s.Timestamp.IsZero() {
					t.Error("Timestamp should not be zero")
				}
			},
		},
		{
			name: "cpu_error",
			collector: &stubCollector{
				cpuErr: errors.New("cpu read error"),
				vmStat: okVM,
				uptime: 3600,
			},
			wantErr:  true,
			wantCode: apperrors.ErrInternal,
		},
		{
			name: "memory_error",
			collector: &stubCollector{
				cpuPercent: 10.0,
				vmErr:      errors.New("mem read error"),
				uptime:     3600,
			},
			wantErr:  true,
			wantCode: apperrors.ErrInternal,
		},
		{
			name: "uptime_error",
			collector: &stubCollector{
				cpuPercent: 10.0,
				vmStat:     okVM,
				uptimeErr:  errors.New("uptime read error"),
			},
			wantErr:  true,
			wantCode: apperrors.ErrInternal,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMonitorWithCollector(tc.collector)
			snap, err := m.Snapshot(context.Background())

			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var ae *apperrors.AppError
				if errors.As(err, &ae) {
					if ae.Code != tc.wantCode {
						t.Errorf("error code: got %v, want %v", ae.Code, tc.wantCode)
					}
				} else {
					t.Errorf("expected *AppError, got %T", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if snap == nil {
				t.Fatal("snapshot is nil")
			}
			if tc.check != nil {
				tc.check(t, snap)
			}
		})
	}
}

// ─── benchmarks ──────────────────────────────────────────────────────────────

// BenchmarkMonitor_Snapshot measures the overhead of the Monitor.Snapshot path
// using a stub Collector (i.e. without real /proc reads).  This isolates the
// struct-building and error-handling overhead, not the OS data-collection cost.
// The PRD states a P95 target of < 200 ms for monitoring-class interfaces.
func BenchmarkMonitor_Snapshot(b *testing.B) {
	okVM := &mem.VirtualMemoryStat{
		Total:       8 * 1024 * 1024 * 1024,
		Used:        4 * 1024 * 1024 * 1024,
		UsedPercent: 50.0,
	}
	collector := &stubCollector{
		cpuPercent: 25.5,
		vmStat:     okVM,
		uptime:     3600,
	}
	m := NewMonitorWithCollector(collector)
	ctx := context.Background()

	b.ResetTimer()
	for b.Loop() {
		_, _ = m.Snapshot(ctx)
	}
}
