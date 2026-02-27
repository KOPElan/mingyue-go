package linux

import (
	"fmt"
	"os"
	"strings"
)

// Capability represents a Linux capability name (e.g. "cap_sys_admin").
type Capability string

const (
	CapSysAdmin  Capability = "cap_sys_admin"
	CapNetAdmin  Capability = "cap_net_admin"
	CapSysNice   Capability = "cap_sys_nice"
	CapKill      Capability = "cap_kill"
	CapDACSSetID Capability = "cap_dac_override"
)

// HasCapability reports whether the current process has the named capability
// in its effective set.  It reads /proc/self/status and parses CapEff.
//
// This is a best-effort heuristic.  If /proc/self/status cannot be read the
// function returns false, nil.
func HasCapability(cap Capability) (bool, error) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return false, fmt.Errorf("read /proc/self/status: %w", err)
	}

	var capEffHex string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "CapEff:") {
			parts := strings.Fields(line)
			if len(parts) == 2 {
				capEffHex = parts[1]
			}
			break
		}
	}

	if capEffHex == "" {
		return false, nil
	}

	var capBits uint64
	_, err = fmt.Sscanf(capEffHex, "%x", &capBits)
	if err != nil {
		return false, fmt.Errorf("parse CapEff %q: %w", capEffHex, err)
	}

	bit, ok := capabilityBit(cap)
	if !ok {
		// Unknown capability — conservatively return false.
		return false, nil
	}

	return (capBits>>bit)&1 == 1, nil
}

// IsRoot returns true when the effective UID of the process is 0.
func IsRoot() bool {
	return os.Geteuid() == 0
}

// capabilityBit maps a Capability constant to its bit position in the
// Linux capability word.  Only capabilities used by the agent are listed.
func capabilityBit(cap Capability) (uint, bool) {
	bits := map[Capability]uint{
		CapDACSSetID: 1,  // CAP_DAC_OVERRIDE
		CapKill:      5,  // CAP_KILL
		CapSysNice:   23, // CAP_SYS_NICE
		CapNetAdmin:  12, // CAP_NET_ADMIN
		CapSysAdmin:  21, // CAP_SYS_ADMIN
	}
	b, ok := bits[cap]
	return b, ok
}
