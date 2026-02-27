package disk

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	apperrors "kopelan/mingyue-go/internal/errors"
)

// ─── sample hdparm -C output ──────────────────────────────────────────────────

const hdparmActiveOutput = `/dev/sda:
 drive state is:  active/idle
`

const hdparmStandbyOutput = `/dev/sda:
 drive state is:  standby
`

const hdparmSleepingOutput = `/dev/sda:
 drive state is:  sleeping
`

// ─── parseHdparmPowerMode tests ───────────────────────────────────────────────

func TestParseHdparmPowerMode_Active(t *testing.T) {
	mode, ok := parseHdparmPowerMode(hdparmActiveOutput)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if mode != "active" {
		t.Errorf("mode: got %q, want %q", mode, "active")
	}
}

func TestParseHdparmPowerMode_Standby(t *testing.T) {
	mode, ok := parseHdparmPowerMode(hdparmStandbyOutput)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if mode != "standby" {
		t.Errorf("mode: got %q, want %q", mode, "standby")
	}
}

func TestParseHdparmPowerMode_Sleeping(t *testing.T) {
	mode, ok := parseHdparmPowerMode(hdparmSleepingOutput)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if mode != "sleeping" {
		t.Errorf("mode: got %q, want %q", mode, "sleeping")
	}
}

func TestParseHdparmPowerMode_NoMatch(t *testing.T) {
	_, ok := parseHdparmPowerMode("/dev/sda:\n something else\n")
	if ok {
		t.Error("expected ok=false for unrecognised output")
	}
}

// ─── PowerService.GetStatus tests ────────────────────────────────────────────

func TestPowerService_GetStatus_Active(t *testing.T) {
	svc := NewPowerServiceWithCommander(&smartCommanderOK{output: hdparmActiveOutput}, nil)
	power, err := svc.GetStatus(context.Background(), "/dev/sda")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if power.Device != "/dev/sda" {
		t.Errorf("Device: got %q, want /dev/sda", power.Device)
	}
	if power.PowerMode != "active" {
		t.Errorf("PowerMode: got %q, want active", power.PowerMode)
	}
}

func TestPowerService_GetStatus_ShortDeviceName(t *testing.T) {
	// normalizeDevice should expand short names without a leading slash.
	if got := normalizeDevice("sda"); got != "/dev/sda" {
		t.Errorf("normalizeDevice(%q): got %q, want /dev/sda", "sda", got)
	}
	if got := normalizeDevice("/dev/sda"); got != "/dev/sda" {
		t.Errorf("normalizeDevice(%q): got %q, want /dev/sda", "/dev/sda", got)
	}
}

func TestPowerService_GetStatus_BinaryNotFound_ReturnsErrNotFound(t *testing.T) {
	svc := NewPowerServiceWithCommander(&hdparmNotFoundCommander{}, nil)
	_, err := svc.GetStatus(context.Background(), "/dev/sda")
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

func TestPowerService_GetStatus_NoOutput_ReturnsErrInternal(t *testing.T) {
	svc := NewPowerServiceWithCommander(&smartCommanderNoOutput{err: errors.New("some error")}, nil)
	_, err := svc.GetStatus(context.Background(), "/dev/sda")
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

// ─── PowerService.SetMode tests ───────────────────────────────────────────────

func TestPowerService_SetMode_Standby_Success(t *testing.T) {
	cmd := &stubCommander{}
	al := &mockAuditLogger{}
	svc := NewPowerServiceWithCommander(cmd, al)

	if err := svc.SetMode(context.Background(), "/dev/sda", "standby", "cli"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cmd.calls) == 0 {
		t.Fatal("expected hdparm command call")
	}
	// Verify -y flag was used for standby.
	call := cmd.calls[0]
	if call[0] != "hdparm" {
		t.Errorf("expected hdparm, got %q", call[0])
	}
	found := false
	for _, arg := range call {
		if arg == "-y" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected -y flag in hdparm args: %v", call)
	}
	// Verify audit event.
	if len(al.events) == 0 || al.events[0].Result != "success" {
		t.Error("expected success audit event")
	}
}

func TestPowerService_SetMode_Sleep_Success(t *testing.T) {
	cmd := &stubCommander{}
	svc := NewPowerServiceWithCommander(cmd, nil)

	if err := svc.SetMode(context.Background(), "/dev/sda", "sleep", "cli"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify -Y flag was used for sleep.
	call := cmd.calls[0]
	found := false
	for _, arg := range call {
		if arg == "-Y" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected -Y flag in hdparm args: %v", call)
	}
}

func TestPowerService_SetMode_InvalidAction_ReturnsErrInvalidInput(t *testing.T) {
	al := &mockAuditLogger{}
	svc := NewPowerServiceWithCommander(&stubCommander{}, al)
	err := svc.SetMode(context.Background(), "/dev/sda", "wakeup", "cli")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ae *apperrors.AppError
	if !errors.As(err, &ae) {
		t.Fatalf("expected *AppError, got %T", err)
	}
	if ae.Code != apperrors.ErrInvalidInput {
		t.Errorf("expected ErrInvalidInput, got %q", ae.Code)
	}
	// Verify failure audit event is recorded for invalid action.
	if len(al.events) == 0 {
		t.Fatal("expected failure audit event for invalid action, got none")
	}
	if al.events[0].Result != "failure" {
		t.Errorf("expected audit result %q, got %q", "failure", al.events[0].Result)
	}
}

func TestPowerService_SetMode_BinaryNotFound_ReturnsErrNotFound(t *testing.T) {
	al := &mockAuditLogger{}
	svc := NewPowerServiceWithCommander(&hdparmNotFoundCommander{}, al)
	err := svc.SetMode(context.Background(), "/dev/sda", "standby", "cli")
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
	// Verify failure audit event.
	if len(al.events) == 0 || al.events[0].Result != "failure" {
		t.Error("expected failure audit event")
	}
}

// ─── stubs ────────────────────────────────────────────────────────────────────

// hdparmNotFoundCommander simulates "hdparm binary not found".
type hdparmNotFoundCommander struct{}

func (c *hdparmNotFoundCommander) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, &exec.Error{Name: "hdparm", Err: exec.ErrNotFound}
}

// hdparmActiveCommander returns active/idle power state output.
type hdparmActiveCommander struct{}

func (c *hdparmActiveCommander) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return []byte(hdparmActiveOutput), nil
}
