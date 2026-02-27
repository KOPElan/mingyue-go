package errors_test

import (
	"encoding/json"
	"strings"
	"testing"

	apperrors "kopelan/mingyue-go/internal/errors"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		code    apperrors.ErrorCode
		message string
	}{
		{"not found", apperrors.ErrNotFound, "resource not found"},
		{"unauthorized", apperrors.ErrUnauthorized, "authentication required"},
		{"forbidden", apperrors.ErrForbidden, "access denied"},
		{"internal", apperrors.ErrInternal, "unexpected error"},
		{"invalid input", apperrors.ErrInvalidInput, "bad request parameter"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := apperrors.New(tc.code, tc.message)
			if err.Code != tc.code {
				t.Errorf("Code = %q, want %q", err.Code, tc.code)
			}
			if err.Message != tc.message {
				t.Errorf("Message = %q, want %q", err.Message, tc.message)
			}
			if err.Cause != nil {
				t.Error("Cause should be nil for New()")
			}
		})
	}
}

func TestWrap(t *testing.T) {
	cause := apperrors.New(apperrors.ErrInternal, "db error")
	err := apperrors.Wrap(apperrors.ErrInternal, "service failure", cause)

	if err.Cause == nil {
		t.Fatal("Cause should not be nil for Wrap()")
	}
	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestError_Message(t *testing.T) {
	tests := []struct {
		name     string
		err      *apperrors.AppError
		wantSubs []string
	}{
		{
			name:     "without cause",
			err:      apperrors.New(apperrors.ErrNotFound, "item missing"),
			wantSubs: []string{"NOT_FOUND", "item missing"},
		},
		{
			name:     "with cause",
			err:      apperrors.Wrap(apperrors.ErrInternal, "wrapped", apperrors.New(apperrors.ErrInternal, "root")),
			wantSubs: []string{"INTERNAL", "wrapped", "root"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			msg := tc.err.Error()
			for _, sub := range tc.wantSubs {
				if !strings.Contains(msg, sub) {
					t.Errorf("Error() = %q, want substring %q", msg, sub)
				}
			}
		})
	}
}

func TestAppError_JSONSerialization(t *testing.T) {
	err := apperrors.New(apperrors.ErrNotFound, "the item was not found")

	data, jsonErr := json.Marshal(err)
	if jsonErr != nil {
		t.Fatalf("json.Marshal error: %v", jsonErr)
	}

	var got map[string]interface{}
	if jsonErr = json.Unmarshal(data, &got); jsonErr != nil {
		t.Fatalf("json.Unmarshal error: %v", jsonErr)
	}

	if got["code"] != "NOT_FOUND" {
		t.Errorf("code = %v, want NOT_FOUND", got["code"])
	}
	if got["message"] != "the item was not found" {
		t.Errorf("message = %v, want 'the item was not found'", got["message"])
	}
	if _, hasCause := got["cause"]; hasCause {
		t.Error("cause must not appear in JSON output")
	}
}

func TestAppError_JSONExcludesCause(t *testing.T) {
	inner := apperrors.New(apperrors.ErrInternal, "sensitive DB error")
	outer := apperrors.Wrap(apperrors.ErrInternal, "public message", inner)

	data, err := json.Marshal(outer)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, "sensitive") {
		t.Errorf("JSON output must not contain cause message, got: %s", jsonStr)
	}
}

func TestAppError_Unwrap(t *testing.T) {
	cause := apperrors.New(apperrors.ErrInternal, "root")
	wrapped := apperrors.Wrap(apperrors.ErrInternal, "outer", cause)

	if wrapped.Unwrap() != cause {
		t.Errorf("Unwrap() = %v, want %v", wrapped.Unwrap(), cause)
	}
}
