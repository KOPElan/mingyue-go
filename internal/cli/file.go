package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"kopelan/mingyue-go/internal/audit"
	fileService "kopelan/mingyue-go/internal/service/file"
)

func newFileCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "file",
		Short: "File management commands",
		Long:  "Commands for listing, reading, writing, and managing files and directories.",
	}
	cmd.AddCommand(newFileListCmd())
	cmd.AddCommand(newFileStatCmd())
	cmd.AddCommand(newFileMkdirCmd())
	cmd.AddCommand(newFileRmCmd())
	cmd.AddCommand(newFileMvCmd())
	cmd.AddCommand(newFileCpCmd())
	cmd.AddCommand(newFileReadCmd())
	cmd.AddCommand(newFileWriteCmd())
	return cmd
}

func newFileMgr() (*fileService.Manager, func()) {
	logger := audit.NewFileLogger("")
	mgr := fileService.NewManager("", logger)
	return mgr, func() { logger.Close() }
}

func newFileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list <path>",
		Short: "List directory contents",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			mgr, cleanup := newFileMgr()
			defer cleanup()

			entries, err := mgr.List(ctx, args[0])
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]interface{}{
					"path":    args[0],
					"entries": entries,
				})
			}

			fmt.Printf("%-8s %-12s %-10s  %s\n", "TYPE", "SIZE", "MODE", "NAME")
			for _, e := range entries {
				typ := "file"
				if e.IsDir {
					typ = "dir"
				}
				fmt.Printf("%-8s %-12s %-10s  %s\n",
					typ, formatBytes(uint64(e.Size)), e.Mode, e.Name)
			}
			return nil
		},
	}
}

func newFileStatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stat <path>",
		Short: "Show file or directory metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			mgr, cleanup := newFileMgr()
			defer cleanup()

			fe, err := mgr.Stat(ctx, args[0])
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(fe)
			}

			typ := "file"
			if fe.IsDir {
				typ = "directory"
			}
			fmt.Printf("Path    : %s\n", fe.Path)
			fmt.Printf("Type    : %s\n", typ)
			fmt.Printf("Size    : %s\n", formatBytes(uint64(fe.Size)))
			fmt.Printf("Mode    : %s\n", fe.Mode)
			fmt.Printf("ModTime : %s\n", fe.ModTime.Format(time.RFC3339))
			fmt.Printf("Owner   : %s\n", fe.Owner)
			return nil
		},
	}
}

func newFileMkdirCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mkdir <path>",
		Short: "Create a directory (and all parents)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			mgr, cleanup := newFileMgr()
			defer cleanup()

			if err := mgr.Mkdir(ctx, args[0], "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"path": args[0], "result": "created"})
			}
			fmt.Printf("Directory created: %s\n", args[0])
			return nil
		},
	}
}

func newFileRmCmd() *cobra.Command {
	var recursive bool
	cmd := &cobra.Command{
		Use:   "rm <path>",
		Short: "Remove a file or directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			mgr, cleanup := newFileMgr()
			defer cleanup()

			if err := mgr.Remove(ctx, args[0], recursive, "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"path": args[0], "result": "removed"})
			}
			fmt.Printf("Removed: %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVarP(&recursive, "recursive", "r", false, "remove directory and its contents recursively")
	return cmd
}

func newFileMvCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mv <src> <dst>",
		Short: "Move (rename) a file or directory",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			mgr, cleanup := newFileMgr()
			defer cleanup()

			if err := mgr.Move(ctx, args[0], args[1], "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"src": args[0], "dst": args[1], "result": "moved"})
			}
			fmt.Printf("Moved: %s → %s\n", args[0], args[1])
			return nil
		},
	}
}

func newFileCpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "cp <src> <dst>",
		Short: "Copy a file",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			mgr, cleanup := newFileMgr()
			defer cleanup()

			if err := mgr.Copy(ctx, args[0], args[1], "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"src": args[0], "dst": args[1], "result": "copied"})
			}
			fmt.Printf("Copied: %s → %s\n", args[0], args[1])
			return nil
		},
	}
}

func newFileReadCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "read <path>",
		Short: "Print the contents of a file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			mgr, cleanup := newFileMgr()
			defer cleanup()

			data, err := mgr.Read(ctx, args[0])
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"path": args[0], "content": string(data)})
			}
			fmt.Print(string(data))
			return nil
		},
	}
}

func newFileWriteCmd() *cobra.Command {
	var content string
	cmd := &cobra.Command{
		Use:   "write <path>",
		Short: "Write content to a file (creates or overwrites)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			mgr, cleanup := newFileMgr()
			defer cleanup()

			if err := mgr.Write(ctx, args[0], []byte(content), "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"path": args[0], "result": "written"})
			}
			fmt.Printf("Written: %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&content, "content", "", "content to write to the file")
	return cmd
}
