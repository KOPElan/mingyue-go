package domain

// ACLInfo holds permission and optional POSIX ACL data for a file or directory.
type ACLInfo struct {
	// Path is the absolute path of the file or directory.
	Path string `json:"path"`
	// Owner is the owning user (UID or name).
	Owner string `json:"owner"`
	// Group is the owning group (GID or name).
	Group string `json:"group"`
	// Mode is the standard Unix permission string, e.g. "-rwxr-xr-x".
	Mode string `json:"mode"`
	// ACLEntries contains extended POSIX ACL entries when the filesystem
	// supports ACLs and entries exist beyond the base permissions.
	// Empty when no extended ACL is present.
	ACLEntries []ACLPermission `json:"acl_entries,omitempty"`
}

// ACLPermission represents a single POSIX ACL entry.
type ACLPermission struct {
	// Type is "user", "group", "mask", or "other".
	Type string `json:"type"`
	// Qualifier is the username or group name; empty for the owning user/group.
	Qualifier string `json:"qualifier,omitempty"`
	// Permissions is a three-character string such as "rwx", "r--", or "---".
	Permissions string `json:"permissions"`
}
