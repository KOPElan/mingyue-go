package network

import (
	"context"
	"os/exec"
)

// runCmd executes an external command with context and returns its combined output.
func runCmd(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}
