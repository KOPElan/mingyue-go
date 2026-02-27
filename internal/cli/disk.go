package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"kopelan/mingyue-go/internal/audit"
	diskService "kopelan/mingyue-go/internal/service/disk"
)

func newDiskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "disk",
		Short: "Disk and mount management commands",
		Long:  "Commands for listing mounts, mounting/unmounting filesystems, and querying SMART health.",
	}
	cmd.AddCommand(newDiskListCmd())
	cmd.AddCommand(newDiskMountCmd())
	cmd.AddCommand(newDiskUmountCmd())
	cmd.AddCommand(newDiskSmartCmd())
	return cmd
}

// newDiskListCmd returns `mingyue disk list` — lists all current mount points.
func newDiskListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List current mount points",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			logger := audit.NewFileLogger("")
			defer logger.Close()
			svc := diskService.NewMountService(logger)

			mounts, err := svc.List(ctx)
			if err != nil {
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]interface{}{"mounts": mounts})
			}

			fmt.Printf("%-30s %-30s %-10s  %s\n", "MOUNT POINT", "DEVICE", "FS TYPE", "OPTIONS")
			for _, m := range mounts {
				fmt.Printf("%-30s %-30s %-10s  %s\n",
					truncate(m.MountPoint, 30), truncate(m.Device, 30),
					m.FSType, m.Options)
			}
			fmt.Printf("\n(%d mount(s) found)\n", len(mounts))
			return nil
		},
	}
}

// newDiskMountCmd returns `mingyue disk mount` — mounts a filesystem.
func newDiskMountCmd() *cobra.Command {
	var (
		fsType   string
		readOnly bool
		options  string
		username string
		password string
		domain   string
	)

	cmd := &cobra.Command{
		Use:   "mount --type <fstype> <source> <mountpoint>",
		Short: "Mount a filesystem (local, cifs, or nfs)",
		Long: `Mount a filesystem at the given mount point.

For CIFS mounts, use --username and --password flags to provide credentials.
Credentials are passed via a secure temporary file and are never logged.

Examples:
  mingyue disk mount --type ext4 /dev/sdb1 /mnt/data
  mingyue disk mount --type nfs //server/export /mnt/nfs
  mingyue disk mount --type cifs //server/share /mnt/share --username user --password pass`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			mountpoint := args[1]

			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			logger := audit.NewFileLogger("")
			defer logger.Close()
			svc := diskService.NewMountService(logger)

			opts := diskService.MountOptions{
				Source:     source,
				MountPoint: mountpoint,
				FSType:     fsType,
				ReadOnly:   readOnly,
				Options:    options,
				Username:   username,
				Password:   password,
				Domain:     domain,
			}

			if err := svc.Mount(ctx, opts, "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]interface{}{
					"source":      source,
					"mount_point": mountpoint,
					"result":      "mounted",
				})
			}
			fmt.Printf("Mounted %s at %s\n", source, mountpoint)
			return nil
		},
	}

	cmd.Flags().StringVar(&fsType, "type", "", "filesystem type (e.g. ext4, cifs, nfs, auto)")
	cmd.Flags().BoolVar(&readOnly, "read-only", false, "mount read-only")
	cmd.Flags().StringVar(&options, "options", "", "additional mount options (comma-separated)")
	cmd.Flags().StringVar(&username, "username", "", "CIFS username (never logged)")
	cmd.Flags().StringVar(&password, "password", "", "CIFS password (never logged)")
	cmd.Flags().StringVar(&domain, "domain", "", "CIFS domain (optional)")
	return cmd
}

// newDiskUmountCmd returns `mingyue disk umount <mountpoint>`.
func newDiskUmountCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "umount <mountpoint>",
		Short: "Unmount a filesystem",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mountpoint := args[0]

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			logger := audit.NewFileLogger("")
			defer logger.Close()
			svc := diskService.NewMountService(logger)

			if err := svc.Umount(ctx, mountpoint, "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]interface{}{
					"mount_point": mountpoint,
					"result":      "unmounted",
				})
			}
			fmt.Printf("Unmounted %s\n", mountpoint)
			return nil
		},
	}
}

// newDiskSmartCmd returns `mingyue disk smart <device>`.
func newDiskSmartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "smart <device>",
		Short: "Query SMART health information for a block device",
		Long: `Query SMART (Self-Monitoring, Analysis and Reporting Technology) health
data from the given block device using smartctl.

Requires the smartmontools package and root privileges (or CAP_SYS_RAWIO).

Examples:
  mingyue disk smart /dev/sda
  mingyue disk smart sda   (shorthand, assumes /dev/ prefix)`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			device := args[0]

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			svc := diskService.NewSmartService()
			health, err := svc.Query(ctx, device)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(health)
			}

			status := "PASSED"
			if !health.HealthOK {
				status = "FAILING"
			}
			fmt.Printf("Device        : %s\n", health.Device)
			fmt.Printf("Model         : %s\n", health.Model)
			fmt.Printf("Serial        : %s\n", health.Serial)
			fmt.Printf("Health        : %s\n", status)
			fmt.Printf("Temperature   : %d°C\n", health.Temperature)
			fmt.Printf("Power-On Hours: %d h\n", health.PowerOnHours)
			return nil
		},
	}
}
