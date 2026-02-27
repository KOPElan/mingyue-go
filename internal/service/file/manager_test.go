package file

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	apperrors "kopelan/mingyue-go/internal/errors"
)

// ── stub FS ───────────────────────────────────────────────────────────────────

// memFS is an in-memory FS stub for tests.
type memFS struct {
	files map[string][]byte
	dirs  map[string]bool
}

func newMemFS() *memFS {
	return &memFS{
		files: make(map[string][]byte),
		dirs:  map[string]bool{"/testroot": true},
	}
}

func (f *memFS) ReadDir(path string) ([]os.DirEntry, error) {
	if !f.dirs[path] {
		return nil, &os.PathError{Op: "readdir", Path: path, Err: os.ErrNotExist}
	}
	var entries []os.DirEntry
	for p := range f.files {
		if filepath.Dir(p) == path {
			entries = append(entries, &memDirEntry{name: filepath.Base(p), isDir: false})
		}
	}
	for d := range f.dirs {
		if d != path && filepath.Dir(d) == path {
			entries = append(entries, &memDirEntry{name: filepath.Base(d), isDir: true})
		}
	}
	return entries, nil
}

func (f *memFS) Stat(path string) (os.FileInfo, error) {
	if f.dirs[path] {
		return &memFileInfo{name: filepath.Base(path), isDir: true}, nil
	}
	if data, ok := f.files[path]; ok {
		return &memFileInfo{name: filepath.Base(path), size: int64(len(data))}, nil
	}
	return nil, &os.PathError{Op: "stat", Path: path, Err: os.ErrNotExist}
}

func (f *memFS) MkdirAll(path string, _ os.FileMode) error {
	f.dirs[path] = true
	return nil
}

func (f *memFS) Remove(path string) error {
	if f.dirs[path] {
		delete(f.dirs, path)
		return nil
	}
	if _, ok := f.files[path]; ok {
		delete(f.files, path)
		return nil
	}
	return &os.PathError{Op: "remove", Path: path, Err: os.ErrNotExist}
}

func (f *memFS) RemoveAll(path string) error {
	delete(f.dirs, path)
	delete(f.files, path)
	return nil
}

func (f *memFS) Rename(src, dst string) error {
	if data, ok := f.files[src]; ok {
		f.files[dst] = data
		delete(f.files, src)
		return nil
	}
	return &os.PathError{Op: "rename", Path: src, Err: os.ErrNotExist}
}

func (f *memFS) CopyFile(src, dst string) error {
	data, ok := f.files[src]
	if !ok {
		return &os.PathError{Op: "copy", Path: src, Err: os.ErrNotExist}
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	f.files[dst] = cp
	return nil
}

func (f *memFS) ReadFile(path string) ([]byte, error) {
	data, ok := f.files[path]
	if !ok {
		return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
	}
	return data, nil
}

func (f *memFS) WriteFile(path string, data []byte, _ os.FileMode) error {
	f.files[path] = data
	return nil
}

// EvalSymlinks always returns the path unchanged in the in-memory FS
// because there are no symlinks.
func (f *memFS) EvalSymlinks(path string) (string, error) {
	// Return not-exist for paths that don't exist so the symlink-check
	// loop in safePath can walk up to an ancestor that does exist.
	if f.dirs[path] || f.files[path] != nil {
		return path, nil
	}
	return "", &os.PathError{Op: "evalSymlinks", Path: path, Err: os.ErrNotExist}
}

// memDirEntry satisfies os.DirEntry.
type memDirEntry struct {
	name  string
	isDir bool
}

func (e *memDirEntry) Name() string               { return e.name }
func (e *memDirEntry) IsDir() bool                { return e.isDir }
func (e *memDirEntry) Type() os.FileMode          { return 0 }
func (e *memDirEntry) Info() (os.FileInfo, error) { return &memFileInfo{name: e.name, isDir: e.isDir}, nil }

// memFileInfo satisfies os.FileInfo.
type memFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (i *memFileInfo) Name() string      { return i.name }
func (i *memFileInfo) Size() int64       { return i.size }
func (i *memFileInfo) Mode() os.FileMode { return 0o644 }
func (i *memFileInfo) ModTime() time.Time { return time.Time{} }
func (i *memFileInfo) IsDir() bool       { return i.isDir }
func (i *memFileInfo) Sys() interface{}  { return nil }

// ── helpers ───────────────────────────────────────────────────────────────────

const testRoot = "/testroot"

func newTestManager() (*Manager, *memFS) {
	fs := newMemFS()
	return NewManagerWithFS(testRoot, nil, fs), fs
}

func assertForbidden(t *testing.T, err error) {
	t.Helper()
	var ae *apperrors.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AppError, got %T: %v", err, err)
	}
	if ae.Code != apperrors.ErrForbidden {
		t.Errorf("code: got %q, want %q", ae.Code, apperrors.ErrForbidden)
	}
}

