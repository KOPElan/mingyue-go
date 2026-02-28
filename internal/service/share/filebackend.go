package share

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// Commander runs a system command and returns its combined output.
// It is defined here so the file backend can be tested with a stub.
type Commander interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// fileBackendConfig holds the paths and command executor for a fileBackend.
type fileBackendConfig struct {
	// StatePath is the path of the JSON state file (source of truth).
	StatePath string
	// SambaConfPath is the path of the generated samba config snippet.
	SambaConfPath string
	// NFSExportsPath is the path of the generated NFS exports snippet.
	NFSExportsPath string
	// Commander is used to invoke service reload commands.
	Commander Commander
}

// fileBackend is a thread-safe Backend that:
//   - Keeps share state in memory (loaded from JSON on startup).
//   - On Reload(), persists state to JSON and regenerates Samba/NFS config files,
//     then signals the relevant service(s) to reload their configuration.
type fileBackend struct {
	mu     sync.RWMutex
	shares map[string]domain.Share
	cfg    fileBackendConfig
}

// newFileBackend returns the production file-backed Backend.
// It reads existing configuration from /var/lib/mingyue/shares.json on startup.
func newFileBackend() Backend {
	return newFileBackendWithConfig(fileBackendConfig{
		StatePath:      "/var/lib/mingyue/shares.json",
		SambaConfPath:  "/etc/samba/smb.conf.d/mingyue.conf",
		NFSExportsPath: "/etc/exports.d/mingyue.exports",
		Commander:      &osShareCommander{},
	})
}

// newFileBackendWithConfig creates a fileBackend with injected configuration.
// Primarily used for unit tests.
func newFileBackendWithConfig(cfg fileBackendConfig) *fileBackend {
	b := &fileBackend{
		shares: make(map[string]domain.Share),
		cfg:    cfg,
	}
	b.load()
	return b
}

// load initialises in-memory state from the JSON state file.
// A missing state file is treated as a clean first-run and silently ignored.
// Any other read or parse error is logged to stderr so operators can detect
// and fix configuration problems without a full daemon restart.
func (b *fileBackend) load() {
	data, err := os.ReadFile(b.cfg.StatePath)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("share: warning: could not read state file %s: %v", b.cfg.StatePath, err)
		}
		return
	}
	var shares []domain.Share
	if err := json.Unmarshal(data, &shares); err != nil {
		log.Printf("share: warning: could not parse state file %s: %v (starting with empty state)", b.cfg.StatePath, err)
		return
	}
	for _, s := range shares {
		b.shares[s.Name] = s
	}
}

// ── Backend interface ─────────────────────────────────────────────────────────

func (b *fileBackend) List(_ context.Context) ([]domain.Share, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]domain.Share, 0, len(b.shares))
	for _, s := range b.shares {
		result = append(result, s)
	}
	return result, nil
}

func (b *fileBackend) Get(_ context.Context, name string) (*domain.Share, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	s, ok := b.shares[name]
	if !ok {
		return nil, apperrors.New(apperrors.ErrNotFound, fmt.Sprintf("share %q not found", name))
	}
	cp := s
	return &cp, nil
}

func (b *fileBackend) Create(_ context.Context, s domain.Share) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.shares[s.Name]; exists {
		return apperrors.New(apperrors.ErrConflict, fmt.Sprintf("share %q already exists", s.Name))
	}
	b.shares[s.Name] = s
	return nil
}

func (b *fileBackend) Update(_ context.Context, s domain.Share) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.shares[s.Name]; !exists {
		return apperrors.New(apperrors.ErrNotFound, fmt.Sprintf("share %q not found", s.Name))
	}
	b.shares[s.Name] = s
	return nil
}

func (b *fileBackend) Delete(_ context.Context, name string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.shares[name]; !exists {
		return apperrors.New(apperrors.ErrNotFound, fmt.Sprintf("share %q not found", name))
	}
	delete(b.shares, name)
	return nil
}

