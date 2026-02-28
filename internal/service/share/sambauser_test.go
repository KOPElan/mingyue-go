package share

import (
	"context"
	"errors"
	"testing"
)

// ── stub SambaUserCommander ───────────────────────────────────────────────────

type stubSambaCommander struct {
	calls      []stubCall
	err        error
	listOutput []byte
}

func (c *stubSambaCommander) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	c.calls = append(c.calls, stubCall{name: name, args: args})
	return c.listOutput, c.err
}

func (c *stubSambaCommander) RunWithInput(_ context.Context, _ string, name string, args ...string) ([]byte, error) {
	c.calls = append(c.calls, stubCall{name: name, args: args})
	return nil, c.err
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newTestSambaUserManager(cmd SambaUserCommander) *SambaUserManager {
	return NewSambaUserManagerWithCommander(cmd)
}

// ── ListUsers ─────────────────────────────────────────────────────────────────

func TestSambaUserManager_ListUsers_Empty(t *testing.T) {
	stub := &stubSambaCommander{listOutput: []byte("")}
	mgr := newTestSambaUserManager(stub)

	users, err := mgr.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
	if len(stub.calls) != 1 || stub.calls[0].name != "pdbedit" {
		t.Errorf("expected pdbedit call, got %+v", stub.calls)
	}
}

func TestSambaUserManager_ListUsers_ParseOutput(t *testing.T) {
	output := "alice:1000:Alice Smith\nbob:1001:Bob Jones\n"
	stub := &stubSambaCommander{listOutput: []byte(output)}
	mgr := newTestSambaUserManager(stub)

	users, err := mgr.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0].Username != "alice" {
		t.Errorf("expected alice, got %q", users[0].Username)
	}
	if users[1].Username != "bob" {
		t.Errorf("expected bob, got %q", users[1].Username)
	}
}

func TestSambaUserManager_ListUsers_CommandError(t *testing.T) {
	stub := &stubSambaCommander{err: errors.New("pdbedit not found")}
	mgr := newTestSambaUserManager(stub)

	_, err := mgr.ListUsers(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── AddUser ───────────────────────────────────────────────────────────────────

func TestSambaUserManager_AddUser_OK(t *testing.T) {
	stub := &stubSambaCommander{}
	mgr := newTestSambaUserManager(stub)

	if err := mgr.AddUser(context.Background(), "alice", "s3cr3t"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(stub.calls))
	}
	call := stub.calls[0]
	if call.name != "smbpasswd" {
		t.Errorf("expected smbpasswd, got %q", call.name)
	}
	// must include -a (add) and -s (stdin) flags
	wantArgs := []string{"-a", "-s", "alice"}
	for i, a := range wantArgs {
		if i >= len(call.args) || call.args[i] != a {
			t.Errorf("arg[%d]: want %q, got %q", i, a, call.args[i])
		}
	}
}

func TestSambaUserManager_AddUser_EmptyUsername(t *testing.T) {
	stub := &stubSambaCommander{}
	mgr := newTestSambaUserManager(stub)

	err := mgr.AddUser(context.Background(), "", "pass")
	if err == nil {
		t.Fatal("expected error for empty username")
	}
	if len(stub.calls) != 0 {
		t.Errorf("should not call commander for empty username")
	}
}

func TestSambaUserManager_AddUser_CommandError(t *testing.T) {
	stub := &stubSambaCommander{err: errors.New("smbpasswd failed")}
	mgr := newTestSambaUserManager(stub)

	err := mgr.AddUser(context.Background(), "alice", "pass")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── RemoveUser ────────────────────────────────────────────────────────────────

func TestSambaUserManager_RemoveUser_OK(t *testing.T) {
	stub := &stubSambaCommander{}
	mgr := newTestSambaUserManager(stub)

	if err := mgr.RemoveUser(context.Background(), "alice"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(stub.calls))
	}
	call := stub.calls[0]
	if call.name != "smbpasswd" {
		t.Errorf("expected smbpasswd, got %q", call.name)
	}
	if len(call.args) < 2 || call.args[0] != "-x" || call.args[1] != "alice" {
		t.Errorf("unexpected args: %v", call.args)
	}
}

func TestSambaUserManager_RemoveUser_EmptyUsername(t *testing.T) {
	stub := &stubSambaCommander{}
	mgr := newTestSambaUserManager(stub)

	if err := mgr.RemoveUser(context.Background(), "  "); err == nil {
		t.Fatal("expected error for blank username")
	}
}

func TestSambaUserManager_RemoveUser_CommandError(t *testing.T) {
	stub := &stubSambaCommander{err: errors.New("user not found")}
	mgr := newTestSambaUserManager(stub)

	if err := mgr.RemoveUser(context.Background(), "nobody"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── SetPassword ───────────────────────────────────────────────────────────────

func TestSambaUserManager_SetPassword_OK(t *testing.T) {
	stub := &stubSambaCommander{}
	mgr := newTestSambaUserManager(stub)

	if err := mgr.SetPassword(context.Background(), "alice", "newpass"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stub.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(stub.calls))
	}
	call := stub.calls[0]
	if call.name != "smbpasswd" {
		t.Errorf("expected smbpasswd, got %q", call.name)
	}
	// must include -s (stdin) but NOT -a (not adding a new user)
	wantArgs := []string{"-s", "alice"}
	for i, a := range wantArgs {
		if i >= len(call.args) || call.args[i] != a {
			t.Errorf("arg[%d]: want %q, got %q", i, a, call.args[i])
		}
	}
}

func TestSambaUserManager_SetPassword_EmptyUsername(t *testing.T) {
	stub := &stubSambaCommander{}
	mgr := newTestSambaUserManager(stub)

	if err := mgr.SetPassword(context.Background(), "", "pass"); err == nil {
		t.Fatal("expected error for empty username")
	}
}

func TestSambaUserManager_SetPassword_CommandError(t *testing.T) {
	stub := &stubSambaCommander{err: errors.New("smbpasswd failed")}
	mgr := newTestSambaUserManager(stub)

	if err := mgr.SetPassword(context.Background(), "alice", "pass"); err == nil {
		t.Fatal("expected error, got nil")
	}
}
