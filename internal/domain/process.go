package domain

// Process represents a running OS process.
type Process struct {
	// PID is the process identifier.
	PID int32 `json:"pid"`
	// Name is the process name.
	Name string `json:"name"`
	// Status is the one-letter process status (R/S/Z/…).
	Status string `json:"status"`
	// CPUPercent is the CPU usage percentage.
	CPUPercent float64 `json:"cpu_percent"`
	// MemRSS is the resident set size in bytes.
	MemRSS uint64 `json:"mem_rss"`
	// User is the owning user name.
	User string `json:"user"`
	// Cmdline is the full command line.
	Cmdline string `json:"cmdline"`
}
