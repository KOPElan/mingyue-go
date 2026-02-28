// Package api wires up all HTTP routes for the mingyue agent API server.
// All routes are mounted under the /api/v1 prefix.
package api

import (
	"io"
	"net/http"

	"kopelan/mingyue-go/internal/api/middleware"
	"kopelan/mingyue-go/internal/audit"
	diskService "kopelan/mingyue-go/internal/service/disk"
	fileService "kopelan/mingyue-go/internal/service/file"
	procService "kopelan/mingyue-go/internal/service/process"
	shareService "kopelan/mingyue-go/internal/service/share"
	sysService "kopelan/mingyue-go/internal/service/system"
)

// Router wraps an http.Handler and owns the resources it was built with.
// Call Close when the server shuts down to release the audit log file handle.
type Router struct {
	http.Handler
	auditLogger io.Closer
}

// Close releases resources held by the router (e.g. the audit log file).
// It is safe to call Close multiple times; subsequent calls are no-ops if
// the underlying logger does not hold additional resources.
func (r *Router) Close() error {
	if r.auditLogger != nil {
		return r.auditLogger.Close()
	}
	return nil
}

// NewRouter returns a Router with all /api/v1 routes registered.
// Middleware is applied in the order: auth → handler.
// The caller must call Close() when the server shuts down.
func NewRouter() *Router {
	monitor := sysService.NewMonitor()
	auditLogger := audit.NewFileLogger("")
	procMgr := procService.NewManager(auditLogger)
	mountSvc := diskService.NewMountService(auditLogger)
	smartSvc := diskService.NewSmartService()
	powerSvc := diskService.NewPowerService(auditLogger)
	devSvc := diskService.NewDeviceService()
	fileMgr := fileService.NewManager("", auditLogger)
	shareMgr := shareService.NewManager(auditLogger)
	sambaUserMgr := shareService.NewSambaUserManager()

	return &Router{
		Handler:     NewRouterWithDeps(monitor, procMgr, mountSvc, smartSvc, powerSvc, devSvc, fileMgr, shareMgr, sambaUserMgr),
		auditLogger: auditLogger,
	}
}

// NewRouterWithDeps creates a router with injected dependencies.
// Exported so that contract tests can inject stubs.
func NewRouterWithDeps(
	monitor *sysService.Monitor,
	procMgr *procService.Manager,
	mountSvc *diskService.MountService,
	smartSvc *diskService.SmartService,
	powerSvc *diskService.PowerService,
	devSvc *diskService.DeviceService,
	fileMgr *fileService.Manager,
	shareMgr *shareService.Manager,
	sambaUserMgr *shareService.SambaUserManager,
) http.Handler {
	mux := http.NewServeMux()

	// Health check — intentionally unauthenticated so that load balancers and
	// monitoring systems can probe liveness without credentials.
	mux.HandleFunc("/api/v1/health", HealthHandler)

	// Version — unauthenticated informational endpoint.
	mux.HandleFunc("/api/v1/version", VersionHandler)

	// ── Authenticated routes ──────────────────────────────────────────────
	auth := middleware.Auth

	// System overview — read-only; any authenticated role.
	mux.Handle("/api/v1/system/overview", auth(systemOverviewHandler(monitor)))

	// Process list and single-process get — read-only; any authenticated role.
	mux.Handle("/api/v1/processes", auth(processListHandler(procMgr)))

	// Process individual routes (get + kill share the /processes/{pid} prefix).
	mux.Handle("/api/v1/processes/", auth(processDispatchHandler(procMgr)))

	// ── File management routes ────────────────────────────────────────────
	mux.Handle("/api/v1/files", auth(fileRootHandler(fileMgr)))
	mux.Handle("/api/v1/files/", auth(fileDispatchHandler(fileMgr)))

	// ── Share management routes ───────────────────────────────────────────
	mux.Handle("/api/v1/shares", auth(shareRootHandler(shareMgr)))
	mux.Handle("/api/v1/shares/", auth(shareDispatchHandler(shareMgr)))

	// ── SMB-specific routes ───────────────────────────────────────────────
	mux.Handle("/api/v1/smb/shares", auth(smbShareRootHandler(shareMgr)))
	mux.Handle("/api/v1/smb/shares/", auth(smbShareDispatchHandler(shareMgr)))
	if sambaUserMgr != nil {
		mux.Handle("/api/v1/smb/users", auth(smbUserRootHandler(sambaUserMgr)))
		mux.Handle("/api/v1/smb/users/", auth(smbUserDispatchHandler(sambaUserMgr)))
	}

	// ── NFS-specific routes ───────────────────────────────────────────────
	mux.Handle("/api/v1/nfs/exports", auth(nfsExportRootHandler(shareMgr)))
	mux.Handle("/api/v1/nfs/exports/", auth(nfsExportDispatchHandler(shareMgr)))

	// ── Disk / mount routes ───────────────────────────────────────────────

	// Exact: GET all block devices (including unmounted) via lsblk.
	mux.Handle("/api/v1/disks/devices", auth(diskDevicesListHandler(devSvc)))

	// Exact: GET (list) + POST (create) on /api/v1/disks/mounts.
	mux.Handle("/api/v1/disks/mounts", auth(diskMountsHandler(mountSvc)))

	// Subtree: DELETE /api/v1/disks/mounts/{mountpoint} (URL-encoded mountpoint).
	// More specific than /api/v1/disks/ so it wins for mounts paths.
	mux.Handle("/api/v1/disks/mounts/", auth(diskMountDeleteHandler(mountSvc)))

	// Subtree: GET /api/v1/disks/{device}/smart and GET/POST /api/v1/disks/{device}/power.
	mux.Handle("/api/v1/disks/", auth(diskDeviceHandler(smartSvc, powerSvc)))

	return mux
}

// fileRootHandler dispatches GET (list) and DELETE and POST on /api/v1/files.
func fileRootHandler(mgr *fileService.Manager) http.Handler {
	listH := fileListHandler(mgr)
	writeH := fileWriteHandler(mgr)
	deleteH := fileDeleteHandler(mgr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listH.ServeHTTP(w, r)
		case http.MethodPost:
			writeH.ServeHTTP(w, r)
		case http.MethodDelete:
			deleteH.ServeHTTP(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

// shareRootHandler dispatches GET (list) and POST (create) on /api/v1/shares.
func shareRootHandler(mgr *shareService.Manager) http.Handler {
	listH := shareListHandler(mgr)
	createH := shareCreateHandler(mgr)
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

// processDispatchHandler routes requests to the per-PID handlers based on
// HTTP method, since the standard library mux does not support method-based
// routing natively.
func processDispatchHandler(mgr *procService.Manager) http.Handler {
	getH := processGetHandler(mgr)
	killH := processKillHandler(mgr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getH.ServeHTTP(w, r)
		case http.MethodDelete:
			killH.ServeHTTP(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

