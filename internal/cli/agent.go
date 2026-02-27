package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"kopelan/mingyue-go/internal/agent"
)

func newAgentCmd() *cobra.Command {
	agentCmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage the mingyue daemon",
		Long:  "Start, stop, or query the status of the mingyue background daemon.",
	}

	agentCmd.AddCommand(newAgentStartCmd())
	agentCmd.AddCommand(newAgentStopCmd())
	agentCmd.AddCommand(newAgentStatusCmd())

	return agentCmd
}

func newAgentStartCmd() *cobra.Command {
	var listenAddr string

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the mingyue daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			d := agent.NewDaemon(listenAddr)

			if IsJSONOutput() {
				_ = WriteJSON(map[string]string{
					"status":  "starting",
					"address": d.ListenAddr,
					"pidFile": d.PIDPath,
				})
			} else {
				fmt.Printf("Starting mingyue daemon on %s (pid file: %s)\n",
					d.ListenAddr, d.PIDPath)
			}

			// NewRouter is imported lazily to avoid a circular dependency;
			// in practice the main package wires this up.  Here we use the
			// packaged router directly.
			return d.Start(nil)
		},
	}

	cmd.Flags().StringVar(&listenAddr, "listen", "", "HTTP listen address (default :7070)")
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
