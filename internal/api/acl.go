package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"

	"kopelan/mingyue-go/internal/api/middleware"
	"kopelan/mingyue-go/internal/auth"
	apperrors "kopelan/mingyue-go/internal/errors"
	aclService "kopelan/mingyue-go/internal/service/acl"
)

// aclGetHandler handles GET /api/v1/acl?path=<path>.
// Requires viewer or above role.
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

		acl, err := mgr.Get(r.Context(), path)
		if err != nil {
			writeAppError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(acl)
	}
}

// aclSetHandler handles PUT /api/v1/acl?path=<path>.
// Requires admin role.
func aclSetHandler(mgr *aclService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Enforce admin role.
		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleAdmin) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "admin role required"))
			return
		}

		path := r.URL.Query().Get("path")
		if path == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "path query parameter is required"))
			return
		}

		var body struct {
			Mode  string `json:"mode"`  // octal string e.g. "0644"
			Owner string `json:"owner"` // user name, empty means no change
			Group string `json:"group"` // group name, empty means no change
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid request body"))
			return
		}

		req := aclService.SetRequest{
			Owner: body.Owner,
			Group: body.Group,
		}
		if body.Mode != "" {
			n, err := strconv.ParseUint(body.Mode, 8, 32)
			if err != nil {
				writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "mode must be an octal string, e.g. \"0644\""))
				return
			}
			req.Mode = os.FileMode(n)
		}

		if err := mgr.Set(r.Context(), path, req, r.RemoteAddr); err != nil {
			writeAppError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// aclDispatchHandler routes /api/v1/acl based on HTTP method.
func aclDispatchHandler(mgr *aclService.Manager) http.Handler {
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
