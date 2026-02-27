package domain

// NetworkInterface represents a network interface with its addresses and state.
type NetworkInterface struct {
	// Name is the interface name, e.g. "eth0", "lo".
	Name string `json:"name"`
	// HardwareAddr is the MAC address string.
	HardwareAddr string `json:"hardware_addr"`
	// Flags is a list of interface flags, e.g. ["up", "broadcast", "multicast"].
	Flags []string `json:"flags"`
	// Addrs contains the CIDR addresses assigned to the interface.
	Addrs []string `json:"addrs"`
	// IsUp reports whether the interface is up.
	IsUp bool `json:"is_up"`
}

// Route represents a single entry in the kernel routing table.
type Route struct {
	// Destination is the destination network in CIDR notation, or "default".
	Destination string `json:"destination"`
	// Gateway is the next-hop gateway address, empty when directly connected.
	Gateway string `json:"gateway"`
	// Interface is the output network interface name.
	Interface string `json:"interface"`
	// Metric is the route metric/priority (lower is preferred).
	Metric string `json:"metric,omitempty"`
}
