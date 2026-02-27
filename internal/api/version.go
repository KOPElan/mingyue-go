// Package api contains the mingyue HTTP API handler and router.
// This file exports the build-time version so that the health endpoint
// and other handlers can reference it from a single source of truth.
package api

import (
	"encoding/json"
	"net/http"
)

// Version is the current application version.  Override at build time with:
//
//	go build -ldflags "-X kopelan/mingyue-go/internal/api.Version=v1.2.3"
var Version = "dev"

// VersionResponse is the JSON body returned by GET /api/v1/version.
type VersionResponse struct {
	Version string `json:"version"`
}

// VersionHandler handles GET /api/v1/version.
func VersionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(VersionResponse{Version: Version})
}
