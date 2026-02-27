// Package disk provides mount management and SMART health query services.
// MountService and SmartService share the Commander interface so that
// system calls can be replaced by stubs in unit tests.
package disk

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// MountsReader is the interface for reading the mount table.
// It can be replaced by a stub in unit tests.
type MountsReader interface {
	ReadMounts() (io.ReadCloser, error)
}

// Commander is the interface for running system commands with context support.
// It can be replaced by a stub in unit tests.
type Commander interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// MountOptions holds the parameters for a mount operation.
type MountOptions struct {
	// Source is the device or network path, e.g. "/dev/sdb1" or "//server/share".
	Source string
	// MountPoint is the directory to mount on.
	MountPoint string
	// FSType is the filesystem type (e.g. "ext4", "cifs", "nfs").
	// Use "auto" or empty string to let the kernel auto-detect.
	FSType string
	// ReadOnly mounts the filesystem read-only when true.
	ReadOnly bool
	// Options contains extra mount options (comma-separated), excluding credentials.
	Options string
	// Username is the CIFS user — NEVER logged or included in API responses.
	Username string
	// Password is the CIFS password — NEVER logged or included in API responses.
	Password string
	// Domain is the optional CIFS domain — NEVER logged or included in API responses.
	Domain string
}

// MountService handles mount point listing, mounting, and unmounting.
type MountService struct {
	reader    MountsReader
	commander Commander
	logger    audit.Logger
	mu        sync.Mutex // protects concurrent mount/umount
}

// NewMountService creates a production MountService backed by real system calls.
func NewMountService(al audit.Logger) *MountService {
	return &MountService{
		reader:    &procMountsReader{},
		commander: &osCommander{},
		logger:    al,
	}
}

// NewMountServiceWithDeps creates a MountService with injected dependencies (for testing).
func NewMountServiceWithDeps(reader MountsReader, commander Commander, al audit.Logger) *MountService {
	return &MountService{
		reader:    reader,
		commander: commander,
		logger:    al,
	}
}

// List reads and returns all currently active mount entries from /proc/mounts.
func (s *MountService) List(ctx context.Context) ([]domain.Mount, error) {
	rc, err := s.reader.ReadMounts()
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternal, "failed to read mount table", err)
	}
	defer rc.Close()
	return parseMounts(rc)
}

// Mount performs a mount operation using the provided options.
// Returns ErrConflict when the mountpoint is already in use.
// For CIFS mounts, credentials are passed via a temporary file and are never
// written to logs, audit records, or error messages.
func (s *MountService) Mount(ctx context.Context, opts MountOptions, source string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Idempotency check: reject if mountpoint already mounted.
	mounts, err := s.listLocked(ctx)
	if err != nil {
		return err
	}
	for _, m := range mounts {
		if m.MountPoint == opts.MountPoint {
			s.logAudit(source, "disk.mount", opts.MountPoint, "failure", apperrors.ErrConflict)
			return apperrors.New(apperrors.ErrConflict,
				fmt.Sprintf("mountpoint %s is already mounted", opts.MountPoint))
		}
	}

	var mountErr error
	if strings.EqualFold(opts.FSType, "cifs") {
		mountErr = s.mountCIFS(ctx, opts)
	} else {
		mountErr = s.mountGeneric(ctx, opts)
	}
	if mountErr != nil {
		s.logAudit(source, "disk.mount", opts.MountPoint, "failure", apperrors.ErrInternal)
		return mountErr
	}

	s.logAudit(source, "disk.mount", opts.MountPoint, "success", "")
	return nil
}

// Umount unmounts the filesystem at the given mountpoint.
// Returns ErrNotFound when the mountpoint is not currently mounted.
func (s *MountService) Umount(ctx context.Context, mountpoint string, source string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Idempotency check: reject if mountpoint is not currently mounted.
	mounts, err := s.listLocked(ctx)
	if err != nil {
		return err
	}
	found := false
	for _, m := range mounts {
		if m.MountPoint == mountpoint {
			found = true
			break
		}
	}
	if !found {
		s.logAudit(source, "disk.umount", mountpoint, "failure", apperrors.ErrNotFound)
		return apperrors.New(apperrors.ErrNotFound,
			fmt.Sprintf("mountpoint %s is not currently mounted", mountpoint))
	}

	if _, umountErr := s.commander.Run(ctx, "umount", mountpoint); umountErr != nil {
		s.logAudit(source, "disk.umount", mountpoint, "failure", apperrors.ErrInternal)
		return apperrors.Wrap(apperrors.ErrInternal,
			fmt.Sprintf("failed to unmount %s", mountpoint), umountErr)
	}

	s.logAudit(source, "disk.umount", mountpoint, "success", "")
	return nil
}

