package domain

import "time"

// FileEntry represents a single file or directory listing entry.
type FileEntry struct {
	// Name is the base name of the file.
	Name string `json:"name"`
	// Path is the absolute path.
	Path string `json:"path"`
	// IsDir indicates whether the entry is a directory.
	IsDir bool `json:"is_dir"`
	// Size is the file size in bytes (0 for directories).
	Size int64 `json:"size"`
	// Mode is the file permission string, e.g. "-rwxr-xr-x".
	Mode string `json:"mode"`
	// ModTime is the last modification time.
	ModTime time.Time `json:"mod_time"`
	// Owner is the owning user name.
	Owner string `json:"owner"`
	// Unreadable is true when the entry's metadata could not be retrieved
	// (e.g. a permission error or race condition).  Other fields may be empty.
	Unreadable bool `json:"unreadable,omitempty"`
}
