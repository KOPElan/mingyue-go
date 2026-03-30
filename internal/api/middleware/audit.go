package middleware

import (
	"fmt"
	"net/http"
	"time"

	"kopelan/mingyue-go/internal/audit"
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

// AuditWithLogger returns a middleware that records HTTP-level audit events for
// mutating requests (POST, PUT, PATCH, DELETE) using the provided audit.Logger.
// Read-only methods (GET, HEAD, OPTIONS) are intentionally excluded to reduce
// log noise, in accordance with ADR-006.
//
// The audit event captures: caller IP (Source), HTTP method+path (Action),
// request path (Target), and outcome (Result / ErrorCode).
//
// Usage:
//
//	handler = middleware.AuditWithLogger(auditLogger)(handler)
func AuditWithLogger(logger audit.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only audit mutating operations; skip read-only methods.
			if !isMutatingMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
			start := time.Now().UTC()

			next.ServeHTTP(rw, r)

			result := "success"
			errCode := ""
			if rw.statusCode >= http.StatusBadRequest {
				result = "failure"
				errCode = fmt.Sprintf("HTTP_%d", rw.statusCode)
			}

			event := audit.AuditEvent{
				Time:      start,
				Source:    r.RemoteAddr,
				Action:    r.Method + " " + r.URL.Path,
				Target:    r.URL.Path,
				Result:    result,
				ErrorCode: errCode,
			}
			// Log on a best-effort basis; a logging failure must not affect
			// the HTTP response already written to the client.
			_ = logger.Log(event)
		})
	}
}

// isMutatingMethod reports whether the HTTP method modifies state.
func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
