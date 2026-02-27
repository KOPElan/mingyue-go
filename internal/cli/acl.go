package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"kopelan/mingyue-go/internal/audit"
	aclService "kopelan/mingyue-go/internal/service/acl"
)

func newACLCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "acl",
		Short: "File permission and ACL commands",
		Long:  "Commands for querying and modifying file permissions and ACL entries.",
	}
	cmd.AddCommand(newACLListCmd())
	cmd.AddCommand(newACLSetCmd())
	return cmd
}

// newACLListCmd returns the `mingyue acl list <path>` command.
func newACLListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <path>",
		Short: "Show permissions and ACL entries for a path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			logger := audit.NewFileLogger("")
			defer logger.Close()
			mgr := aclService.NewManager(logger)

			acl, err := mgr.Get(ctx, args[0])
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(acl)
			}

			fmt.Printf("Path  : %s\n", acl.Path)
			fmt.Printf("Mode  : %s\n", acl.Mode)
			fmt.Printf("Owner : %s\n", acl.Owner)
			fmt.Printf("Group : %s\n", acl.Group)
			if len(acl.Entries) > 0 {
				fmt.Println("ACL Entries:")
				for _, e := range acl.Entries {
					name := e.Name
					if name == "" {
						name = "-"
					}
					fmt.Printf("  %-8s %-20s %s\n", e.Type, name, e.Permissions)
				}
			}
			return nil
		},
	}
}

// newACLSetCmd returns the `mingyue acl set <path>` command.
func newACLSetCmd() *cobra.Command {
	var modeStr string
	var owner string
	var group string

	cmd := &cobra.Command{
		Use:   "set <path>",
		Short: "Set permissions, owner, or group for a path",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if modeStr == "" && owner == "" && group == "" {
				return fmt.Errorf("at least one of --mode, --owner, or --group must be specified")
			}

			req := aclService.SetRequest{
				Owner: owner,
				Group: group,
			}
			if modeStr != "" {
				n, err := strconv.ParseUint(modeStr, 8, 32)
				if err != nil {
					fmt.Fprintln(os.Stderr, "Error: --mode must be an octal string, e.g. 0644")
					return err
				}
				req.Mode = os.FileMode(n)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			logger := audit.NewFileLogger("")
			defer logger.Close()
			mgr := aclService.NewManager(logger)

			if err := mgr.Set(ctx, args[0], req, "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"path": args[0], "result": "success"})
			}
			fmt.Printf("Permissions updated: %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&modeStr, "mode", "", "octal permission bits, e.g. 0644")
	cmd.Flags().StringVar(&owner, "owner", "", "owning user name")
	cmd.Flags().StringVar(&group, "group", "", "owning group name")
	return cmd
}
