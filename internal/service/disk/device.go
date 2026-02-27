// Package disk provides mount management and SMART health query services.
package disk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// DeviceService lists all block devices on the system using lsblk.
type DeviceService struct {
	commander Commander
}

// NewDeviceService creates a production DeviceService backed by the real lsblk binary.
func NewDeviceService() *DeviceService {
	return &DeviceService{commander: &osCommander{}}
}

// NewDeviceServiceWithCommander creates a DeviceService with an injected Commander (for testing).
func NewDeviceServiceWithCommander(c Commander) *DeviceService {
	return &DeviceService{commander: c}
}

// List returns all block devices on the system, including devices that are not mounted.
// It runs lsblk -J -b to enumerate disk, partition, and other block devices.
//
// Error semantics:
//   - ErrNotFound: lsblk binary is not installed (install util-linux).
//   - ErrInternal: all other failures (parse errors, unexpected output, etc.).
func (s *DeviceService) List(ctx context.Context) ([]domain.BlockDevice, error) {
	output, runErr := s.commander.Run(ctx, "lsblk", "-J", "-b", "-o", "NAME,SIZE,TYPE,MOUNTPOINT,MODEL,RM")

	if len(output) > 0 {
		devices, parseErr := parseLsblkJSON(output)
		if parseErr == nil {
			return devices, nil
		}
	}

	if runErr == nil {
		return nil, apperrors.New(apperrors.ErrInternal, "lsblk returned no parseable output")
	}

	if errors.Is(runErr, exec.ErrNotFound) {
		return nil, apperrors.Wrap(apperrors.ErrNotFound,
			"lsblk not found: install the util-linux package to list block devices", runErr)
	}

	return nil, apperrors.Wrap(apperrors.ErrInternal, "failed to list block devices", runErr)
}

// ─── lsblk JSON parsing ───────────────────────────────────────────────────────

// lsblkOutput is the top-level JSON structure produced by lsblk -J.
type lsblkOutput struct {
	BlockDevices []lsblkDevice `json:"blockdevices"`
}

// lsblkDevice is a single entry in lsblk -J output.
// Fields use interface{} or json.RawMessage where the type varies across
// lsblk versions (e.g. rm may be bool or "0"/"1" string).
type lsblkDevice struct {
	Name       string          `json:"name"`
	Size       interface{}     `json:"size"`       // string (bytes) or JSON number
	Type       string          `json:"type"`
	MountPoint *string         `json:"mountpoint"` // null when not mounted
	Model      *string         `json:"model"`      // null for partitions
	RM         json.RawMessage `json:"rm"`         // bool or "0"/"1" string
	Children   []lsblkDevice   `json:"children,omitempty"`
}

// parseLsblkJSON parses lsblk -J -b JSON output into a flat list of BlockDevices.
func parseLsblkJSON(data []byte) ([]domain.BlockDevice, error) {
	var out lsblkOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("parse lsblk JSON: %w", err)
	}
	var devices []domain.BlockDevice
	for _, d := range out.BlockDevices {
		flattenDevice(d, &devices)
	}
	return devices, nil
}

// flattenDevice recursively converts an lsblkDevice tree into a flat slice.
func flattenDevice(d lsblkDevice, out *[]domain.BlockDevice) {
	dev := domain.BlockDevice{
		Name:      d.Name,
		SizeBytes: parseLsblkSize(d.Size),
		Type:      d.Type,
		Removable: parseLsblkRM(d.RM),
	}
	if d.MountPoint != nil {
		dev.MountPoint = *d.MountPoint
	}
	if d.Model != nil {
		dev.Model = strings.TrimSpace(*d.Model)
	}
	*out = append(*out, dev)
	for _, child := range d.Children {
		flattenDevice(child, out)
	}
}

// parseLsblkSize converts a size value (JSON string or number) to uint64 bytes.
func parseLsblkSize(v interface{}) uint64 {
	switch s := v.(type) {
	case float64:
		return uint64(s)
	case string:
		n, _ := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
		return n
	}
	return 0
}

// parseLsblkRM converts the lsblk "rm" field to a bool.
// Different lsblk versions emit a JSON bool or the strings "0"/"1".
func parseLsblkRM(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var b bool
	if json.Unmarshal(raw, &b) == nil {
		return b
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s == "1" || s == "true"
	}
	return false
}
