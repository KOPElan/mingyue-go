// keystore.go provides persistent API key management backed by a JSON file.
// Keys are stored in a JSON array on disk and loaded into the in-memory store
// at agent start-up.  File permissions are 0600 so that only the owner can read
// the credentials.
package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// DefaultKeystorePath is the default location for the API key store file.
const DefaultKeystorePath = "/var/lib/mingyue/apikeys.json"

// KeyEntry represents one API key entry persisted to disk.
type KeyEntry struct {
	// Key is the opaque bearer token value.
	Key string `json:"key"`
	// Role is the role assigned to the token.
	Role Role `json:"role"`
	// Subject is a human-readable label identifying the principal.
	Subject string `json:"subject"`
	// CreatedAt is the UTC creation timestamp.
	CreatedAt time.Time `json:"created_at"`
}

// GenerateKey returns a cryptographically secure random 32-byte hex key string.
func GenerateKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate key: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// LoadKeyStore reads the keystore file at path, registers every key into the
// in-memory store, and returns the slice of entries.  If the file does not
// exist the function returns (nil, nil) without error.
func LoadKeyStore(path string) ([]KeyEntry, error) {
	entries, err := loadEntries(path)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		RegisterAPIKey(e.Key, Token{Raw: e.Key, Role: e.Role, Subject: e.Subject})
	}
	return entries, nil
}

// SaveKeyEntry appends entry to the keystore file, creating it if necessary,
// then registers the key in the in-memory store.
func SaveKeyEntry(path string, entry KeyEntry) error {
	entries, err := loadEntries(path)
	if err != nil {
		return err
	}
	entries = append(entries, entry)
	if err := writeEntries(path, entries); err != nil {
		return err
	}
	RegisterAPIKey(entry.Key, Token{Raw: entry.Key, Role: entry.Role, Subject: entry.Subject})
	return nil
}

// ListKeyEntries returns all entries stored in the keystore file.
func ListKeyEntries(path string) ([]KeyEntry, error) {
	return loadEntries(path)
}

// RevokeKey removes the key from both the keystore file and the in-memory
// store.  Returns an error when the key does not exist.
func RevokeKey(path, key string) error {
	entries, err := loadEntries(path)
	if err != nil {
		return err
	}
	filtered := entries[:0]
	found := false
	for _, e := range entries {
		if e.Key == key {
			found = true
			continue
		}
		filtered = append(filtered, e)
	}
	if !found {
		return fmt.Errorf("key not found in keystore")
	}
	if err := writeEntries(path, filtered); err != nil {
		return err
	}
	// Remove from the shared in-memory store.
	keysMu.Lock()
	delete(keys, key)
	keysMu.Unlock()
	return nil
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func loadEntries(path string) ([]KeyEntry, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read keystore %s: %w", path, err)
	}
	var entries []KeyEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse keystore %s: %w", path, err)
	}
	return entries, nil
}

func writeEntries(path string, entries []KeyEntry) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create keystore dir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal keystore: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}
