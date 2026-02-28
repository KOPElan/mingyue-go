package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"kopelan/mingyue-go/internal/api/middleware"
	"kopelan/mingyue-go/internal/auth"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
	shareService "kopelan/mingyue-go/internal/service/share"
)

// shareListHandler handles GET /api/v1/shares
func shareListHandler(mgr *shareService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		shares, err := mgr.List(r.Context())
		if err != nil {
			writeAppError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"shares": shares,
		})
	}
}

// shareGetHandler handles GET /api/v1/shares/{name}
func shareGetHandler(mgr *shareService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		name := shareNameFromPath(r.URL.Path)
		if name == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "share name is required"))
			return
		}

		s, err := mgr.Get(r.Context(), name)
		if err != nil {
			writeAppError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(s)
	}
}

// shareCreateHandler handles POST /api/v1/shares
// Requires operator or admin role.
func shareCreateHandler(mgr *shareService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleOperator) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "operator or admin role required"))
			return
		}

		var s domain.Share
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid JSON body"))
			return
		}

		if err := mgr.Create(r.Context(), s, r.RemoteAddr); err != nil {
			writeAppError(w, err)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

// shareUpdateHandler handles PUT /api/v1/shares/{name}
// Requires operator or admin role.
func shareUpdateHandler(mgr *shareService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleOperator) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "operator or admin role required"))
			return
		}

		name := shareNameFromPath(r.URL.Path)
		if name == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "share name is required"))
			return
		}

		var s domain.Share
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid JSON body"))
			return
		}
		// Ensure the path name matches the body name; path wins.
		s.Name = name

		if err := mgr.Update(r.Context(), s, r.RemoteAddr); err != nil {
			writeAppError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// shareDeleteHandler handles DELETE /api/v1/shares/{name}
// Requires operator or admin role.
func shareDeleteHandler(mgr *shareService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleOperator) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "operator or admin role required"))
			return
		}

		name := shareNameFromPath(r.URL.Path)
		if name == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "share name is required"))
			return
		}

		if err := mgr.Delete(r.Context(), name, r.RemoteAddr); err != nil {
			writeAppError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// shareDispatchHandler routes /api/v1/shares/{name} by HTTP method.
func shareDispatchHandler(mgr *shareService.Manager) http.Handler {
	getH := shareGetHandler(mgr)
	updateH := shareUpdateHandler(mgr)
	deleteH := shareDeleteHandler(mgr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getH.ServeHTTP(w, r)
		case http.MethodPut:
			updateH.ServeHTTP(w, r)
		case http.MethodDelete:
			deleteH.ServeHTTP(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

// shareNameFromPath extracts the share name from paths like /api/v1/shares/{name}.
func shareNameFromPath(path string) string {
	const prefix = "/api/v1/shares/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	name := path[len(prefix):]
	// Trim any trailing slashes or sub-paths.
	if idx := strings.Index(name, "/"); idx >= 0 {
		name = name[:idx]
	}
	return name
}