// ── Path traversal tests ──────────────────────────────────────────────────────

func TestSafePath_TraversalBlocked(t *testing.T) {
	mgr, _ := newTestManager()

	traversalPaths := []string{
		"../etc/passwd",
		"../../etc/shadow",
		"/etc/passwd",       // absolute path outside root
		"/testroot/../etc",  // normalises to /etc
	}

	for _, p := range traversalPaths {
		t.Run(p, func(t *testing.T) {
			_, err := mgr.safePath(p)
			if err == nil {
				t.Fatalf("expected error for path %q, got nil", p)
			}
			assertForbidden(t, err)
		})
	}
}

func TestSafePath_ValidPathsAllowed(t *testing.T) {
	mgr, _ := newTestManager()

	validPaths := []string{
		testRoot,
		testRoot + "/subdir",
		"subdir",
		"subdir/file.txt",
	}

	for _, p := range validPaths {
		t.Run(p, func(t *testing.T) {
			abs, err := mgr.safePath(p)
			if err != nil {
				t.Fatalf("unexpected error for path %q: %v", p, err)
			}
			if abs == "" {
				t.Error("expected non-empty abs path")
			}
		})
	}
}

// ── List tests ────────────────────────────────────────────────────────────────

func TestManager_List(t *testing.T) {
	mgr, fs := newTestManager()
	fs.files[testRoot+"/a.txt"] = []byte("hello")
	fs.files[testRoot+"/b.txt"] = []byte("world")

	entries, err := mgr.List(context.Background(), testRoot)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("len: got %d, want 2", len(entries))
	}
}

func TestManager_List_NotFound(t *testing.T) {
	mgr, _ := newTestManager()

	_, err := mgr.List(context.Background(), testRoot+"/nonexistent")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) || ae.Code != apperrors.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestManager_List_Traversal(t *testing.T) {
	mgr, _ := newTestManager()

	_, err := mgr.List(context.Background(), "../etc")
	assertForbidden(t, err)
}

// ── Stat tests ────────────────────────────────────────────────────────────────

func TestManager_Stat(t *testing.T) {
	mgr, fs := newTestManager()
	fs.files[testRoot+"/hello.txt"] = []byte("hello")

	fe, err := mgr.Stat(context.Background(), testRoot+"/hello.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fe.Name != "hello.txt" {
		t.Errorf("Name: got %q, want %q", fe.Name, "hello.txt")
	}
	if fe.Size != 5 {
		t.Errorf("Size: got %d, want 5", fe.Size)
	}
}

func TestManager_Stat_Traversal(t *testing.T) {
	mgr, _ := newTestManager()
	_, err := mgr.Stat(context.Background(), "../etc/passwd")
	assertForbidden(t, err)
}

// ── Mkdir tests ───────────────────────────────────────────────────────────────

