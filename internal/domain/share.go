package domain

// ShareType enumerates the supported network file-share protocols.
type ShareType string

const (
	ShareTypeSamba ShareType = "smb"
	ShareTypeNFS   ShareType = "nfs"
)

// Share represents a network file share exported by this host.
type Share struct {
	// Name is the share name as seen by remote clients.
	Name string `json:"name"`
	// Type is the share protocol.
	Type ShareType `json:"type"`
	// Path is the local directory being shared.
	Path string `json:"path"`
	// Comment is the optional human-readable description.
	Comment string `json:"comment,omitempty"`
	// ReadOnly indicates whether the share is exported read-only.
	ReadOnly bool `json:"read_only"`
	// Enabled indicates whether the share is currently active.
	Enabled bool `json:"enabled"`
}
