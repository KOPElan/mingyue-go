package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"

	"kopelan/mingyue-go/internal/api"
	"kopelan/mingyue-go/internal/auth"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
	diskService "kopelan/mingyue-go/internal/service/disk"
	fileService "kopelan/mingyue-go/internal/service/file"
	procService "kopelan/mingyue-go/internal/service/process"
	shareService "kopelan/mingyue-go/internal/service/share"
	sysService "kopelan/mingyue-go/internal/service/system"

	"github.com/shirou/gopsutil/v3/mem"
)

// ─── stubs ───────────────────────────────────────────────────────────────────

// stubSysCollector is a test double for sysService.Collector.
type stubSysCollector struct {
	snap  *domain.HostSnapshot
	err   error
}

func (s *stubSysCollector) CPUPercent(_ context.Context) (float64, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.snap.CPUPercent, nil
}

func (s *stubSysCollector) VirtualMemory(_ context.Context) (*mem.VirtualMemoryStat, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &mem.VirtualMemoryStat{
		Total:       s.snap.MemTotal,
		Used:        s.snap.MemUsed,
		UsedPercent: s.snap.MemPercent,
	}, nil
}

func (s *stubSysCollector) Uptime(_ context.Context) (uint64, error) {
	if s.err != nil {
		return 0, s.err
	}
	return s.snap.Uptime, nil
}

// stubProcessLister is a test double for procService.ProcessLister.
type stubProcessLister struct {
	pids    []int32
	procs   map[int32]*domain.Process
	pidsErr error
}

func (s *stubProcessLister) Pids(_ context.Context) ([]int32, error) {
	return s.pids, s.pidsErr
}

func (s *stubProcessLister) Info(_ context.Context, pid int32) (*domain.Process, error) {
	if p, ok := s.procs[pid]; ok {
		return p, nil
	}
	return nil, errors.New("not found")
}

// stubFS is a minimal no-op FS for tests that don't need file operations.
type stubFS struct{}

func (stubFS) ReadDir(_ string) ([]os.DirEntry, error)                      { return nil, nil }
func (stubFS) Stat(path string) (os.FileInfo, error)                         { return nil, os.ErrNotExist }
func (stubFS) MkdirAll(_ string, _ os.FileMode) error                        { return nil }
func (stubFS) Remove(_ string) error                                          { return nil }
func (stubFS) RemoveAll(_ string) error                                       { return nil }
func (stubFS) Rename(_, _ string) error                                       { return nil }
func (stubFS) CopyFile(_, _ string) error                                     { return nil }
func (stubFS) ReadFile(_ string) ([]byte, error)                              { return nil, os.ErrNotExist }
func (stubFS) WriteFile(_ string, _ []byte, _ os.FileMode) error              { return nil }
func (stubFS) EvalSymlinks(path string) (string, error)                       { return path, nil }

// stubShareBackend is a minimal no-op share backend for tests.
type stubShareBackend struct {
	shares map[string]domain.Share
}

func (b *stubShareBackend) List(_ context.Context) ([]domain.Share, error) {
	result := make([]domain.Share, 0)
	for _, s := range b.shares {
		result = append(result, s)
	}
	return result, nil
}

func (b *stubShareBackend) Get(_ context.Context, name string) (*domain.Share, error) {
	if b.shares != nil {
		if s, ok := b.shares[name]; ok {
			cp := s
			return &cp, nil
		}
	}
	return nil, apperrors.New(apperrors.ErrNotFound, "share not found")
}

func (b *stubShareBackend) Create(_ context.Context, s domain.Share) error {
	if b.shares == nil {
		b.shares = make(map[string]domain.Share)
	}
	b.shares[s.Name] = s
	return nil
}

func (b *stubShareBackend) Update(_ context.Context, s domain.Share) error {
	if b.shares == nil || b.shares[s.Name].Name == "" {
		return apperrors.New(apperrors.ErrNotFound, "share not found")
	}
	b.shares[s.Name] = s
	return nil
}

func (b *stubShareBackend) Delete(_ context.Context, name string) error {
	if b.shares != nil {
		delete(b.shares, name)
	}
	return nil
}

func (b *stubShareBackend) Reload(_ context.Context) error { return nil }

