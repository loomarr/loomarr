package settings

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
)

// GeneratedSecret is one of the three secrets Loomarr MINTS rather than demands
// (config-design §4): they are created idempotently at first run, env-overridable,
// and displayed per their purpose. They are NOT registry settings (those are
// entered/pinned) — they live here so "generated, never demanded" is a distinct
// code path from the app-managed keys.
type GeneratedSecret string

const (
	SecretSession GeneratedSecret = "session_secret" // signs session cookies (§11); never displayed
	SecretAPI     GeneratedSecret = "api_token"      // machine + break-glass admin; viewable
)

// dbKey is the settings-table key a generated secret persists under.
func (g GeneratedSecret) dbKey() string { return "secret." + string(g) }

// envVar is the env pin that overrides a generated secret (config-design §15).
func (g GeneratedSecret) envVar() string {
	switch g {
	case SecretSession:
		return "SESSION_SECRET"
	case SecretAPI:
		return "API_TOKEN"
	default:
		return ""
	}
}

// Displayable reports whether an admin may view the secret's value (§4). The
// session secret has nothing to paste anywhere → never displayed (Regenerate is
// the only affordance); the API token is an operational value pasted into machine
// clients → viewable.
func (g GeneratedSecret) Displayable() bool { return g != SecretSession }

// SecretStore is the persistence the secrets lifecycle needs (accept-interfaces;
// no store import). The composition root adapts store.Store to it.
type SecretStore interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string) error
}

// Secrets manages the three generated secrets (config-design §4). Values resolve
// env > db (a generated secret is never "default" — it's always minted if unset).
// It also feeds the Redactor so no secret value is ever logged.
type Secrets struct {
	store SecretStore
	env   func(string) (string, bool)

	mu     sync.RWMutex
	values map[GeneratedSecret]string // resolved live values (for redaction + reads)
}

// allGenerated is the fixed set, in display order.
func allGenerated() []GeneratedSecret {
	return []GeneratedSecret{SecretSession, SecretAPI}
}

// NewSecrets resolves (env) or generates+persists (idempotent) each secret, then
// caches the live values. Generation is 256-bit base64url (§4). An env-supplied
// secret wins and is NOT persisted (it's pinned, like any env value).
func NewSecrets(ctx context.Context, store SecretStore, env func(string) (string, bool)) (*Secrets, error) {
	if env == nil {
		env = func(string) (string, bool) { return "", false }
	}
	s := &Secrets{store: store, env: env, values: map[GeneratedSecret]string{}}
	for _, g := range allGenerated() {
		v, err := s.resolveOrMint(ctx, g)
		if err != nil {
			return nil, err
		}
		s.values[g] = v
	}
	return s, nil
}

// resolveOrMint returns env > db, minting + persisting a fresh value if neither
// is set. Idempotent across restarts: once persisted, the same value is read back.
func (s *Secrets) resolveOrMint(ctx context.Context, g GeneratedSecret) (string, error) {
	if v, ok := s.env(g.envVar()); ok && v != "" {
		return v, nil // env pin wins; not persisted
	}
	if v, ok, err := s.store.Get(ctx, g.dbKey()); err != nil {
		return "", fmt.Errorf("read %s: %w", g, err)
	} else if ok && v != "" {
		return v, nil
	}
	fresh, err := generateSecret()
	if err != nil {
		return "", err
	}
	if err := s.store.Set(ctx, g.dbKey(), fresh); err != nil {
		return "", fmt.Errorf("persist %s: %w", g, err)
	}
	return fresh, nil
}

// Value returns a generated secret's live value (for wiring — session signer,
// webhook verifier, token authorizer). Never log this.
func (s *Secrets) Value(g GeneratedSecret) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.values[g]
}

// Regenerate mints a fresh value, persists it, updates the live cache, and
// returns the new value (config-design §4). The CALLER performs the side-effects
// (revoke sessions for SESSION_SECRET, etc.) — this only rotates the value.
// An env-pinned secret cannot be regenerated (the env wins on next read).
func (s *Secrets) Regenerate(ctx context.Context, g GeneratedSecret) (string, error) {
	if v, ok := s.env(g.envVar()); ok && v != "" {
		return "", fmt.Errorf("%s is pinned by %s and cannot be regenerated", g, g.envVar())
	}
	fresh, err := generateSecret()
	if err != nil {
		return "", err
	}
	if err := s.store.Set(ctx, g.dbKey(), fresh); err != nil {
		return "", fmt.Errorf("persist regenerated %s: %w", g, err)
	}
	s.mu.Lock()
	s.values[g] = fresh
	s.mu.Unlock()
	return fresh, nil
}

// RedactionValues returns the current secret values for the Redactor (§4). A
// separate method so the Redactor gets a snapshot without exposing the map.
func (s *Secrets) RedactionValues() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.values))
	for _, v := range s.values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// generateSecret is 256-bit base64url randomness (config-design §4).
func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// preview renders the masked "set · …a1b2" form for a secret VALUE (§4): the last
// 4 chars, never more. Empty value → empty preview.
func preview(value string) string {
	if value == "" {
		return ""
	}
	if len(value) <= 4 {
		return "…" + value
	}
	return "…" + value[len(value)-4:]
}