// Reload persists the current in-memory share state to disk and regenerates
// Samba and NFS configuration snippets, then signals the appropriate services.
//
// It reloads a service if there are currently shares of that type OR if the
// previous on-disk state contained shares of that type (handles the "delete
// last share" case so the service picks up the now-empty config immediately).
//
// If a service reload command fails the on-disk files are restored to the state
// they were in before this Reload call, so that a process restart will not load
// a partially-updated configuration.
func (b *fileBackend) Reload(ctx context.Context) error {
	b.mu.RLock()
	shares := make([]domain.Share, 0, len(b.shares))
	for _, s := range b.shares {
		shares = append(shares, s)
	}
	b.mu.RUnlock()

	// Deterministic output order.
	sort.Slice(shares, func(i, j int) bool { return shares[i].Name < shares[j].Name })

	// Snapshot on-disk files before writing so we can roll them back if the
	// service reload command fails.  Non-existence is treated as a nil snapshot
	// (restoreFile will remove the file during rollback in that case).
	oldState, _ := os.ReadFile(b.cfg.StatePath)
	oldSambaConf, _ := os.ReadFile(b.cfg.SambaConfPath)
	oldNFSExports, _ := os.ReadFile(b.cfg.NFSExportsPath)

	// Determine which service types were present on disk before this reload so
	// that we reload a service even when its last share was just deleted.
	hadSamba, hadNFS := diskHasShareTypes(oldState)

	// Persist the authoritative JSON state file first.
	if err := b.saveState(shares); err != nil {
		return apperrors.Wrap(apperrors.ErrInternal, "failed to persist share state", err)
	}

	// Write both config files so they always reflect the current state.
	if err := b.writeSambaConf(shares); err != nil {
		return err
	}
	if err := b.writeNFSExports(shares); err != nil {
		return err
	}

	// Reload services that have shares in the current configuration OR had them
	// in the previous configuration.
	hasSamba, hasNFS := false, false
	for _, s := range shares {
		switch s.Type {
		case domain.ShareTypeSamba:
			hasSamba = true
		case domain.ShareTypeNFS:
			hasNFS = true
		}
	}

	// restoreDisk rolls back the on-disk files to the snapshot taken above.
	restoreDisk := func() {
		restoreFile(b.cfg.StatePath, oldState, 0640)
		restoreFile(b.cfg.SambaConfPath, oldSambaConf, 0640)
		restoreFile(b.cfg.NFSExportsPath, oldNFSExports, 0640)
	}

	if hasSamba || hadSamba {
		if _, err := b.cfg.Commander.Run(ctx, "smbcontrol", "all", "reload-config"); err != nil {
			restoreDisk()
			return apperrors.Wrap(apperrors.ErrInternal, "failed to reload samba", err)
		}
	}
	if hasNFS || hadNFS {
		if _, err := b.cfg.Commander.Run(ctx, "exportfs", "-ra"); err != nil {
			restoreDisk()
			return apperrors.Wrap(apperrors.ErrInternal, "failed to reload NFS exports", err)
		}
	}

	return nil
}

// ── persistence ───────────────────────────────────────────────────────────────

// diskHasShareTypes parses a JSON state snapshot and returns whether it
// contains Samba or NFS shares.  Errors (including a nil/empty input) are
// silently ignored and both booleans default to false.
func diskHasShareTypes(data []byte) (hasSamba, hasNFS bool) {
	if len(data) == 0 {
		return
	}
	var shares []domain.Share
	if err := json.Unmarshal(data, &shares); err != nil {
		return
	}
	for _, s := range shares {
		switch s.Type {
		case domain.ShareTypeSamba:
			hasSamba = true
		case domain.ShareTypeNFS:
			hasNFS = true
		}
	}
	return
}

// restoreFile writes content back to path, or removes path when content is nil
// (indicating the file did not exist before the current Reload cycle).
// Errors are logged but not propagated; the caller is already handling a failure path.
func restoreFile(path string, content []byte, perm os.FileMode) {
	if content != nil {
		if err := os.WriteFile(path, content, perm); err != nil {
			log.Printf("share: warning: could not restore %s during rollback: %v", path, err)
		}
	} else {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			log.Printf("share: warning: could not remove %s during rollback: %v", path, err)
		}
	}
}

// saveState writes the given shares as a JSON array to the state file.
// Parent directories are created if necessary.
func (b *fileBackend) saveState(shares []domain.Share) error {
	if err := os.MkdirAll(filepath.Dir(b.cfg.StatePath), 0750); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}
	data, err := json.MarshalIndent(shares, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal share state: %w", err)
	}
	if err := os.WriteFile(b.cfg.StatePath, data, 0640); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	return nil
}

// ── Samba config generation ───────────────────────────────────────────────────

