package disk

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"testing"

	apperrors "kopelan/mingyue-go/internal/errors"
)

// ─── sample smartctl JSON output ─────────────────────────────────────────────

const sampleSmartJSON = `{
  "model_name": "Samsung SSD 860 EVO 250GB",
  "serial_number": "S3EVNX0K123456",
  "smart_status": { "passed": true },
  "temperature": { "current": 26 },
  "power_on_time": { "hours": 8765 }
}`

const smartJSONHealthFailing = `{
  "model_name": "WDC WD10EZEX",
  "serial_number": "WD-WCC6Y7NK7XYZ",
  "smart_status": { "passed": false },
  "temperature": { "current": 55 },
  "power_on_time": { "hours": 43200 }
}`

// ─── stubs ───────────────────────────────────────────────────────────────────

// smartCommanderOK always returns valid JSON output with exit code 0.
type smartCommanderOK struct {
	output string
}

func (c *smartCommanderOK) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return []byte(c.output), nil
}

// smartCommanderExitErr returns JSON output with a non-zero exit code.
type smartCommanderExitErr struct {
	output   string
	exitCode int
}

func (c *smartCommanderExitErr) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return []byte(c.output), &fakeExitError{code: c.exitCode}
}

// fakeExitError implements an error compatible with exec.ExitError-like checking for tests.
type fakeExitError struct {
	code int
}

func (e *fakeExitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// Note: fakeExitError does NOT implement exec.ExitError directly (it's a struct in os package).
// For the tests that exercise the exec.ExitError path, we use the real exec package.

// smartCommanderNotFound simulates "binary not found" by returning exec.ErrNotFound.
type smartCommanderNotFound struct{}

func (c *smartCommanderNotFound) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, &exec.Error{Name: "smartctl", Err: exec.ErrNotFound}
}

// smartCommanderNoOutput returns no output and an error.
type smartCommanderNoOutput struct {
	err error
}

func (c *smartCommanderNoOutput) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, c.err
}

// ─── parseSmartctlJSON tests ──────────────────────────────────────────────────

func TestParseSmartctlJSON_Success(t *testing.T) {
	health, err := parseSmartctlJSON([]byte(sampleSmartJSON), "/dev/sda")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if health.Device != "/dev/sda" {
		t.Errorf("Device: got %q, want %q", health.Device, "/dev/sda")
	}
	if health.Model != "Samsung SSD 860 EVO 250GB" {
		t.Errorf("Model: got %q", health.Model)
	}
	if health.Serial != "S3EVNX0K123456" {
		t.Errorf("Serial: got %q", health.Serial)
	}
	if !health.HealthOK {
		t.Error("HealthOK: expected true")
	}
	if health.Temperature != 26 {
		t.Errorf("Temperature: got %d, want 26", health.Temperature)
	}
	if health.PowerOnHours != 8765 {
		t.Errorf("PowerOnHours: got %d, want 8765", health.PowerOnHours)
	}
}

func TestParseSmartctlJSON_HealthFailing(t *testing.T) {
	health, err := parseSmartctlJSON([]byte(smartJSONHealthFailing), "/dev/sdb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if health.HealthOK {
		t.Error("HealthOK: expected false for failing disk")
	}
}

func TestParseSmartctlJSON_InvalidJSON(t *testing.T) {
	_, err := parseSmartctlJSON([]byte("not json"), "/dev/sda")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// ─── SmartService.Query tests ─────────────────────────────────────────────────

func TestSmartService_Query_Success(t *testing.T) {
	svc := NewSmartServiceWithCommander(&smartCommanderOK{output: sampleSmartJSON})
	health, err := svc.Query(context.Background(), "/dev/sda")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !health.HealthOK {
		t.Error("HealthOK: expected true")
	}
}

func TestSmartService_Query_OutputWithNonZeroExit_StillParses(t *testing.T) {
	// Exit code 8 (bit 3 set = DISK FAILING) should still produce parseable output.
	svc := NewSmartServiceWithCommander(&smartCommanderExitErr{
		output:   smartJSONHealthFailing,
		exitCode: 8,
	})
	health, err := svc.Query(context.Background(), "/dev/sdb")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if health.HealthOK {
		t.Error("HealthOK: expected false for failing disk")
	}
}

func TestSmartService_Query_BinaryNotFound_ReturnsErrNotFound(t *testing.T) {
	svc := NewSmartServiceWithCommander(&smartCommanderNotFound{})
	_, err := svc.Query(context.Background(), "/dev/sda")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AppError, got %T", err)
	}
	if ae.Code != apperrors.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %q", ae.Code)
	}
}

func TestSmartService_Query_NoOutput_ReturnsErrInternal(t *testing.T) {
	svc := NewSmartServiceWithCommander(&smartCommanderNoOutput{err: errors.New("some other error")})
	_, err := svc.Query(context.Background(), "/dev/sda")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AppError, got %T", err)
	}
	if ae.Code != apperrors.ErrInternal {
		t.Errorf("expected ErrInternal, got %q", ae.Code)
	}
}

func TestSmartService_Query_ContextTimeout(t *testing.T) {
	cmd := &stubContextCancelledCommander{}
	svc := NewSmartServiceWithCommander(cmd)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := svc.Query(ctx, "/dev/sda")
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}

// ─── device normalization tests ───────────────────────────────────────────────

// recordingCommander records the args it was called with.
type recordingCommander struct {
	mu       sync.Mutex
	lastArgs []string
}

func (c *recordingCommander) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	c.mu.Lock()
	c.lastArgs = append([]string{name}, args...)
	c.mu.Unlock()
	return []byte(sampleSmartJSON), nil
}

func TestSmartService_Query_ShortDeviceName_NormalizesToDevPrefix(t *testing.T) {
	rec := &recordingCommander{}
	svc := NewSmartServiceWithCommander(rec)

	health, err := svc.Query(context.Background(), "sda")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The returned DiskHealth must carry the normalised device path.
	if health.Device != "/dev/sda" {
		t.Errorf("Device: got %q, want %q", health.Device, "/dev/sda")
	}
	// smartctl must be invoked with the full path.
	rec.mu.Lock()
	lastArg := rec.lastArgs[len(rec.lastArgs)-1]
	rec.mu.Unlock()
	if lastArg != "/dev/sda" {
		t.Errorf("smartctl device arg: got %q, want %q", lastArg, "/dev/sda")
	}
}

func TestSmartService_Query_FullDevicePath_Unchanged(t *testing.T) {
	rec := &recordingCommander{}
	svc := NewSmartServiceWithCommander(rec)

	_, err := svc.Query(context.Background(), "/dev/nvme0n1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rec.mu.Lock()
	lastArg := rec.lastArgs[len(rec.lastArgs)-1]
	rec.mu.Unlock()
	if lastArg != "/dev/nvme0n1" {
		t.Errorf("smartctl device arg: got %q, want %q", lastArg, "/dev/nvme0n1")
	}
}
