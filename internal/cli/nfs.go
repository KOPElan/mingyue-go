package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"kopelan/mingyue-go/internal/domain"
)

func newNfsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "nfs",
		Short: "NFS export management",
		Long: `Commands for managing NFS (Network File System) exports.

Export configuration is persisted to /var/lib/mingyue/shares.json.
Each mutating operation regenerates /etc/exports.d/mingyue.exports
and signals the NFS server to reload:

  exportfs -ra

One-time setup: ensure /etc/exports.d/ is included by /etc/exports.
Most distributions include the line:
  /etc/exports.d/*.exports

Required permissions: write access to /etc/exports.d/ and nfs-kernel-server running.`,
	}
	cmd.AddCommand(newNfsListCmd())
	cmd.AddCommand(newNfsGetCmd())
	cmd.AddCommand(newNfsCreateCmd())
	cmd.AddCommand(newNfsUpdateCmd())
	cmd.AddCommand(newNfsDeleteCmd())
	return cmd
}

func newNfsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all NFS exports",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			mgr, cleanup := newShareMgr()
			defer cleanup()

			all, err := mgr.List(ctx)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			var exports []domain.Share
			for _, s := range all {
				if s.Type == domain.ShareTypeNFS {
					exports = append(exports, s)
				}
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]interface{}{"exports": exports})
			}

			if len(exports) == 0 {
				fmt.Println("No NFS exports configured.")
				return nil
			}
			fmt.Printf("%-20s %-8s %-16s  %s\n", "NAME", "ENABLED", "HOSTS", "PATH")
			for _, s := range exports {
				enabled := "yes"
				if !s.Enabled {
					enabled = "no"
				}
				hosts := s.Hosts
				if hosts == "" {
					hosts = "*"
				}
				fmt.Printf("%-20s %-8s %-16s  %s\n", s.Name, enabled, hosts, s.Path)
			}
			return nil
		},
	}
}

func newNfsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show details of an NFS export",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			mgr, cleanup := newShareMgr()
			defer cleanup()

			s, err := mgr.Get(ctx, args[0])
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(s)
			}

			hosts := s.Hosts
			if hosts == "" {
				hosts = "*"
			}
			fmt.Printf("Name     : %s\n", s.Name)
			fmt.Printf("Path     : %s\n", s.Path)
			fmt.Printf("Comment  : %s\n", s.Comment)
			fmt.Printf("ReadOnly : %v\n", s.ReadOnly)
			fmt.Printf("Enabled  : %v\n", s.Enabled)
			fmt.Printf("Hosts    : %s\n", hosts)
			return nil
		},
	}
}

func newNfsCreateCmd() *cobra.Command {
	var path, comment, hosts string
	var readOnly, enabled bool

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new NFS export",
		Long: `Create a new NFS export and reload the NFS server.

The export is written to /var/lib/mingyue/shares.json and appended to
/etc/exports.d/mingyue.exports, then exportfs is called to reload.

Example:
  mingyue nfs create data --path /data/nfs
  mingyue nfs create restricted --path /srv/data --hosts "192.168.1.0/24 10.0.0.5" --read-only`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			mgr, cleanup := newShareMgr()
			defer cleanup()

			s := domain.Share{
				Name:     args[0],
				Type:     domain.ShareTypeNFS,
				Path:     path,
				Comment:  comment,
				ReadOnly: readOnly,
				Enabled:  enabled,
				Hosts:    hosts,
			}
			if err := mgr.Create(ctx, s, "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"name": args[0], "result": "created"})
			}
			fmt.Printf("NFS export created: %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "local directory to export (required)")
	cmd.Flags().StringVar(&comment, "comment", "", "optional description")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "export as read-only")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "enable the export immediately")
	cmd.Flags().StringVar(&hosts, "hosts", "", `space-separated hosts/CIDRs allowed to mount (default: * = all)`)
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func newNfsUpdateCmd() *cobra.Command {
	var path, comment, hosts string
	var readOnly, enabled bool

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an existing NFS export",
		Long: `Update an NFS export configuration and reload the NFS server.

All fields are replaced with the supplied values.  Omitting optional flags
clears those settings from the export configuration.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			mgr, cleanup := newShareMgr()
			defer cleanup()

			s := domain.Share{
				Name:     args[0],
				Type:     domain.ShareTypeNFS,
				Path:     path,
				Comment:  comment,
				ReadOnly: readOnly,
				Enabled:  enabled,
				Hosts:    hosts,
			}
			if err := mgr.Update(ctx, s, "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"name": args[0], "result": "updated"})
			}
			fmt.Printf("NFS export updated: %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "local directory to export (required)")
	cmd.Flags().StringVar(&comment, "comment", "", "optional description")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "export as read-only")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "enable the export")
	cmd.Flags().StringVar(&hosts, "hosts", "", `space-separated hosts/CIDRs allowed to mount (default: * = all)`)
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func newNfsDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an NFS export",
		Long: `Remove an NFS export from the configuration and reload the NFS server.

The export is removed from /var/lib/mingyue/shares.json and the exports
file is regenerated and reloaded.  On reload failure the export is
automatically restored.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			mgr, cleanup := newShareMgr()
			defer cleanup()

			if err := mgr.Delete(ctx, args[0], "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"name": args[0], "result": "deleted"})
			}
			fmt.Printf("NFS export deleted: %s\n", args[0])
			return nil
		},
	}
}
