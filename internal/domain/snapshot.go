package domain

import "time"

// HostSnapshot is an aggregated point-in-time view of the host's resource
// utilisation.  It is the primary response payload for "system overview"
// queries.
type HostSnapshot struct {
	// Timestamp is when the snapshot was collected (UTC).
	Timestamp time.Time `json:"timestamp"`

	// CPU usage percentage in [0, 100].
	CPUPercent float64 `json:"cpu_percent"`

	// MemTotal is the total physical memory in bytes.
	MemTotal uint64 `json:"mem_total"`
	// MemUsed is the used physical memory in bytes.
	MemUsed uint64 `json:"mem_used"`
	// MemPercent is the memory usage percentage in [0, 100].
	MemPercent float64 `json:"mem_percent"`

	// Uptime is the system uptime in seconds.
	Uptime uint64 `json:"uptime"`
}
