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

func TestValidate_UnknownToken_ReturnsUnauthorized(t *testing.T) {
	// Tokens that have not been registered must be rejected.
	tokens := []string{"sometoken", "Bearer abc123", "eyJhbGciOiJIUzI1NiJ9"}

	for _, tok := range tokens {
		t.Run(tok, func(t *testing.T) {
			_, err := auth.Validate(tok)
			if err == nil {
				t.Fatal("expected error for unknown token, got nil")
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

func TestValidate_RegisteredKey_ReturnsRole(t *testing.T) {
	auth.RegisterAPIKey("test-viewer-key", auth.Token{Raw: "test-viewer-key", Role: auth.RoleViewer, Subject: "viewer"})
	auth.RegisterAPIKey("test-admin-key", auth.Token{Raw: "test-admin-key", Role: auth.RoleAdmin, Subject: "admin"})

	tests := []struct {
		token    string
		wantRole auth.Role
	}{
		{"test-viewer-key", auth.RoleViewer},
		{"test-admin-key", auth.RoleAdmin},
	}
	for _, tc := range tests {
		role, err := auth.Validate(tc.token)
		if err != nil {
			t.Errorf("Validate(%q): unexpected error %v", tc.token, err)
			continue
		}
		if role != tc.wantRole {
			t.Errorf("Validate(%q): role = %q, want %q", tc.token, role, tc.wantRole)
		}
	}
}

func TestHasRole(t *testing.T) {
	tests := []struct {
		role    auth.Role
		minimum auth.Role
		want    bool
	}{
		{auth.RoleViewer, auth.RoleViewer, true},
		{auth.RoleOperator, auth.RoleViewer, true},
		{auth.RoleAdmin, auth.RoleViewer, true},
		{auth.RoleAdmin, auth.RoleOperator, true},
		{auth.RoleAdmin, auth.RoleAdmin, true},
		{auth.RoleViewer, auth.RoleOperator, false},
		{auth.RoleViewer, auth.RoleAdmin, false},
		{auth.RoleOperator, auth.RoleAdmin, false},
	}
	for _, tc := range tests {
		got := auth.HasRole(tc.role, tc.minimum)
		if got != tc.want {
			t.Errorf("HasRole(%q, %q) = %v, want %v", tc.role, tc.minimum, got, tc.want)
		}
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

