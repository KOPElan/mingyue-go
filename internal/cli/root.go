// Package cli contains all cobra command definitions for the mingyue CLI.
// root.go registers global flags and assembles the command tree.
package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Global flags shared across all sub-commands.
var (
	// jsonOutput causes command output to be emitted as JSON instead of the
	// default human-readable format.
	jsonOutput bool
	// configPath overrides the default config file location.
	configPath string
)

// rootCmd is the base command; every sub-command is a child of this.
var rootCmd = &cobra.Command{
	Use:   "mingyue",
	Short: "mingyue — Linux system operations agent",
	Long: `mingyue is a CLI tool and daemon for managing Linux hosts.

It supports system monitoring, process management, disk / mount operations,
file management, and network share management.

Use --json to switch all output to machine-readable JSON format.`,
}

// Execute is the entry point called by main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		if jsonOutput {
			_ = json.NewEncoder(os.Stderr).Encode(map[string]string{
				"error": err.Error(),
			})
		} else {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		os.Exit(1)
	}
}

func init() {
	// Global persistent flags available on every sub-command.
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "output results as JSON")
	rootCmd.PersistentFlags().StringVar(&configPath, "config", "", "path to config file (default: /etc/mingyue/mingyue.yaml)")

	// Register sub-commands.
	rootCmd.AddCommand(newVersionCmd())
	rootCmd.AddCommand(newAgentCmd())
	rootCmd.AddCommand(newSystemCmd())
	rootCmd.AddCommand(newProcessCmd())
}

// IsJSONOutput returns true when the global --json flag is set.
// Sub-commands can call this to decide their output format.
func IsJSONOutput() bool {
	return jsonOutput
}

// WriteJSON serialises v as indented JSON to stdout.
func WriteJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// WriteText prints msg to stdout followed by a newline.
func WriteText(msg string) {
	fmt.Println(msg)
}
