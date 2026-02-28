package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	shareService "kopelan/mingyue-go/internal/service/share"
)

func newSmbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "smb",
		Short: "Samba (SMB/CIFS) share management",
		Long: `Commands for managing Samba (SMB/CIFS) network shares and users.

Share configuration is persisted to /var/lib/mingyue/shares.json.
Each mutating operation regenerates /etc/samba/smb.conf.d/mingyue.conf
and signals smbd to reload:

  smbcontrol all reload-config

One-time setup — add to /etc/samba/smb.conf:
  include = /etc/samba/smb.conf.d/mingyue.conf

Required permissions: write access to /etc/samba/smb.conf.d/ and smbd running.`,
	}
	cmd.AddCommand(newSmbListCmd())
	cmd.AddCommand(newSmbGetCmd())
	cmd.AddCommand(newSmbCreateCmd())
	cmd.AddCommand(newSmbUpdateCmd())
	cmd.AddCommand(newSmbDeleteCmd())
	cmd.AddCommand(newSmbUserCmd())
	return cmd
}

// ── share CRUD ────────────────────────────────────────────────────────────────

func newSmbListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all Samba shares",
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

			var shares []domain.Share
			for _, s := range all {
				if s.Type == domain.ShareTypeSamba {
					shares = append(shares, s)
				}
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]interface{}{"shares": shares})
			}

			if len(shares) == 0 {
				fmt.Println("No Samba shares configured.")
				return nil
			}
			fmt.Printf("%-20s %-8s  %s\n", "NAME", "ENABLED", "PATH")
			for _, s := range shares {
				enabled := "yes"
				if !s.Enabled {
					enabled = "no"
				}
				fmt.Printf("%-20s %-8s  %s\n", s.Name, enabled, s.Path)
			}
			return nil
		},
	}
}

func newSmbGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show details of a Samba share",
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

			fmt.Printf("Name          : %s\n", s.Name)
			fmt.Printf("Path          : %s\n", s.Path)
			fmt.Printf("Comment       : %s\n", s.Comment)
			fmt.Printf("ReadOnly      : %v\n", s.ReadOnly)
			fmt.Printf("Enabled       : %v\n", s.Enabled)
			fmt.Printf("ValidUsers    : %s\n", s.ValidUsers)
			fmt.Printf("WriteList     : %s\n", s.WriteList)
			fmt.Printf("CreateMask    : %s\n", s.CreateMask)
			fmt.Printf("DirectoryMask : %s\n", s.DirectoryMask)
			return nil
		},
	}
}

