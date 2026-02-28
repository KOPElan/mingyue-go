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

// ── SMB share handlers ────────────────────────────────────────────────────────

// smbShareListHandler handles GET /api/v1/smb/shares
func smbShareListHandler(mgr *shareService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all, err := mgr.List(r.Context())
		if err != nil {
			writeAppError(w, err)
			return
		}
		var shares []domain.Share
		for _, s := range all {
			if s.Type == domain.ShareTypeSamba {
				shares = append(shares, s)
			}
		}
		if shares == nil {
			shares = []domain.Share{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"shares": shares})
	}
}

// smbShareGetHandler handles GET /api/v1/smb/shares/{name}
func smbShareGetHandler(mgr *shareService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := smbShareNameFromPath(r.URL.Path)
		if name == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "share name is required"))
			return
		}
		s, err := mgr.Get(r.Context(), name)
		if err != nil {
			writeAppError(w, err)
			return
		}
		if s.Type != domain.ShareTypeSamba {
			writeAppError(w, apperrors.New(apperrors.ErrNotFound, "SMB share not found"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(s)
	}
}

// smbShareCreateHandler handles POST /api/v1/smb/shares
// Requires operator or admin role.
func smbShareCreateHandler(mgr *shareService.Manager) http.HandlerFunc {
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
		s.Type = domain.ShareTypeSamba
		if err := mgr.Create(r.Context(), s, r.RemoteAddr); err != nil {
			writeAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

// smbShareUpdateHandler handles PUT /api/v1/smb/shares/{name}
// Requires operator or admin role.
func smbShareUpdateHandler(mgr *shareService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleOperator) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "operator or admin role required"))
			return
		}
		name := smbShareNameFromPath(r.URL.Path)
		if name == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "share name is required"))
			return
		}
		// Verify the existing resource is an SMB share before overwriting it.
		existing, err := mgr.Get(r.Context(), name)
		if err != nil {
			writeAppError(w, err)
			return
		}
		if existing.Type != domain.ShareTypeSamba {
			writeAppError(w, apperrors.New(apperrors.ErrNotFound, "SMB share not found"))
			return
		}
		var s domain.Share
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid JSON body"))
			return
		}
		s.Name = name
		s.Type = domain.ShareTypeSamba
		if err := mgr.Update(r.Context(), s, r.RemoteAddr); err != nil {
			writeAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// smbShareDeleteHandler handles DELETE /api/v1/smb/shares/{name}
// Requires operator or admin role.
func smbShareDeleteHandler(mgr *shareService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleOperator) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "operator or admin role required"))
			return
		}
		name := smbShareNameFromPath(r.URL.Path)
		if name == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "share name is required"))
			return
		}
		// Verify the target is an SMB share before deleting it.
		existing, err := mgr.Get(r.Context(), name)
		if err != nil {
			writeAppError(w, err)
			return
		}
		if existing.Type != domain.ShareTypeSamba {
			writeAppError(w, apperrors.New(apperrors.ErrNotFound, "SMB share not found"))
			return
		}
		if err := mgr.Delete(r.Context(), name, r.RemoteAddr); err != nil {
			writeAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// smbShareRootHandler dispatches GET and POST on /api/v1/smb/shares.
func smbShareRootHandler(mgr *shareService.Manager) http.Handler {
	listH := smbShareListHandler(mgr)
	createH := smbShareCreateHandler(mgr)
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

// smbShareDispatchHandler routes /api/v1/smb/shares/{name} by HTTP method.
func smbShareDispatchHandler(mgr *shareService.Manager) http.Handler {
	getH := smbShareGetHandler(mgr)
	updateH := smbShareUpdateHandler(mgr)
	deleteH := smbShareDeleteHandler(mgr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Reject paths with sub-resources (no sub-paths defined under shares/{name}).
		if strings.Contains(r.URL.Path[len("/api/v1/smb/shares/"):], "/") {
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

// smbShareNameFromPath extracts the share name from /api/v1/smb/shares/{name}.
func smbShareNameFromPath(path string) string {
	const prefix = "/api/v1/smb/shares/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	name := path[len(prefix):]
	if idx := strings.Index(name, "/"); idx >= 0 {
		name = name[:idx]
	}
	return name
}

// ── SMB user handlers ─────────────────────────────────────────────────────────

// smbUserListHandler handles GET /api/v1/smb/users
func smbUserListHandler(mgr *shareService.SambaUserManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		users, err := mgr.ListUsers(r.Context())
		if err != nil {
			writeAppError(w, err)
			return
		}
		if users == nil {
			users = []shareService.SambaUser{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"users": users})
	}
}

// smbUserAddHandler handles POST /api/v1/smb/users
// Requires operator or admin role.
// Body: {"username": "alice", "password": "s3cr3t"}
func smbUserAddHandler(mgr *shareService.SambaUserManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleOperator) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "operator or admin role required"))
			return
		}
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid JSON body"))
			return
		}
		username := strings.TrimSpace(req.Username)
		password := strings.TrimSpace(req.Password)
		if username == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "username is required"))
			return
		}
		if password == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "password is required"))
			return
		}
		if err := mgr.AddUser(r.Context(), username, password); err != nil {
			writeAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

// smbUserRemoveHandler handles DELETE /api/v1/smb/users/{username}
// Requires operator or admin role.
func smbUserRemoveHandler(mgr *shareService.SambaUserManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleOperator) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "operator or admin role required"))
			return
		}
		username := smbUsernameFromPath(r.URL.Path)
		if username == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "username is required"))
			return
		}
		if err := mgr.RemoveUser(r.Context(), username); err != nil {
			writeAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// smbUserSetPasswordHandler handles PUT /api/v1/smb/users/{username}/password
// Requires operator or admin role.
// Body: {"password": "newpassword"}
func smbUserSetPasswordHandler(mgr *shareService.SambaUserManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleOperator) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "operator or admin role required"))
			return
		}
		username := smbUsernameFromPath(r.URL.Path)
		if username == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "username is required"))
			return
		}
		var req struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid JSON body"))
			return
		}
		password := strings.TrimSpace(req.Password)
		if password == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "password is required"))
			return
		}
		if err := mgr.SetPassword(r.Context(), username, password); err != nil {
			writeAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// smbUserRootHandler dispatches GET and POST on /api/v1/smb/users.
func smbUserRootHandler(mgr *shareService.SambaUserManager) http.Handler {
	listH := smbUserListHandler(mgr)
	addH := smbUserAddHandler(mgr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listH.ServeHTTP(w, r)
		case http.MethodPost:
			addH.ServeHTTP(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

// smbUserDispatchHandler routes /api/v1/smb/users/{username}[/password] by method and path.
func smbUserDispatchHandler(mgr *shareService.SambaUserManager) http.Handler {
	removeH := smbUserRemoveHandler(mgr)
	passwdH := smbUserSetPasswordHandler(mgr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Detect /api/v1/smb/users/{username}/password sub-path.
		suffix := r.URL.Path[len("/api/v1/smb/users/"):]
		if strings.HasSuffix(suffix, "/password") {
			if r.Method == http.MethodPut {
				passwdH.ServeHTTP(w, r)
				return
			}
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// /api/v1/smb/users/{username}
		switch r.Method {
		case http.MethodDelete:
			removeH.ServeHTTP(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

// smbUsernameFromPath extracts the username from paths like:
//   /api/v1/smb/users/{username}
//   /api/v1/smb/users/{username}/password
func smbUsernameFromPath(path string) string {
	const prefix = "/api/v1/smb/users/"
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	rest := path[len(prefix):]
	// Strip /password suffix if present.
	rest = strings.TrimSuffix(rest, "/password")
	if idx := strings.Index(rest, "/"); idx >= 0 {
		rest = rest[:idx]
	}
	return rest
}
