// Package api wires up all HTTP routes for the mingyue agent API server.
// All routes are mounted under the /api/v1 prefix.
package api

import (
	"net/http"
)

// NewRouter returns an http.Handler with all /api/v1 routes registered.
// Middleware is applied in the order: auth → audit → handler.
func NewRouter() http.Handler {
	mux := http.NewServeMux()

	// Health check — intentionally unauthenticated so that load balancers and
	// monitoring systems can probe liveness without credentials.
	mux.HandleFunc("/api/v1/health", HealthHandler)

	return mux
}
