package network_test

import (
	"context"
	"testing"

	"kopelan/mingyue-go/internal/service/network"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubOKCommander struct{}

func (c *stubOKCommander) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return []byte("ok"), nil
}

type stubErrCommander struct{ err error }

func (c *stubErrCommander) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return nil, c.err
}

// ── ListInterfaces ────────────────────────────────────────────────────────────

func TestListInterfaces_ReturnsAtLeastOne(t *testing.T) {
	mgr := network.NewManagerWithCommander(&stubOKCommander{}, nil)
	ifaces, err := mgr.ListInterfaces(context.Background())
	if err != nil {
		t.Fatalf("ListInterfaces: unexpected error: %v", err)
	}
	// Every Linux host has at least the loopback interface.
	if len(ifaces) == 0 {
		t.Error("expected at least one interface, got none")
	}
}

func TestListInterfaces_FlagsNotNil(t *testing.T) {
	mgr := network.NewManagerWithCommander(&stubOKCommander{}, nil)
	ifaces, err := mgr.ListInterfaces(context.Background())
	if err != nil {
		t.Fatalf("ListInterfaces: unexpected error: %v", err)
	}
	for _, iface := range ifaces {
		if iface.Flags == nil {
			t.Errorf("interface %q: Flags should not be nil", iface.Name)
		}
		if iface.Addresses == nil {
			t.Errorf("interface %q: Addresses should not be nil", iface.Name)
		}
	}
}

// ── GetInterface ──────────────────────────────────────────────────────────────

func TestGetInterface_Loopback(t *testing.T) {
	mgr := network.NewManagerWithCommander(&stubOKCommander{}, nil)
	iface, err := mgr.GetInterface(context.Background(), "lo")
	if err != nil {
		t.Skipf("loopback interface not available: %v", err)
	}
	if iface.Name != "lo" {
		t.Errorf("Name: got %q, want %q", iface.Name, "lo")
	}
}

func TestGetInterface_NotFound(t *testing.T) {
	mgr := network.NewManagerWithCommander(&stubOKCommander{}, nil)
	_, err := mgr.GetInterface(context.Background(), "nonexistent_iface_xyz")
	if err == nil {
		t.Fatal("expected error for nonexistent interface, got nil")
	}
}

// ── SetLinkUp / SetLinkDown ───────────────────────────────────────────────────

func TestSetLinkUp_CommandCalled(t *testing.T) {
	mgr := network.NewManagerWithCommander(&stubOKCommander{}, nil)
	if err := mgr.SetLinkUp(context.Background(), "eth0", "test"); err != nil {
		t.Fatalf("SetLinkUp: unexpected error: %v", err)
	}
}

func TestSetLinkDown_CommandCalled(t *testing.T) {
	mgr := network.NewManagerWithCommander(&stubOKCommander{}, nil)
	if err := mgr.SetLinkDown(context.Background(), "eth0", "test"); err != nil {
		t.Fatalf("SetLinkDown: unexpected error: %v", err)
	}
}

func TestSetLinkUp_CommandError(t *testing.T) {
	mgr := network.NewManagerWithCommander(&stubErrCommander{err: context.DeadlineExceeded}, nil)
	err := mgr.SetLinkUp(context.Background(), "eth0", "test")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── RenewDHCP ─────────────────────────────────────────────────────────────────

func TestRenewDHCP_SuccessOnFirstTry(t *testing.T) {
	mgr := network.NewManagerWithCommander(&stubOKCommander{}, nil)
	if err := mgr.RenewDHCP(context.Background(), "eth0", "test"); err != nil {
		t.Fatalf("RenewDHCP: unexpected error: %v", err)
	}
}

func TestRenewDHCP_BothCommandsFail(t *testing.T) {
	mgr := network.NewManagerWithCommander(&stubErrCommander{err: context.DeadlineExceeded}, nil)
	err := mgr.RenewDHCP(context.Background(), "eth0", "test")
	if err == nil {
		t.Fatal("expected error when both dhclient and dhcpcd fail")
	}
}
