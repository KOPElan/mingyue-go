// Package disk provides mount management and SMART health query services.
// MountService and SmartService share the Commander interface so that
// system calls can be replaced by stubs in unit tests.
package disk

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

const (
	defaultFstabPath              = "/etc/fstab"
	defaultCIFSCredentialsDirPath = "/etc/mingyue/cifs-credentials"
	cifsCredentialFileExtension   = ".cred"
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
	// Persistent writes the mount configuration to /etc/fstab after a successful mount.
	Persistent bool
}

type mountPersister interface {
	Persist(opts MountOptions) error
}

// MountService handles mount point listing, mounting, and unmounting.
type MountService struct {
	reader    MountsReader
	commander Commander
	persister mountPersister
	logger    audit.Logger
	mu        sync.Mutex // protects concurrent mount/umount
}

// NewMountService creates a production MountService backed by real system calls.
func NewMountService(al audit.Logger) *MountService {
	return newMountService(
		&procMountsReader{},
		&osCommander{},
		&fstabPersister{
			fstabPath:    defaultFstabPath,
			cifsCredsDir: defaultCIFSCredentialsDirPath,
		},
		al,
	)
}

// NewMountServiceWithDeps creates a MountService with injected dependencies (for testing).
func NewMountServiceWithDeps(reader MountsReader, commander Commander, al audit.Logger) *MountService {
	return newMountService(reader, commander, noopMountPersister{}, al)
}

func newMountService(reader MountsReader, commander Commander, persister mountPersister, al audit.Logger) *MountService {
	return &MountService{
		reader:    reader,
		commander: commander,
		persister: persister,
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

	if err := validatePersistentMountOptions(opts); err != nil {
		s.logAudit(source, "disk.mount", opts.MountPoint, "failure", apperrors.ErrInvalidInput)
		return err
	}

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

	if opts.Persistent {
		if err := s.persister.Persist(opts); err != nil {
			s.logAudit(source, "disk.mount.persist", opts.MountPoint, "failure", apperrors.ErrInternal)
			return apperrors.Wrap(
				apperrors.ErrInternal,
				fmt.Sprintf("mounted %s at %s but failed to persist in /etc/fstab", opts.Source, opts.MountPoint),
				err,
			)
		}
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

	if output, err := s.commander.Run(ctx, "mount", args...); err != nil {
		return wrapMountCommandError(
			fmt.Sprintf("mount failed for %s at %s", opts.Source, opts.MountPoint),
			output,
			err,
		)
	}
	return nil
}

// mountCIFS runs mount.cifs, passing credentials via a temporary file so that
// username/password are never visible in command-line arguments.
func (s *MountService) mountCIFS(ctx context.Context, opts MountOptions) error {
	source, err := normalizeCIFSSource(opts.Source)
	if err != nil {
		return err
	}

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
	args := []string{"-t", "cifs", "-o", mountOpts, source, opts.MountPoint}
	if output, err := s.commander.Run(ctx, "mount", args...); err != nil {
		return wrapMountCommandError(
			fmt.Sprintf("cifs mount failed for %s at %s", source, opts.MountPoint),
			output,
			err,
		)
	}
	return nil
}

func normalizeCIFSSource(source string) (string, error) {
	trimmed := strings.TrimSpace(source)
	if trimmed == "" {
		return "", apperrors.New(apperrors.ErrInvalidInput, "cifs source is required")
	}

	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	segments := strings.FieldsFunc(normalized, func(r rune) bool {
		return r == '/'
	})
	if len(segments) < 2 {
		return "", apperrors.New(apperrors.ErrInvalidInput, "cifs source must be in the form //server/share")
	}

	return "//" + segments[0] + "/" + strings.Join(segments[1:], "/"), nil
}

func validatePersistentMountOptions(opts MountOptions) error {
	if !opts.Persistent {
		return nil
	}

	if containsWhitespace(opts.Source) || containsWhitespace(opts.MountPoint) {
		return apperrors.New(
			apperrors.ErrInvalidInput,
			"persistent mounts do not support whitespace in source or mountpoint",
		)
	}

	if !strings.EqualFold(opts.FSType, "cifs") {
		return nil
	}

	optionParts := splitMountOptions(opts.Options)
	forbiddenCIFSPrefixes := []string{"username=", "password=", "domain="}
	for _, part := range optionParts {
		lower := strings.ToLower(part)
		for _, prefix := range forbiddenCIFSPrefixes {
			if strings.HasPrefix(lower, prefix) {
				return apperrors.New(
					apperrors.ErrInvalidInput,
					"persistent CIFS mounts do not allow username/password/domain in options; use dedicated flags instead",
				)
			}
		}
	}

	if hasOptionWithPrefix(optionParts, "credentials=") && (opts.Username != "" || opts.Password != "" || opts.Domain != "") {
		return apperrors.New(
			apperrors.ErrInvalidInput,
			"persistent CIFS mounts cannot combine credentials= in options with username/password/domain flags",
		)
	}

	return nil
}

func containsWhitespace(value string) bool {
	return strings.IndexFunc(value, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == '\r'
	}) >= 0
}

func wrapMountCommandError(message string, output []byte, err error) error {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return apperrors.Wrap(apperrors.ErrInternal, message, err)
	}

	detail = strings.ReplaceAll(detail, "\r\n", "\n")
	detail = strings.ReplaceAll(detail, "\r", "\n")
	lines := strings.Split(detail, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		filtered = append(filtered, line)
	}
	detail = strings.Join(filtered, ": ")
	return apperrors.Wrap(apperrors.ErrInternal, message+": "+detail, err)
}

