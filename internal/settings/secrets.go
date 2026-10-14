package settings

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"
)

// GeneratedSecret is one of the two tokens Loomarr MINTS rather than demands
// (config-design §4): they are created idempotently at first run, env-overridable,
// and displayed per their purpose. They are NOT registry settings (those are
// entered/pinned) — they live here so "generated, never demanded" is a distinct
// code path from the app-managed keys.
type GeneratedSecret string

const (
	SecretAPI GeneratedSecret = "api_token" // machine + break-glass admin; viewable
	// SecretPlayout signs every internal-playout segment request (§9.1), so only the
	// operator's media server can pull a stream. Viewable BECAUSE it must be pasted
	// into a tuner/listings URL by hand when auto-wiring isn't used.
	//
	// It authenticates a DEVICE, not a person (§11) — a television cannot hold a
	// session cookie. Deliberately NOT the same secret as api_token: that one is
	// break-glass ADMIN with full authority; this one grants nothing beyond reading
	// streams. Conflating them would hand an appliance the keys to the instance.
	SecretPlayout GeneratedSecret = "playout_token"
)

// dbKey is the settings-table key a generated secret persists under.
func (g GeneratedSecret) dbKey() string { return "secret." + string(g) }

// envVar is the env pin that overrides a generated secret (config-design §15).
func (g GeneratedSecret) envVar() string {
	switch g {
	case SecretAPI:
		return "API_TOKEN"
	case SecretPlayout:
		return "PLAYOUT_TOKEN"
	default:
		return ""
	}
}

// SecretStore is the persistence the secrets lifecycle needs (accept-interfaces;
// no store import). The composition root adapts store.Store to it.
type SecretStore interface {
	Get(ctx context.Context, key string) (string, bool, error)
	Set(ctx context.Context, key, value string) error
}

// Secrets manages the two generated tokens (config-design §4). Values resolve
// env > db (a generated secret is never "default" — it's always minted if unset).
// It also feeds the Redactor so no secret value is ever logged.
type Secrets struct {
	store SecretStore
	env   func(string) (string, bool)

	mu      sync.RWMutex
	values  map[GeneratedSecret]string // resolved live values (for redaction + reads)
	webPush WebPushIdentity
}

// WebPushIdentity is the installation's stable RFC 8292 application-server identity. The public
// key is returned to browsers during explicit subscription; PrivateKey never crosses that API.
type WebPushIdentity struct {
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
}

const webPushIdentityKey = "secret.web_push_vapid_identity"

// allGenerated is the fixed set, in display order.
func allGenerated() []GeneratedSecret {
	return []GeneratedSecret{SecretAPI, SecretPlayout}
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
	if err := s.resolveWebPushIdentity(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Secrets) resolveWebPushIdentity(ctx context.Context) error {
	resolve := func(lockCtx context.Context) error {
		if raw, ok, err := s.store.Get(lockCtx, webPushIdentityKey); err != nil {
			return fmt.Errorf("read Web Push identity: %w", err)
		} else if ok {
			identity, decodeErr := decodeWebPushIdentity(raw)
			if decodeErr != nil {
				return decodeErr
			}
			s.webPush = identity
			return nil
		}
		identity, err := generateWebPushIdentity()
		if err != nil {
			return err
		}
		raw, err := json.Marshal(identity)
		if err != nil {
			return fmt.Errorf("encode Web Push identity: %w", err)
		}
		if err := s.store.Set(lockCtx, webPushIdentityKey, string(raw)); err != nil {
			return fmt.Errorf("persist Web Push identity: %w", err)
		}
		s.webPush = identity
		return nil
	}
	if locker, ok := s.store.(interface {
		WithLock(context.Context, string, func(context.Context) error) error
	}); ok {
		return locker.WithLock(ctx, webPushIdentityKey, resolve)
	}
	return resolve(ctx)
}

func generateWebPushIdentity() (WebPushIdentity, error) {
	private, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return WebPushIdentity{}, fmt.Errorf("generate Web Push identity: %w", err)
	}
	return WebPushIdentity{
		PrivateKey: base64.RawURLEncoding.EncodeToString(private.Bytes()),
		PublicKey:  base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()),
	}, nil
}

func decodeWebPushIdentity(raw string) (WebPushIdentity, error) {
	var identity WebPushIdentity
	if err := json.Unmarshal([]byte(raw), &identity); err != nil {
		return WebPushIdentity{}, fmt.Errorf("decode Web Push identity: %w", err)
	}
	privateBytes, privateErr := base64.RawURLEncoding.DecodeString(identity.PrivateKey)
	publicBytes, publicErr := base64.RawURLEncoding.DecodeString(identity.PublicKey)
	curve := ecdh.P256()
	private, privateKeyErr := curve.NewPrivateKey(privateBytes)
	public, publicKeyErr := curve.NewPublicKey(publicBytes)
	if privateErr != nil || publicErr != nil || privateKeyErr != nil || publicKeyErr != nil {
		return WebPushIdentity{}, fmt.Errorf("decode Web Push identity: invalid P-256 key pair")
	}
	if !private.PublicKey().Equal(public) {
		return WebPushIdentity{}, fmt.Errorf("decode Web Push identity: public and private keys do not match")
	}
	return identity, nil
}

func (s *Secrets) WebPushIdentity() WebPushIdentity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.webPush
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

// Value returns a generated token's live value for authorization and URL wiring. Never log this.
func (s *Secrets) Value(g GeneratedSecret) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.values[g]
}

// Current returns the generated secret currently authoritative in the durable
// store and updates this process's cache before returning it. Postgres replicas
// use this at security and publication boundaries: Regenerate updates the
// handling process immediately, but another replica cannot otherwise observe the
// new value until restart. Environment pins remain authoritative and never touch
// the database.
//
// Unlike resolveOrMint, Current never creates a missing value. NewSecrets owns
// generation during boot; silently minting on a request path would let a
// transient/missing row rotate a credential without running its required side
// effects.
func (s *Secrets) Current(ctx context.Context, g GeneratedSecret) (string, error) {
	if s == nil || s.store == nil {
		return "", fmt.Errorf("read %s: secret store is unavailable", g)
	}
	if g.envVar() == "" {
		return "", fmt.Errorf("read %s: unknown generated secret", g)
	}
	if v, ok := s.env(g.envVar()); ok && v != "" {
		s.cache(g, v)
		return v, nil
	}
	v, ok, err := s.store.Get(ctx, g.dbKey())
	if err != nil {
		return "", fmt.Errorf("read %s: %w", g, err)
	}
	if !ok || v == "" {
		return "", fmt.Errorf("read %s: durable value is missing", g)
	}
	s.cache(g, v)
	return v, nil
}

func (s *Secrets) cache(g GeneratedSecret, value string) {
	s.mu.Lock()
	s.values[g] = value
	s.mu.Unlock()
}

// Regenerate mints a fresh value, persists it, updates the live cache, and
// returns the new value (config-design §4). The caller performs token-specific
// publication side-effects; this method only rotates the value.
// An env-pinned secret cannot be regenerated (the env wins on next read).
func (s *Secrets) Regenerate(ctx context.Context, g GeneratedSecret) (string, error) {
	if g.envVar() == "" {
		return "", fmt.Errorf("regenerate %s: unknown generated secret", g)
	}
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
	s.cache(g, fresh)
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
	if s.webPush.PrivateKey != "" {
		out = append(out, s.webPush.PrivateKey)
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
