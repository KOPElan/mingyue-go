// Package agent implements the mingyue daemon lifecycle: start, stop, status.
//
// PID file location: /var/run/mingyue/mingyue.pid
// If that directory is not writable (e.g. in a development environment) the
// pid file is placed in os.TempDir()/mingyue.pid instead.
package agent

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	pidDir  = "/var/run/mingyue"
	pidFile = "mingyue.pid"

	defaultListenAddr = ":7070"
)

// Daemon holds the runtime state of the agent.
type Daemon struct {
	// ListenAddr is the TCP address the HTTP server binds to.
	ListenAddr string
	// PIDPath is the full path to the PID file.
	PIDPath string

	server *http.Server
}

// NewDaemon creates a Daemon with sensible defaults.
func NewDaemon(listenAddr string) *Daemon {
	if listenAddr == "" {
		listenAddr = defaultListenAddr
	}
	return &Daemon{
		ListenAddr: listenAddr,
		PIDPath:    resolvePIDPath(),
	}
}

// Start launches the HTTP API server and blocks until a SIGTERM or SIGINT is
// received, then performs a graceful shutdown.
func (d *Daemon) Start(handler http.Handler) error {
	if err := d.writePID(); err != nil {
		return fmt.Errorf("agent start: write pid file: %w", err)
	}
	defer d.removePID()

	d.server = &http.Server{
		Addr:              d.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := d.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// Wait for either a signal or a server error.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(quit)

	select {
	case sig := <-quit:
		fmt.Fprintf(os.Stderr, "agent: received signal %s, shutting down\n", sig)
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("agent: server error: %w", err)
		}
		return nil
	}

	// Graceful shutdown with a 30-second deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := d.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("agent: graceful shutdown: %w", err)
	}
	return nil
}

// Stop sends SIGTERM to the process whose PID is recorded in the PID file.
func (d *Daemon) Stop() error {
	pid, err := d.readPID()
	if err != nil {
		return fmt.Errorf("agent stop: %w", err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("agent stop: find process %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("agent stop: send SIGTERM to %d: %w", pid, err)
	}
	return nil
}

// Status returns a human-readable string describing whether the agent is
// running.  It inspects the PID file and checks whether the process is alive.
func (d *Daemon) Status() string {
	pid, err := d.readPID()
	if err != nil {
		return "stopped (no pid file)"
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Sprintf("stopped (pid %d not found)", pid)
	}

	// Signal 0 checks whether the process is alive without sending a signal.
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return fmt.Sprintf("stopped (pid %d not running: %v)", pid, err)
	}
	return fmt.Sprintf("running (pid %d)", pid)
}

// writePID writes the current process PID to the PID file.
func (d *Daemon) writePID() error {
	dir := filepath.Dir(d.PIDPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create pid dir %s: %w", dir, err)
	}
	content := strconv.Itoa(os.Getpid()) + "\n"
	return os.WriteFile(d.PIDPath, []byte(content), 0o644)
}

// readPID reads and returns the PID recorded in the PID file.
func (d *Daemon) readPID() (int, error) {
	data, err := os.ReadFile(d.PIDPath)
	if err != nil {
		return 0, fmt.Errorf("read pid file %s: %w", d.PIDPath, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, fmt.Errorf("parse pid file %s: %w", d.PIDPath, err)
	}
	return pid, nil
}

// removePID deletes the PID file (best-effort; errors are ignored).
func (d *Daemon) removePID() {
	_ = os.Remove(d.PIDPath)
}

// resolvePIDPath returns the preferred PID file path, falling back to the
// temporary directory when /var/run/mingyue is not writable.
func resolvePIDPath() string {
	preferred := filepath.Join(pidDir, pidFile)
	if err := os.MkdirAll(pidDir, 0o755); err == nil {
		return preferred
	}
	return filepath.Join(os.TempDir(), pidFile)
}
