// Package errors defines the unified error structure for the mingyue application.
// All errors returned from services, CLI handlers, and HTTP handlers use AppError
// so that CLI --json output and HTTP API responses share the same shape.
package errors

import (
	"encoding/json"
	"fmt"
)

// ErrorCode is a machine-readable string that identifies the error category.
type ErrorCode string

const (
	ErrNotFound     ErrorCode = "NOT_FOUND"
	ErrUnauthorized ErrorCode = "UNAUTHORIZED"
	ErrForbidden    ErrorCode = "FORBIDDEN"
	ErrInternal     ErrorCode = "INTERNAL"
	ErrInvalidInput ErrorCode = "INVALID_INPUT"
)

// AppError is the unified error type used throughout the application.
// It implements the error interface and can be serialised to JSON.
type AppError struct {
	// Code is a machine-readable error category string.
	Code ErrorCode `json:"code"`
	// Message is a human-readable description of the error.
	Message string `json:"message"`
	// Cause is the underlying error, if any.  It is omitted from JSON output.
	Cause error `json:"-"`
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying cause so errors.Is / errors.As work correctly.
func (e *AppError) Unwrap() error {
	return e.Cause
}

// MarshalJSON serialises AppError to JSON.
// The "cause" field is intentionally excluded to avoid leaking internal details.
func (e *AppError) MarshalJSON() ([]byte, error) {
	type alias struct {
		Code    ErrorCode `json:"code"`
		Message string    `json:"message"`
	}
	return json.Marshal(alias{Code: e.Code, Message: e.Message})
}

// New creates an AppError with the given code and message.
func New(code ErrorCode, message string) *AppError {
	return &AppError{Code: code, Message: message}
}

// Wrap creates an AppError with the given code, message, and underlying cause.
func Wrap(code ErrorCode, message string, cause error) *AppError {
	return &AppError{Code: code, Message: message, Cause: cause}
}

// Is reports whether target matches this error by comparing error codes when
// both sides are *AppError values.
func (e *AppError) Is(target error) bool {
	t, ok := target.(*AppError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}
