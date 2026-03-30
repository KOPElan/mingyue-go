package middleware_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"kopelan/mingyue-go/internal/api/middleware"
	"kopelan/mingyue-go/internal/audit"
)

// echoHandler returns an HTTP handler that writes the given status code.
func echoHandler(code int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(code)
	})
}

func TestAuditWithLogger_MutatingMethods_Logged(t *testing.T) {
	mutatingMethods := []string{
		http.MethodPost,
		http.MethodPut,
		http.MethodPatch,
		http.MethodDelete,
	}

	for _, method := range mutatingMethods {
		t.Run(method, func(t *testing.T) {
			var buf bytes.Buffer
			logger := audit.NewWriterLogger(&buf)
			h := middleware.AuditWithLogger(logger)(echoHandler(http.StatusOK))

			req := httptest.NewRequest(method, "/api/v1/test", nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			line := strings.TrimSpace(buf.String())
			if line == "" {
				t.Fatalf("%s: expected an audit log line, got empty", method)
			}

			var event map[string]interface{}
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				t.Fatalf("%s: audit log is not valid JSON: %v — got: %s", method, err, line)
			}

			wantAction := method + " /api/v1/test"
			if event["action"] != wantAction {
				t.Errorf("%s: action = %v, want %q", method, event["action"], wantAction)
			}
			if event["result"] != "success" {
				t.Errorf("%s: result = %v, want success", method, event["result"])
			}
			// HTTPStatus must be present for all mutating requests (200 here).
			if event["http_status"] != float64(http.StatusOK) {
				t.Errorf("%s: http_status = %v, want %d", method, event["http_status"], http.StatusOK)
			}
		})
	}
}

func TestAuditWithLogger_ReadOnlyMethods_NotLogged(t *testing.T) {
	readOnlyMethods := []string{
		http.MethodGet,
		http.MethodHead,
		http.MethodOptions,
	}

	for _, method := range readOnlyMethods {
		t.Run(method, func(t *testing.T) {
			var buf bytes.Buffer
			logger := audit.NewWriterLogger(&buf)
			h := middleware.AuditWithLogger(logger)(echoHandler(http.StatusOK))

			req := httptest.NewRequest(method, "/api/v1/system/overview", nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if buf.Len() > 0 {
				t.Errorf("%s: expected no audit log, got: %s", method, buf.String())
			}
		})
	}
}

func TestAuditWithLogger_FailureStatusCode_LoggedWithErrorCode(t *testing.T) {
	var buf bytes.Buffer
	logger := audit.NewWriterLogger(&buf)
	h := middleware.AuditWithLogger(logger)(echoHandler(http.StatusNotFound))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/shares/gone", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	line := strings.TrimSpace(buf.String())
	var event map[string]interface{}
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("audit log is not valid JSON: %v — got: %s", err, line)
	}

	if event["result"] != "failure" {
		t.Errorf("result = %v, want failure", event["result"])
	}
	if event["error_code"] != "HTTP_404" {
		t.Errorf("error_code = %v, want HTTP_404", event["error_code"])
	}
	if event["http_status"] != float64(http.StatusNotFound) {
		t.Errorf("http_status = %v, want %d", event["http_status"], http.StatusNotFound)
	}
}

func TestAuditWithLogger_ServerError_RecordedAsFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := audit.NewWriterLogger(&buf)
	h := middleware.AuditWithLogger(logger)(echoHandler(http.StatusInternalServerError))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/disks/mounts", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	line := strings.TrimSpace(buf.String())
	var event map[string]interface{}
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("audit log is not valid JSON: %v", err)
	}
	if event["result"] != "failure" {
		t.Errorf("result = %v, want failure", event["result"])
	}
	if event["error_code"] != "HTTP_500" {
		t.Errorf("error_code = %v, want HTTP_500", event["error_code"])
	}
	if event["http_status"] != float64(http.StatusInternalServerError) {
		t.Errorf("http_status = %v, want %d", event["http_status"], http.StatusInternalServerError)
	}
}

func TestAuditWithLogger_RemoteAddrCapturedAsSource(t *testing.T) {
	var buf bytes.Buffer
	logger := audit.NewWriterLogger(&buf)
	h := middleware.AuditWithLogger(logger)(echoHandler(http.StatusOK))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", nil)
	req.RemoteAddr = "192.0.2.1:12345"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	line := strings.TrimSpace(buf.String())
	var event map[string]interface{}
	if err := json.Unmarshal([]byte(line), &event); err != nil {
		t.Fatalf("audit log is not valid JSON: %v", err)
	}
	if event["source"] != "192.0.2.1:12345" {
		t.Errorf("source = %v, want 192.0.2.1:12345", event["source"])
	}
}

func TestAuditWithLogger_SuccessOmitsErrorCode(t *testing.T) {
	var buf bytes.Buffer
	logger := audit.NewWriterLogger(&buf)
	h := middleware.AuditWithLogger(logger)(echoHandler(http.StatusCreated))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/shares", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	line := strings.TrimSpace(buf.String())
	if strings.Contains(line, "error_code") {
		t.Errorf("error_code must be omitted on success, got: %s", line)
	}
}

func BenchmarkAuditWithLogger_Post(b *testing.B) {
	// Use io.Discard to eliminate buffer-growth noise so the benchmark
	// measures the middleware logic (status capture, struct build, JSON
	// serialise) rather than allocator/GC pressure from an ever-growing
	// bytes.Buffer.
	logger := audit.NewWriterLogger(io.Discard)
	h := middleware.AuditWithLogger(logger)(echoHandler(http.StatusOK))

	// Pre-build a single request and recorder; httptest.NewRequest allocations
	// would otherwise dominate the measurement.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/files", nil)
	b.ResetTimer()
	for b.Loop() {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
	}
}
