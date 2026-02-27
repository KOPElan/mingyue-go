// Package auth provides authentication and authorisation primitives.
// Validate supports opaque API key tokens stored in memory.  The key store
// is populated via RegisterAPIKey and is safe for concurrent use.
package auth

import (
	"sync"

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

// Token holds the parsed token payload.
type Token struct {
	// Raw is the original token string.
	Raw string
	// Role is the role associated with the token.
	Role Role
	// Subject identifies the principal (user / service account).
	Subject string
}

// roleOrder assigns a numeric level so that HasRole can compare privilege.
var roleOrder = map[Role]int{
	RoleViewer:   1,
	RoleOperator: 2,
	RoleAdmin:    3,
}

// HasRole reports whether role meets or exceeds the minimum required role.
func HasRole(role, minimum Role) bool {
	return roleOrder[role] >= roleOrder[minimum]
}

// ─── In-memory API key store ─────────────────────────────────────────────────

var (
	keysMu sync.RWMutex
	keys   = map[string]Token{}
)

// RegisterAPIKey adds an opaque API key to the in-memory store.
// Calling this function again with the same key overwrites the previous entry.
func RegisterAPIKey(key string, tok Token) {
	keysMu.Lock()
	defer keysMu.Unlock()
	keys[key] = tok
}

// Validate parses and verifies the given token string.
// It looks up the token in the in-memory API key store.  Returns ErrUnauthorized
// when the token is missing or unknown.
func Validate(token string) (Role, error) {
	if token == "" {
		return "", apperrors.New(apperrors.ErrUnauthorized, "missing authentication token")
	}
	keysMu.RLock()
	tok, ok := keys[token]
	keysMu.RUnlock()
	if !ok {
		return "", apperrors.New(apperrors.ErrUnauthorized, "invalid or unknown token")
	}
	return tok.Role, nil
}

