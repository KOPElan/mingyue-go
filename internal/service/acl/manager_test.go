package acl_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"kopelan/mingyue-go/internal/service/acl"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubOKCommander struct {
	output []byte
}

func (c *stubOKCommander) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return c.output, nil
}

type stubErrCommander struct {
	err error
}

func (c *stubErrCommander) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, c.err
}

// ── Get ───────────────────────────────────────────────────────────────────────

func TestGet_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "testfile.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	mgr := acl.NewManagerWithCommander(dir, &stubOKCommander{}, nil)
	info, err := mgr.Get(context.Background(), f)
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if info.Path != f {
		t.Errorf("Path: got %q, want %q", info.Path, f)
	}
	if info.Mode == "" {
		t.Error("Mode should not be empty")
	}
}

func TestGet_NotFound(t *testing.T) {
	dir := t.TempDir()
	mgr := acl.NewManagerWithCommander(dir, &stubOKCommander{}, nil)
	_, err := mgr.Get(context.Background(), filepath.Join(dir, "nonexistent"))
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
}

func TestGet_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	mgr := acl.NewManagerWithCommander(dir, &stubOKCommander{}, nil)
	_, err := mgr.Get(context.Background(), "/etc/passwd")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestGet_WithACLOutput(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "aclfile.txt")
	if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	aclOutput := fmt.Sprintf("user::rwx\ngroup::r-x\nother::r--\n")
	mgr := acl.NewManagerWithCommander(dir, &stubOKCommander{output: []byte(aclOutput)}, nil)
	info, err := mgr.Get(context.Background(), f)
	if err != nil {
		t.Fatalf("Get: unexpected error: %v", err)
	}
	if len(info.ACLEntries) == 0 {
		t.Error("expected ACL entries, got none")
	}
}

// ── SetACL ────────────────────────────────────────────────────────────────────

func TestSetACL_Success(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "setfile.txt")
	if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	mgr := acl.NewManagerWithCommander(dir, &stubOKCommander{}, nil)
	if err := mgr.SetACL(context.Background(), f, []string{"u:alice:rwx"}, "test"); err != nil {
		t.Fatalf("SetACL: unexpected error: %v", err)
	}
}

func TestSetACL_EmptyEntries(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	mgr := acl.NewManagerWithCommander(dir, &stubOKCommander{}, nil)
	err := mgr.SetACL(context.Background(), f, []string{}, "test")
	if err == nil {
		t.Fatal("expected error for empty entries")
	}
}

func TestSetACL_PathTraversal(t *testing.T) {
	dir := t.TempDir()
	mgr := acl.NewManagerWithCommander(dir, &stubOKCommander{}, nil)
	err := mgr.SetACL(context.Background(), "/etc/passwd", []string{"u:alice:rwx"}, "test")
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestSetACL_CommandError(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(f, []byte("data"), 0o644); err != nil {
		t.Fatalf("create test file: %v", err)
	}

	mgr := acl.NewManagerWithCommander(dir, &stubErrCommander{err: errors.New("setfacl: not found")}, nil)
	err := mgr.SetACL(context.Background(), f, []string{"u:alice:rwx"}, "test")
	if err == nil {
		t.Fatal("expected error when setfacl fails")
	}
}
