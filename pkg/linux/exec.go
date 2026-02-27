// Package linux provides low-level Linux utilities used by the mingyue agent.
// All direct interactions with /proc, system capabilities, and exec are
// isolated here so that higher-level packages remain platform-agnostic and
// easier to test.
package linux

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

// RunResult holds the output of a completed command.
type RunResult struct {
	// Stdout is the standard output captured from the command.
	Stdout []byte
	// Stderr is the standard error captured from the command.
	Stderr []byte
	// ExitCode is the process exit code.
	ExitCode int
}

// Run executes the named program with the given arguments, respecting the
// provided context for timeout / cancellation.  It captures stdout and stderr
// separately and always returns a RunResult (even on non-zero exit).
// A non-nil error is returned only for OS-level failures (e.g. the binary is
// not found); a non-zero ExitCode is not treated as an error.
func Run(ctx context.Context, name string, args ...string) (RunResult, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	result := RunResult{
		Stdout: stdout.Bytes(),
		Stderr: stderr.Bytes(),
	}

	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			return result, nil // non-zero exit is not an OS-level error
		}
		return result, fmt.Errorf("exec %q: %w", name, runErr)
	}

	return result, nil
}
