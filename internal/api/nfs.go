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

// ── NFS export handlers ───────────────────────────────────────────────────────

// nfsExportListHandler handles GET /api/v1/nfs/exports
func nfsExportListHandler(mgr *shareService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all, err := mgr.List(r.Context())
		if err != nil {
			writeAppError(w, err)
			return
		}
		var exports []domain.Share
		for _, s := range all {
			if s.Type == domain.ShareTypeNFS {
				exports = append(exports, s)
			}
		}
		if exports == nil {
			exports = []domain.Share{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"exports": exports})
	}
}

// nfsExportGetHandler handles GET /api/v1/nfs/exports/{name}
func nfsExportGetHandler(mgr *shareService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := nfsExportNameFromPath(r.URL.Path)
		if name == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "export name is required"))
			return
		}
		s, err := mgr.Get(r.Context(), name)
		if err != nil {
			writeAppError(w, err)
			return
		}
		if s.Type != domain.ShareTypeNFS {
			writeAppError(w, apperrors.New(apperrors.ErrNotFound, "NFS export not found"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s)
	}
}

// nfsExportCreateHandler handles POST /api/v1/nfs/exports
// Requires operator or admin role.
func nfsExportCreateHandler(mgr *shareService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		// Force the type so that the URL path is the source of truth.
		s.Type = domain.ShareTypeNFS
		if err := mgr.Create(r.Context(), s, r.RemoteAddr); err != nil {
			writeAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

// nfsExportUpdateHandler handles PUT /api/v1/nfs/exports/{name}
// Requires operator or admin role.
func nfsExportUpdateHandler(mgr *shareService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleOperator) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "operator or admin role required"))
			return
		}
		name := nfsExportNameFromPath(r.URL.Path)
		if name == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "export name is required"))
			return
		}
		// Verify the existing resource is an NFS export before overwriting it.
		existing, err := mgr.Get(r.Context(), name)
		if err != nil {
			writeAppError(w, err)
			return
		}
		if existing.Type != domain.ShareTypeNFS {
			writeAppError(w, apperrors.New(apperrors.ErrNotFound, "NFS export not found"))
			return
		}
		var s domain.Share
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid JSON body"))
			return
		}
		s.Name = name
		s.Type = domain.ShareTypeNFS
		if err := mgr.Update(r.Context(), s, r.RemoteAddr); err != nil {
			writeAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// nfsExportDeleteHandler handles DELETE /api/v1/nfs/exports/{name}
// Requires operator or admin role.
func nfsExportDeleteHandler(mgr *shareService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleOperator) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "operator or admin role required"))
			return
		}
		name := nfsExportNameFromPath(r.URL.Path)
		if name == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "export name is required"))
			return
		}
		// Verify the target is an NFS export before deleting it.
		existing, err := mgr.Get(r.Context(), name)
		if err != nil {
			writeAppError(w, err)
			return
		}
		if existing.Type != domain.ShareTypeNFS {
			writeAppError(w, apperrors.New(apperrors.ErrNotFound, "NFS export not found"))
			return
		}
		if err := mgr.Delete(r.Context(), name, r.RemoteAddr); err != nil {
			writeAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// nfsExportRootHandler dispatches GET and POST on /api/v1/nfs/exports.
func nfsExportRootHandler(mgr *shareService.Manager) http.Handler {
	listH := nfsExportListHandler(mgr)
	createH := nfsExportCreateHandler(mgr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listH.ServeHTTP(w, r)
		case http.MethodPost:
			createH.ServeHTTP(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

// nfsExportDispatchHandler routes /api/v1/nfs/exports/{name} by HTTP method.
func nfsExportDispatchHandler(mgr *shareService.Manager) http.Handler {
	getH := nfsExportGetHandler(mgr)
	updateH := nfsExportUpdateHandler(mgr)
	deleteH := nfsExportDeleteHandler(mgr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reject paths with sub-resources (no sub-paths defined under exports/{name}).
		if strings.Contains(r.URL.Path[len("/api/v1/nfs/exports/"):], "/") {
			http.NotFound(w, r)
			return
		}
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

// nfsExportNameFromPath extracts the export name from /api/v1/nfs/exports/{name}.
func nfsExportNameFromPath(path string) string {
	const prefix = "/api/v1/nfs/exports/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	name := path[len(prefix):]
	if idx := strings.Index(name, "/"); idx >= 0 {
		name = name[:idx]
	}
	return name
}
