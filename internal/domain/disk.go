package domain

// DiskHealth holds the SMART / physical health status of a block device.
type DiskHealth struct {
	// Device is the block device path, e.g. "/dev/sda".
	Device string `json:"device"`
	// Model is the drive model string.
	Model string `json:"model"`
	// Serial is the drive serial number.
	Serial string `json:"serial"`
	// HealthOK is true when the overall SMART assessment is "PASSED".
	HealthOK bool `json:"health_ok"`
	// Temperature is the current drive temperature in Celsius.
	Temperature int `json:"temperature_c"`
	// PowerOnHours is the accumulated power-on time.
	PowerOnHours uint64 `json:"power_on_hours"`
}

// BlockDevice represents a block device on the system (disk, partition, loop, etc.).
type BlockDevice struct {
	// Name is the short device name, e.g. "sda" or "sda1".
	Name string `json:"name"`
	// SizeBytes is the device size in bytes (0 if unknown).
	SizeBytes uint64 `json:"size_bytes"`
	// Type is the device type, e.g. "disk", "part", "rom", "loop".
	Type string `json:"type"`
	// MountPoint is the directory where the device is currently mounted, or empty if not mounted.
	MountPoint string `json:"mount_point,omitempty"`
	// Model is the device model string (may be empty for partitions or virtual devices).
	Model string `json:"model,omitempty"`
	// Removable is true for removable devices such as USB drives.
	Removable bool `json:"removable"`
}

// DiskPower holds the power state of a block device as reported by hdparm.
type DiskPower struct {
	// Device is the block device path, e.g. "/dev/sda".
	Device string `json:"device"`
	// PowerMode is the current power state: "active", "standby", "sleeping", or "unknown".
	PowerMode string `json:"power_mode"`
}
