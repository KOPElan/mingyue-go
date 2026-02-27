package auth_test

import (
	"errors"
	"testing"

	"kopelan/mingyue-go/internal/auth"
	apperrors "kopelan/mingyue-go/internal/errors"
)

func TestValidate_EmptyToken(t *testing.T) {
	_, err := auth.Validate("")
	if err == nil {
		t.Fatal("expected error for empty token, got nil")
	}

	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("expected *AppError, got %T: %v", err, err)
	}
	if appErr.Code != apperrors.ErrUnauthorized {
		t.Errorf("Code = %q, want %q", appErr.Code, apperrors.ErrUnauthorized)
	}
}

func TestValidate_NonEmptyToken_ReturnsUnauthorized(t *testing.T) {
	// Phase 1 stub: any non-empty token should still return an error
	// (real validation is deferred to Phase 2).
	tokens := []string{"sometoken", "Bearer abc123", "eyJhbGciOiJIUzI1NiJ9"}

	for _, tok := range tokens {
		t.Run(tok, func(t *testing.T) {
			_, err := auth.Validate(tok)
			if err == nil {
				t.Fatal("expected error from Phase-1 stub, got nil")
			}
			var appErr *apperrors.AppError
			if !errors.As(err, &appErr) {
				t.Fatalf("expected *AppError, got %T: %v", err, err)
			}
			if appErr.Code != apperrors.ErrUnauthorized {
				t.Errorf("Code = %q, want %q", appErr.Code, apperrors.ErrUnauthorized)
			}
		})
	}
}

func TestRoleConstants(t *testing.T) {
	tests := []struct {
		role auth.Role
		want string
	}{
		{auth.RoleViewer, "viewer"},
		{auth.RoleOperator, "operator"},
		{auth.RoleAdmin, "admin"},
	}
	for _, tc := range tests {
		if string(tc.role) != tc.want {
			t.Errorf("Role %q: string value = %q, want %q", tc.role, string(tc.role), tc.want)
		}
	}
}

func TestToken_Fields(t *testing.T) {
	tok := auth.Token{
		Raw:     "raw-value",
		Role:    auth.RoleAdmin,
		Subject: "user@example.com",
	}
	if tok.Raw != "raw-value" {
		t.Errorf("Raw = %q, want raw-value", tok.Raw)
	}
	if tok.Role != auth.RoleAdmin {
		t.Errorf("Role = %q, want %q", tok.Role, auth.RoleAdmin)
	}
	if tok.Subject != "user@example.com" {
		t.Errorf("Subject = %q, want user@example.com", tok.Subject)
	}
}
