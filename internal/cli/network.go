package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"kopelan/mingyue-go/internal/audit"
	netService "kopelan/mingyue-go/internal/service/network"
)

func newNetworkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network",
		Short: "Network interface management commands",
		Long: `Commands for querying and managing network interfaces.

Read-only operations (list, get) require no special permissions.
Mutating operations (up, down, dhcp) require admin privileges on the host.`,
	}
	cmd.AddCommand(newNetworkListCmd())
	cmd.AddCommand(newNetworkGetCmd())
	cmd.AddCommand(newNetworkUpCmd())
	cmd.AddCommand(newNetworkDownCmd())
	cmd.AddCommand(newNetworkDHCPCmd())
	return cmd
}

func newNetMgr() (*netService.Manager, func()) {
	logger := audit.NewFileLogger("")
	mgr := netService.NewManager(logger)
	return mgr, func() { logger.Close() }
}

func newNetworkListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all network interfaces",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			mgr, cleanup := newNetMgr()
			defer cleanup()

			ifaces, err := mgr.ListInterfaces(ctx)
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]interface{}{"interfaces": ifaces})
			}

			fmt.Printf("%-20s %-8s %-20s  %s\n", "NAME", "INDEX", "FLAGS", "ADDRESSES")
			for _, iface := range ifaces {
				addrs := make([]string, 0, len(iface.Addresses))
				for _, a := range iface.Addresses {
					addrs = append(addrs, fmt.Sprintf("%s/%d", a.IP, a.Prefix))
				}
				fmt.Printf("%-20s %-8d %-20s  %s\n",
					iface.Name,
					iface.Index,
					strings.Join(iface.Flags, ","),
					strings.Join(addrs, ", "))
			}
			return nil
		},
	}
}

func newNetworkGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "get <name>",
		Short: "Show details for a network interface",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			mgr, cleanup := newNetMgr()
			defer cleanup()

			iface, err := mgr.GetInterface(ctx, args[0])
			if err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(iface)
			}

			fmt.Printf("Name    : %s\n", iface.Name)
			fmt.Printf("Index   : %d\n", iface.Index)
			fmt.Printf("MTU     : %d\n", iface.MTU)
			fmt.Printf("HWAddr  : %s\n", iface.HardwareAddr)
			fmt.Printf("Flags   : %s\n", strings.Join(iface.Flags, ", "))
			fmt.Printf("Addresses:\n")
			for _, a := range iface.Addresses {
				fmt.Printf("  %s/%d (%s)\n", a.IP, a.Prefix, a.Family)
			}
			return nil
		},
	}
}

func newNetworkUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up <name>",
		Short: "Bring a network interface up (requires admin)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			mgr, cleanup := newNetMgr()
			defer cleanup()

			if err := mgr.SetLinkUp(ctx, args[0], "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"interface": args[0], "result": "up"})
			}
			fmt.Printf("Interface %s is now up\n", args[0])
			return nil
		},
	}
}

func newNetworkDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down <name>",
		Short: "Bring a network interface down (requires admin)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			mgr, cleanup := newNetMgr()
			defer cleanup()

			if err := mgr.SetLinkDown(ctx, args[0], "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"interface": args[0], "result": "down"})
			}
			fmt.Printf("Interface %s is now down\n", args[0])
			return nil
		},
	}
}

func newNetworkDHCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dhcp <name>",
		Short: "Renew DHCP lease on a network interface (requires admin)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			mgr, cleanup := newNetMgr()
			defer cleanup()

			if err := mgr.RenewDHCP(ctx, args[0], "cli"); err != nil {
				fmt.Fprintln(os.Stderr, "Error:", err)
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"interface": args[0], "result": "dhcp-renewed"})
			}
			fmt.Printf("DHCP lease renewed on %s\n", args[0])
			return nil
		},
	}
}