// listLocked reads mounts without acquiring the mutex (caller must hold it).
func (s *MountService) listLocked(ctx context.Context) ([]domain.Mount, error) {
	rc, err := s.reader.ReadMounts()
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternal, "failed to read mount table", err)
	}
	defer rc.Close()
	return parseMounts(rc)
}

// mountGeneric runs mount for non-CIFS filesystems.
func (s *MountService) mountGeneric(ctx context.Context, opts MountOptions) error {
	args := []string{}
	if opts.FSType != "" && !strings.EqualFold(opts.FSType, "auto") {
		args = append(args, "-t", opts.FSType)
	}

	mountOpts := opts.Options
	if opts.ReadOnly {
		if mountOpts != "" {
			mountOpts += ",ro"
		} else {
			mountOpts = "ro"
		}
	}
	if mountOpts != "" {
		args = append(args, "-o", mountOpts)
	}

	// source and mountpoint are positional and must come after all options.
	args = append(args, opts.Source, opts.MountPoint)

	if _, err := s.commander.Run(ctx, "mount", args...); err != nil {
		return apperrors.Wrap(apperrors.ErrInternal,
			fmt.Sprintf("mount failed for %s at %s", opts.Source, opts.MountPoint), err)
	}
	return nil
}

// mountCIFS runs mount.cifs, passing credentials via a temporary file so that
// username/password are never visible in command-line arguments.
func (s *MountService) mountCIFS(ctx context.Context, opts MountOptions) error {
	// Create a temporary credentials file.
	tmpFile, err := os.CreateTemp("", "mingyue-cifs-*")
	if err != nil {
		return apperrors.Wrap(apperrors.ErrInternal, "failed to create CIFS credentials file", err)
	}
	defer os.Remove(tmpFile.Name())

	// Restrict access before writing sensitive data.
	if err := os.Chmod(tmpFile.Name(), 0600); err != nil {
		tmpFile.Close()
		return apperrors.Wrap(apperrors.ErrInternal, "failed to secure CIFS credentials file", err)
	}

	// Write credentials — username/password/domain are NEVER logged.
	if _, err := fmt.Fprintf(tmpFile, "username=%s\npassword=%s\n", opts.Username, opts.Password); err != nil {
		tmpFile.Close()
		return apperrors.Wrap(apperrors.ErrInternal, "failed to write CIFS credentials", err)
	}
	if opts.Domain != "" {
		if _, err := fmt.Fprintf(tmpFile, "domain=%s\n", opts.Domain); err != nil {
			tmpFile.Close()
			return apperrors.Wrap(apperrors.ErrInternal, "failed to write CIFS domain", err)
		}
	}
	if err := tmpFile.Close(); err != nil {
		return apperrors.Wrap(apperrors.ErrInternal, "failed to flush CIFS credentials file", err)
	}

	// Build mount options: reference credentials by file path (not inline args).
	mountOpts := "credentials=" + tmpFile.Name()
	if opts.ReadOnly {
		mountOpts += ",ro"
	}
	if opts.Options != "" {
		mountOpts += "," + opts.Options
	}

	// source and mountpoint are positional and must come after all options.
	args := []string{"-t", "cifs", "-o", mountOpts, opts.Source, opts.MountPoint}
	if _, err := s.commander.Run(ctx, "mount", args...); err != nil {
		return apperrors.Wrap(apperrors.ErrInternal,
			fmt.Sprintf("cifs mount failed for %s at %s", opts.Source, opts.MountPoint), err)
	}
	return nil
}

// logAudit writes an audit event for a mount/umount operation.
func (s *MountService) logAudit(source, action, target, result string, code apperrors.ErrorCode) {
	if s.logger == nil {
		return
	}
	event := audit.AuditEvent{
		Source: source,
		Action: action,
		Target: target,
		Result: result,
	}
	if code != "" {
		event.ErrorCode = string(code)
	}
	_ = s.logger.Log(event)
}

// parseMounts parses /proc/mounts format lines into []domain.Mount.
// Each line: device mountpoint fstype options dump pass
// Returns ErrInternal if the scanner encounters a read error (e.g. line too long).
func parseMounts(r io.Reader) ([]domain.Mount, error) {
	var mounts []domain.Mount
	scanner := bufio.NewScanner(r)
	// Use a 1 MiB buffer to handle uncommon but valid long options strings.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		mounts = append(mounts, domain.Mount{
			Device:     fields[0],
			MountPoint: fields[1],
			FSType:     fields[2],
			Options:    fields[3],
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternal, "error reading mount table", err)
	}
	return mounts, nil
}

// ─── production implementations ─────────────────────────────────────────────

// procMountsReader reads from the real /proc/mounts file.
type procMountsReader struct{}

func (r *procMountsReader) ReadMounts() (io.ReadCloser, error) {
	return os.Open("/proc/mounts")
}

// osCommander executes real system commands using exec.CommandContext.
type osCommander struct{}

func (c *osCommander) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
