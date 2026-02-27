package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"kopelan/mingyue-go/internal/audit"
	procService "kopelan/mingyue-go/internal/service/process"
)

func newProcessCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "process",
		Short: "Process management commands",
		Long:  "Commands for querying and managing OS processes.",
	}
	cmd.AddCommand(newProcessListCmd())
	cmd.AddCommand(newProcessGetCmd())
	cmd.AddCommand(newProcessKillCmd())
	return cmd
}

func newProcessListCmd() *cobra.Command {
	var limit int
	var page int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List running processes",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			logger := audit.NewFileLogger("")
			defer logger.Close()
			mgr := procService.NewManager(logger)
			procs, total, err := mgr.List(ctx, procService.ListOptions{Limit: limit, Page: page})
			if err != nil {
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]interface{}{
					"total":     total,
					"processes": procs,
				})
			}

			fmt.Printf("%-8s %-20s %-8s %8s %10s  %s\n", "PID", "NAME", "STATUS", "CPU%", "MEM(RSS)", "USER")
			for _, p := range procs {
				fmt.Printf("%-8d %-20s %-8s %8.1f %10s  %s\n",
					p.PID, truncate(p.Name, 20), p.Status,
					p.CPUPercent, formatBytes(p.MemRSS), p.User)
			}
			fmt.Printf("\n(%d of %d processes shown)\n", len(procs), total)
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 0, "maximum number of processes to return (0 = all)")
	cmd.Flags().IntVar(&page, "page", 1, "page number when --limit is set")
	return cmd
}

func newProcessGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <pid>",
		Short: "Show details of a specific process",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, err := parsePID(args[0])
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error: invalid pid:", args[0])
				return err
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			logger := audit.NewFileLogger("")
			defer logger.Close()
			mgr := procService.NewManager(logger)
			proc, err := mgr.Get(ctx, pid)
			if err != nil {
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(proc)
			}

			fmt.Printf("PID     : %d\n", proc.PID)
			fmt.Printf("Name    : %s\n", proc.Name)
			fmt.Printf("Status  : %s\n", proc.Status)
			fmt.Printf("CPU%%    : %.2f\n", proc.CPUPercent)
			fmt.Printf("Mem RSS : %s\n", formatBytes(proc.MemRSS))
			fmt.Printf("User    : %s\n", proc.User)
			fmt.Printf("Cmdline : %s\n", proc.Cmdline)
			return nil
		},
	}
}

func newProcessKillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "kill <pid>",
		Short: "Send SIGTERM to a process",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, err := parsePID(args[0])
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error: invalid pid:", args[0])
				return err
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			logger := audit.NewFileLogger("")
			defer logger.Close()
			mgr := procService.NewManager(logger)
			if err := mgr.Kill(ctx, pid, "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]interface{}{
					"pid":    pid,
					"result": "signal sent",
				})
			}
			fmt.Printf("SIGTERM sent to process %d\n", pid)
			return nil
		},
	}
}

// parsePID converts a string to int32 suitable for use as a PID.
func parsePID(s string) (int32, error) {
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid pid %q: %w", s, err)
	}
	return int32(n), nil
}

// truncate shortens s to at most n runes, appending "…" when truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}
