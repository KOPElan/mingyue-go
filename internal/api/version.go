// Package api contains the mingyue HTTP API handler and router.
// This file exports the build-time version so that the health endpoint
// and other handlers can reference it from a single source of truth.
package api

// Version is the current application version.  Override at build time with:
//
//	go build -ldflags "-X kopelan/mingyue-go/internal/api.Version=v1.2.3"
var Version = "dev"
