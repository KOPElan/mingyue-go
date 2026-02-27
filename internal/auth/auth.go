// Package auth provides authentication and authorisation primitives.
// The Validate function and Token type are intentional stubs for Phase 1;
// full JWT / API-key verification will be added in Phase 2.
package auth

import (
	apperrors "kopelan/mingyue-go/internal/errors"
)

// Role represents a named permission level.
type Role string

const (
	// RoleViewer may perform read-only operations.
	RoleViewer Role = "viewer"
	// RoleOperator may perform read and non-destructive write operations.
	RoleOperator Role = "operator"
	// RoleAdmin has unrestricted access.
	RoleAdmin Role = "admin"
)

// Token holds the parsed token payload.  Fields will be populated once a real
// token format (JWT / opaque API key) is chosen in Phase 2.
type Token struct {
	// Raw is the original token string.
	Raw string
	// Role is the role extracted from the token.
	Role Role
	// Subject identifies the principal (user / service account).
	Subject string
}

// Validate parses and verifies the given token string, returning the
// associated Role.  In Phase 1 this is a skeleton; it always returns
// ErrUnauthorized so that callers can be wired up without a real auth backend.
func Validate(token string) (Role, error) {
	if token == "" {
		return "", apperrors.New(apperrors.ErrUnauthorized, "missing authentication token")
	}
	// TODO(phase-2): implement real token verification (JWT / HMAC API key).
	return "", apperrors.New(apperrors.ErrUnauthorized, "token validation not yet implemented")
}
