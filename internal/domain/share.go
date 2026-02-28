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

	// ── Samba-specific fields (ignored for NFS shares) ────────────────────

	// ValidUsers is a space/comma-separated list of users or @groups allowed
	// to connect to this share.  An empty value means all authenticated users.
	ValidUsers string `json:"valid_users,omitempty"`
	// WriteList is a space/comma-separated list of users or @groups that are
	// granted write access even when the share is read-only.
	WriteList string `json:"write_list,omitempty"`
	// CreateMask is the octal permission mask applied to newly created files,
	// e.g. "0644".  An empty value leaves the Samba default (0744) unchanged.
	CreateMask string `json:"create_mask,omitempty"`
	// DirectoryMask is the octal permission mask applied to newly created
	// directories, e.g. "0755".  An empty value leaves the Samba default unchanged.
	DirectoryMask string `json:"directory_mask,omitempty"`

	// ── NFS-specific fields (ignored for Samba shares) ────────────────────

	// Hosts is a space-separated list of hostnames, IP addresses, or CIDR
	// ranges that are allowed to mount the NFS export.  Defaults to "*" (all).
	Hosts string `json:"hosts,omitempty"`
}
