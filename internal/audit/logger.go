// Package audit provides structured audit logging for the mingyue agent.
// All mutating operations must record an AuditEvent so that operators can
// reconstruct who did what and when.
//
// Log format is JSON Lines (one JSON object per line) written to
// /var/log/mingyue/audit.log.  In development environments (or when the path
// is not writable) output falls back to os.Stderr.
package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

const defaultLogPath = "/var/log/mingyue/audit.log"

// AuditEvent represents a single auditable action performed by a principal.
type AuditEvent struct {
	// Time is when the action occurred (UTC).
	Time time.Time `json:"time"`
	// Source identifies the caller: "cli", "api", or a remote address.
	Source string `json:"source"`
	// Action is the operation name, e.g. "disk.mount", "file.delete".
	Action string `json:"action"`
	// Target is the resource affected, e.g. a path, PID, or share name.
	Target string `json:"target"`
	// Result is "success" or "failure".
	Result string `json:"result"`
	// ErrorCode is the machine-readable error code on failure, empty on success.
	ErrorCode string `json:"error_code,omitempty"`
}

// Logger is the interface for writing audit events.
type Logger interface {
	Log(event AuditEvent) error
	Close() error
}

// FileLogger writes JSON Lines to a file (or a fallback writer).
type FileLogger struct {
	mu     sync.Mutex
	writer io.Writer
	closer io.Closer // non-nil only when we opened the file ourselves
}

// NewFileLogger opens the audit log file at path.
// If path is empty, defaultLogPath is used.
// If the file cannot be opened (e.g. in a dev environment without /var/log),
// writes fall back to os.Stderr.
func NewFileLogger(path string) *FileLogger {
	if path == "" {
		path = defaultLogPath
	}

	// Ensure the parent directory exists (best-effort).
	dir := dirOf(path)
	if dir != "" {
		_ = os.MkdirAll(dir, 0o750)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o640)
	if err != nil {
		// Development fallback: write to Stderr.
		return &FileLogger{writer: os.Stderr}
	}
	return &FileLogger{writer: f, closer: f}
}

// NewWriterLogger creates a FileLogger that writes to the provided writer.
// Useful for testing.
func NewWriterLogger(w io.Writer) *FileLogger {
	return &FileLogger{writer: w}
}

// Log serialises event as a JSON line and writes it to the underlying writer.
func (l *FileLogger) Log(event AuditEvent) error {
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("audit: marshal event: %w", err)
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	_, err = fmt.Fprintf(l.writer, "%s\n", data)
	return err
}

// Close releases any file handle held by the logger.
func (l *FileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closer != nil {
		return l.closer.Close()
	}
	return nil
}

// dirOf returns the directory component of path using a simple byte scan so
// that the audit package does not need to import "path/filepath" for just this.
func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return path[:i]
		}
	}
	return ""
}
