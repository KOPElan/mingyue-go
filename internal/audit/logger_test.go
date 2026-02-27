package audit_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"kopelan/mingyue-go/internal/audit"
)

func TestFileLogger_Log_WritesJSONLine(t *testing.T) {
	var buf bytes.Buffer
	logger := audit.NewWriterLogger(&buf)

	event := audit.AuditEvent{
		Time:      time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		Source:    "cli",
		Action:    "disk.mount",
		Target:    "/dev/sdb1",
		Result:    "success",
		ErrorCode: "",
	}

	if err := logger.Log(event); err != nil {
		t.Fatalf("Log() error: %v", err)
	}

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected non-empty output")
	}

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v — output: %s", err, line)
	}

	if got["source"] != "cli" {
		t.Errorf("source = %v, want cli", got["source"])
	}
	if got["action"] != "disk.mount" {
		t.Errorf("action = %v, want disk.mount", got["action"])
	}
	if got["result"] != "success" {
		t.Errorf("result = %v, want success", got["result"])
	}
}

func TestFileLogger_Log_SetsTimeWhenZero(t *testing.T) {
	var buf bytes.Buffer
	logger := audit.NewWriterLogger(&buf)

	before := time.Now().UTC().Add(-time.Second)
	if err := logger.Log(audit.AuditEvent{
		Source: "api",
		Action: "file.delete",
		Target: "/tmp/foo",
		Result: "success",
	}); err != nil {
		t.Fatalf("Log() error: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	rawTime, ok := got["time"].(string)
	if !ok {
		t.Fatalf("time field missing or wrong type: %v", got["time"])
	}
	parsed, err := time.Parse(time.RFC3339Nano, rawTime)
	if err != nil {
		t.Fatalf("cannot parse time %q: %v", rawTime, err)
	}
	if parsed.Before(before) || parsed.After(after) {
		t.Errorf("auto-set time %v is outside expected range [%v, %v]", parsed, before, after)
	}
}

func TestFileLogger_Log_IncludesErrorCode(t *testing.T) {
	var buf bytes.Buffer
	logger := audit.NewWriterLogger(&buf)

	if err := logger.Log(audit.AuditEvent{
		Source:    "api",
		Action:    "process.kill",
		Target:    "1234",
		Result:    "failure",
		ErrorCode: "NOT_FOUND",
	}); err != nil {
		t.Fatalf("Log() error: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got["error_code"] != "NOT_FOUND" {
		t.Errorf("error_code = %v, want NOT_FOUND", got["error_code"])
	}
}

func TestFileLogger_Log_OmitsErrorCodeOnSuccess(t *testing.T) {
	var buf bytes.Buffer
	logger := audit.NewWriterLogger(&buf)

	if err := logger.Log(audit.AuditEvent{
		Source: "cli",
		Action: "share.create",
		Target: "myshare",
		Result: "success",
	}); err != nil {
		t.Fatalf("Log() error: %v", err)
	}

	line := strings.TrimSpace(buf.String())
	if strings.Contains(line, "error_code") {
		t.Errorf("error_code must be omitted when empty, got: %s", line)
	}
}

func TestFileLogger_Close_NoError(t *testing.T) {
	var buf bytes.Buffer
	logger := audit.NewWriterLogger(&buf)
	if err := logger.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

func TestFileLogger_MultipleLines(t *testing.T) {
	var buf bytes.Buffer
	logger := audit.NewWriterLogger(&buf)

	for i := 0; i < 3; i++ {
		if err := logger.Log(audit.AuditEvent{
			Source: "test",
			Action: "noop",
			Target: "none",
			Result: "success",
		}); err != nil {
			t.Fatalf("Log() error on iteration %d: %v", i, err)
		}
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d:\n%s", len(lines), buf.String())
	}
}