// writeSambaConf generates a Samba configuration snippet and writes it to
// SambaConfPath. Each enabled share gets an INI section; disabled shares are
// included as commented-out sections so their names are preserved.
//
// Include this file in /etc/samba/smb.conf via:
//
//	include = /etc/samba/smb.conf.d/mingyue.conf
func (b *fileBackend) writeSambaConf(shares []domain.Share) error {
	if err := os.MkdirAll(filepath.Dir(b.cfg.SambaConfPath), 0750); err != nil {
		return apperrors.Wrap(apperrors.ErrInternal, "create samba config directory", err)
	}

	var buf bytes.Buffer
	buf.WriteString("# Generated by mingyue – do not edit manually.\n")
	buf.WriteString("# To activate: add  include = " + b.cfg.SambaConfPath + "  to /etc/samba/smb.conf\n\n")

	for _, s := range shares {
		if s.Type != domain.ShareTypeSamba {
			continue
		}
		readOnly := "no"
		if s.ReadOnly {
			readOnly = "yes"
		}
		if !s.Enabled {
			// Write as a comment block so the share name is preserved.
			buf.WriteString(fmt.Sprintf("# [%s] (disabled)\n", s.Name))
			buf.WriteString(fmt.Sprintf("#    path = %s\n", s.Path))
			buf.WriteString(fmt.Sprintf("#    comment = %s\n", s.Comment))
			buf.WriteString(fmt.Sprintf("#    read only = %s\n\n", readOnly))
			continue
		}
		buf.WriteString(fmt.Sprintf("[%s]\n", s.Name))
		buf.WriteString(fmt.Sprintf("    path = %s\n", s.Path))
		if s.Comment != "" {
			buf.WriteString(fmt.Sprintf("    comment = %s\n", s.Comment))
		}
		buf.WriteString(fmt.Sprintf("    read only = %s\n", readOnly))
		if s.ValidUsers != "" {
			buf.WriteString(fmt.Sprintf("    valid users = %s\n", s.ValidUsers))
		}
		if s.WriteList != "" {
			buf.WriteString(fmt.Sprintf("    write list = %s\n", s.WriteList))
		}
		if s.CreateMask != "" {
			buf.WriteString(fmt.Sprintf("    create mask = %s\n", s.CreateMask))
		}
		if s.DirectoryMask != "" {
			buf.WriteString(fmt.Sprintf("    directory mask = %s\n", s.DirectoryMask))
		}
		buf.WriteString("    browsable = yes\n\n")
	}

	if err := os.WriteFile(b.cfg.SambaConfPath, buf.Bytes(), 0640); err != nil {
		return apperrors.Wrap(apperrors.ErrInternal, "write samba config", err)
	}
	return nil
}

// ── NFS exports generation ────────────────────────────────────────────────────

// writeNFSExports generates an NFS exports file and writes it to NFSExportsPath.
// Only enabled shares of type NFS are exported. Disabled shares are omitted.
//
// The generated file follows the exports(5) format accepted by exportfs(8).
func (b *fileBackend) writeNFSExports(shares []domain.Share) error {
	if err := os.MkdirAll(filepath.Dir(b.cfg.NFSExportsPath), 0750); err != nil {
		return apperrors.Wrap(apperrors.ErrInternal, "create NFS exports directory", err)
	}

	var buf bytes.Buffer
	buf.WriteString("# Generated by mingyue – do not edit manually.\n\n")

	for _, s := range shares {
		if s.Type != domain.ShareTypeNFS || !s.Enabled {
			continue
		}
		opts := "sync,no_subtree_check"
		if s.ReadOnly {
			opts = "ro," + opts
		} else {
			opts = "rw," + opts
		}
		comment := ""
		if s.Comment != "" {
			comment = " # " + s.Comment
		}
		hosts := s.Hosts
		if hosts == "" {
			hosts = "*"
		}
		// hosts may be a single entry or space-separated list.  Each entry is
		// wrapped with the export options in parentheses.
		hostEntries := strings.Fields(hosts)
		var hostParts []string
		for _, h := range hostEntries {
			hostParts = append(hostParts, fmt.Sprintf("%s(%s)", h, opts))
		}
		buf.WriteString(fmt.Sprintf("%s %s%s\n", s.Path, strings.Join(hostParts, " "), comment))
	}

	if err := os.WriteFile(b.cfg.NFSExportsPath, buf.Bytes(), 0640); err != nil {
		return apperrors.Wrap(apperrors.ErrInternal, "write NFS exports", err)
	}
	return nil
}

// ── production Commander ──────────────────────────────────────────────────────

// osShareCommander runs real system commands using exec.CommandContext.
type osShareCommander struct{}

func (c *osShareCommander) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).CombinedOutput()
}
