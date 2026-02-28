package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"kopelan/mingyue-go/internal/audit"
	aclService "kopelan/mingyue-go/internal/service/acl"
)

// aclRoot is the persistent --root flag value shared across all acl sub-commands.
var aclRoot string

func newACLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "acl",
		Short: "File permission and ACL management commands",
		Long: `Commands for querying and setting file/directory permissions and POSIX ACLs.

All operations are scoped to the root directory specified by --root (default: "/").
Setting --root to a specific path limits access to that directory subtree and
prevents path-traversal attacks.

The 'set' sub-command requires setfacl to be installed (acl package).`,
	}
	cmd.PersistentFlags().StringVar(&aclRoot, "root", "/", "root directory that constrains all ACL operations")
	cmd.AddCommand(newACLGetCmd())
	cmd.AddCommand(newACLSetCmd())
	return cmd
}

func newACLMgr() (*aclService.Manager, func()) {
	logger := audit.NewFileLogger("")
	mgr := aclService.NewManager(aclRoot, logger)
	return mgr, func() { logger.Close() }
}

func newACLGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <path>",
		Short: "Show permissions and ACL entries for a file or directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			mgr, cleanup := newACLMgr()
			defer cleanup()

			info, err := mgr.Get(ctx, args[0])
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(info)
			}

			fmt.Printf("Path  : %s\n", info.Path)
			fmt.Printf("Owner : %s\n", info.Owner)
			fmt.Printf("Group : %s\n", info.Group)
			fmt.Printf("Mode  : %s\n", info.Mode)
			if len(info.ACLEntries) > 0 {
				fmt.Println("ACL Entries:")
				for _, e := range info.ACLEntries {
					if e.Qualifier != "" {
						fmt.Printf("  %s:%s:%s\n", e.Type, e.Qualifier, e.Permissions)
					} else {
						fmt.Printf("  %s::%s\n", e.Type, e.Permissions)
					}
				}
			}
			return nil
		},
	}
}

func newACLSetCmd() *cobra.Command {
	var entries []string
	cmd := &cobra.Command{
		Use:   "set <path>",
		Short: "Set POSIX ACL entries on a file or directory (requires operator/admin)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(entries) == 0 {
				return fmt.Errorf("at least one --entry is required")
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			mgr, cleanup := newACLMgr()
			defer cleanup()

			if err := mgr.SetACL(ctx, args[0], entries, "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]interface{}{
					"path":    args[0],
					"entries": entries,
					"result":  "set",
				})
			}
			fmt.Printf("ACL updated: %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&entries, "entry", nil,
		`ACL entry in the form "type:qualifier:perms" (e.g. "u:alice:rwx"). Repeatable.`)
	return cmd
}
