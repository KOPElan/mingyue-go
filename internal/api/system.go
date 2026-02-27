package api

import (
	"encoding/json"
	"net/http"

	"kopelan/mingyue-go/internal/service/system"
)

// systemOverviewHandler handles GET /api/v1/system/overview.
// It returns a JSON-encoded HostSnapshot reflecting the current resource usage.
func systemOverviewHandler(monitor *system.Monitor) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		snap, err := monitor.Snapshot(r.Context())
		if err != nil {
			writeAppError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(snap)
	}
}
