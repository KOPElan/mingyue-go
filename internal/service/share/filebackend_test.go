package share

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// ── stub Commander ────────────────────────────────────────────────────────────

type stubCommander struct {
	calls []stubCall
	err   error
}

type stubCall struct {
	name string
	args []string
}

func (c *stubCommander) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	c.calls = append(c.calls, stubCall{name: name, args: args})
	return nil, c.err
}

// ── helpers ───────────────────────────────────────────────────────────────────

// newTestFileBackend creates a fileBackend wired to a temp directory.
func newTestFileBackend(t *testing.T, cmd Commander) (*fileBackend, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := fileBackendConfig{
		StatePath:      filepath.Join(dir, "shares.json"),
		SambaConfPath:  filepath.Join(dir, "mingyue.conf"),
		NFSExportsPath: filepath.Join(dir, "mingyue.exports"),
		Commander:      cmd,
	}
	b := newFileBackendWithConfig(cfg)
	return b, dir
}

func makeSambaShare(name string) domain.Share {
	return domain.Share{
		Name:     name,
		Type:     domain.ShareTypeSamba,
		Path:     "/srv/" + name,
		ReadOnly: false,
		Enabled:  true,
		Comment:  "test share",
	}
}

func makeNFSShare(name string) domain.Share {
	return domain.Share{
		Name:     name,
		Type:     domain.ShareTypeNFS,
		Path:     "/export/" + name,
		ReadOnly: true,
		Enabled:  true,
	}
}

// ── CRUD tests ────────────────────────────────────────────────────────────────

func TestFileBackend_CreateAndGet(t *testing.T) {
	cmd := &stubCommander{}
	b, _ := newTestFileBackend(t, cmd)
	ctx := context.Background()

	s := makeSambaShare("docs")
	if err := b.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := b.Get(ctx, "docs")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "docs" || got.Path != "/srv/docs" {
		t.Errorf("unexpected share: %+v", got)
	}
}

