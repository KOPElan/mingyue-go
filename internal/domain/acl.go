package domain

// FileACL represents the POSIX permissions and optional ACL entries of a path.
type FileACL struct {
	// Path is the absolute path that was queried.
	Path string `json:"path"`
	// Mode is the octal permission string, e.g. "0644".
	Mode string `json:"mode"`
	// Owner is the name of the owning user.
	Owner string `json:"owner"`
	// Group is the name of the owning group.
	Group string `json:"group"`
	// Entries contains extended ACL entries; empty when the filesystem does not
	// support ACLs or when no extended entries are set.
	Entries []ACLEntry `json:"entries,omitempty"`
}

// ACLEntry represents a single extended ACL entry.
type ACLEntry struct {
	// Type is "user", "group", "other", or "mask".
	Type string `json:"type"`
	// Name is the user or group name; empty for "other" and "mask" entries.
	Name string `json:"name,omitempty"`
	// Permissions is the permission string, e.g. "rwx", "r--", "---".
	Permissions string `json:"permissions"`
}
