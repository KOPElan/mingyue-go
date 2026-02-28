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

// networkInterfacesHandler handles GET /api/v1/network/interfaces
// Returns all network interfaces. Any authenticated role may call this.
func networkInterfacesHandler(mgr *netService.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ifaces, err := mgr.ListInterfaces(r.Context())
		if err != nil {
			writeAppError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"interfaces": ifaces,
		})
	}
}

// networkInterfaceDispatchHandler routes /api/v1/network/interfaces/{name} requests.
// GET returns a single interface; PUT performs a mutating action (up/down/dhcp)
// and requires admin role.
func networkInterfaceDispatchHandler(mgr *netService.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip prefix to extract "{name}" or "{name}/{action}".
		sub := strings.TrimPrefix(r.URL.Path, "/api/v1/network/interfaces/")
		parts := strings.SplitN(sub, "/", 2)
		name := parts[0]
		if name == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "interface name is required"))
			return
		}

		switch r.Method {
		case http.MethodGet:
			iface, err := mgr.GetInterface(r.Context(), name)
			if err != nil {
				writeAppError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(iface)

		case http.MethodPut:
			role := middleware.RoleFromContext(r.Context())
			if !auth.HasRole(role, auth.RoleAdmin) {
				writeAppError(w, apperrors.New(apperrors.ErrForbidden, "admin role required"))
				return
			}

			var req struct {
				Action string `json:"action"` // "up", "down", or "dhcp"
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid JSON body"))
				return
			}

			source := r.RemoteAddr
			var opErr error
			switch req.Action {
			case "up":
				opErr = mgr.SetLinkUp(r.Context(), name, source)
			case "down":
				opErr = mgr.SetLinkDown(r.Context(), name, source)
			case "dhcp":
				opErr = mgr.RenewDHCP(r.Context(), name, source)
			default:
				writeAppError(w, apperrors.New(apperrors.ErrInvalidInput,
					`action must be "up", "down", or "dhcp"`))
				return
			}
			if opErr != nil {
				writeAppError(w, opErr)
				return
			}
			w.WriteHeader(http.StatusNoContent)

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}
