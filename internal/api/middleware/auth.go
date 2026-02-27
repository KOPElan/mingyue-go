// Package middleware provides HTTP middleware for the mingyue API server.
package middleware

import (
	"net/http"
)

// Auth is a placeholder authentication middleware.
// In Phase 1 it is a pass-through; Phase 2 will add real token validation.
//
// Usage:
//
//	mux.Handle("/api/v1/secure", middleware.Auth(myHandler))
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO(phase-2): extract Bearer token, call auth.Validate, attach role
		// to request context, and return 401/403 on failure.
		next.ServeHTTP(w, r)
	})
}
