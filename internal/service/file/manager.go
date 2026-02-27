// Package file provides file management services with path security enforcement.
// All operations validate the requested path against a configured root directory
// to prevent directory traversal attacks (e.g. "../etc/passwd").
package file

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"kopelan/mingyue-go/internal/audit"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// defaultRoot is the filesystem root used when no root is configured.
// When set to "/", all absolute paths on the host are accessible.
// In production deployments, configure a more restrictive root (e.g.
// /var/lib/mingyue/data) to limit the accessible filesystem namespace.
const defaultRoot = "/"

// Manager provides file operations scoped to a root directory.
// Path security is enforced on every operation: any path that resolves
// outside the root returns ErrForbidden without touching the filesystem.
type Manager struct {
	root        string
	auditLogger audit.Logger
	// fs abstracts filesystem operations for testing.
	fs FS
}

// NewManager creates a production Manager rooted at root.
// If root is empty, "/" is used.  The caller must call Close on the audit
// logger when done.
func NewManager(root string, al audit.Logger) *Manager {
	if root == "" {
		root = defaultRoot
	}
	return &Manager{root: filepath.Clean(root), auditLogger: al, fs: &osFS{}}
}

// NewManagerWithFS creates a Manager with an injectable FS (for testing).
func NewManagerWithFS(root string, al audit.Logger, fs FS) *Manager {
	if root == "" {
		root = defaultRoot
	}
	return &Manager{root: filepath.Clean(root), auditLogger: al, fs: fs}
}

// ── Path security ─────────────────────────────────────────────────────────────

// safePath resolves path relative to the manager root and verifies it does
// not escape the root.  Returns the cleaned absolute path on success.
func (m *Manager) safePath(path string) (string, error) {
	// Make path absolute relative to root.
	abs := path
	if !filepath.IsAbs(path) {
		abs = filepath.Join(m.root, path)
	}
	abs = filepath.Clean(abs)

	// Enforce root boundary.
	// When root is "/", every cleaned absolute path is valid.
	// Otherwise, the path must be equal to root or start with root + separator.
	if m.root != "/" && !strings.HasPrefix(abs, m.root+string(filepath.Separator)) && abs != m.root {
		return "", apperrors.New(apperrors.ErrForbidden, "path escapes root directory")
	}
	return abs, nil
}

// ── List ──────────────────────────────────────────────────────────────────────

// List returns the directory entries under path.
func (m *Manager) List(_ context.Context, path string) ([]domain.FileEntry, error) {
	abs, err := m.safePath(path)
	if err != nil {
		return nil, err
	}

	entries, err := m.fs.ReadDir(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperrors.Wrap(apperrors.ErrNotFound, "directory not found", err)
		}
		return nil, apperrors.Wrap(apperrors.ErrInternal, "failed to read directory", err)
	}

	result := make([]domain.FileEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		fe := domain.FileEntry{
			Name:    e.Name(),
			Path:    filepath.Join(abs, e.Name()),
			IsDir:   e.IsDir(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime().UTC(),
		}
		if sys, ok := info.Sys().(*syscall.Stat_t); ok {
			fe.Owner = fmt.Sprintf("%d", sys.Uid)
		}
		result = append(result, fe)
	}
	return result, nil
}

// ── Stat ──────────────────────────────────────────────────────────────────────

// Stat returns metadata for the file or directory at path.
func (m *Manager) Stat(_ context.Context, path string) (*domain.FileEntry, error) {
	abs, err := m.safePath(path)
	if err != nil {
		return nil, err
	}

	info, err := m.fs.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperrors.Wrap(apperrors.ErrNotFound, "path not found", err)
		}
		return nil, apperrors.Wrap(apperrors.ErrInternal, "stat failed", err)
	}

	fe := &domain.FileEntry{
		Name:    info.Name(),
		Path:    abs,
		IsDir:   info.IsDir(),
		Size:    info.Size(),
		Mode:    info.Mode().String(),
		ModTime: info.ModTime().UTC(),
	}
	if sys, ok := info.Sys().(*syscall.Stat_t); ok {
		fe.Owner = fmt.Sprintf("%d", sys.Uid)
	}
	return fe, nil
}

// ── Mkdir ─────────────────────────────────────────────────────────────────────

// Mkdir creates the directory at path (and all necessary parents).
func (m *Manager) Mkdir(_ context.Context, path, source string) error {
	abs, err := m.safePath(path)
	if err != nil {
		m.logAudit(source, "file.mkdir", path, "failure", apperrors.ErrForbidden)
		return err
	}

	if err := m.fs.MkdirAll(abs, 0o755); err != nil {
		m.logAudit(source, "file.mkdir", path, "failure", apperrors.ErrInternal)
		return apperrors.Wrap(apperrors.ErrInternal, "failed to create directory", err)
	}
	m.logAudit(source, "file.mkdir", path, "success", "")
	return nil
}

// ── Remove ────────────────────────────────────────────────────────────────────

