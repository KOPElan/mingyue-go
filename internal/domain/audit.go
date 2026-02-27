// Package domain contains pure data structures shared across services, CLI
// handlers, and HTTP handlers.  No business logic lives here.
package domain

import "time"

// AuditEvent mirrors the audit.AuditEvent structure at the domain layer so
// that service packages can reference it without importing internal/audit.
type AuditEvent struct {
	Time      time.Time `json:"time"`
	Source    string    `json:"source"`
	Action    string    `json:"action"`
	Target    string    `json:"target"`
	Result    string    `json:"result"`
	ErrorCode string    `json:"error_code,omitempty"`
}
