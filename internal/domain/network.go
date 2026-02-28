package domain

// NetworkInterface represents a network interface and its assigned addresses.
type NetworkInterface struct {
	// Name is the interface name, e.g. "eth0" or "lo".
	Name string `json:"name"`
	// Index is the OS-assigned interface index.
	Index int `json:"index"`
	// Flags lists the interface flags, e.g. ["UP", "BROADCAST", "MULTICAST"].
	Flags []string `json:"flags"`
	// MTU is the maximum transmission unit in bytes.
	MTU int `json:"mtu"`
	// HardwareAddr is the MAC address string, empty for loopback/virtual interfaces.
	HardwareAddr string `json:"hardware_addr,omitempty"`
	// Addresses is the list of unicast addresses assigned to this interface.
	Addresses []NetworkAddress `json:"addresses"`
}

// NetworkAddress represents a single unicast address assigned to an interface.
type NetworkAddress struct {
	// IP is the IP address string (IPv4 or IPv6).
	IP string `json:"ip"`
	// Prefix is the prefix length (CIDR notation).
	Prefix int `json:"prefix"`
	// Family is "ipv4" or "ipv6".
	Family string `json:"family"`
}
