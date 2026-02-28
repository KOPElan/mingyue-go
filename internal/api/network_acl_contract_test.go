package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"kopelan/mingyue-go/internal/api"
	"kopelan/mingyue-go/internal/auth"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
	aclService "kopelan/mingyue-go/internal/service/acl"
	netService "kopelan/mingyue-go/internal/service/network"
)

// ─── network stubs ────────────────────────────────────────────────────────────

type stubNetReader struct {
	ifaces    []domain.NetworkInterface
	ifacesErr error
	routes    []domain.Route
	routesErr error
}

func (s *stubNetReader) Interfaces() ([]domain.NetworkInterface, error) {
	return s.ifaces, s.ifacesErr
}

func (s *stubNetReader) Routes(_ context.Context) ([]domain.Route, error) {
	return s.routes, s.routesErr
}

type stubNetCommander struct {
	err error
}

func (s *stubNetCommander) SetInterfaceState(_ context.Context, _ string, _ bool) error {
	return s.err
}

// ─── ACL stubs ────────────────────────────────────────────────────────────────

type stubACLReader struct {
	acl *domain.FileACL
	err error
}

func (s *stubACLReader) GetACL(_ context.Context, _ string) (*domain.FileACL, error) {
	return s.acl, s.err
}

type stubACLWriter struct {
	err error
}

func (s *stubACLWriter) SetMode(_ context.Context, _ string, _ os.FileMode) error {
	return s.err
}

func (s *stubACLWriter) SetOwner(_ context.Context, _, _, _ string) error {
	return s.err
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func buildNetRouter(nr netService.Reader, nc netService.Commander, ar aclService.Reader, aw aclService.Writer) http.Handler {
	netMgr := netService.NewManagerWithDeps(nr, nc, nil)
	aclMgr := aclService.NewManagerWithDeps(ar, aw, nil)
	return api.NewRouterWithDeps(nil, nil, nil, nil, nil, nil, nil, nil, nil, netMgr, aclMgr)
}

func addAdminToken() string {
	const key = "contract-test-admin"
	auth.RegisterAPIKey(key, auth.Token{Raw: key, Role: auth.RoleAdmin, Subject: "test-admin"})
	return "Bearer " + key
}

// ─── GET /api/v1/network/interfaces ──────────────────────────────────────────

func TestNetworkInterfaces_Success(t *testing.T) {
	ifaces := []domain.NetworkInterface{
		{Name: "eth0", IsUp: true, HardwareAddr: "aa:bb:cc:dd:ee:ff"},
		{Name: "lo", IsUp: true},
	}
	handler := buildNetRouter(
		&stubNetReader{ifaces: ifaces},
		&stubNetCommander{},
		&stubACLReader{},
		&stubACLWriter{},
	)
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/network/interfaces", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var got []domain.NetworkInterface
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(interfaces): got %d, want 2", len(got))
	}
	if got[0].Name != "eth0" {
		t.Errorf("interfaces[0].Name: got %q, want %q", got[0].Name, "eth0")
	}
}

