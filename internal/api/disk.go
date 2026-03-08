package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"kopelan/mingyue-go/internal/api/middleware"
	"kopelan/mingyue-go/internal/auth"
	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
	diskService "kopelan/mingyue-go/internal/service/disk"
)

// ─── GET/POST /api/v1/disks/mounts ───────────────────────────────────────────

// diskMountsHandler handles GET (list) and POST (mount) on /api/v1/disks/mounts.
func diskMountsHandler(mountSvc *diskService.MountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			diskMountListHandler(mountSvc)(w, r)
		case http.MethodPost:
			diskMountCreateHandler(mountSvc)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// diskMountListHandler handles GET /api/v1/disks/mounts.
func diskMountListHandler(mountSvc *diskService.MountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mounts, err := mountSvc.List(r.Context())
		if err != nil {
			writeAppError(w, err)
			return
		}
		type response struct {
			Mounts interface{} `json:"mounts"`
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response{Mounts: mounts})
	}
}

// mountRequest is the JSON body for POST /api/v1/disks/mounts.
type mountRequest struct {
	Source     string `json:"source"`
	MountPoint string `json:"mount_point"`
	FSType     string `json:"fs_type"`
	ReadOnly   bool   `json:"read_only"`
	Options    string `json:"options"`
	Persistent bool   `json:"persistent,omitempty"`
	// CIFS credentials — never echoed back in any response or log.
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Domain   string `json:"domain,omitempty"`
}

// diskMountCreateHandler handles POST /api/v1/disks/mounts.
// Requires operator or admin role.
func diskMountCreateHandler(mountSvc *diskService.MountService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleOperator) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "operator or admin role required"))
			return
		}

		var req mountRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid request body"))
			return
		}
		if req.Source == "" || req.MountPoint == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "source and mount_point are required"))
			return
		}

		opts := diskService.MountOptions{
			Source:     req.Source,
			MountPoint: req.MountPoint,
			FSType:     req.FSType,
			ReadOnly:   req.ReadOnly,
			Options:    req.Options,
			Username:   req.Username,
			Password:   req.Password,
			Domain:     req.Domain,
			Persistent: req.Persistent,
		}

		source := r.RemoteAddr
		if err := mountSvc.Mount(r.Context(), opts, source); err != nil {
			writeAppError(w, err)
			return
		}

		w.WriteHeader(http.StatusCreated)
	}
}

// ─── DELETE /api/v1/disks/mounts/{mountpoint} ────────────────────────────────

