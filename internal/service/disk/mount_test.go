package disk

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"kopelan/mingyue-go/internal/audit"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// ─── stubs ───────────────────────────────────────────────────────────────────

type stubMountsReader struct {
	content string
	err     error
}

func (r *stubMountsReader) ReadMounts() (io.ReadCloser, error) {
	if r.err != nil {
		return nil, r.err
	}
	return io.NopCloser(strings.NewReader(r.content)), nil
}

type stubCommander struct {
	// calls records each invocation: [name, args...]
	calls  [][]string
	output []byte
	err    error
}

func (c *stubCommander) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	entry := append([]string{name}, args...)
	c.calls = append(c.calls, entry)
	return c.output, c.err
}

// ─── mock audit logger ───────────────────────────────────────────────────────

type mockAuditLogger struct {
	events []audit.AuditEvent
}

func (m *mockAuditLogger) Log(e audit.AuditEvent) error {
	m.events = append(m.events, e)
	return nil
}
func (m *mockAuditLogger) Close() error { return nil }

// ─── sample /proc/mounts content ─────────────────────────────────────────────

const sampleMounts = `sysfs /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0
proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0
/dev/sda1 / ext4 rw,relatime 0 0
/dev/sdb1 /mnt/data ext4 rw,relatime 0 0
//server/share /mnt/cifs cifs rw,relatime 0 0
`

// ─── parseMounts ─────────────────────────────────────────────────────────────

func TestParseMounts(t *testing.T) {
	mounts, err := parseMounts(strings.NewReader(sampleMounts))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mounts) != 5 {
		t.Fatalf("expected 5 mounts, got %d", len(mounts))
	}
	// Check first data mount.
	found := false
	for _, m := range mounts {
		if m.MountPoint == "/mnt/data" {
			found = true
			if m.Device != "/dev/sdb1" {
				t.Errorf("Device: got %q, want %q", m.Device, "/dev/sdb1")
			}
			if m.FSType != "ext4" {
				t.Errorf("FSType: got %q, want %q", m.FSType, "ext4")
			}
		}
	}
	if !found {
		t.Error("expected to find /mnt/data mount")
	}
}

func TestParseMounts_Empty(t *testing.T) {
	mounts, err := parseMounts(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mounts) != 0 {
		t.Errorf("expected 0 mounts, got %d", len(mounts))
	}
}

func TestParseMounts_SkipsComments(t *testing.T) {
	content := "# this is a comment\n/dev/sda1 / ext4 rw 0 0\n"
	mounts, err := parseMounts(strings.NewReader(content))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mounts) != 1 {
		t.Errorf("expected 1 mount, got %d", len(mounts))
	}
}

// ─── MountService.List ───────────────────────────────────────────────────────

func TestMountService_List_Success(t *testing.T) {
	reader := &stubMountsReader{content: sampleMounts}
	svc := NewMountServiceWithDeps(reader, nil, nil)

	mounts, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mounts) != 5 {
		t.Errorf("expected 5 mounts, got %d", len(mounts))
	}
}

