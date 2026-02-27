package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"kopelan/mingyue-go/internal/api/middleware"
	"kopelan/mingyue-go/internal/auth"
	apperrors "kopelan/mingyue-go/internal/errors"
	procService "kopelan/mingyue-go/internal/service/process"
)

// processListHandler handles GET /api/v1/processes.
// Query parameters: limit (int), page (int).
func processListHandler(mgr *procService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		opts := procService.ListOptions{}
		if s := r.URL.Query().Get("limit"); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 0 {
				writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "limit must be a non-negative integer"))
				return
			}
			opts.Limit = n
		}
		if s := r.URL.Query().Get("page"); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 {
				writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "page must be a positive integer"))
				return
			}
			opts.Page = n
		}

		procs, total, err := mgr.List(r.Context(), opts)
		if err != nil {
			writeAppError(w, err)
			return
		}

		type response struct {
			Total     int         `json:"total"`
			Processes interface{} `json:"processes"`
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response{Total: total, Processes: procs})
	}
}

// processGetHandler handles GET /api/v1/processes/{pid}.
func processGetHandler(mgr *procService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		pid, err := parsePIDFromPath(r.URL.Path)
		if err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid pid in path"))
			return
		}

		proc, err := mgr.Get(r.Context(), pid)
		if err != nil {
			writeAppError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(proc)
	}
}

// processKillHandler handles DELETE /api/v1/processes/{pid}.
// Requires at least operator role.
func processKillHandler(mgr *procService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Enforce minimum role (operator or admin).
		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleOperator) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "operator or admin role required"))
			return
		}

		pid, err := parsePIDFromPath(r.URL.Path)
		if err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid pid in path"))
			return
		}

		source := r.RemoteAddr
		if err := mgr.Kill(r.Context(), pid, source); err != nil {
			writeAppError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// parsePIDFromPath extracts the last path segment and parses it as an int32.
// e.g. "/api/v1/processes/123" → 123
func parsePIDFromPath(path string) (int32, error) {
	// Find the last '/' and take everything after it.
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			segment := path[i+1:]
			n, err := strconv.ParseInt(segment, 10, 32)
			if err != nil {
				return 0, err
			}
			if n <= 0 {
				return 0, apperrors.New(apperrors.ErrInvalidInput, "pid must be positive")
			}
			return int32(n), nil
		}
	}
	return 0, apperrors.New(apperrors.ErrInvalidInput, "missing pid segment")
}
