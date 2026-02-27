package linux

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ProcStatus holds selected fields from /proc/<pid>/status.
type ProcStatus struct {
	Name   string
	State  string
	PID    int
	VmRSS  uint64 // kB
	Threads int
}

// ReadProcStatus reads and parses /proc/<pid>/status.
// It returns a non-nil error if the file cannot be read or parsed.
func ReadProcStatus(pid int) (ProcStatus, error) {
	path := fmt.Sprintf("/proc/%d/status", pid)
	data, err := os.ReadFile(path)
	if err != nil {
		return ProcStatus{}, fmt.Errorf("read %s: %w", path, err)
	}
	return parseProcStatus(string(data))
}

// parseProcStatus parses the text content of a /proc/<pid>/status file.
func parseProcStatus(content string) (ProcStatus, error) {
	var s ProcStatus
	for _, line := range strings.Split(content, "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "Name":
			s.Name = value
		case "State":
			s.State = value
		case "Pid":
			n, err := strconv.Atoi(value)
			if err == nil {
				s.PID = n
			}
		case "VmRSS":
			// Format: "1234 kB"
			fields := strings.Fields(value)
			if len(fields) > 0 {
				n, err := strconv.ParseUint(fields[0], 10, 64)
				if err == nil {
					s.VmRSS = n
				}
			}
		case "Threads":
			n, err := strconv.Atoi(value)
			if err == nil {
				s.Threads = n
			}
		}
	}
	return s, nil
}
