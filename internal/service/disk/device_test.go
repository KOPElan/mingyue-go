package disk

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	apperrors "kopelan/mingyue-go/internal/errors"
)

// ─── sample lsblk JSON output ─────────────────────────────────────────────────

// sampleLsblkJSON represents a typical lsblk -J -b output with two disks:
// sda (disk, not mounted) with one partition sda1 (mounted at /boot), and
// sdb (disk, mounted at /mnt/data).
const sampleLsblkJSON = `{
   "blockdevices": [
      {
         "name": "sda",
         "size": "21474836480",
         "type": "disk",
         "mountpoint": null,
         "model": "VBOX HARDDISK   ",
         "rm": false,
         "children": [
            {
               "name": "sda1",
               "size": "1073741824",
               "type": "part",
               "mountpoint": "/boot",
               "model": null,
               "rm": false
            }
         ]
      },
      {
         "name": "sdb",
         "size": "10737418240",
         "type": "disk",
         "mountpoint": "/mnt/data",
         "model": "USB Drive  ",
         "rm": true
      }
   ]
}`

// sampleLsblkJSONStringRM tests the "0"/"1" rm variant used by older lsblk versions.
const sampleLsblkJSONStringRM = `{
   "blockdevices": [
      {
         "name": "sdc",
         "size": "4294967296",
         "type": "disk",
         "mountpoint": null,
         "model": "SD Card",
         "rm": "1"
      }
   ]
}`

// ─── parseLsblkJSON tests ─────────────────────────────────────────────────────

func TestParseLsblkJSON_Success(t *testing.T) {
	devices, err := parseLsblkJSON([]byte(sampleLsblkJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expect 3 devices: sda, sda1, sdb
	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}

	// sda: disk, not mounted
	if devices[0].Name != "sda" {
		t.Errorf("devices[0].Name: got %q, want %q", devices[0].Name, "sda")
	}
	if devices[0].Type != "disk" {
		t.Errorf("devices[0].Type: got %q, want %q", devices[0].Type, "disk")
	}
	if devices[0].MountPoint != "" {
		t.Errorf("devices[0].MountPoint: got %q, want empty", devices[0].MountPoint)
	}
	if devices[0].Model != "VBOX HARDDISK" {
		t.Errorf("devices[0].Model: got %q, want %q", devices[0].Model, "VBOX HARDDISK")
	}
	if devices[0].Removable {
		t.Error("devices[0].Removable: expected false")
	}
	if devices[0].SizeBytes != 21474836480 {
		t.Errorf("devices[0].SizeBytes: got %d, want 21474836480", devices[0].SizeBytes)
	}

	// sda1: partition, mounted at /boot
	if devices[1].Name != "sda1" {
		t.Errorf("devices[1].Name: got %q, want %q", devices[1].Name, "sda1")
	}
	if devices[1].MountPoint != "/boot" {
		t.Errorf("devices[1].MountPoint: got %q, want %q", devices[1].MountPoint, "/boot")
	}

	// sdb: removable disk, mounted at /mnt/data
	if devices[2].Name != "sdb" {
		t.Errorf("devices[2].Name: got %q, want %q", devices[2].Name, "sdb")
	}
	if devices[2].MountPoint != "/mnt/data" {
		t.Errorf("devices[2].MountPoint: got %q, want %q", devices[2].MountPoint, "/mnt/data")
	}
	if !devices[2].Removable {
		t.Error("devices[2].Removable: expected true")
	}
}

func TestParseLsblkJSON_StringRM(t *testing.T) {
	devices, err := parseLsblkJSON([]byte(sampleLsblkJSONStringRM))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("expected 1 device, got %d", len(devices))
	}
	if !devices[0].Removable {
		t.Error("Removable: expected true for rm=1")
	}
}

func TestParseLsblkJSON_Empty(t *testing.T) {
	devices, err := parseLsblkJSON([]byte(`{"blockdevices":[]}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 0 {
		t.Errorf("expected 0 devices, got %d", len(devices))
	}
}

func TestParseLsblkJSON_InvalidJSON(t *testing.T) {
	_, err := parseLsblkJSON([]byte("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

// ─── DeviceService.List tests ─────────────────────────────────────────────────

func TestDeviceService_List_Success(t *testing.T) {
	svc := NewDeviceServiceWithCommander(&smartCommanderOK{output: sampleLsblkJSON})
	devices, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(devices) != 3 {
		t.Errorf("expected 3 devices, got %d", len(devices))
	}
}

func TestDeviceService_List_BinaryNotFound_ReturnsErrNotFound(t *testing.T) {
	svc := NewDeviceServiceWithCommander(&smartCommanderNotFound{})
	_, err := svc.List(context.Background())
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

func TestDeviceService_List_NoOutput_ReturnsErrInternal(t *testing.T) {
	svc := NewDeviceServiceWithCommander(&smartCommanderNoOutput{err: errors.New("some error")})
	_, err := svc.List(context.Background())
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

func TestDeviceService_List_BinaryNotFoundWithExecError(t *testing.T) {
	cmd := &lsblkNotFoundCommander{}
	svc := NewDeviceServiceWithCommander(cmd)
	_, err := svc.List(context.Background())
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

// lsblkNotFoundCommander simulates "lsblk not found" by returning exec.ErrNotFound.
type lsblkNotFoundCommander struct{}

func (c *lsblkNotFoundCommander) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, &exec.Error{Name: "lsblk", Err: exec.ErrNotFound}
}