// compile-time check that stubShareBackend implements shareService.Backend.
var _ shareService.Backend = (*stubShareBackend)(nil)

// stubSambaUserCommander is a no-op SambaUserCommander for tests.
type stubSambaUserCommander struct{}

func (c *stubSambaUserCommander) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

func (c *stubSambaUserCommander) RunWithInput(_ context.Context, _ string, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

// stubMountsReader is a test double for diskService.MountsReader.
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

// stubSmartCommander is a test double for diskService.Commander (SMART only).
type stubSmartCommander struct {
	output []byte
	err    error
}

func (c *stubSmartCommander) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return c.output, c.err
}
// ─── helpers ─────────────────────────────────────────────────────────────────

func testSnap() *domain.HostSnapshot {
	return &domain.HostSnapshot{
		CPUPercent: 42.0,
		MemTotal:   8 * 1024 * 1024 * 1024,
		MemUsed:    4 * 1024 * 1024 * 1024,
		MemPercent: 50.0,
		Uptime:     3600,
	}
}

func testProcs() ([]int32, map[int32]*domain.Process) {
	pids := []int32{1, 2, 3}
	procs := map[int32]*domain.Process{
		1: {PID: 1, Name: "init"},
		2: {PID: 2, Name: "kthreadd"},
		3: {PID: 3, Name: "bash"},
	}
	return pids, procs
}

// buildRouter creates a test router with stub dependencies.
func buildRouter(collector sysService.Collector, lister procService.ProcessLister) http.Handler {
	monitor := sysService.NewMonitorWithCollector(collector)
	procMgr := procService.NewManagerWithLister(lister, nil)
	mountSvc := diskService.NewMountServiceWithDeps(&stubMountsReader{}, &stubCommanderNoErr{}, nil)
	smartSvc := diskService.NewSmartServiceWithCommander(&stubSmartCommander{})
	powerSvc := diskService.NewPowerServiceWithCommander(&stubCommanderNoErr{}, nil)
	devSvc := diskService.NewDeviceServiceWithCommander(&stubDeviceCommander{})
	fileMgr := fileService.NewManagerWithFS("/", nil, &stubFS{})
	shareMgr := shareService.NewManagerWithBackend(&stubShareBackend{}, nil)
	sambaUserMgr := shareService.NewSambaUserManagerWithCommander(&stubSambaUserCommander{})
	return api.NewRouterWithDeps(monitor, procMgr, mountSvc, smartSvc, powerSvc, devSvc, fileMgr, shareMgr, sambaUserMgr)
}

// addViewerToken registers a viewer-role API key and returns an
// Authorization header value.
func addViewerToken() string {
	const key = "contract-test-viewer"
	auth.RegisterAPIKey(key, auth.Token{Raw: key, Role: auth.RoleViewer, Subject: "test-viewer"})
	return "Bearer " + key
}

// addOperatorToken registers an operator-role API key.
func addOperatorToken() string {
	const key = "contract-test-operator"
	auth.RegisterAPIKey(key, auth.Token{Raw: key, Role: auth.RoleOperator, Subject: "test-operator"})
	return "Bearer " + key
}

// ─── /api/v1/system/overview ─────────────────────────────────────────────────

func TestSystemOverview_Success(t *testing.T) {
	snap := testSnap()
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: snap}, &stubProcessLister{pids: pids, procs: procs})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/overview", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var got domain.HostSnapshot
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.CPUPercent != snap.CPUPercent {
		t.Errorf("CPUPercent: got %v, want %v", got.CPUPercent, snap.CPUPercent)
	}
	if got.MemTotal != snap.MemTotal {
		t.Errorf("MemTotal: got %v, want %v", got.MemTotal, snap.MemTotal)
	}
}

func TestSystemOverview_Unauthenticated(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/overview", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
	checkAppError(t, w, apperrors.ErrUnauthorized)
}

func TestSystemOverview_CollectorError(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(
		&stubSysCollector{err: errors.New("kernel panic")},
		&stubProcessLister{pids: pids, procs: procs},
	)
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/system/overview", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusInternalServerError, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrInternal)
}

// ─── /api/v1/processes ───────────────────────────────────────────────────────

