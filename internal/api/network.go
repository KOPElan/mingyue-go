package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"kopelan/mingyue-go/internal/api/middleware"
	"kopelan/mingyue-go/internal/auth"
	apperrors "kopelan/mingyue-go/internal/errors"
	netService "kopelan/mingyue-go/internal/service/network"
)

// networkInterfacesHandler handles GET /api/v1/network/interfaces.
// Requires viewer or above role.
func networkInterfacesHandler(mgr *netService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ifaces, err := mgr.Interfaces()
		if err != nil {
			writeAppError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(ifaces)
	}
}

// networkRoutesHandler handles GET /api/v1/network/routes.
// Requires viewer or above role.
func networkRoutesHandler(mgr *netService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		routes, err := mgr.Routes(r.Context())
		if err != nil {
			writeAppError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(routes)
	}
}

// networkInterfaceDispatchHandler routes /api/v1/network/interfaces/:name
// based on HTTP method.
func networkInterfaceDispatchHandler(mgr *netService.Manager) http.Handler {
	putH := networkInterfaceSetStateHandler(mgr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			putH.ServeHTTP(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

// networkInterfaceSetStateHandler handles PUT /api/v1/network/interfaces/:name.
// Requires admin role.
func networkInterfaceSetStateHandler(mgr *netService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Enforce admin role.
		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleAdmin) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "admin role required"))
			return
		}

		name := interfaceNameFromPath(r.URL.Path)
		if name == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "missing interface name in path"))
			return
		}

		var body struct {
			State string `json:"state"` // "up" or "down"
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid request body"))
			return
		}
		var up bool
		switch strings.ToLower(body.State) {
		case "up":
			up = true
		case "down":
			up = false
		default:
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, `state must be "up" or "down"`))
			return
		}

		if err := mgr.SetInterfaceState(r.Context(), name, up, r.RemoteAddr); err != nil {
			writeAppError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// interfaceNameFromPath extracts the interface name from a path like
// /api/v1/network/interfaces/eth0.
func interfaceNameFromPath(path string) string {
	// Find the last '/' and return everything after it.
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[i+1:]
		}
	}
	return ""
}
