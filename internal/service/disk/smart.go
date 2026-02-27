// Package disk provides mount management and SMART health query services.
package disk

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"

	"kopelan/mingyue-go/internal/domain"
	apperrors "kopelan/mingyue-go/internal/errors"
)

// SmartService queries SMART health information from block devices using smartctl.
type SmartService struct {
	commander Commander
}

// NewSmartService creates a production SmartService backed by the real smartctl binary.
func NewSmartService() *SmartService {
	return &SmartService{commander: &osCommander{}}
}

// NewSmartServiceWithCommander creates a SmartService with an injected Commander (for testing).
func NewSmartServiceWithCommander(c Commander) *SmartService {
	return &SmartService{commander: c}
}

// Query retrieves the SMART health information for the given device.
//
// Error semantics:
//   - ErrNotFound: smartctl binary is not installed (install smartmontools).
//   - ErrForbidden: insufficient permissions to query the device.
//   - ErrInternal: all other failures (invalid device, parse errors, etc.).
func (s *SmartService) Query(ctx context.Context, device string) (*domain.DiskHealth, error) {
	output, runErr := s.commander.Run(ctx, "smartctl", "-j", "-a", device)

	// Attempt to parse any output we received, regardless of exit code.
	// smartctl may exit non-zero yet still produce valid JSON (e.g. when the
	// disk has SMART errors but is otherwise readable).
	if len(output) > 0 {
		if health, parseErr := parseSmartctlJSON(output, device); parseErr == nil {
			return health, nil
		}
	}

	// No parseable output — classify the error.
	if runErr == nil {
		return nil, apperrors.New(apperrors.ErrInternal,
			fmt.Sprintf("smartctl returned no parseable output for device %s", device))
	}

	if errors.Is(runErr, exec.ErrNotFound) {
		return nil, apperrors.Wrap(apperrors.ErrNotFound,
			"smartctl not found: install the smartmontools package to query SMART data", runErr)
	}

	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		// Exit bit 1 (value 2) means device open failed — often a permission issue.
		if exitErr.ExitCode()&0x02 != 0 {
			return nil, apperrors.Wrap(apperrors.ErrForbidden,
				fmt.Sprintf("cannot open device %s: check that the process runs as root or has CAP_SYS_RAWIO", device), runErr)
		}
	}

	return nil, apperrors.Wrap(apperrors.ErrInternal,
		fmt.Sprintf("smartctl failed for device %s: check device path and permissions", device), runErr)
}

// ─── smartctl JSON parsing ────────────────────────────────────────────────────

// smartctlOutput is the minimal subset of smartctl -j output that we use.
type smartctlOutput struct {
	ModelName    string `json:"model_name"`
	SerialNumber string `json:"serial_number"`
	SmartStatus  struct {
		Passed bool `json:"passed"`
	} `json:"smart_status"`
	Temperature struct {
		Current int `json:"current"`
	} `json:"temperature"`
	PowerOnTime struct {
		Hours uint64 `json:"hours"`
	} `json:"power_on_time"`
}

// parseSmartctlJSON parses JSON output from smartctl -j and returns a DiskHealth.
func parseSmartctlJSON(data []byte, device string) (*domain.DiskHealth, error) {
	var out smartctlOutput
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return &domain.DiskHealth{
		Device:       device,
		Model:        out.ModelName,
		Serial:       out.SerialNumber,
		HealthOK:     out.SmartStatus.Passed,
		Temperature:  out.Temperature.Current,
		PowerOnHours: out.PowerOnTime.Hours,
	}, nil
}
