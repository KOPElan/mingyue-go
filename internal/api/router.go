// Package api wires up all HTTP routes for the mingyue agent API server.
// All routes are mounted under the /api/v1 prefix.
package api

import (
	"io"
	"net/http"

	"kopelan/mingyue-go/internal/api/middleware"
	"kopelan/mingyue-go/internal/audit"
	procService "kopelan/mingyue-go/internal/service/process"
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

	return &Router{
		Handler:     NewRouterWithDeps(monitor, procMgr),
		auditLogger: auditLogger,
	}
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

