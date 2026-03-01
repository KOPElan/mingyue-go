package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"kopelan/mingyue-go/internal/agent"
	"kopelan/mingyue-go/internal/api"
	"kopelan/mingyue-go/internal/auth"
	"kopelan/mingyue-go/internal/discovery"
)

func newAgentCmd() *cobra.Command {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage the mingyue daemon",
		Long:  "Start, stop, query the status of, or discover mingyue agent instances.",
	}

	agentCmd.AddCommand(newAgentStartCmd())
	agentCmd.AddCommand(newAgentStopCmd())
	agentCmd.AddCommand(newAgentStatusCmd())
	agentCmd.AddCommand(newAgentDiscoverCmd())

	return agentCmd
}

func newAgentStartCmd() *cobra.Command {
	var listenAddr, keystorePath string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the mingyue daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			if keystorePath == "" {
				keystorePath = auth.DefaultKeystorePath
			}

			d := agent.NewDaemon(listenAddr)

			// Load persisted API keys into the in-memory store so that
			// authenticated requests can be served immediately after start.
			entries, err := auth.LoadKeyStore(keystorePath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not load keystore %s: %v\n", keystorePath, err)
			}

			// On first run (empty keystore) generate an initial admin key so
			// that the web frontend can authenticate without manual setup.
			if len(entries) == 0 {
				key, err := auth.GenerateKey()
				if err != nil {
					return fmt.Errorf("generate initial key: %w", err)
				}
				entry := auth.KeyEntry{
					Key:       key,
					Role:      auth.RoleAdmin,
					Subject:   "initial-admin",
					CreatedAt: time.Now().UTC(),
				}
				if saveErr := auth.SaveKeyEntry(keystorePath, entry); saveErr != nil {
					// Non-fatal: print to stderr and carry on with the
					// in-memory key only.
					fmt.Fprintf(os.Stderr, "warning: could not persist initial key: %v\n", saveErr)
					auth.RegisterAPIKey(key, auth.Token{Raw: key, Role: auth.RoleAdmin, Subject: "initial-admin"})
				}
				if IsJSONOutput() {
					_ = WriteJSON(map[string]string{
						"status":      "starting",
						"address":     d.ListenAddr,
						"pidFile":     d.PIDPath,
						"initialKey":  key,
						"initialRole": "admin",
					})
				} else {
					fmt.Printf("Starting mingyue daemon on %s (pid file: %s)\n", d.ListenAddr, d.PIDPath)
					fmt.Printf("\n*** Initial admin API key (save this) ***\n%s\n\n", key)
				}
			} else {
				if IsJSONOutput() {
					_ = WriteJSON(map[string]string{
						"status":  "starting",
						"address": d.ListenAddr,
						"pidFile": d.PIDPath,
					})
				} else {
					fmt.Printf("Starting mingyue daemon on %s (pid file: %s)\n", d.ListenAddr, d.PIDPath)
				}
			}

			// Advertise on LAN so web frontends can discover this agent.
			hostname, err := os.Hostname()
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not resolve hostname for discovery: %v\n", err)
				hostname = "unknown"
			}
			advCtx, advCancel := context.WithCancel(context.Background())
			defer advCancel()
			go func() {
				_ = discovery.Advertise(advCtx, discovery.AgentInfo{
					Hostname: hostname,
					Addr:     d.ListenAddr,
					Version:  BuildVersion,
				})
			}()

			router := api.NewRouter()
			defer router.Close()
			return d.Start(router)
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", "", "HTTP listen address (default :7070)")
	cmd.Flags().StringVar(&keystorePath, "keystore", "", "API key store file (default: "+auth.DefaultKeystorePath+")")
	return cmd
}

func newAgentStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the running mingyue daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			d := agent.NewDaemon("")

			if err := d.Stop(); err != nil {
				return err
			}

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"status": "stopped"})
			}
			fmt.Println("mingyue daemon stopped.")
			return nil
		},
	}
}

func newAgentStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the current status of the mingyue daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			d := agent.NewDaemon("")
			status := d.Status()

			if IsJSONOutput() {
				return WriteJSON(map[string]string{"status": status})
			}
			fmt.Println(status)
			return nil
		},
	}
}

func newAgentDiscoverCmd() *cobra.Command {
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Discover mingyue agents on the local network",
		Long: `Listen for agent discovery announcements on the LAN and list
every agent that responds within the given timeout.

Agents broadcast their address every 3 s once started.  Run this
command shortly after starting an agent to see it appear.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !IsJSONOutput() {
				fmt.Printf("Scanning for mingyue agents (%.0fs)...\n", timeout.Seconds())
			}

			agents, err := discovery.Browse(timeout)
			if err != nil {
				return fmt.Errorf("discover: %w", err)
			}

			if IsJSONOutput() {
				return WriteJSON(agents)
			}

			if len(agents) == 0 {
				fmt.Println("No agents found.")
				return nil
			}

			fmt.Printf("%-30s  %-20s  %s\n", "HOSTNAME", "ADDRESS", "VERSION")
			fmt.Printf("%-30s  %-20s  %s\n", "------------------------------", "--------------------", "-------")
			for _, a := range agents {
				fmt.Printf("%-30s  %-20s  %s\n", a.Hostname, a.Addr, a.Version)
			}
			return nil
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 3*time.Second, "how long to wait for responses")
	return cmd
}