func TestManager_Mkdir(t *testing.T) {
	mgr, fs := newTestManager()

	if err := mgr.Mkdir(context.Background(), testRoot+"/newdir", "test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fs.dirs[testRoot+"/newdir"] {
		t.Error("directory was not created")
	}
}

func TestManager_Mkdir_Traversal(t *testing.T) {
	mgr, _ := newTestManager()
	err := mgr.Mkdir(context.Background(), "../evil", "test")
	assertForbidden(t, err)
}

// ── Remove tests ──────────────────────────────────────────────────────────────

func TestManager_Remove(t *testing.T) {
	mgr, fs := newTestManager()
	fs.files[testRoot+"/todelete.txt"] = []byte("bye")

	if err := mgr.Remove(context.Background(), testRoot+"/todelete.txt", false, "test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := fs.files[testRoot+"/todelete.txt"]; ok {
		t.Error("file was not deleted")
	}
}

func TestManager_Remove_Traversal(t *testing.T) {
	mgr, _ := newTestManager()
	err := mgr.Remove(context.Background(), "../../etc/passwd", false, "test")
	assertForbidden(t, err)
}

// ── Move tests ────────────────────────────────────────────────────────────────

func TestManager_Move(t *testing.T) {
	mgr, fs := newTestManager()
	fs.files[testRoot+"/src.txt"] = []byte("data")

	if err := mgr.Move(context.Background(), testRoot+"/src.txt", testRoot+"/dst.txt", "test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := fs.files[testRoot+"/dst.txt"]; !ok {
		t.Error("dst file not created")
	}
	if _, ok := fs.files[testRoot+"/src.txt"]; ok {
		t.Error("src file still exists")
	}
}

func TestManager_Move_SrcTraversal(t *testing.T) {
	mgr, _ := newTestManager()
	err := mgr.Move(context.Background(), "../etc/passwd", testRoot+"/dst.txt", "test")
	assertForbidden(t, err)
}

func TestManager_Move_DstTraversal(t *testing.T) {
	mgr, fs := newTestManager()
	fs.files[testRoot+"/src.txt"] = []byte("data")
	err := mgr.Move(context.Background(), testRoot+"/src.txt", "../evil.txt", "test")
	assertForbidden(t, err)
}

// ── Copy tests ────────────────────────────────────────────────────────────────

func TestManager_Copy(t *testing.T) {
	mgr, fs := newTestManager()
	fs.files[testRoot+"/orig.txt"] = []byte("content")

	if err := mgr.Copy(context.Background(), testRoot+"/orig.txt", testRoot+"/copy.txt", "test"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := fs.files[testRoot+"/copy.txt"]; !ok {
		t.Error("copy not created")
	}
	if _, ok := fs.files[testRoot+"/orig.txt"]; !ok {
		t.Error("original was deleted")
	}
}

func TestManager_Copy_DstTraversal(t *testing.T) {
	mgr, fs := newTestManager()
	fs.files[testRoot+"/orig.txt"] = []byte("x")
	err := mgr.Copy(context.Background(), testRoot+"/orig.txt", "../../evil.txt", "test")
	assertForbidden(t, err)
}

// ── Read/Write tests ──────────────────────────────────────────────────────────

func TestManager_ReadWrite(t *testing.T) {
	mgr, _ := newTestManager()

	if err := mgr.Write(context.Background(), testRoot+"/rw.txt", []byte("hello"), "test"); err != nil {
		t.Fatalf("write error: %v", err)
	}

	data, err := mgr.Read(context.Background(), testRoot+"/rw.txt")
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("data: got %q, want %q", data, "hello")
	}
}

func TestManager_Write_Traversal(t *testing.T) {
	mgr, _ := newTestManager()
	err := mgr.Write(context.Background(), "../evil.txt", []byte("x"), "test")
	assertForbidden(t, err)
}

func TestManager_Read_Traversal(t *testing.T) {
	mgr, _ := newTestManager()
	_, err := mgr.Read(context.Background(), "../etc/passwd")
	assertForbidden(t, err)
}
