package cli

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// BuildVersion, BuildCommit, and BuildDate are injected at build time via
// -ldflags.  They fall back to development-friendly defaults.
var (
	BuildVersion = "dev"
	BuildCommit  = "none"
	BuildDate    = "unknown"
)

// versionInfo is the JSON-serialisable version payload.
type versionInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Long:  "Print build version, commit hash, build date, and Go runtime information.",
		RunE: func(cmd *cobra.Command, args []string) error {
			info := versionInfo{
				Version:   BuildVersion,
				Commit:    BuildCommit,
				BuildDate: BuildDate,
				GoVersion: runtime.Version(),
				OS:        runtime.GOOS,
				Arch:      runtime.GOARCH,
			}

			if IsJSONOutput() {
				return WriteJSON(info)
			}

			fmt.Printf("mingyue %s\n", info.Version)
			fmt.Printf("  Commit:     %s\n", info.Commit)
			fmt.Printf("  Built:      %s\n", info.BuildDate)
			fmt.Printf("  Go version: %s\n", info.GoVersion)
			fmt.Printf("  OS/Arch:    %s/%s\n", info.OS, info.Arch)
			return nil
		},
	}
}