func TestProcessList_Success(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/processes", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var resp struct {
		Total     int               `json:"total"`
		Processes []*domain.Process `json:"processes"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("Total: got %d, want 3", resp.Total)
	}
	if len(resp.Processes) != 3 {
		t.Errorf("len(Processes): got %d, want 3", len(resp.Processes))
	}
}

func TestProcessList_Pagination(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/processes?limit=2&page=1", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Total     int               `json:"total"`
		Processes []*domain.Process `json:"processes"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Total != 3 {
		t.Errorf("Total: got %d, want 3", resp.Total)
	}
	if len(resp.Processes) != 2 {
		t.Errorf("len(Processes): got %d, want 2", len(resp.Processes))
	}
}

func TestProcessList_Unauthenticated(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/processes", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestProcessGet_Success(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/processes/1", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var got domain.Process
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PID != 1 {
		t.Errorf("PID: got %d, want 1", got.PID)
	}
}

func TestProcessGet_NotFound(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/processes/9999", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrNotFound)
}

func TestProcessKill_Success_OperatorRole(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addOperatorToken()

	// Start a child process that we can safely terminate.
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Skipf("could not start child process: %v", err)
	}
	t.Cleanup(func() {
		// Ensure the child is reaped even if the test fails before the kill.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	childPID := cmd.Process.Pid
	req := httptest.NewRequest(http.MethodDelete,
		fmt.Sprintf("/api/v1/processes/%d", childPID), nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestProcessKill_Forbidden_ViewerRole(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/processes/1", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrForbidden)
}

func TestProcessKill_Unauthenticated(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/processes/1", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ─── helpers ─────────────────────────────────────────────────────────────────

// checkAppError verifies that the response body contains an AppError with the
// expected code.
func checkAppError(t *testing.T, w *httptest.ResponseRecorder, wantCode apperrors.ErrorCode) {
	t.Helper()
	var ae apperrors.AppError
	if err := json.NewDecoder(w.Body).Decode(&ae); err != nil {
		t.Fatalf("decode AppError: %v (body: %s)", err, w.Body.String())
	}
	if ae.Code != wantCode {
		t.Errorf("AppError.Code: got %q, want %q", ae.Code, wantCode)
	}
}


// buildDiskRouter creates a test router with disk stubs, using the provided
// mounts reader and smart commander.
func buildDiskRouter(reader diskService.MountsReader, smartCmd diskService.Commander) http.Handler {
	pids, procs := testProcs()
	monitor := sysService.NewMonitorWithCollector(&stubSysCollector{snap: testSnap()})
	procMgr := procService.NewManagerWithLister(&stubProcessLister{pids: pids, procs: procs}, nil)
	mountSvc := diskService.NewMountServiceWithDeps(reader, &stubCommanderNoErr{}, nil)
	smartSvc := diskService.NewSmartServiceWithCommander(smartCmd)
	powerSvc := diskService.NewPowerServiceWithCommander(&stubPowerCommander{}, nil)
	devSvc := diskService.NewDeviceServiceWithCommander(&stubDeviceCommander{})
	fileMgr := fileService.NewManagerWithFS("/", nil, &stubFS{})
	shareMgr := shareService.NewManagerWithBackend(&stubShareBackend{}, nil)
	sambaUserMgr := shareService.NewSambaUserManagerWithCommander(&stubSambaUserCommander{})
	return api.NewRouterWithDeps(monitor, procMgr, mountSvc, smartSvc, powerSvc, devSvc, fileMgr, shareMgr, sambaUserMgr)
}

// stubCommanderNoErr is a Commander stub that always succeeds.
type stubCommanderNoErr struct{}

func (c *stubCommanderNoErr) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, nil
}

// stubDeviceCommander returns an empty lsblk JSON response.
type stubDeviceCommander struct{}

func (c *stubDeviceCommander) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return []byte(`{"blockdevices":[]}`), nil
}

// stubPowerCommander returns a valid hdparm -C active/idle response.
type stubPowerCommander struct{}

func (c *stubPowerCommander) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return []byte("/dev/sda:\n drive state is:  active/idle\n"), nil
}

// ─── /api/v1/files ───────────────────────────────────────────────────────────

func TestFileList_Unauthenticated(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files?path=/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestFileList_MissingPath(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrInvalidInput)
}

func TestFileWrite_Unauthenticated(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", strings.NewReader(`{"path":"/x","content":"y"}`))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ─── /api/v1/shares ──────────────────────────────────────────────────────────

func TestShareList_Success(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/shares", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["shares"]; !ok {
		t.Error("response missing 'shares' key")
	}
}

func TestShareList_Unauthenticated(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/shares", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestShareCreate_Forbidden_ViewerRole(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addViewerToken()

	body := `{"name":"test","type":"smb","path":"/srv/test"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/shares", strings.NewReader(body))
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrForbidden)
}

func TestShareCreate_Success_OperatorRole(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addOperatorToken()

	body := `{"name":"myshare","type":"smb","path":"/srv/myshare"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/shares", strings.NewReader(body))
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
}

func TestShareGet_NotFound(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/shares/nonexistent", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrNotFound)
}

func TestShareDelete_Forbidden_ViewerRole(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/shares/some", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrForbidden)
}

// ─── /api/v1/disks/mounts (GET) ──────────────────────────────────────────────

const sampleProcMounts = `/dev/sda1 / ext4 rw,relatime 0 0
/dev/sdb1 /mnt/data ext4 rw,relatime 0 0
`

func TestDiskMountList_Success(t *testing.T) {
	reader := &stubMountsReader{content: sampleProcMounts}
	handler := buildDiskRouter(reader, &stubSmartCommander{})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks/mounts", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Mounts []domain.Mount `json:"mounts"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Mounts) != 2 {
		t.Errorf("len(mounts): got %d, want 2", len(resp.Mounts))
	}
}

func TestDiskMountList_Unauthenticated(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	handler := buildDiskRouter(reader, &stubSmartCommander{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks/mounts", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ─── /api/v1/disks/mounts (POST) ─────────────────────────────────────────────

func TestDiskMount_Success_OperatorRole(t *testing.T) {
	reader := &stubMountsReader{content: ""} // no existing mounts
	handler := buildDiskRouter(reader, &stubSmartCommander{})
	token := addOperatorToken()

	body := strings.NewReader(`{"source":"/dev/sdb1","mount_point":"/mnt/test","fs_type":"ext4"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/disks/mounts", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusCreated, w.Body.String())
	}
}

func TestDiskMount_AlreadyMounted_Conflict(t *testing.T) {
	reader := &stubMountsReader{content: sampleProcMounts} // /mnt/data already mounted
	handler := buildDiskRouter(reader, &stubSmartCommander{})
	token := addOperatorToken()

	body := strings.NewReader(`{"source":"/dev/sdb1","mount_point":"/mnt/data","fs_type":"ext4"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/disks/mounts", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusConflict, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrConflict)
}

func TestDiskMount_Forbidden_ViewerRole(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	handler := buildDiskRouter(reader, &stubSmartCommander{})
	token := addViewerToken()

	body := strings.NewReader(`{"source":"/dev/sdb1","mount_point":"/mnt/test","fs_type":"ext4"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/disks/mounts", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusForbidden)
	}
	checkAppError(t, w, apperrors.ErrForbidden)
}

// ─── /api/v1/disks/mounts/{mountpoint} (DELETE) ──────────────────────────────

func TestDiskUmount_Success_OperatorRole(t *testing.T) {
	reader := &stubMountsReader{content: sampleProcMounts}
	handler := buildDiskRouter(reader, &stubSmartCommander{})
	token := addOperatorToken()

	// URL-encode the mountpoint: /mnt/data → %2Fmnt%2Fdata
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/disks/mounts/%2Fmnt%2Fdata", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestDiskUmount_NotMounted_NotFound(t *testing.T) {
	reader := &stubMountsReader{content: sampleProcMounts}
	handler := buildDiskRouter(reader, &stubSmartCommander{})
	token := addOperatorToken()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/disks/mounts/%2Fmnt%2Fnonexistent", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrNotFound)
}

func TestDiskUmount_Forbidden_ViewerRole(t *testing.T) {
	reader := &stubMountsReader{content: sampleProcMounts}
	handler := buildDiskRouter(reader, &stubSmartCommander{})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/disks/mounts/%2Fmnt%2Fdata", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusForbidden)
	}
}

// ─── /api/v1/disks/{device}/smart (GET) ──────────────────────────────────────

const sampleSmartJSON = `{
  "model_name": "Samsung SSD",
  "serial_number": "ABC123",
  "smart_status": {"passed": true},
  "temperature": {"current": 30},
  "power_on_time": {"hours": 1000}
}`

func TestDiskSmart_Success(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	smartCmd := &stubSmartCommander{output: []byte(sampleSmartJSON)}
	handler := buildDiskRouter(reader, smartCmd)
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks/sda/smart", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var health domain.DiskHealth
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !health.HealthOK {
		t.Error("HealthOK: expected true")
	}
	if health.Model != "Samsung SSD" {
		t.Errorf("Model: got %q", health.Model)
	}
}

func TestDiskSmart_NotFound_BinaryMissing(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	smartCmd := &stubSmartCommander{
		err: &exec.Error{Name: "smartctl", Err: exec.ErrNotFound},
	}
	handler := buildDiskRouter(reader, smartCmd)
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks/sda/smart", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusNotFound, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrNotFound)
}

func TestDiskSmart_Unauthenticated(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	handler := buildDiskRouter(reader, &stubSmartCommander{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks/sda/smart", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ─── /api/v1/disks/devices (GET) ─────────────────────────────────────────────

func TestDiskDevicesList_Success(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	handler := buildDiskRouter(reader, &stubSmartCommander{})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks/devices", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var resp struct {
		Devices []domain.BlockDevice `json:"devices"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// stubDeviceCommander returns an empty lsblk JSON response.
	if len(resp.Devices) != 0 {
		t.Errorf("expected empty devices slice, got %d", len(resp.Devices))
	}
}

func TestDiskDevicesList_Unauthenticated(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	handler := buildDiskRouter(reader, &stubSmartCommander{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks/devices", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ─── /api/v1/disks/{device}/power (GET) ──────────────────────────────────────

func TestDiskPowerGet_Success(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	handler := buildDiskRouter(reader, &stubSmartCommander{})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks/sda/power", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}
	var power domain.DiskPower
	if err := json.NewDecoder(w.Body).Decode(&power); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if power.PowerMode != "active" {
		t.Errorf("PowerMode: got %q, want active", power.PowerMode)
	}
}

func TestDiskPowerGet_Unauthenticated(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	handler := buildDiskRouter(reader, &stubSmartCommander{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/disks/sda/power", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ─── /api/v1/disks/{device}/power (POST) ─────────────────────────────────────

func TestDiskPowerSet_Success_OperatorRole(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	handler := buildDiskRouter(reader, &stubSmartCommander{})
	token := addOperatorToken()

	body := strings.NewReader(`{"action":"standby"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/disks/sda/power", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestDiskPowerSet_Forbidden_ViewerRole(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	handler := buildDiskRouter(reader, &stubSmartCommander{})
	token := addViewerToken()

	body := strings.NewReader(`{"action":"standby"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/disks/sda/power", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusForbidden)
	}
	checkAppError(t, w, apperrors.ErrForbidden)
}

func TestDiskPowerSet_InvalidAction_BadRequest(t *testing.T) {
	reader := &stubMountsReader{content: ""}
	handler := buildDiskRouter(reader, &stubSmartCommander{})
	token := addOperatorToken()

	body := strings.NewReader(`{"action":"wakeup"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/disks/sda/power", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrInvalidInput)
}

// ─── file write role enforcement ─────────────────────────────────────────────

func TestFileWrite_Forbidden_ViewerRole(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addViewerToken()

	body := `{"path":"/x","type":"file","content":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", strings.NewReader(body))
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrForbidden)
}

func TestFileDelete_Forbidden_ViewerRole(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/files?path=/x", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrForbidden)
}

func TestFileMove_Forbidden_ViewerRole(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addViewerToken()

	body := `{"src":"/a","dst":"/b"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/files/move", strings.NewReader(body))
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrForbidden)
}

func TestFileWrite_InvalidType(t *testing.T) {
	pids, procs := testProcs()
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{pids: pids, procs: procs})
	token := addOperatorToken()

	body := `{"path":"/x","type":"ftp","content":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", strings.NewReader(body))
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrInvalidInput)
}
