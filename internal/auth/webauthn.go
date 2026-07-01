package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/go-webauthn/webauthn/webauthn"
)

// StoredCredential represents a registered authenticator (fingerprint/face).
type StoredCredential struct {
	ID              []byte `json:"id"`
	PublicKey        []byte `json:"public_key"`
	AttestationType string `json:"attestation_type"`
	AAGUID          []byte `json:"aaguid"`
	SignCount       uint32 `json:"sign_count"`
}

// CredentialStore persists WebAuthn credentials to a JSON file.
type CredentialStore struct {
	mu          sync.RWMutex
	path        string
	Credentials []StoredCredential `json:"credentials"`
}

// NewCredentialStore loads or creates a credential store at dir/credentials.json.
func NewCredentialStore(dir string) (*CredentialStore, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	p := filepath.Join(dir, "credentials.json")
	cs := &CredentialStore{path: p}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return cs, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, cs); err != nil {
		return nil, err
	}
	return cs, nil
}

func (cs *CredentialStore) save() error {
	data, err := json.MarshalIndent(cs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cs.path, data, 0600)
}

// HasCredentials returns true if at least one credential is registered.
func (cs *CredentialStore) HasCredentials() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.Credentials) > 0
}

// AddCredential stores a new credential.
func (cs *CredentialStore) AddCredential(c StoredCredential) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.Credentials = append(cs.Credentials, c)
	return cs.save()
}

// UpdateSignCount updates the sign count for a credential.
func (cs *CredentialStore) UpdateSignCount(credID []byte, count uint32) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	for i, c := range cs.Credentials {
		if string(c.ID) == string(credID) {
			cs.Credentials[i].SignCount = count
			return cs.save()
		}
	}
	return nil
}

// Clear removes all credentials.
func (cs *CredentialStore) Clear() error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.Credentials = nil
	return cs.save()
}

// ─── WebAuthn User (implements webauthn.User) ───

// WebAuthnUser is a single-user wrapper for webtmux.
type WebAuthnUser struct {
	credStore *CredentialStore
	userID    []byte
	name      string
}

// NewWebAuthnUser creates the single user for webtmux.
func NewWebAuthnUser(store *CredentialStore) *WebAuthnUser {
	return &WebAuthnUser{
		credStore: store,
		userID:    []byte("webtmux-user"),
		name:      "webtmux",
	}
}

func (u *WebAuthnUser) WebAuthnID() []byte                          { return u.userID }
func (u *WebAuthnUser) WebAuthnName() string                        { return u.name }
func (u *WebAuthnUser) WebAuthnDisplayName() string                 { return "webtmux" }
func (u *WebAuthnUser) WebAuthnIcon() string                        { return "" }
func (u *WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	u.credStore.mu.RLock()
	defer u.credStore.mu.RUnlock()
	creds := make([]webauthn.Credential, len(u.credStore.Credentials))
	for i, c := range u.credStore.Credentials {
		creds[i] = webauthn.Credential{
			ID:              c.ID,
			PublicKey:        c.PublicKey,
			AttestationType: c.AttestationType,
			Authenticator: webauthn.Authenticator{
				AAGUID:    c.AAGUID,
				SignCount: c.SignCount,
			},
		}
	}
	return creds
}

// ─── WebAuthn instance factory ───

// NewWebAuthn creates a configured webauthn.WebAuthn instance.
func NewWebAuthn(relyingPartyID string, origin string) (*webauthn.WebAuthn, error) {
	if relyingPartyID == "" {
		relyingPartyID = "localhost"
	}
	w, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "webtmux",
		RPID:          relyingPartyID,
		RPOrigins:     []string{origin},
	})
	if err != nil {
		return nil, fmt.Errorf("webauthn init: %w", err)
	}
	return w, nil
}

// GenerateChallenge creates a random 32-byte challenge encoded as base64url.
func GenerateChallenge() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// CredentialToStored converts a webauthn.Credential to StoredCredential.
func CredentialToStored(c *webauthn.Credential) StoredCredential {
	return StoredCredential{
		ID:              c.ID,
		PublicKey:        c.PublicKey,
		AttestationType: c.AttestationType,
		AAGUID:          c.Authenticator.AAGUID,
		SignCount:       c.Authenticator.SignCount,
	}
}

// SaveCredential persists a credential from a registration response.
func (cs *CredentialStore) SaveCredential(c *webauthn.Credential) error {
	stored := CredentialToStored(c)
	return cs.AddCredential(stored)
}

// SaveCredentialFromParsed saves a credential from parsed registration data.
// This is a convenience for the HTTP handler layer.
func (cs *CredentialStore) SaveCredentialFromParsed(cred *webauthn.Credential) error {
	return cs.SaveCredential(cred)
}

// CredentialExists returns true if a credential with the given ID exists.
func (cs *CredentialStore) CredentialExists(credID []byte) bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	for _, c := range cs.Credentials {
		if string(c.ID) == string(credID) {
			return true
		}
	}
	return false
}


