package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSwaggerUI_OK verifies that GET /swagger/ returns an HTML page with the
// Swagger UI scaffold (no authentication required).
func TestSwaggerUI_OK(t *testing.T) {
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{})

	req := httptest.NewRequest(http.MethodGet, "/swagger/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type: got %q, want prefix \"text/html\"", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "swagger-ui") {
		t.Errorf("expected swagger-ui in response body")
	}
	if !strings.Contains(body, "/swagger/openapi.yaml") {
		t.Errorf("expected /swagger/openapi.yaml reference in response body")
	}
}

// TestSwaggerUI_MethodNotAllowed verifies that non-GET requests to /swagger/ are rejected.
func TestSwaggerUI_MethodNotAllowed(t *testing.T) {
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{})

	req := httptest.NewRequest(http.MethodPost, "/swagger/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

// TestSwaggerSpec_OK verifies that GET /swagger/openapi.yaml returns the YAML spec
// with correct content type and non-empty body (no authentication required).
func TestSwaggerSpec_OK(t *testing.T) {
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{})

	req := httptest.NewRequest(http.MethodGet, "/swagger/openapi.yaml", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusOK)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/yaml" {
		t.Errorf("Content-Type: got %q, want \"application/yaml\"", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "openapi:") {
		t.Errorf("expected openapi spec content in response body")
	}
}

// TestSwaggerSpec_MethodNotAllowed verifies that non-GET requests to /swagger/openapi.yaml are rejected.
func TestSwaggerSpec_MethodNotAllowed(t *testing.T) {
	handler := buildRouter(&stubSysCollector{snap: testSnap()}, &stubProcessLister{})

	req := httptest.NewRequest(http.MethodDelete, "/swagger/openapi.yaml", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}
