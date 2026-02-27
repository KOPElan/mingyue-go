// Package middleware provides HTTP middleware for the mingyue API server.
package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"kopelan/mingyue-go/internal/auth"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// contextKey is an unexported context key type to avoid collisions.
type contextKey struct{ name string }

// roleContextKey is the key used to store the authenticated role in a request context.
var roleContextKey = &contextKey{"role"}

// Auth is an authentication middleware that extracts a Bearer token from the
// Authorization header, validates it, and attaches the resolved Role to the
// request context.
//
// On missing or invalid token it returns 401; on insufficient permissions the
// downstream RequireRole middleware returns 403.
func Auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		role, err := auth.Validate(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, apperrors.New(apperrors.ErrUnauthorized, "authentication required"))
			return
		}
		ctx := context.WithValue(r.Context(), roleContextKey, role)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole returns middleware that enforces a minimum role.
// Requests with a lower-privilege role receive 403 Forbidden.
func RequireRole(minRole auth.Role, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, _ := r.Context().Value(roleContextKey).(auth.Role)
		if !auth.HasRole(role, minRole) {
			writeError(w, http.StatusForbidden, apperrors.New(apperrors.ErrForbidden, "insufficient permissions"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RoleFromContext extracts the authenticated role from a request context.
// Returns an empty string when no role is present (unauthenticated request).
func RoleFromContext(ctx context.Context) auth.Role {
	role, _ := ctx.Value(roleContextKey).(auth.Role)
	return role
}

// extractBearerToken returns the token from the Authorization header or an
// empty string when no valid Bearer scheme is present.
func extractBearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(header, "Bearer ")
}

// writeError writes an AppError as a JSON response.
func writeError(w http.ResponseWriter, status int, err *apperrors.AppError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(err)
}