// Remove deletes the file or directory at path.  Directories must be empty
// unless recursive is true.
func (m *Manager) Remove(_ context.Context, path string, recursive bool, source string) error {
	abs, err := m.safePath(path)
	if err != nil {
		m.logAudit(source, "file.remove", path, "failure", apperrors.ErrForbidden)
		return err
	}

	var rmErr error
	if recursive {
		rmErr = m.fs.RemoveAll(abs)
	} else {
		rmErr = m.fs.Remove(abs)
	}
	if rmErr != nil {
		if os.IsNotExist(rmErr) {
			m.logAudit(source, "file.remove", path, "failure", apperrors.ErrNotFound)
			return apperrors.Wrap(apperrors.ErrNotFound, "path not found", rmErr)
		}
		m.logAudit(source, "file.remove", path, "failure", apperrors.ErrInternal)
		return apperrors.Wrap(apperrors.ErrInternal, "failed to remove path", rmErr)
	}
	m.logAudit(source, "file.remove", path, "success", "")
	return nil
}

// ── Move ──────────────────────────────────────────────────────────────────────

// Move renames src to dst.
func (m *Manager) Move(_ context.Context, src, dst, source string) error {
	absSrc, err := m.safePath(src)
	if err != nil {
		m.logAudit(source, "file.move", src+"→"+dst, "failure", apperrors.ErrForbidden)
		return err
	}
	absDst, err := m.safePath(dst)
	if err != nil {
		m.logAudit(source, "file.move", src+"→"+dst, "failure", apperrors.ErrForbidden)
		return err
	}

	if err := m.fs.Rename(absSrc, absDst); err != nil {
		m.logAudit(source, "file.move", src+"→"+dst, "failure", apperrors.ErrInternal)
		return apperrors.Wrap(apperrors.ErrInternal, "failed to move path", err)
	}
	m.logAudit(source, "file.move", src+"→"+dst, "success", "")
	return nil
}

// ── Copy ──────────────────────────────────────────────────────────────────────

// Copy duplicates the file at src to dst.
func (m *Manager) Copy(_ context.Context, src, dst, source string) error {
	absSrc, err := m.safePath(src)
	if err != nil {
		m.logAudit(source, "file.copy", src+"→"+dst, "failure", apperrors.ErrForbidden)
		return err
	}
	absDst, err := m.safePath(dst)
	if err != nil {
		m.logAudit(source, "file.copy", src+"→"+dst, "failure", apperrors.ErrForbidden)
		return err
	}

	if err := m.fs.CopyFile(absSrc, absDst); err != nil {
		m.logAudit(source, "file.copy", src+"→"+dst, "failure", apperrors.ErrInternal)
		return apperrors.Wrap(apperrors.ErrInternal, "failed to copy file", err)
	}
	m.logAudit(source, "file.copy", src+"→"+dst, "success", "")
	return nil
}

// ── Read ──────────────────────────────────────────────────────────────────────

// Read returns the contents of the file at path.
func (m *Manager) Read(_ context.Context, path string) ([]byte, error) {
	abs, err := m.safePath(path)
	if err != nil {
		return nil, err
	}

	data, err := m.fs.ReadFile(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, apperrors.Wrap(apperrors.ErrNotFound, "file not found", err)
		}
		return nil, apperrors.Wrap(apperrors.ErrInternal, "failed to read file", err)
	}
	return data, nil
}

// ── Write ─────────────────────────────────────────────────────────────────────

// Write writes data to the file at path, creating it if necessary.
func (m *Manager) Write(_ context.Context, path string, data []byte, source string) error {
	abs, err := m.safePath(path)
	if err != nil {
		m.logAudit(source, "file.write", path, "failure", apperrors.ErrForbidden)
		return err
	}

	if err := m.fs.WriteFile(abs, data, 0o644); err != nil {
		m.logAudit(source, "file.write", path, "failure", apperrors.ErrInternal)
		return apperrors.Wrap(apperrors.ErrInternal, "failed to write file", err)
	}
	m.logAudit(source, "file.write", path, "success", "")
	return nil
}

// ── audit ─────────────────────────────────────────────────────────────────────

func (m *Manager) logAudit(source, action, target, result string, code apperrors.ErrorCode) {
	if m.auditLogger == nil {
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
	_ = m.auditLogger.Log(event)
}

// ── FS interface ──────────────────────────────────────────────────────────────

// FS abstracts filesystem operations so they can be replaced in tests.
type FS interface {
	ReadDir(path string) ([]os.DirEntry, error)
	Stat(path string) (os.FileInfo, error)
	MkdirAll(path string, perm os.FileMode) error
	Remove(path string) error
	RemoveAll(path string) error
	Rename(src, dst string) error
	CopyFile(src, dst string) error
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, perm os.FileMode) error
}

// osFS is the production FS backed by the real operating system.
type osFS struct{}

func (osFS) ReadDir(path string) ([]os.DirEntry, error)           { return os.ReadDir(path) }
func (osFS) Stat(path string) (os.FileInfo, error)                 { return os.Stat(path) }
func (osFS) MkdirAll(path string, perm os.FileMode) error          { return os.MkdirAll(path, perm) }
func (osFS) Remove(path string) error                              { return os.Remove(path) }
func (osFS) RemoveAll(path string) error                           { return os.RemoveAll(path) }
func (osFS) Rename(src, dst string) error                          { return os.Rename(src, dst) }
func (osFS) ReadFile(path string) ([]byte, error)                  { return os.ReadFile(path) }
func (osFS) WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

func (osFS) CopyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
