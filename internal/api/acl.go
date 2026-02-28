package api

import (
	"encoding/json"
	"net/http"

	"kopelan/mingyue-go/internal/api/middleware"
	"kopelan/mingyue-go/internal/auth"
	apperrors "kopelan/mingyue-go/internal/errors"
	aclService "kopelan/mingyue-go/internal/service/acl"
)

// aclGetHandler handles GET /api/v1/acl
// Query parameter: path (required). Any authenticated role may call this.
func aclGetHandler(mgr *aclService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		path := r.URL.Query().Get("path")
		if path == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "path query parameter is required"))
			return
		}

		info, err := mgr.Get(r.Context(), path)
		if err != nil {
			writeAppError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(info)
	}
}

// aclSetHandler handles PUT /api/v1/acl
// Applies POSIX ACL entries to the file or directory at path.
// Requires operator or admin role.
//
//	JSON body: {"path":"...","entries":["u:alice:rwx","g:devs:r-x"]}
func aclSetHandler(mgr *aclService.Manager) http.HandlerFunc {
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

		var req struct {
			Path    string   `json:"path"`
			Entries []string `json:"entries"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid JSON body"))
			return
		}
		if req.Path == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "path is required"))
			return
		}
		if len(req.Entries) == 0 {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "entries must not be empty"))
			return
		}

		if err := mgr.SetACL(r.Context(), req.Path, req.Entries, r.RemoteAddr); err != nil {
			writeAppError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// aclRootHandler dispatches GET and PUT on /api/v1/acl.
func aclRootHandler(mgr *aclService.Manager) http.Handler {
	getH := aclGetHandler(mgr)
	setH := aclSetHandler(mgr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getH.ServeHTTP(w, r)
		case http.MethodPut:
			setH.ServeHTTP(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}
