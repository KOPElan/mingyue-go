package api

import (
	"encoding/json"
	"errors"
	"net/http"

	apperrors "kopelan/mingyue-go/internal/errors"
)

// writeAppError serialises err as a JSON AppError response.
// When err is an *AppError the HTTP status is derived from its code; otherwise
// 500 is used.
func writeAppError(w http.ResponseWriter, err error) {
	var ae *apperrors.AppError
	if !errors.As(err, &ae) {
		ae = apperrors.Wrap(apperrors.ErrInternal, "internal server error", err)
	}

	status := appErrorStatus(ae.Code)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ae)
}

// appErrorStatus maps an ErrorCode to an HTTP status code.
func appErrorStatus(code apperrors.ErrorCode) int {
	switch code {
	case apperrors.ErrNotFound:
		return http.StatusNotFound
	case apperrors.ErrUnauthorized:
		return http.StatusUnauthorized
	case apperrors.ErrForbidden:
		return http.StatusForbidden
	case apperrors.ErrInvalidInput:
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
