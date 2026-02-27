package middleware

import (
	"net/http"
	"time"
)

// auditKey is an unexported context key type to avoid collisions.
type auditKey struct{}

// AuditRecord holds the data captured by the audit middleware.
type AuditRecord struct {
	StartTime  time.Time
	Method     string
	Path       string
	RemoteAddr string
	StatusCode int
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Audit is a placeholder audit-logging middleware.
// In Phase 1 it captures the request metadata but does not yet write to the
// audit log (the audit.Logger dependency will be injected in Phase 2).
//
// Usage:
//
//	mux.Handle("/api/v1/...", middleware.Audit(myHandler))
func Audit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// TODO(phase-2): inject audit.Logger, write AuditEvent after handler
		// returns (including the outcome and error code when statusCode >= 400).
		_ = AuditRecord{
			StartTime:  time.Now(),
			Method:     r.Method,
			Path:       r.URL.Path,
			RemoteAddr: r.RemoteAddr,
		}

		next.ServeHTTP(rw, r)
	})
}
