// Package acl provides file permission and ACL query/set services.
// Path safety validation is internal to this package to prevent directory
// traversal attacks (compliant with spec FR-011).
// All mutating operations emit audit events (spec FR-013).
package acl

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// Reader is the interface for querying file permissions and ACL.
// It can be replaced by a stub in unit tests.
type Reader interface {
	// GetACL returns the permissions and optional ACL entries for path.
	GetACL(ctx context.Context, path string) (*domain.FileACL, error)
}

// Writer is the interface for modifying file permissions and ACL.
// It can be replaced by a stub in unit tests.
type Writer interface {
	// SetMode applies chmod-style permissions to path.
	SetMode(ctx context.Context, path string, mode os.FileMode) error
	// SetOwner changes the owning user and/or group of path.
	// An empty string means "do not change".
	SetOwner(ctx context.Context, path string, owner, group string) error
}

// SetRequest carries the desired ACL changes for a path.
type SetRequest struct {
	// Mode is the new octal permission bits, e.g. 0o644.  Zero means no change.
	Mode os.FileMode
	// Owner is the new owning user name.  Empty means no change.
	Owner string
	// Group is the new owning group name.  Empty means no change.
	Group string
}

// Manager is the shared service for ACL operations.
type Manager struct {
	reader      Reader
	writer      Writer
	auditLogger audit.Logger
}

// NewManager creates a production Manager backed by the OS.
func NewManager(al audit.Logger) *Manager {
	return &Manager{
		reader:      &osReader{},
		writer:      &osWriter{},
		auditLogger: al,
	}
}

// NewManagerWithDeps creates a Manager with injected dependencies (for testing).
func NewManagerWithDeps(reader Reader, writer Writer, al audit.Logger) *Manager {
	return &Manager{reader: reader, writer: writer, auditLogger: al}
}

// Get returns the ACL and permissions for the given absolute path.
func (m *Manager) Get(ctx context.Context, path string) (*domain.FileACL, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	acl, err := m.reader.GetACL(ctx, path)
	if err != nil {
		return nil, err
	}
	return acl, nil
}

// Set applies the requested permission changes to the given absolute path.
// source identifies the caller and is recorded in the audit log.
func (m *Manager) Set(ctx context.Context, path string, req SetRequest, source string) error {
	if err := validatePath(path); err != nil {
		return err
	}

	if req.Mode != 0 {
		if err := m.writer.SetMode(ctx, path, req.Mode); err != nil {
			m.logAudit(source, path, "failure", apperrors.ErrInternal)
			return apperrors.Wrap(apperrors.ErrInternal,
				fmt.Sprintf("failed to set mode on %s", path), err)
		}
	}

	if req.Owner != "" || req.Group != "" {
		if err := m.writer.SetOwner(ctx, path, req.Owner, req.Group); err != nil {
			m.logAudit(source, path, "failure", apperrors.ErrInternal)
			return apperrors.Wrap(apperrors.ErrInternal,
				fmt.Sprintf("failed to set owner on %s", path), err)
		}
	}

	m.logAudit(source, path, "success", "")
	return nil
}

// validatePath checks that path is a non-empty absolute path that does not
// contain directory traversal sequences.
// We check for ".." segments in the raw (pre-clean) path: any literal ".."
// component indicates an attempted traversal regardless of surrounding segments.
// We then require the cleaned result to be absolute, catching purely-relative
// inputs such as "relative/path".
func validatePath(path string) error {
	if path == "" {
		return apperrors.New(apperrors.ErrInvalidInput, "path must not be empty")
	}
	// Reject any ".." component in the raw path before cleaning.
	for _, seg := range strings.Split(path, string(filepath.Separator)) {
		if seg == ".." {
			return apperrors.New(apperrors.ErrForbidden, "path traversal not permitted")
		}
	}
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		return apperrors.New(apperrors.ErrForbidden, "path must be absolute")
	}
	return nil
}

func (m *Manager) logAudit(source, path, result string, code apperrors.ErrorCode) {
	if m.auditLogger == nil {
		return
	}
	event := audit.AuditEvent{
		Source: source,
		Action: "acl.set",
		Target: path,
		Result: result,
	}
	if code != "" {
		event.ErrorCode = string(code)
	}
	_ = m.auditLogger.Log(event)
}

// ─── OS-backed implementations ────────────────────────────────────────────────

type osReader struct{}

func (o *osReader) GetACL(ctx context.Context, path string) (*domain.FileACL, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperrors.Wrap(apperrors.ErrNotFound,
				fmt.Sprintf("%s not found", path), err)
		}
		return nil, apperrors.Wrap(apperrors.ErrInternal,
			fmt.Sprintf("stat %s failed", path), err)
	}

	mode := info.Mode()
	ownerName, groupName := lookupOwnerGroup(info)

	acl := &domain.FileACL{
		Path:  path,
		Mode:  fmt.Sprintf("%04o", mode.Perm()),
		Owner: ownerName,
		Group: groupName,
	}

	// Best-effort: attempt to read extended ACL entries via getfacl.
	if entries, err := readGetfacl(ctx, path); err == nil {
		acl.Entries = entries
	}

	return acl, nil
}

// lookupOwnerGroup resolves uid/gid to names; falls back to numeric strings.
func lookupOwnerGroup(info os.FileInfo) (ownerName, groupName string) {
	sys, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", ""
	}
	if u, err := user.LookupId(strconv.FormatUint(uint64(sys.Uid), 10)); err == nil {
		ownerName = u.Username
	} else {
		ownerName = strconv.FormatUint(uint64(sys.Uid), 10)
	}
	if g, err := user.LookupGroupId(strconv.FormatUint(uint64(sys.Gid), 10)); err == nil {
		groupName = g.Name
	} else {
		groupName = strconv.FormatUint(uint64(sys.Gid), 10)
	}
	return ownerName, groupName
}

// readGetfacl runs getfacl and parses the output into ACLEntry values.
// Returns an error when getfacl is not available or fails.
func readGetfacl(ctx context.Context, path string) ([]domain.ACLEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "getfacl", "--omit-header", "--absolute-names", path).Output()
	if err != nil {
		return nil, err
	}
	return parseGetfaclOutput(out), nil
}

// parseGetfaclOutput converts getfacl output lines to ACLEntry values.
func parseGetfaclOutput(output []byte) []domain.ACLEntry {
	var entries []domain.ACLEntry
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Line format: <type>:<name>:<perms>  or  <type>::<perms>
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		entries = append(entries, domain.ACLEntry{
			Type:        parts[0],
			Name:        parts[1],
			Permissions: parts[2],
		})
	}
	return entries
}

type osWriter struct{}

func (o *osWriter) SetMode(_ context.Context, path string, mode os.FileMode) error {
	return os.Chmod(path, mode)
}

func (o *osWriter) SetOwner(ctx context.Context, path string, owner, group string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if owner == "" && group == "" {
		return nil
	}
	var arg string
	switch {
	case owner != "" && group != "":
		arg = owner + ":" + group
	case owner != "":
		arg = owner
	default:
		arg = ":" + group
	}
	out, err := exec.CommandContext(ctx, "chown", arg, path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
