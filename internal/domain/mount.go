package domain

// Mount represents a filesystem mount point.
type Mount struct {
	// Device is the block device or network path, e.g. "/dev/sda1" or "//server/share".
	Device string `json:"device"`
	// MountPoint is the directory where the filesystem is mounted.
	MountPoint string `json:"mount_point"`
	// FSType is the filesystem type, e.g. "ext4", "cifs", "nfs".
	FSType string `json:"fs_type"`
	// Options are the mount options string.
	Options string `json:"options"`
	// Total is the total size of the filesystem in bytes.
	Total uint64 `json:"total"`
	// Used is the used size in bytes.
	Used uint64 `json:"used"`
	// Free is the free size in bytes.
	Free uint64 `json:"free"`
}
