package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	shareService "kopelan/mingyue-go/internal/service/share"
)

func newShareCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:        "share",
		Short:      "Network share management (deprecated: use 'smb' or 'nfs')",
		Deprecated: "use 'mingyue smb' for Samba shares and 'mingyue nfs' for NFS exports.",
		Long: `Legacy unified share management commands (Samba + NFS).

This command group is deprecated. Use the protocol-specific commands instead:
  mingyue smb  — Samba (SMB/CIFS) share management and user administration
  mingyue nfs  — NFS export management

Share configuration is persisted to /var/lib/mingyue/shares.json and
survives process restarts. Each create/update/delete operation also
regenerates the relevant service configuration files and signals a reload:

  Samba shares  → /etc/samba/smb.conf.d/mingyue.conf
                  (reload via: smbcontrol all reload-config)
  NFS shares    → /etc/exports.d/mingyue.exports
                  (reload via: exportfs -ra)`,
	}
	cmd.AddCommand(newShareListCmd())
	cmd.AddCommand(newShareGetCmd())
	cmd.AddCommand(newShareCreateCmd())
	cmd.AddCommand(newShareUpdateCmd())
	cmd.AddCommand(newShareDeleteCmd())
	return cmd
}

func newShareMgr() (*shareService.Manager, func()) {
	logger := audit.NewFileLogger("")
	mgr := shareService.NewManager(logger)
	return mgr, func() { logger.Close() }
}

func newShareListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all configured network shares",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			mgr, cleanup := newShareMgr()
			defer cleanup()

			shares, err := mgr.List(ctx)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]interface{}{"shares": shares})
			}

			if len(shares) == 0 {
				fmt.Println("No shares configured.")
				return nil
			}
			fmt.Printf("%-20s %-6s %-8s  %s\n", "NAME", "TYPE", "ENABLED", "PATH")
			for _, s := range shares {
				enabled := "yes"
				if !s.Enabled {
					enabled = "no"
				}
				fmt.Printf("%-20s %-6s %-8s  %s\n", s.Name, s.Type, enabled, s.Path)
			}
			return nil
		},
	}
}

func newShareGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show details of a specific share",
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

			fmt.Printf("Name     : %s\n", s.Name)
			fmt.Printf("Type     : %s\n", s.Type)
			fmt.Printf("Path     : %s\n", s.Path)
			fmt.Printf("Comment  : %s\n", s.Comment)
			fmt.Printf("ReadOnly : %v\n", s.ReadOnly)
			fmt.Printf("Enabled  : %v\n", s.Enabled)
			return nil
		},
	}
}

func newShareCreateCmd() *cobra.Command {
	var shareType, path, comment string
	var readOnly, enabled bool

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new network share",
		Long: `Create a new Samba (smb) or NFS (nfs) network share and reload the service.

The share is appended to /var/lib/mingyue/shares.json and the relevant
service configuration file is regenerated and reloaded immediately.

Example — create a read-write Samba share:
  mingyue share create myshare --path /srv/myshare --type smb

Example — create a read-only NFS export:
  mingyue share create nfsdata --path /data/nfs --type nfs --read-only`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			mgr, cleanup := newShareMgr()
			defer cleanup()

			s := domain.Share{
				Name:     args[0],
				Type:     domain.ShareType(shareType),
				Path:     path,
				Comment:  comment,
				ReadOnly: readOnly,
				Enabled:  enabled,
			}

			if err := mgr.Create(ctx, s, "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"name": args[0], "result": "created"})
			}
			fmt.Printf("Share created: %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&shareType, "type", "smb", "share type: smb or nfs")
	cmd.Flags().StringVar(&path, "path", "", "local directory to share (required)")
	cmd.Flags().StringVar(&comment, "comment", "", "optional description")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "export share as read-only")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "enable the share immediately")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func newShareUpdateCmd() *cobra.Command {
	var shareType, path, comment string
	var readOnly, enabled bool

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an existing network share",
		Long: `Update the configuration of an existing share and reload the service.

The share record is updated in /var/lib/mingyue/shares.json and the
service configuration file is regenerated and reloaded immediately.
If the reload fails, the previous configuration is automatically restored.

Disabling a share (--enabled=false) keeps it in the configuration but
removes it from the active Samba/NFS service until re-enabled.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			mgr, cleanup := newShareMgr()
			defer cleanup()

			s := domain.Share{
				Name:     args[0],
				Type:     domain.ShareType(shareType),
				Path:     path,
				Comment:  comment,
				ReadOnly: readOnly,
				Enabled:  enabled,
			}

			if err := mgr.Update(ctx, s, "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"name": args[0], "result": "updated"})
			}
			fmt.Printf("Share updated: %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&shareType, "type", "smb", "share type: smb or nfs")
	cmd.Flags().StringVar(&path, "path", "", "local directory to share (required)")
	cmd.Flags().StringVar(&comment, "comment", "", "optional description")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "export share as read-only")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "enable the share")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func newShareDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a network share",
		Long: `Remove a share from the configuration and reload the service.

The share is removed from /var/lib/mingyue/shares.json and the service
configuration file is regenerated and reloaded immediately.
If the reload fails, the share is automatically re-added to preserve
a consistent state.`,
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
			fmt.Printf("Share deleted: %s\n", args[0])
			return nil
		},
	}
}