// diskMountDeleteHandler handles DELETE /api/v1/disks/mounts/{mountpoint}.
// The mountpoint path segment must be URL-encoded (e.g. %2Fmnt%2Fdata → /mnt/data).
// Requires operator or admin role.
func diskMountDeleteHandler(mountSvc *diskService.MountService) http.HandlerFunc {
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

		mountpoint, err := extractMountpointFromPath(r.URL.EscapedPath())
		if err != nil {
			writeAppError(w, err)
			return
		}

		source := r.RemoteAddr
		if err := mountSvc.Umount(r.Context(), mountpoint, source); err != nil {
			writeAppError(w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── GET /api/v1/disks/devices ───────────────────────────────────────────────

// diskDevicesListHandler handles GET /api/v1/disks/devices.
// Lists all block devices, including unmounted ones, using lsblk.
func diskDevicesListHandler(devSvc *diskService.DeviceService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		devices, err := devSvc.List(r.Context())
		if err != nil {
			writeAppError(w, err)
			return
		}
		if devices == nil {
			devices = []domain.BlockDevice{}
		}
		type response struct {
			Devices []domain.BlockDevice `json:"devices"`
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response{Devices: devices})
	}
}

// ─── GET /api/v1/disks/{device}/smart ────────────────────────────────────────

// diskDeviceHandler handles device-specific routes under /api/v1/disks/{device}/...
// Supports GET /api/v1/disks/{device}/smart and GET/POST /api/v1/disks/{device}/power.
func diskDeviceHandler(smartSvc *diskService.SmartService, powerSvc *diskService.PowerService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		device, action, err := extractDeviceAndAction(r.URL.EscapedPath())
		if err != nil {
			writeAppError(w, err)
			return
		}

		switch action {
		case "smart":
			if r.Method != http.MethodGet {
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return
			}
			health, err := smartSvc.Query(r.Context(), device)
			if err != nil {
				writeAppError(w, err)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(health)
		case "power":
			diskPowerDispatchHandler(powerSvc, device)(w, r)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}
}

// ─── GET/POST /api/v1/disks/{device}/power ───────────────────────────────────

// diskPowerDispatchHandler routes GET (status) and POST (set mode) for a device's power endpoint.
func diskPowerDispatchHandler(powerSvc *diskService.PowerService, device string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			diskPowerGetHandler(powerSvc, device)(w, r)
		case http.MethodPost:
			diskPowerSetHandler(powerSvc, device)(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// diskPowerGetHandler handles GET /api/v1/disks/{device}/power.
// Returns the current power mode of the device using hdparm -C.
func diskPowerGetHandler(powerSvc *diskService.PowerService, device string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		power, err := powerSvc.GetStatus(r.Context(), device)
		if err != nil {
			writeAppError(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(power)
	}
}

// powerRequest is the JSON body for POST /api/v1/disks/{device}/power.
type powerRequest struct {
	// Action is the desired power mode: "standby" or "sleep".
	Action string `json:"action"`
}

// diskPowerSetHandler handles POST /api/v1/disks/{device}/power.
// Requires operator or admin role.
func diskPowerSetHandler(powerSvc *diskService.PowerService, device string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := middleware.RoleFromContext(r.Context())
		if !auth.HasRole(role, auth.RoleOperator) {
			writeAppError(w, apperrors.New(apperrors.ErrForbidden, "operator or admin role required"))
			return
		}

		var req powerRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "invalid request body"))
			return
		}
		if req.Action == "" {
			writeAppError(w, apperrors.New(apperrors.ErrInvalidInput, "action is required (standby or sleep)"))
			return
		}

		source := r.RemoteAddr
		if err := powerSvc.SetMode(r.Context(), device, req.Action, source); err != nil {
			writeAppError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── path helpers ─────────────────────────────────────────────────────────────

// extractMountpointFromPath extracts and URL-decodes the mountpoint from a path
// like /api/v1/disks/mounts/%2Fmnt%2Fdata.
func extractMountpointFromPath(escapedPath string) (string, error) {
	const prefix = "/api/v1/disks/mounts/"
	if !strings.HasPrefix(escapedPath, prefix) {
		return "", apperrors.New(apperrors.ErrInvalidInput, "missing mountpoint in path")
	}
	encoded := strings.TrimPrefix(escapedPath, prefix)
	if encoded == "" {
		return "", apperrors.New(apperrors.ErrInvalidInput, "missing mountpoint in path")
	}
	mp, err := url.PathUnescape(encoded)
	if err != nil {
		return "", apperrors.New(apperrors.ErrInvalidInput, "invalid mountpoint encoding")
	}
	if !strings.HasPrefix(mp, "/") {
		mp = "/" + mp
	}
	return mp, nil
}

// extractDeviceAndAction parses paths like /api/v1/disks/{device}/smart.
// device may be URL-encoded; if it has no leading '/' the handler prepends "/dev/".
func extractDeviceAndAction(escapedPath string) (device, action string, err error) {
	const prefix = "/api/v1/disks/"
	if !strings.HasPrefix(escapedPath, prefix) {
		return "", "", apperrors.New(apperrors.ErrInvalidInput, "invalid disk route")
	}
	rest := strings.TrimPrefix(escapedPath, prefix)
	// rest = "sda/smart" or "%2Fdev%2Fsda/smart"
	idx := strings.LastIndex(rest, "/")
	if idx < 0 {
		return "", "", apperrors.New(apperrors.ErrInvalidInput, "missing action in disk route (expected /smart)")
	}
	deviceEncoded := rest[:idx]
	action = rest[idx+1:]
	if deviceEncoded == "" {
		return "", "", apperrors.New(apperrors.ErrInvalidInput, "missing device in disk route")
	}
	device, err = url.PathUnescape(deviceEncoded)
	if err != nil {
		return "", "", apperrors.New(apperrors.ErrInvalidInput, "invalid device encoding")
	}
	if !strings.HasPrefix(device, "/") {
		device = "/dev/" + device
	}
	return device, action, nil
}
