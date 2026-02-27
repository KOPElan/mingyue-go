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