func TestNetworkInterfaces_Unauthenticated(t *testing.T) {
	handler := buildNetRouter(
		&stubNetReader{ifaces: nil},
		&stubNetCommander{},
		&stubACLReader{},
		&stubACLWriter{},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/network/interfaces", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ─── GET /api/v1/network/routes ───────────────────────────────────────────────

func TestNetworkRoutes_Success(t *testing.T) {
	routes := []domain.Route{
		{Destination: "default", Gateway: "192.168.1.1", Interface: "eth0"},
	}
	handler := buildNetRouter(
		&stubNetReader{routes: routes},
		&stubNetCommander{},
		&stubACLReader{},
		&stubACLWriter{},
	)
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/network/routes", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var got []domain.Route
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("len(routes): got %d, want 1", len(got))
	}
}

// ─── PUT /api/v1/network/interfaces/:name ─────────────────────────────────────

func TestNetworkSetInterfaceState_AdminRequired(t *testing.T) {
	handler := buildNetRouter(
		&stubNetReader{},
		&stubNetCommander{},
		&stubACLReader{},
		&stubACLWriter{},
	)
	token := addViewerToken() // viewer, not admin

	body := bytes.NewBufferString(`{"state":"up"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/network/interfaces/eth0", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrForbidden)
}

func TestNetworkSetInterfaceState_Success(t *testing.T) {
	handler := buildNetRouter(
		&stubNetReader{},
		&stubNetCommander{},
		&stubACLReader{},
		&stubACLWriter{},
	)
	token := addAdminToken()

	body := bytes.NewBufferString(`{"state":"up"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/network/interfaces/eth0", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestNetworkSetInterfaceState_InvalidState(t *testing.T) {
	handler := buildNetRouter(
		&stubNetReader{},
		&stubNetCommander{},
		&stubACLReader{},
		&stubACLWriter{},
	)
	token := addAdminToken()

	body := bytes.NewBufferString(`{"state":"sideways"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/network/interfaces/eth0", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrInvalidInput)
}

// ─── GET /api/v1/acl ─────────────────────────────────────────────────────────

func TestACLGet_Success(t *testing.T) {
	want := &domain.FileACL{Path: "/tmp/test", Mode: "0644", Owner: "root", Group: "root"}
	handler := buildNetRouter(
		&stubNetReader{},
		&stubNetCommander{},
		&stubACLReader{acl: want},
		&stubACLWriter{},
	)
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/acl?path=/tmp/test", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var got domain.FileACL
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Path != want.Path {
		t.Errorf("Path: got %q, want %q", got.Path, want.Path)
	}
	if got.Mode != want.Mode {
		t.Errorf("Mode: got %q, want %q", got.Mode, want.Mode)
	}
}

func TestACLGet_MissingPath(t *testing.T) {
	handler := buildNetRouter(
		&stubNetReader{},
		&stubNetCommander{},
		&stubACLReader{},
		&stubACLWriter{},
	)
	token := addViewerToken()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/acl", nil)
	req.Header.Set("Authorization", token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrInvalidInput)
}

func TestACLGet_Unauthenticated(t *testing.T) {
	handler := buildNetRouter(
		&stubNetReader{},
		&stubNetCommander{},
		&stubACLReader{},
		&stubACLWriter{},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/acl?path=/tmp/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ─── PUT /api/v1/acl ─────────────────────────────────────────────────────────

func TestACLSet_AdminRequired(t *testing.T) {
	handler := buildNetRouter(
		&stubNetReader{},
		&stubNetCommander{},
		&stubACLReader{},
		&stubACLWriter{},
	)
	token := addViewerToken() // viewer, not admin

	body := bytes.NewBufferString(`{"mode":"0644"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/acl?path=/tmp/test", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrForbidden)
}

func TestACLSet_Success(t *testing.T) {
	handler := buildNetRouter(
		&stubNetReader{},
		&stubNetCommander{},
		&stubACLReader{},
		&stubACLWriter{},
	)
	token := addAdminToken()

	body := bytes.NewBufferString(`{"mode":"0644","owner":"root","group":"root"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/acl?path=/tmp/test", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusNoContent, w.Body.String())
	}
}

func TestACLSet_InvalidMode(t *testing.T) {
	handler := buildNetRouter(
		&stubNetReader{},
		&stubNetCommander{},
		&stubACLReader{},
		&stubACLWriter{},
	)
	token := addAdminToken()

	body := bytes.NewBufferString(`{"mode":"not-octal"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/acl?path=/tmp/test", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusBadRequest, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrInvalidInput)
}

func TestACLSet_TraversalPath(t *testing.T) {
	handler := buildNetRouter(
		&stubNetReader{},
		&stubNetCommander{},
		&stubACLReader{},
		&stubACLWriter{},
	)
	token := addAdminToken()

	body := bytes.NewBufferString(`{"mode":"0644"}`)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/acl?path=../etc/passwd", body)
	req.Header.Set("Authorization", token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d; body: %s", w.Code, http.StatusForbidden, w.Body.String())
	}
	checkAppError(t, w, apperrors.ErrForbidden)
}
