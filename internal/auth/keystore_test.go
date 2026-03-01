package auth_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"kopelan/mingyue-go/internal/auth"
)

// tempKeystore creates a temporary file path in t.TempDir() for keystore tests.
func tempKeystore(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "apikeys.json")
}

func TestGenerateKey_Length(t *testing.T) {
	key, err := auth.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error: %v", err)
	}
	// 32 random bytes → 64 hex chars.
	if len(key) != 64 {
		t.Errorf("GenerateKey() len = %d, want 64", len(key))
	}
}

func TestGenerateKey_Uniqueness(t *testing.T) {
	k1, _ := auth.GenerateKey()
	k2, _ := auth.GenerateKey()
	if k1 == k2 {
		t.Error("GenerateKey() returned identical keys on consecutive calls")
	}
}

func TestLoadKeyStore_NoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nonexistent.json")
	entries, err := auth.LoadKeyStore(path)
	if err != nil {
		t.Fatalf("LoadKeyStore() on non-existent path: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestSaveAndLoadKeyEntry(t *testing.T) {
	path := tempKeystore(t)

	entry := auth.KeyEntry{
		Key:       "test-key-save-load",
		Role:      auth.RoleAdmin,
		Subject:   "test-subject",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
	}

	if err := auth.SaveKeyEntry(path, entry); err != nil {
		t.Fatalf("SaveKeyEntry() error: %v", err)
	}

	entries, err := auth.ListKeyEntries(path)
	if err != nil {
		t.Fatalf("ListKeyEntries() error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Key != entry.Key {
		t.Errorf("Key = %q, want %q", entries[0].Key, entry.Key)
	}
	if entries[0].Role != entry.Role {
		t.Errorf("Role = %q, want %q", entries[0].Role, entry.Role)
	}
	if entries[0].Subject != entry.Subject {
		t.Errorf("Subject = %q, want %q", entries[0].Subject, entry.Subject)
	}
}

func TestSaveKeyEntry_RegistersKey(t *testing.T) {
	path := tempKeystore(t)
	key := "test-key-registers"

	entry := auth.KeyEntry{
		Key:       key,
		Role:      auth.RoleOperator,
		Subject:   "svc",
		CreatedAt: time.Now().UTC(),
	}

	if err := auth.SaveKeyEntry(path, entry); err != nil {
		t.Fatalf("SaveKeyEntry() error: %v", err)
	}

	// The key should now be usable for authentication.
	role, err := auth.Validate(key)
	if err != nil {
		t.Fatalf("Validate() after SaveKeyEntry() error: %v", err)
	}
	if role != auth.RoleOperator {
		t.Errorf("role = %q, want %q", role, auth.RoleOperator)
	}
}

func TestLoadKeyStore_RegistersAllKeys(t *testing.T) {
	path := tempKeystore(t)

	for _, e := range []auth.KeyEntry{
		{Key: "load-key-viewer", Role: auth.RoleViewer, Subject: "v", CreatedAt: time.Now().UTC()},
		{Key: "load-key-admin", Role: auth.RoleAdmin, Subject: "a", CreatedAt: time.Now().UTC()},
	} {
		if err := auth.SaveKeyEntry(path, e); err != nil {
			t.Fatalf("SaveKeyEntry() error: %v", err)
		}
	}

	// Re-load into a fresh run (simulate restart by calling LoadKeyStore).
	entries, err := auth.LoadKeyStore(path)
	if err != nil {
		t.Fatalf("LoadKeyStore() error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	// Both keys must be reachable.
	for _, key := range []string{"load-key-viewer", "load-key-admin"} {
		if _, err := auth.Validate(key); err != nil {
			t.Errorf("Validate(%q) after LoadKeyStore() error: %v", key, err)
		}
	}
}

func TestRevokeKey(t *testing.T) {
	path := tempKeystore(t)
	key := "test-key-revoke"

	if err := auth.SaveKeyEntry(path, auth.KeyEntry{
		Key: key, Role: auth.RoleAdmin, Subject: "a", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveKeyEntry() error: %v", err)
	}

	if err := auth.RevokeKey(path, key); err != nil {
		t.Fatalf("RevokeKey() error: %v", err)
	}

	// File must no longer contain the key.
	entries, _ := auth.ListKeyEntries(path)
	for _, e := range entries {
		if e.Key == key {
			t.Error("revoked key still present in keystore file")
		}
	}

	// In-memory store must reject the key.
	_, err := auth.Validate(key)
	if err == nil {
		t.Error("Validate() expected error for revoked key, got nil")
	}
}

func TestRevokeKey_NotFound(t *testing.T) {
	path := tempKeystore(t)
	err := auth.RevokeKey(path, "no-such-key")
	if err == nil {
		t.Fatal("RevokeKey() on missing key expected error, got nil")
	}
}

func TestWriteEntries_FilePermissions(t *testing.T) {
	path := tempKeystore(t)

	if err := auth.SaveKeyEntry(path, auth.KeyEntry{
		Key: "perm-test", Role: auth.RoleViewer, Subject: "x", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveKeyEntry() error: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file permissions = %o, want 600", info.Mode().Perm())
	}
}
