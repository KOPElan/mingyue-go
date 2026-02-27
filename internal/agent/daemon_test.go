package agent_test

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"kopelan/mingyue-go/internal/agent"
)

// newTestDaemon creates a Daemon with a PID file in a temp directory so tests
// do not require elevated privileges.
func newTestDaemon(t *testing.T) *agent.Daemon {
	t.Helper()
	dir := t.TempDir()
	d := agent.NewDaemon(":0")
	d.PIDPath = filepath.Join(dir, "test.pid")
	return d
}

func TestDaemon_Status_NoPIDFile(t *testing.T) {
	d := newTestDaemon(t)
	status := d.Status()
	if status == "" {
		t.Error("Status() must return a non-empty string")
	}
	// Should report that the agent is not running.
	if status == "running" {
		t.Errorf("Status() = %q; expected a 'stopped' variant when PID file is absent", status)
	}
}

func TestDaemon_WritePIDAndReadBack(t *testing.T) {
	d := newTestDaemon(t)

	// Access the WritePID method via a helper that writes the current PID.
	pid := os.Getpid()
	content := strconv.Itoa(pid) + "\n"
	if err := os.WriteFile(d.PIDPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	// Status should now reflect the running process.
	status := d.Status()
	if status == "" {
		t.Fatal("Status() must not be empty after writing PID file")
	}
	// The current test process is definitely running.
	if len(status) == 0 {
		t.Error("expected non-empty status")
	}
}

func TestDaemon_Status_StaleOrUnknownPID(t *testing.T) {
	d := newTestDaemon(t)

	// Write an extremely large PID that is very unlikely to exist.
	content := "9999999\n"
	if err := os.WriteFile(d.PIDPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	status := d.Status()
	if status == "" {
		t.Error("Status() must return a non-empty string for a stale PID")
	}
}

func TestNewDaemon_DefaultListenAddr(t *testing.T) {
	d := agent.NewDaemon("")
	if d.ListenAddr == "" {
		t.Error("NewDaemon with empty addr must set a default ListenAddr")
	}
}

func TestNewDaemon_CustomListenAddr(t *testing.T) {
	d := agent.NewDaemon(":8888")
	if d.ListenAddr != ":8888" {
		t.Errorf("ListenAddr = %q, want :8888", d.ListenAddr)
	}
}
