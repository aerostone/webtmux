package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCredentialStoreLoadOrCreate(t *testing.T) {
	dir := t.TempDir()
	cs, err := NewCredentialStore(dir)
	if err != nil {
		t.Fatalf("NewCredentialStore: %v", err)
	}
	if cs.HasCredentials() {
		t.Fatal("expected no credentials")
	}
}

func TestCredentialStoreAddAndPersist(t *testing.T) {
	dir := t.TempDir()
	cs, err := NewCredentialStore(dir)
	if err != nil {
		t.Fatalf("NewCredentialStore: %v", err)
	}
	cred := StoredCredential{
		ID:              []byte("test-cred-id"),
		PublicKey:        []byte("test-public-key"),
		AttestationType: "none",
		AAGUID:          make([]byte, 16),
		SignCount:       0,
	}
	if err := cs.AddCredential(cred); err != nil {
		t.Fatalf("AddCredential: %v", err)
	}
	if !cs.HasCredentials() {
		t.Fatal("expected HasCredentials=true")
	}
	if !cs.CredentialExists([]byte("test-cred-id")) {
		t.Fatal("expected CredentialExists=true")
	}

	// Reload from disk
	cs2, err := NewCredentialStore(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !cs2.HasCredentials() {
		t.Fatal("expected persisted credentials")
	}
	if len(cs2.Credentials) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(cs2.Credentials))
	}
	if string(cs2.Credentials[0].ID) != "test-cred-id" {
		t.Fatalf("wrong credential ID: %s", cs2.Credentials[0].ID)
	}
}

func TestCredentialStoreUpdateSignCount(t *testing.T) {
	dir := t.TempDir()
	cs, _ := NewCredentialStore(dir)
	cs.AddCredential(StoredCredential{
		ID:        []byte("cred1"),
		PublicKey:  []byte("pk"),
		SignCount: 0,
	})
	if err := cs.UpdateSignCount([]byte("cred1"), 5); err != nil {
		t.Fatalf("UpdateSignCount: %v", err)
	}
	if cs.Credentials[0].SignCount != 5 {
		t.Fatalf("expected sign count 5, got %d", cs.Credentials[0].SignCount)
	}
}

func TestCredentialStoreClear(t *testing.T) {
	dir := t.TempDir()
	cs, _ := NewCredentialStore(dir)
	cs.AddCredential(StoredCredential{ID: []byte("c1"), PublicKey: []byte("pk")})
	cs.AddCredential(StoredCredential{ID: []byte("c2"), PublicKey: []byte("pk")})
	if len(cs.Credentials) != 2 {
		t.Fatal("expected 2 creds")
	}
	if err := cs.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}
	if cs.HasCredentials() {
		t.Fatal("expected no credentials after clear")
	}
	// Verify persisted cleared state
	cs2, _ := NewCredentialStore(dir)
	if cs2.HasCredentials() {
		t.Fatal("expected clear persisted to disk")
	}
}

func TestCredentialStoreExistsFalse(t *testing.T) {
	dir := t.TempDir()
	cs, _ := NewCredentialStore(dir)
	if cs.CredentialExists([]byte("nonexistent")) {
		t.Fatal("expected false for nonexistent credential")
	}
}

func TestNewWebAuthn(t *testing.T) {
	w, err := NewWebAuthn("localhost", "http://localhost:3400")
	if err != nil {
		t.Fatalf("NewWebAuthn: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil")
	}
}

func TestNewWebAuthnDefaultRP(t *testing.T) {
	// Empty RP ID should fall back to "localhost" (valid domain)
	w, err := NewWebAuthn("", "http://localhost:3400")
	if err != nil {
		t.Fatalf("NewWebAuthn with empty RP: %v", err)
	}
	if w == nil {
		t.Fatal("expected non-nil")
	}
}

func TestWebAuthnUserImplementsInterface(t *testing.T) {
	dir := t.TempDir()
	cs, _ := NewCredentialStore(dir)
	user := NewWebAuthnUser(cs)
	if string(user.WebAuthnID()) != "webtmux-user" {
		t.Fatalf("wrong user ID: %s", user.WebAuthnID())
	}
	if user.WebAuthnName() != "webtmux" {
		t.Fatalf("wrong name: %s", user.WebAuthnName())
	}
	if len(user.WebAuthnCredentials()) != 0 {
		t.Fatal("expected 0 credentials")
	}
}

func TestWebAuthnUserWithCredentials(t *testing.T) {
	dir := t.TempDir()
	cs, _ := NewCredentialStore(dir)
	cs.AddCredential(StoredCredential{
		ID:        []byte("cred1"),
		PublicKey:  []byte("pk1"),
		SignCount: 3,
	})
	user := NewWebAuthnUser(cs)
	creds := user.WebAuthnCredentials()
	if len(creds) != 1 {
		t.Fatalf("expected 1 credential, got %d", len(creds))
	}
	if string(creds[0].ID) != "cred1" {
		t.Fatalf("wrong cred ID")
	}
	if creds[0].Authenticator.SignCount != 3 {
		t.Fatalf("wrong sign count: %d", creds[0].Authenticator.SignCount)
	}
}

func TestCredentialStoreFilePermissions(t *testing.T) {
	dir := t.TempDir()
	cs, _ := NewCredentialStore(dir)
	cs.AddCredential(StoredCredential{ID: []byte("c"), PublicKey: []byte("pk")})
	info, err := os.Stat(filepath.Join(dir, "credentials.json"))
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	// File should be 0600 (owner read/write only)
	if info.Mode().Perm() != 0600 {
		t.Fatalf("expected perm 0600, got %o", info.Mode().Perm())
	}
}
