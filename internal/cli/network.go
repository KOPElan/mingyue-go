package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"kopelan/mingyue-go/internal/audit"
	netService "kopelan/mingyue-go/internal/service/network"
)

func newNetworkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "network",
		Short: "Network management commands",
		Long:  "Commands for querying and managing network interfaces and routes.",
	}
	cmd.AddCommand(newNetworkInterfacesCmd())
	cmd.AddCommand(newNetworkRoutesCmd())
	cmd.AddCommand(newNetworkInterfaceCmd())
	return cmd
}

// newNetworkInterfacesCmd returns the `mingyue network interfaces` command.
func newNetworkInterfacesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "interfaces",
		Short: "List network interfaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			logger := audit.NewFileLogger("")
			defer logger.Close()
			mgr := netService.NewManager(logger)

			ifaces, err := mgr.Interfaces()
			if err != nil {
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(ifaces)
			}

			fmt.Printf("%-20s %-20s %-8s  %s\n", "NAME", "HARDWARE ADDR", "STATE", "ADDRESSES")
			for _, iface := range ifaces {
				state := "down"
				if iface.IsUp {
					state = "up"
				}
				addrs := ""
				for i, a := range iface.Addrs {
					if i > 0 {
						addrs += ", "
					}
					addrs += a
				}
				fmt.Printf("%-20s %-20s %-8s  %s\n",
					truncate(iface.Name, 20),
					iface.HardwareAddr,
					state,
					addrs,
				)
			}
			return nil
		},
	}
}

// newNetworkRoutesCmd returns the `mingyue network routes` command.
func newNetworkRoutesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "routes",
		Short: "Show kernel routing table",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			logger := audit.NewFileLogger("")
			defer logger.Close()
			mgr := netService.NewManager(logger)

			routes, err := mgr.Routes(ctx)
			if err != nil {
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(routes)
			}

			fmt.Printf("%-30s %-20s %-15s %s\n", "DESTINATION", "GATEWAY", "INTERFACE", "METRIC")
			for _, r := range routes {
				gw := r.Gateway
				if gw == "" {
					gw = "-"
				}
				metric := r.Metric
				if metric == "" {
					metric = "-"
				}
				fmt.Printf("%-30s %-20s %-15s %s\n", r.Destination, gw, r.Interface, metric)
			}
			return nil
		},
	}
}

// newNetworkInterfaceCmd returns the `mingyue network interface` command group.
func newNetworkInterfaceCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "interface",
		Short: "Manage a specific network interface",
	}
	cmd.AddCommand(newNetworkInterfaceUpCmd())
	cmd.AddCommand(newNetworkInterfaceDownCmd())
	return cmd
}

// newNetworkInterfaceUpCmd returns `mingyue network interface up <name>`.
func newNetworkInterfaceUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up <name>",
		Short: "Bring a network interface up",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			logger := audit.NewFileLogger("")
			defer logger.Close()
			mgr := netService.NewManager(logger)

			if err := mgr.SetInterfaceState(ctx, args[0], true, "cli"); err != nil {
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"interface": args[0], "state": "up"})
			}
			fmt.Printf("Interface %s is up\n", args[0])
			return nil
		},
	}
}

// newNetworkInterfaceDownCmd returns `mingyue network interface down <name>`.
func newNetworkInterfaceDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "down <name>",
		Short: "Bring a network interface down",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			logger := audit.NewFileLogger("")
			defer logger.Close()
			mgr := netService.NewManager(logger)

			if err := mgr.SetInterfaceState(ctx, args[0], false, "cli"); err != nil {
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"interface": args[0], "state": "down"})
			}
			fmt.Printf("Interface %s is down\n", args[0])
			return nil
		},
	}
}
