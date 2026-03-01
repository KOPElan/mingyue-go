package discovery_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"kopelan/mingyue-go/internal/discovery"
)

// TestAdvertiseAndBrowse verifies that Browse receives an AgentInfo packet
// that was sent by Advertise within the same test.
//
// NOTE: This test requires multicast to work on the loopback interface.
// It is skipped automatically on platforms where multicast binding fails.
func TestAdvertiseAndBrowse(t *testing.T) {
	info := discovery.AgentInfo{
		Hostname: "testhost",
		Addr:     ":17071",
		Version:  "test",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	advErr := make(chan error, 1)
	go func() {
		advErr <- discovery.Advertise(ctx, info)
	}()

	// Give Advertise a moment to send the first packet.
	time.Sleep(100 * time.Millisecond)

	results, err := discovery.Browse(1 * time.Second)
	if err != nil {
		t.Skipf("Browse failed (likely no multicast on this host): %v", err)
	}

	cancel() // stop advertising

	if aerr := <-advErr; aerr != nil {
		t.Logf("Advertise goroutine error (not fatal in CI): %v", aerr)
	}

	// If any agent was found, check that our test agent is among them.
	for _, a := range results {
		if a.Hostname == info.Hostname && a.Addr == info.Addr {
			return // found — test passes
		}
	}

	if len(results) == 0 {
		t.Skip("no multicast packets received (multicast not available on this host)")
	}
	t.Errorf("expected to find %+v in browse results, got %+v", info, results)
}

func TestAgentInfo_JSONRoundtrip(t *testing.T) {
	info := discovery.AgentInfo{
		Hostname: "myhost",
		Addr:     ":7070",
		Version:  "0.1.0",
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	var got discovery.AgentInfo
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if got != info {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, info)
	}
}