type noopMountPersister struct{}

func (noopMountPersister) Persist(MountOptions) error {
	return nil
}

type fstabPersister struct {
	fstabPath    string
	cifsCredsDir string
}

func (p *fstabPersister) Persist(opts MountOptions) error {
	entry, err := p.buildEntry(opts)
	if err != nil {
		return err
	}

	lines, err := p.readLines()
	if err != nil {
		return err
	}

	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		if fields[1] == opts.MountPoint {
			lines[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines, entry)
	}

	content := strings.Join(lines, "\n")
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return atomicWriteFile(p.fstabPath, []byte(content), 0o755, 0o644)
}

func (p *fstabPersister) buildEntry(opts MountOptions) (string, error) {
	source := opts.Source
	if strings.EqualFold(opts.FSType, "cifs") {
		normalized, err := normalizeCIFSSource(opts.Source)
		if err != nil {
			return "", err
		}
		source = normalized
	}

	fsType := opts.FSType
	if fsType == "" {
		fsType = "auto"
	}

	mountOpts, err := p.buildOptions(opts)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s\t%s\t%s\t%s\t0\t0", source, opts.MountPoint, fsType, mountOpts), nil
}

func (p *fstabPersister) buildOptions(opts MountOptions) (string, error) {
	parts := splitMountOptions(opts.Options)
	if opts.ReadOnly && !hasExactOption(parts, "ro") {
		parts = append(parts, "ro")
	}

	if strings.EqualFold(opts.FSType, "cifs") && (opts.Username != "" || opts.Password != "" || opts.Domain != "") {
		credPath, err := p.writeCIFSCredentials(opts)
		if err != nil {
			return "", err
		}
		parts = append([]string{"credentials=" + credPath}, parts...)
	}

	if len(parts) == 0 {
		return "defaults", nil
	}
	return strings.Join(parts, ","), nil
}

func (p *fstabPersister) writeCIFSCredentials(opts MountOptions) (string, error) {
	if p.cifsCredsDir == "" {
		return "", apperrors.New(apperrors.ErrInternal, "persistent CIFS credentials directory is not configured")
	}

	var content strings.Builder
	content.WriteString("username=")
	content.WriteString(opts.Username)
	content.WriteString("\npassword=")
	content.WriteString(opts.Password)
	content.WriteString("\n")
	if opts.Domain != "" {
		content.WriteString("domain=")
		content.WriteString(opts.Domain)
		content.WriteString("\n")
	}

	sum := sha256.Sum256([]byte(opts.MountPoint))
	credPath := filepath.Join(p.cifsCredsDir, fmt.Sprintf("%x%s", sum[:], cifsCredentialFileExtension))
	// Keep the credentials directory root-only so persisted CIFS secrets are not world-readable.
	if err := atomicWriteFile(credPath, []byte(content.String()), 0o700, 0o600); err != nil {
		return "", apperrors.Wrap(apperrors.ErrInternal, "failed to persist CIFS credentials", err)
	}
	return credPath, nil
}

func (p *fstabPersister) readLines() ([]string, error) {
	data, err := os.ReadFile(p.fstabPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Wrap(apperrors.ErrInternal, "failed to read /etc/fstab", err)
	}

	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

func atomicWriteFile(path string, data []byte, dirPerm, filePerm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}

	tmpFile, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpPath := tmpFile.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmpFile.Chmod(filePerm); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("chmod temp file %s: %w", tmpPath, err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("write temp file %s: %w", tmpPath, err)
	}
	if err := tmpFile.Sync(); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("fsync temp file %s: %w", tmpPath, err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close temp file %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename temp file %s to %s: %w", tmpPath, path, err)
	}
	cleanup = false

	if dirFile, err := os.Open(dir); err == nil {
		_ = dirFile.Sync()
		_ = dirFile.Close()
	}
	return nil
}

func splitMountOptions(options string) []string {
	if strings.TrimSpace(options) == "" {
		return nil
	}

	raw := strings.Split(options, ",")
	parts := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		parts = append(parts, part)
	}
	return parts
}

func hasExactOption(parts []string, want string) bool {
	for _, part := range parts {
		if strings.EqualFold(part, want) {
			return true
		}
	}
	return false
}

func hasOptionWithPrefix(parts []string, prefix string) bool {
	for _, part := range parts {
		if strings.HasPrefix(strings.ToLower(part), strings.ToLower(prefix)) {
			return true
		}
	}
	return false
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
