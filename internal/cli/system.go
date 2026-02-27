package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"kopelan/mingyue-go/internal/service/system"
)

func newSystemCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "system",
		Short: "System monitoring commands",
		Long:  "Commands for querying CPU, memory, and uptime information.",
	}
	cmd.AddCommand(newSystemOverviewCmd())
	return cmd
}

func newSystemOverviewCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "overview",
		Short: "Show a snapshot of host resource usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			monitor := system.NewMonitor()
			snap, err := monitor.Snapshot(ctx)
			if err != nil {
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(snap)
			}

			fmt.Printf("Timestamp  : %s\n", snap.Timestamp.Format(time.RFC3339))
			fmt.Printf("CPU        : %.1f%%\n", snap.CPUPercent)
			fmt.Printf("Memory     : %s / %s (%.1f%%)\n",
				formatBytes(snap.MemUsed), formatBytes(snap.MemTotal), snap.MemPercent)
			fmt.Printf("Uptime     : %s\n", formatUptime(snap.Uptime))
			return nil
		},
	}
}

// formatBytes converts a byte count to a human-readable string.
func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// formatUptime converts seconds to a "Xd Xh Xm Xs" string.
func formatUptime(secs uint64) string {
	d := secs / 86400
	secs %= 86400
	h := secs / 3600
	secs %= 3600
	m := secs / 60
	s := secs % 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", d, h, m, s)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	return fmt.Sprintf("%dm %ds", m, s)
}