func TestFileBackend_Create_Duplicate(t *testing.T) {
	cmd := &stubCommander{}
	b, _ := newTestFileBackend(t, cmd)
	ctx := context.Background()

	_ = b.Create(ctx, makeSambaShare("x"))
	err := b.Create(ctx, makeSambaShare("x"))
	if err == nil {
		t.Fatal("expected error for duplicate create")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != apperrors.ErrConflict {
		t.Errorf("expected ErrConflict, got %v", err)
	}
}

func TestFileBackend_Get_NotFound(t *testing.T) {
	cmd := &stubCommander{}
	b, _ := newTestFileBackend(t, cmd)

	_, err := b.Get(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != apperrors.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFileBackend_Update(t *testing.T) {
	cmd := &stubCommander{}
	b, _ := newTestFileBackend(t, cmd)
	ctx := context.Background()

	_ = b.Create(ctx, makeSambaShare("data"))

	updated := makeSambaShare("data")
	updated.Comment = "updated"
	if err := b.Update(ctx, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := b.Get(ctx, "data")
	if got.Comment != "updated" {
		t.Errorf("Comment: got %q, want %q", got.Comment, "updated")
	}
}

func TestFileBackend_Update_NotFound(t *testing.T) {
	cmd := &stubCommander{}
	b, _ := newTestFileBackend(t, cmd)

	err := b.Update(context.Background(), makeSambaShare("ghost"))
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != apperrors.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFileBackend_Delete(t *testing.T) {
	cmd := &stubCommander{}
	b, _ := newTestFileBackend(t, cmd)
	ctx := context.Background()

	_ = b.Create(ctx, makeSambaShare("tmp"))
	if err := b.Delete(ctx, "tmp"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	_, err := b.Get(ctx, "tmp")
	if err == nil {
		t.Error("expected share to be deleted")
	}
}

func TestFileBackend_Delete_NotFound(t *testing.T) {
	cmd := &stubCommander{}
	b, _ := newTestFileBackend(t, cmd)

	err := b.Delete(context.Background(), "none")
	if err == nil {
		t.Fatal("expected error")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != apperrors.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestFileBackend_List(t *testing.T) {
	cmd := &stubCommander{}
	b, _ := newTestFileBackend(t, cmd)
	ctx := context.Background()

	_ = b.Create(ctx, makeSambaShare("a"))
	_ = b.Create(ctx, makeSambaShare("b"))
	_ = b.Create(ctx, makeNFSShare("c"))

	shares, err := b.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(shares) != 3 {
		t.Errorf("len: got %d, want 3", len(shares))
	}
}

// ── Reload: JSON state persistence ───────────────────────────────────────────

func TestFileBackend_Reload_PersistsJSON(t *testing.T) {
	cmd := &stubCommander{}
	b, dir := newTestFileBackend(t, cmd)
	ctx := context.Background()

	_ = b.Create(ctx, makeSambaShare("backup"))
	if err := b.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "shares.json"))
	if err != nil {
		t.Fatalf("read state file: %v", err)
	}
	var shares []domain.Share
	if err := json.Unmarshal(data, &shares); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(shares) != 1 || shares[0].Name != "backup" {
		t.Errorf("unexpected state: %+v", shares)
	}
}

// ── Reload: Samba config generation ──────────────────────────────────────────

func TestFileBackend_Reload_WritesSambaConf(t *testing.T) {
	cmd := &stubCommander{}
	b, dir := newTestFileBackend(t, cmd)
	ctx := context.Background()

	s := makeSambaShare("media")
	_ = b.Create(ctx, s)
	if err := b.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "mingyue.conf"))
	if err != nil {
		t.Fatalf("read samba conf: %v", err)
	}
	conf := string(content)
	if !strings.Contains(conf, "[media]") {
		t.Errorf("samba conf missing section [media]:\n%s", conf)
	}
	if !strings.Contains(conf, "path = /srv/media") {
		t.Errorf("samba conf missing path:\n%s", conf)
	}
}

func TestFileBackend_Reload_SambaConf_ReadOnly(t *testing.T) {
	cmd := &stubCommander{}
	b, dir := newTestFileBackend(t, cmd)
	ctx := context.Background()

	s := makeSambaShare("ro")
	s.ReadOnly = true
	_ = b.Create(ctx, s)
	_ = b.Reload(ctx)

	content, _ := os.ReadFile(filepath.Join(dir, "mingyue.conf"))
	if !strings.Contains(string(content), "read only = yes") {
		t.Errorf("samba conf missing 'read only = yes':\n%s", content)
	}
}

func TestFileBackend_Reload_SambaConf_DisabledShare(t *testing.T) {
	cmd := &stubCommander{}
	b, dir := newTestFileBackend(t, cmd)
	ctx := context.Background()

	s := makeSambaShare("disabled")
	s.Enabled = false
	_ = b.Create(ctx, s)
	_ = b.Reload(ctx)

	content, _ := os.ReadFile(filepath.Join(dir, "mingyue.conf"))
	// Disabled shares must appear as comments, not active sections.
	if strings.Contains(string(content), "\n[disabled]") {
		t.Errorf("disabled share should not appear as active section:\n%s", content)
	}
	if !strings.Contains(string(content), "# [disabled]") {
		t.Errorf("disabled share should appear as comment:\n%s", content)
	}
}

// ── Reload: NFS exports generation ───────────────────────────────────────────

func TestFileBackend_Reload_WritesNFSExports(t *testing.T) {
	cmd := &stubCommander{}
	b, dir := newTestFileBackend(t, cmd)
	ctx := context.Background()

	s := makeNFSShare("videos")
	_ = b.Create(ctx, s)
	if err := b.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "mingyue.exports"))
	if err != nil {
		t.Fatalf("read exports: %v", err)
	}
	exports := string(content)
	if !strings.Contains(exports, "/export/videos") {
		t.Errorf("exports missing /export/videos:\n%s", exports)
	}
	if !strings.Contains(exports, "ro,") {
		t.Errorf("exports missing read-only option:\n%s", exports)
	}
}

func TestFileBackend_Reload_NFSExports_WritableShare(t *testing.T) {
	cmd := &stubCommander{}
	b, dir := newTestFileBackend(t, cmd)
	ctx := context.Background()

	s := makeNFSShare("data")
	s.ReadOnly = false
	_ = b.Create(ctx, s)
	_ = b.Reload(ctx)

	content, _ := os.ReadFile(filepath.Join(dir, "mingyue.exports"))
	if !strings.Contains(string(content), "rw,") {
		t.Errorf("exports missing rw option:\n%s", content)
	}
}

func TestFileBackend_Reload_NFSExports_DisabledShare(t *testing.T) {
	cmd := &stubCommander{}
	b, dir := newTestFileBackend(t, cmd)
	ctx := context.Background()

	s := makeNFSShare("disabled")
	s.Enabled = false
	_ = b.Create(ctx, s)
	_ = b.Reload(ctx)

	content, _ := os.ReadFile(filepath.Join(dir, "mingyue.exports"))
	if strings.Contains(string(content), "/export/disabled") {
		t.Errorf("disabled NFS share should not appear in exports:\n%s", content)
	}
}

// ── Reload: service reload commands ──────────────────────────────────────────

func TestFileBackend_Reload_SambaReloadCalledForSambaShare(t *testing.T) {
	cmd := &stubCommander{}
	b, _ := newTestFileBackend(t, cmd)
	ctx := context.Background()

	_ = b.Create(ctx, makeSambaShare("share"))
	if err := b.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	if len(cmd.calls) == 0 {
		t.Fatal("expected at least one command call")
	}
	found := false
	for _, c := range cmd.calls {
		if c.name == "smbcontrol" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected smbcontrol call, got: %+v", cmd.calls)
	}
}

func TestFileBackend_Reload_NFSReloadCalledForNFSShare(t *testing.T) {
	cmd := &stubCommander{}
	b, _ := newTestFileBackend(t, cmd)
	ctx := context.Background()

	_ = b.Create(ctx, makeNFSShare("export"))
	if err := b.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	found := false
	for _, c := range cmd.calls {
		if c.name == "exportfs" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected exportfs call, got: %+v", cmd.calls)
	}
}

func TestFileBackend_Reload_NoSambaReloadWhenNoSambaShares(t *testing.T) {
	cmd := &stubCommander{}
	b, _ := newTestFileBackend(t, cmd)
	ctx := context.Background()

	// Only NFS shares, no samba shares.
	_ = b.Create(ctx, makeNFSShare("data"))
	if err := b.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	for _, c := range cmd.calls {
		if c.name == "smbcontrol" {
			t.Errorf("unexpected smbcontrol call when no samba shares")
		}
	}
}

func TestFileBackend_Reload_NoNFSReloadWhenNoNFSShares(t *testing.T) {
	cmd := &stubCommander{}
	b, _ := newTestFileBackend(t, cmd)
	ctx := context.Background()

	// Only Samba shares, no NFS shares.
	_ = b.Create(ctx, makeSambaShare("share"))
	if err := b.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	for _, c := range cmd.calls {
		if c.name == "exportfs" {
			t.Errorf("unexpected exportfs call when no NFS shares")
		}
	}
}

func TestFileBackend_Reload_SambaReloadFailure(t *testing.T) {
	cmd := &stubCommander{err: fmt.Errorf("smbcontrol not found")}
	b, _ := newTestFileBackend(t, cmd)
	ctx := context.Background()

	_ = b.Create(ctx, makeSambaShare("share"))
	err := b.Reload(ctx)
	if err == nil {
		t.Fatal("expected error when samba reload fails")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != apperrors.ErrInternal {
		t.Errorf("expected ErrInternal, got %v", err)
	}
}

// ── load from JSON on startup ─────────────────────────────────────────────────

func TestFileBackend_LoadsStateOnStartup(t *testing.T) {
	cmd := &stubCommander{}
	b, dir := newTestFileBackend(t, cmd)
	ctx := context.Background()

	// Create shares and persist.
	_ = b.Create(ctx, makeSambaShare("persist"))
	_ = b.Reload(ctx)

	// Create a new backend pointing to the same directory; it should load the
	// previously persisted state.
	b2 := newFileBackendWithConfig(fileBackendConfig{
		StatePath:      filepath.Join(dir, "shares.json"),
		SambaConfPath:  filepath.Join(dir, "mingyue.conf"),
		NFSExportsPath: filepath.Join(dir, "mingyue.exports"),
		Commander:      &stubCommander{},
	})

	shares, err := b2.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(shares) != 1 || shares[0].Name != "persist" {
		t.Errorf("loaded shares: %+v", shares)
	}
}

// ── Samba config format validation ───────────────────────────────────────────

func TestFileBackend_SambaConf_ValidINIFormat(t *testing.T) {
	cmd := &stubCommander{}
	b, dir := newTestFileBackend(t, cmd)
	ctx := context.Background()

	_ = b.Create(ctx, makeSambaShare("docs"))
	_ = b.Reload(ctx)

	content, _ := os.ReadFile(filepath.Join(dir, "mingyue.conf"))
	scanner := bufio.NewScanner(bytes.NewReader(content))
	sectionFound := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "[docs]" {
			sectionFound = true
		}
	}
	if !sectionFound {
		t.Errorf("expected INI section [docs] in samba conf:\n%s", content)
	}
}

// ── mixed share types ─────────────────────────────────────────────────────────

func TestFileBackend_Reload_MixedTypes(t *testing.T) {
	cmd := &stubCommander{}
	b, _ := newTestFileBackend(t, cmd)
	ctx := context.Background()

	_ = b.Create(ctx, makeSambaShare("smb1"))
	_ = b.Create(ctx, makeNFSShare("nfs1"))
	if err := b.Reload(ctx); err != nil {
		t.Fatalf("Reload: %v", err)
	}

	hasSamba, hasNFS := false, false
	for _, c := range cmd.calls {
		if c.name == "smbcontrol" {
			hasSamba = true
		}
		if c.name == "exportfs" {
			hasNFS = true
		}
	}
	if !hasSamba {
		t.Error("expected smbcontrol call for samba share")
	}
	if !hasNFS {
		t.Error("expected exportfs call for NFS share")
	}
}
