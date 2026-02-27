// Package api wires up all HTTP routes for the mingyue agent API server.
// All routes are mounted under the /api/v1 prefix.
package api

import (
	"net/http"

	"kopelan/mingyue-go/internal/api/middleware"
	"kopelan/mingyue-go/internal/audit"
	procService "kopelan/mingyue-go/internal/service/process"
	sysService "kopelan/mingyue-go/internal/service/system"
)

// NewRouter returns an http.Handler with all /api/v1 routes registered.
// Middleware is applied in the order: auth → handler.
func NewRouter() http.Handler {
	monitor := sysService.NewMonitor()
	auditLogger := audit.NewFileLogger("")
	procMgr := procService.NewManager(auditLogger)

	return NewRouterWithDeps(monitor, procMgr)
}

// NewRouterWithDeps creates a router with injected dependencies.
// Exported so that contract tests can inject stubs.
func NewRouterWithDeps(monitor *sysService.Monitor, procMgr *procService.Manager) http.Handler {
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

	return mux
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

