// Package system provides host resource monitoring using gopsutil.
// The Monitor type is the shared service used by both CLI and HTTP API handlers.
package system

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"

	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// Collector is the interface that wraps the gopsutil calls used by Monitor.
// It can be replaced with a stub in tests.
type Collector interface {
	// CPUPercent returns the system-wide CPU usage percentage over an interval.
	CPUPercent(ctx context.Context) (float64, error)
	// VirtualMemory returns memory usage statistics.
	VirtualMemory(ctx context.Context) (*mem.VirtualMemoryStat, error)
	// Uptime returns the host uptime in seconds.
	Uptime(ctx context.Context) (uint64, error)
}

// gopsutilCollector is the production Collector backed by gopsutil.
type gopsutilCollector struct{}

func (g *gopsutilCollector) CPUPercent(ctx context.Context) (float64, error) {
	percents, err := cpu.PercentWithContext(ctx, 0, false)
	if err != nil {
		return 0, err
	}
	if len(percents) == 0 {
		return 0, nil
	}
	return percents[0], nil
}

func (g *gopsutilCollector) VirtualMemory(ctx context.Context) (*mem.VirtualMemoryStat, error) {
	return mem.VirtualMemoryWithContext(ctx)
}

func (g *gopsutilCollector) Uptime(ctx context.Context) (uint64, error) {
	return host.UptimeWithContext(ctx)
}

// Monitor collects system metrics and produces HostSnapshot values.
type Monitor struct {
	collector Collector
}

// NewMonitor creates a production Monitor backed by gopsutil.
func NewMonitor() *Monitor {
	return &Monitor{collector: &gopsutilCollector{}}
}

// NewMonitorWithCollector creates a Monitor with a custom Collector (for testing).
func NewMonitorWithCollector(c Collector) *Monitor {
	return &Monitor{collector: c}
}

// Snapshot collects the current host resource snapshot.
// The provided context controls timeout and cancellation for all underlying
// system calls.
func (m *Monitor) Snapshot(ctx context.Context) (*domain.HostSnapshot, error) {
	cpuPct, err := m.collector.CPUPercent(ctx)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternal, "failed to collect CPU usage", err)
	}

	vmStat, err := m.collector.VirtualMemory(ctx)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternal, "failed to collect memory usage", err)
	}

	uptime, err := m.collector.Uptime(ctx)
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternal, "failed to collect uptime", err)
	}

	return &domain.HostSnapshot{
		Timestamp:  time.Now().UTC(),
		CPUPercent: cpuPct,
		MemTotal:   vmStat.Total,
		MemUsed:    vmStat.Used,
		MemPercent: vmStat.UsedPercent,
		Uptime:     uptime,
	}, nil
}