func newSmbCreateCmd() *cobra.Command {
	var path, comment string
	var readOnly, enabled bool
	var validUsers, writeList, createMask, dirMask string

	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new Samba share",
		Long: `Create a new Samba share and reload smbd.

The share is written to /var/lib/mingyue/shares.json and appended to
/etc/samba/smb.conf.d/mingyue.conf, then smbd is signalled to reload.

Example:
  mingyue smb create myshare --path /srv/myshare
  mingyue smb create data --path /data --valid-users "alice @staff" --write-list alice`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			mgr, cleanup := newShareMgr()
			defer cleanup()

			s := domain.Share{
				Name:          args[0],
				Type:          domain.ShareTypeSamba,
				Path:          path,
				Comment:       comment,
				ReadOnly:      readOnly,
				Enabled:       enabled,
				ValidUsers:    validUsers,
				WriteList:     writeList,
				CreateMask:    createMask,
				DirectoryMask: dirMask,
			}
			if err := mgr.Create(ctx, s, "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"name": args[0], "result": "created"})
			}
			fmt.Printf("Samba share created: %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "local directory to share (required)")
	cmd.Flags().StringVar(&comment, "comment", "", "optional description")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "export share as read-only")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "enable the share immediately")
	cmd.Flags().StringVar(&validUsers, "valid-users", "", "space/comma-separated users or @groups allowed to connect")
	cmd.Flags().StringVar(&writeList, "write-list", "", "space/comma-separated users or @groups with write access")
	cmd.Flags().StringVar(&createMask, "create-mask", "", "octal file creation mask, e.g. 0644")
	cmd.Flags().StringVar(&dirMask, "dir-mask", "", "octal directory creation mask, e.g. 0755")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func newSmbUpdateCmd() *cobra.Command {
	var path, comment string
	var readOnly, enabled bool
	var validUsers, writeList, createMask, dirMask string

	cmd := &cobra.Command{
		Use:   "update <name>",
		Short: "Update an existing Samba share",
		Long: `Update a Samba share configuration and reload smbd.

All fields are replaced with the supplied values.  Omitting optional flags
clears those settings from the share configuration.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			mgr, cleanup := newShareMgr()
			defer cleanup()

			s := domain.Share{
				Name:          args[0],
				Type:          domain.ShareTypeSamba,
				Path:          path,
				Comment:       comment,
				ReadOnly:      readOnly,
				Enabled:       enabled,
				ValidUsers:    validUsers,
				WriteList:     writeList,
				CreateMask:    createMask,
				DirectoryMask: dirMask,
			}
			if err := mgr.Update(ctx, s, "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"name": args[0], "result": "updated"})
			}
			fmt.Printf("Samba share updated: %s\n", args[0])
			return nil
		},
	}

	cmd.Flags().StringVar(&path, "path", "", "local directory to share (required)")
	cmd.Flags().StringVar(&comment, "comment", "", "optional description")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "export share as read-only")
	cmd.Flags().BoolVar(&enabled, "enabled", true, "enable the share")
	cmd.Flags().StringVar(&validUsers, "valid-users", "", "space/comma-separated users or @groups allowed to connect")
	cmd.Flags().StringVar(&writeList, "write-list", "", "space/comma-separated users or @groups with write access")
	cmd.Flags().StringVar(&createMask, "create-mask", "", "octal file creation mask, e.g. 0644")
	cmd.Flags().StringVar(&dirMask, "dir-mask", "", "octal directory creation mask, e.g. 0755")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func newSmbDeleteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a Samba share",
		Long: `Remove a Samba share from the configuration and reload smbd.

The share is removed from /var/lib/mingyue/shares.json and the Samba
configuration file is regenerated and reloaded.  On reload failure the
share is automatically restored.`,
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
			fmt.Printf("Samba share deleted: %s\n", args[0])
			return nil
		},
	}
}

// ── user management ───────────────────────────────────────────────────────────

func newSmbUserCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "user",
		Short: "Samba user management",
		Long: `Manage Samba user accounts in the local Samba database.

Samba maintains its own password database (tdbsam/samba.passdb) separate
from /etc/shadow.  A Linux user must exist on the system before it can be
added as a Samba user.

Passwords are read from standard input (one line for the password).
Use a pipe for non-interactive use:
  echo "s3cr3t" | mingyue smb user add alice`,
	}
	cmd.AddCommand(newSmbUserListCmd())
	cmd.AddCommand(newSmbUserAddCmd())
	cmd.AddCommand(newSmbUserRemoveCmd())
	cmd.AddCommand(newSmbUserPasswdCmd())
	return cmd
}

func newSmbUserMgr() (*shareService.SambaUserManager, func()) {
	logger := audit.NewFileLogger("")
	mgr := shareService.NewSambaUserManager()
	return mgr, func() { logger.Close() }
}

func newSmbUserListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all Samba users",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			mgr, cleanup := newSmbUserMgr()
			defer cleanup()

			users, err := mgr.ListUsers(ctx)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]interface{}{"users": users})
			}

			if len(users) == 0 {
				fmt.Println("No Samba users found.")
				return nil
			}
			for _, u := range users {
				fmt.Println(u.Username)
			}
			return nil
		},
	}
}

func newSmbUserAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <username>",
		Short: "Add a user to the Samba database",
		Long: `Add an existing Linux user to the Samba password database.

The password is read from standard input:
  echo "mypassword" | mingyue smb user add alice`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			password, err := readPasswordFromStdin()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error reading password:", err)
				return err
			}

			mgr, cleanup := newSmbUserMgr()
			defer cleanup()

			if err := mgr.AddUser(ctx, args[0], password); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"username": args[0], "result": "added"})
			}
			fmt.Printf("Samba user added: %s\n", args[0])
			return nil
		},
	}
}

func newSmbUserRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <username>",
		Short: "Remove a user from the Samba database",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			mgr, cleanup := newSmbUserMgr()
			defer cleanup()

			if err := mgr.RemoveUser(ctx, args[0]); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"username": args[0], "result": "removed"})
			}
			fmt.Printf("Samba user removed: %s\n", args[0])
			return nil
		},
	}
}

func newSmbUserPasswdCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "passwd <username>",
		Short: "Change the Samba password for a user",
		Long: `Change the Samba password for an existing Samba user.

The new password is read from standard input:
  echo "newpassword" | mingyue smb user passwd alice`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			password, err := readPasswordFromStdin()
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error reading password:", err)
				return err
			}

			mgr, cleanup := newSmbUserMgr()
			defer cleanup()

			if err := mgr.SetPassword(ctx, args[0], password); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"username": args[0], "result": "password updated"})
			}
			fmt.Printf("Samba password updated for: %s\n", args[0])
			return nil
		},
	}
}

// readPasswordFromStdin reads a single line from stdin and returns it as the
// password string (without the trailing newline).  It returns an error if
// stdin contains no data or a read error occurs.
// Callers should use a pipe to avoid the password appearing in shell history:
//
//	echo "s3cr3t" | mingyue smb user add alice
func readPasswordFromStdin() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return scanner.Text(), nil
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no password provided on stdin")
}