func TestMountService_List_ReaderError(t *testing.T) {
	reader := &stubMountsReader{err: io.ErrUnexpectedEOF}
	svc := NewMountServiceWithDeps(reader, nil, nil)

	_, err := svc.List(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertErrorCode(t, err, apperrors.ErrInternal)
}

// ─── MountService.Mount ──────────────────────────────────────────────────────

func TestMountService_Mount_Generic_Success(t *testing.T) {
	// Start with no active mounts so the idempotency check passes.
	reader := &stubMountsReader{content: ""}
	cmd := &stubCommander{}
	al := &mockAuditLogger{}
	svc := NewMountServiceWithDeps(reader, cmd, al)

	opts := MountOptions{
		Source:     "/dev/sdb1",
		MountPoint: "/mnt/test",
		FSType:     "ext4",
	}
	if err := svc.Mount(context.Background(), opts, "cli"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify mount command was called.
	if len(cmd.calls) == 0 {
		t.Fatal("expected mount command to be called")
	}
	call := cmd.calls[0]
	if call[0] != "mount" {
		t.Errorf("expected 'mount', got %q", call[0])
	}

	// Verify audit event.
	if len(al.events) == 0 {
		t.Fatal("expected audit event")
	}
	if al.events[0].Result != "success" {
		t.Errorf("audit result: got %q, want %q", al.events[0].Result, "success")
	}
	if al.events[0].Action != "disk.mount" {
		t.Errorf("audit action: got %q, want %q", al.events[0].Action, "disk.mount")
	}
}

func TestMountService_Mount_AlreadyMounted_ReturnsConflict(t *testing.T) {
	// /mnt/data is already in the mount table.
	reader := &stubMountsReader{content: sampleMounts}
	cmd := &stubCommander{}
	al := &mockAuditLogger{}
	svc := NewMountServiceWithDeps(reader, cmd, al)

	opts := MountOptions{
		Source:     "/dev/sdb1",
		MountPoint: "/mnt/data",
		FSType:     "ext4",
	}
	err := svc.Mount(context.Background(), opts, "cli")
	if err == nil {
		t.Fatal("expected ErrConflict, got nil")
	}
	assertErrorCode(t, err, apperrors.ErrConflict)

	// Mount command must NOT have been called.
	if len(cmd.calls) != 0 {
		t.Errorf("expected no mount command calls, got %d", len(cmd.calls))
	}

	// Audit event must record failure.
	if len(al.events) == 0 || al.events[0].Result != "failure" {
		t.Error("expected failure audit event")
	}
}

func TestMountService_Mount_ReadOnly(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	cmd := &stubCommander{}
	svc := NewMountServiceWithDeps(reader, cmd, nil)

	opts := MountOptions{
		Source:     "/dev/sdb1",
		MountPoint: "/mnt/ro",
		FSType:     "ext4",
		ReadOnly:   true,
	}
	_ = svc.Mount(context.Background(), opts, "cli")

	if len(cmd.calls) == 0 {
		t.Fatal("expected mount command call")
	}
	args := cmd.calls[0]
	argsJoined := strings.Join(args, " ")
	if !strings.Contains(argsJoined, "ro") {
		t.Errorf("expected 'ro' in mount args, got: %s", argsJoined)
	}
	// -o must appear before source and mountpoint (standard mount syntax).
	oIdx := sliceIndex(args, "-o")
	srcIdx := sliceIndex(args, "/dev/sdb1")
	if oIdx < 0 {
		t.Error("expected '-o' in mount args")
	} else if srcIdx >= 0 && oIdx > srcIdx {
		t.Errorf("'-o' (pos %d) must appear before source (pos %d): %v", oIdx, srcIdx, args)
	}
}

func TestMountService_Mount_CIFS_CredentialsNotInArgs(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	cmd := &stubCommander{}
	svc := NewMountServiceWithDeps(reader, cmd, nil)

	opts := MountOptions{
		Source:     "//server/share",
		MountPoint: "/mnt/cifs",
		FSType:     "cifs",
		Username:   "secretuser",
		Password:   "secretpassword",
		Domain:     "TESTDOMAIN",
	}
	_ = svc.Mount(context.Background(), opts, "cli")

	if len(cmd.calls) == 0 {
		t.Fatal("expected mount command call")
	}
	args := cmd.calls[0]
	// None of the command arguments must contain the password.
	for _, arg := range args {
		if strings.Contains(arg, "secretpassword") {
			t.Errorf("CIFS password must not appear in command args: %v", args)
		}
		if strings.Contains(arg, "secretuser") {
			t.Errorf("CIFS username must not appear in command args: %v", args)
		}
	}
	argsJoined := strings.Join(args, " ")
	// The -o option must reference a credentials file.
	if !strings.Contains(argsJoined, "credentials=") {
		t.Errorf("expected 'credentials=' in mount args, got: %s", argsJoined)
	}
	// -o must appear before source and mountpoint.
	oIdx := sliceIndex(args, "-o")
	srcIdx := sliceIndex(args, "//server/share")
	if oIdx < 0 {
		t.Error("expected '-o' in CIFS mount args")
	} else if srcIdx >= 0 && oIdx > srcIdx {
		t.Errorf("'-o' (pos %d) must appear before source (pos %d): %v", oIdx, srcIdx, args)
	}
}

func TestMountService_Mount_CIFS_NormalizesSourceWithoutUNCPrefix(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	cmd := &stubCommander{}
	svc := NewMountServiceWithDeps(reader, cmd, nil)

	opts := MountOptions{
		Source:     "192.168.1.2/ssd",
		MountPoint: "/mnt/ssd",
		FSType:     "cifs",
		Username:   "user1",
		Password:   "secretpassword",
	}
	if err := svc.Mount(context.Background(), opts, "cli"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cmd.calls) == 0 {
		t.Fatal("expected mount command call")
	}
	args := cmd.calls[0]
	srcIdx := sliceIndex(args, "//192.168.1.2/ssd")
	if srcIdx < 0 {
		t.Fatalf("expected normalized CIFS source in args, got %v", args)
	}
	mountPointIdx := sliceIndex(args, "/mnt/ssd")
	if mountPointIdx < 0 {
		t.Fatalf("expected mountpoint in args, got %v", args)
	}
	if srcIdx > mountPointIdx {
		t.Fatalf("expected normalized CIFS source to appear before mountpoint, got %v", args)
	}
}

func TestMountService_Mount_CIFS_InvalidSourceReturnsErrInvalidInput(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	cmd := &stubCommander{}
	svc := NewMountServiceWithDeps(reader, cmd, nil)

	err := svc.Mount(context.Background(), MountOptions{
		Source:     "192.168.1.2",
		MountPoint: "/mnt/ssd",
		FSType:     "cifs",
		Username:   "user1",
		Password:   "secretpassword",
	}, "cli")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertErrorCode(t, err, apperrors.ErrInvalidInput)
	if len(cmd.calls) != 0 {
		t.Fatalf("expected no mount command call, got %d", len(cmd.calls))
	}
}

func TestMountService_Mount_CIFS_NormalizesBackslashUNCSource(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	cmd := &stubCommander{}
	svc := NewMountServiceWithDeps(reader, cmd, nil)

	err := svc.Mount(context.Background(), MountOptions{
		Source:     `\\192.168.1.2\ssd`,
		MountPoint: "/mnt/ssd",
		FSType:     "cifs",
		Username:   "user1",
		Password:   "secretpassword",
	}, "cli")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cmd.calls) == 0 {
		t.Fatal("expected mount command call")
	}
	if sliceIndex(cmd.calls[0], "//192.168.1.2/ssd") < 0 {
		t.Fatalf("expected normalized CIFS source in args, got %v", cmd.calls[0])
	}
}

// sliceIndex returns the index of target in s, or -1 if not found.
func sliceIndex(s []string, target string) int {
	for i, v := range s {
		if v == target {
			return i
		}
	}
	return -1
}

func TestMountService_Mount_CommandFails_AuditsFailure(t *testing.T) {
	cmdErr := &stubCommanderErr{}
	reader := &stubMountsReader{content: ""}
	al := &mockAuditLogger{}
	svc := NewMountServiceWithDeps(reader, cmdErr, al)

	opts := MountOptions{Source: "/dev/sdb1", MountPoint: "/mnt/test", FSType: "ext4"}
	err := svc.Mount(context.Background(), opts, "cli")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertErrorCode(t, err, apperrors.ErrInternal)
	if len(al.events) == 0 || al.events[0].Result != "failure" {
		t.Error("expected failure audit event")
	}
}

func TestMountService_Mount_CommandFailure_IncludesMountOutput(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	cmd := &stubCommander{
		output: []byte("mount: /mnt/test: mount point does not exist\n"),
		err:    errors.New("exit status 1"),
	}
	svc := NewMountServiceWithDeps(reader, cmd, nil)

	err := svc.Mount(context.Background(), MountOptions{
		Source:     "/dev/sdb1",
		MountPoint: "/mnt/test",
		FSType:     "ext4",
	}, "cli")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	assertErrorCode(t, err, apperrors.ErrInternal)
	if !strings.Contains(err.Error(), "mount point does not exist") {
		t.Fatalf("expected mount output in error, got %v", err)
	}
}

// stubCommanderErr always returns an error.
type stubCommanderErr struct{}

func (c *stubCommanderErr) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, apperrors.New(apperrors.ErrInternal, "simulated command failure")
}

