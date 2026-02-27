package api

import (
	"encoding/json"
	"net/http"
	"runtime"
)

// HealthResponse is the JSON body returned by GET /api/v1/health.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	GoOS    string `json:"go_os"`
	GoArch  string `json:"go_arch"`
}

// HealthHandler handles GET /api/v1/health.
// It returns HTTP 200 with a JSON body as long as the process is alive.
// No authentication is required so that monitoring tools can probe without
// needing a token.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	resp := HealthResponse{
		Status:  "ok",
		Version: Version,
		GoOS:    runtime.GOOS,
		GoArch:  runtime.GOARCH,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
