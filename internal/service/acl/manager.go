// Package acl provides file permission and POSIX ACL query/management services.
// Path security checks are enforced on every operation to prevent directory
// traversal; the same safePath logic used in the file package is reproduced
// here so this package has no import cycle on internal/service/file.
//
// setfacl / getfacl are used for POSIX ACL entries.  When these tools are not
// available the service degrades gracefully: Get returns base Unix permissions,
// and Set returns ErrNotFound with a clear installation hint.
package acl

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// Commander is the interface for running system commands with context support.
type Commander interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// Manager provides ACL and permission query/management operations.
type Manager struct {
	root      string
	commander Commander
	logger    audit.Logger
}

const defaultRoot = "/"

// NewManager creates a production Manager rooted at root.
// If root is empty, "/" is used.
func NewManager(root string, al audit.Logger) *Manager {
	if root == "" {
		root = defaultRoot
	}
	return &Manager{
		root:      filepath.Clean(root),
		commander: &osCommander{},
		logger:    al,
	}
}

// NewManagerWithCommander creates a Manager with injected dependencies (for testing).
func NewManagerWithCommander(root string, c Commander, al audit.Logger) *Manager {
	if root == "" {
		root = defaultRoot
	}
	return &Manager{
		root:      filepath.Clean(root),
		commander: c,
		logger:    al,
	}
}

// ── Get ───────────────────────────────────────────────────────────────────────

// Get returns the permission and ACL information for the file or directory at path.
// Extended ACL entries are populated when getfacl is available; otherwise only
// base Unix permissions are returned.
func (m *Manager) Get(ctx context.Context, path string) (*domain.ACLInfo, error) {
	abs, err := m.safePath(path)
	if err != nil {
		return nil, err
	}

	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperrors.Wrap(apperrors.ErrNotFound, "path not found", err)
		}
		return nil, apperrors.Wrap(apperrors.ErrInternal, "stat failed", err)
	}

	entry := &domain.ACLInfo{
		Path:       abs,
		Mode:       info.Mode().String(),
		ACLEntries: []domain.ACLPermission{},
	}

	if sys, ok := info.Sys().(*syscall.Stat_t); ok {
		entry.Owner = fmt.Sprintf("%d", sys.Uid)
		entry.Group = fmt.Sprintf("%d", sys.Gid)
	}

	// Attempt to read extended ACL entries via getfacl (best-effort).
	out, runErr := m.commander.Run(ctx, "getfacl", "--absolute-names", "--omit-header", abs)
	if runErr == nil && len(out) > 0 {
		entry.ACLEntries = parseGetfaclOutput(out)
	}

	return entry, nil
}

// ── Set ───────────────────────────────────────────────────────────────────────

// SetACL applies POSIX ACL entries to the file or directory at path using setfacl.
// entries is a slice of ACL specification strings in the form "type:qualifier:perms",
// e.g. ["u:alice:rwx", "g:devs:r-x"].
// Requires operator or admin role at the caller; records an audit event.
func (m *Manager) SetACL(ctx context.Context, path string, entries []string, source string) error {
	abs, err := m.safePath(path)
	if err != nil {
		m.logAudit(source, "acl.set", path, "failure", apperrors.ErrForbidden)
		return err
	}

	if len(entries) == 0 {
		return apperrors.New(apperrors.ErrInvalidInput, "entries must not be empty")
	}

	// Build -m argument: comma-joined ACL spec.
	spec := strings.Join(entries, ",")
	_, runErr := m.commander.Run(ctx, "setfacl", "-m", spec, abs)
	if runErr != nil {
		m.logAudit(source, "acl.set", path, "failure", apperrors.ErrInternal)
		// Provide actionable hint when setfacl is not installed.
		if isNotFound(runErr) {
			return apperrors.Wrap(apperrors.ErrNotFound,
				"setfacl not found: install the acl package to manage POSIX ACLs", runErr)
		}
		return apperrors.Wrap(apperrors.ErrInternal, "setfacl failed", runErr)
	}
	m.logAudit(source, "acl.set", path, "success", "")
	return nil
}

// ── path security ─────────────────────────────────────────────────────────────

// safePath resolves path relative to the manager root and verifies it does not
// escape the root boundary.  Returns the cleaned absolute path on success.
func (m *Manager) safePath(path string) (string, error) {
	abs := path
	if !filepath.IsAbs(path) {
		abs = filepath.Join(m.root, path)
	}
	abs = filepath.Clean(abs)

	if m.root != "/" && !strings.HasPrefix(abs, m.root+string(filepath.Separator)) && abs != m.root {
		return "", apperrors.New(apperrors.ErrForbidden, "path escapes root directory")
	}

	// Symlink-escape check.
	check := abs
	for {
		resolved, err := filepath.EvalSymlinks(check)
		if err == nil {
			if m.root != "/" && !strings.HasPrefix(resolved, m.root+string(filepath.Separator)) && resolved != m.root {
				return "", apperrors.New(apperrors.ErrForbidden, "path escapes root directory via symlink")
			}
			break
		}
		parent := filepath.Dir(check)
		if parent == check {
			break
		}
		check = parent
	}

	return abs, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// parseGetfaclOutput parses the output of `getfacl --absolute-names --omit-header`.
// Each non-comment line is in the form "type:qualifier:perms" (e.g. "user::rwx").
// The "default:" prefix common in default ACL entries is stripped before parsing.
// Inline comments (e.g. "\t#effective:r-x") are truncated from the permissions field.
// Only standard types (user/group/mask/other) and three-character permission strings are kept.
func parseGetfaclOutput(out []byte) []domain.ACLPermission {
	entries := []domain.ACLPermission{}
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Strip optional "default:" prefix (e.g. "default:user::rwx").
		if strings.HasPrefix(line, "default:") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "default:"))
			if line == "" {
				continue
			}
		}

		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}

		typ := strings.TrimSpace(parts[0])
		qualifier := strings.TrimSpace(parts[1])

		// Truncate trailing whitespace and inline comment suffix from permissions
		// (e.g. "rwx\t#effective:r-x" → "rwx").
		permsField := strings.TrimSpace(parts[2])
		if idx := strings.IndexAny(permsField, " \t#"); idx >= 0 {
			permsField = permsField[:idx]
		}
		perms := strings.TrimSpace(permsField)

		// Accept only standard ACL types and exactly three-character permission strings.
		switch typ {
		case "user", "group", "mask", "other":
		default:
			continue
		}
		if len(perms) != 3 {
			continue
		}

		entries = append(entries, domain.ACLPermission{
			Type:        typ,
			Qualifier:   qualifier,
			Permissions: perms,
		})
	}
	return entries
}

// isNotFound returns true when the error indicates an executable was not found in PATH.
func isNotFound(err error) bool {
	return errors.Is(err, exec.ErrNotFound)
}

func (m *Manager) logAudit(source, action, target, result string, code apperrors.ErrorCode) {
	if m.logger == nil {
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
	_ = m.logger.Log(event)
}

// ── osCommander ───────────────────────────────────────────────────────────────

type osCommander struct{}

func (c *osCommander) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return runCmd(ctx, name, args...)
}