// ─── MountService.Umount ─────────────────────────────────────────────────────

func TestMountService_Umount_Success(t *testing.T) {
	reader := &stubMountsReader{content: sampleMounts}
	cmd := &stubCommander{}
	al := &mockAuditLogger{}
	svc := NewMountServiceWithDeps(reader, cmd, al)

	if err := svc.Umount(context.Background(), "/mnt/data", "cli"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify umount command called.
	if len(cmd.calls) == 0 {
		t.Fatal("expected umount command call")
	}
	if cmd.calls[0][0] != "umount" {
		t.Errorf("expected 'umount', got %q", cmd.calls[0][0])
	}
	// Audit event.
	if len(al.events) == 0 || al.events[0].Result != "success" {
		t.Error("expected success audit event")
	}
}

func TestMountService_Umount_NotMounted_ReturnsNotFound(t *testing.T) {
	reader := &stubMountsReader{content: sampleMounts}
	cmd := &stubCommander{}
	al := &mockAuditLogger{}
	svc := NewMountServiceWithDeps(reader, cmd, al)

	err := svc.Umount(context.Background(), "/mnt/nonexistent", "cli")
	if err == nil {
		t.Fatal("expected ErrNotFound, got nil")
	}
	assertErrorCode(t, err, apperrors.ErrNotFound)

	// umount command must NOT be called.
	if len(cmd.calls) != 0 {
		t.Errorf("expected no umount calls, got %d", len(cmd.calls))
	}
	// Audit failure.
	if len(al.events) == 0 || al.events[0].Result != "failure" {
		t.Error("expected failure audit event")
	}
}

func TestMountService_Umount_ContextTimeout(t *testing.T) {
	reader := &stubMountsReader{content: sampleMounts}
	// Stub that simulates a context-cancelled error.
	cmd := &stubContextCancelledCommander{}
	svc := NewMountServiceWithDeps(reader, cmd, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := svc.Umount(ctx, "/mnt/data", "cli")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	assertErrorCode(t, err, apperrors.ErrInternal)
}

// stubContextCancelledCommander returns context.Canceled.
type stubContextCancelledCommander struct{}

func (c *stubContextCancelledCommander) Run(ctx context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, ctx.Err()
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func assertErrorCode(t *testing.T, err error, wantCode apperrors.ErrorCode) {
	t.Helper()
	var ae *apperrors.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AppError, got %T: %v", err, err)
	}
	if ae.Code != wantCode {
		t.Errorf("error code: got %q, want %q (message: %s)", ae.Code, wantCode, ae.Message)
	}
}
