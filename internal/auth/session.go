// Package auth issues and validates Loomarr sessions (design §11). Sessions are
// revocable store rows (not JWTs) so disabling a user kills their sessions
// immediately. The cookie carries a random 256-bit token; only its SHA-256 hash
// is stored, so a DB read never yields a usable cookie.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/store"
)

// CookieName is the session cookie (§11).
const CookieName = "loomarr_session"

// SessionStore is the exact persistence role session issuance and validation
// require. Keeping this seam local prevents auth from learning the unrelated
// acquisition, channel, filler, and scheduler surfaces on store.Store.
type SessionStore interface {
	CreateSession(ctx context.Context, sess store.Session) error
	GetSession(ctx context.Context, tokenHash string, now time.Time) (store.Session, error)
	GetUser(ctx context.Context, id string) (store.User, error)
	TouchSession(ctx context.Context, tokenHash string, expiresAt time.Time) error
	RevokeSession(ctx context.Context, tokenHash string) error
	ListSessionsForUser(ctx context.Context, userID string, now time.Time) ([]store.Session, error)
}

// Manager issues, validates, and revokes sessions.
type Manager struct {
	store SessionStore
	ttl   time.Duration // sliding SESSION_TTL (§11)
	now   func() time.Time
}

// NewManager builds a session manager. now defaults to time.Now.
func NewManager(st SessionStore, ttl time.Duration, now func() time.Time) *Manager {
	if now == nil {
		now = time.Now
	}
	if ttl <= 0 {
		ttl = 720 * time.Hour
	}
	return &Manager{store: st, ttl: ttl, now: now}
}

// hashToken returns the SHA-256 hex of a token — what's stored at rest (§11).
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// newToken returns a fresh random 256-bit token, hex-encoded (§11).
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Issue creates a session for a user and returns the plaintext cookie token
// (stored only as its hash). Caller sets the cookie.
func (m *Manager) Issue(ctx context.Context, userID string) (token string, expires time.Time, err error) {
	token, err = newToken()
	if err != nil {
		return "", time.Time{}, err
	}
	now := m.now()
	expires = now.Add(m.ttl)
	err = m.store.CreateSession(ctx, store.Session{
		TokenHash: hashToken(token), UserID: userID, CreatedAt: now, ExpiresAt: expires,
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// Resolve validates a cookie token → the user, sliding the session's expiry. It
// returns store.ErrNotFound for an unknown/expired token, and enforces that the
// user still exists and is not disabled (§11: a disabled user's sessions must
// not authenticate — belt-and-suspenders with immediate revocation on disable).
func (m *Manager) Resolve(ctx context.Context, token string) (store.User, error) {
	now := m.now()
	sess, err := m.store.GetSession(ctx, hashToken(token), now)
	if err != nil {
		return store.User{}, err
	}
	u, err := m.store.GetUser(ctx, sess.UserID)
	if err != nil {
		return store.User{}, err
	}
	if u.Disabled {
		// Defense in depth: even if revocation missed a session, a disabled user
		// never authenticates.
		return store.User{}, store.ErrNotFound
	}
	// Slide the expiry (§11 sliding TTL).
	_ = m.store.TouchSession(ctx, hashToken(token), now.Add(m.ttl))
	return u, nil
}

// Revoke ends a single session (logout).
func (m *Manager) Revoke(ctx context.Context, token string) error {
	return m.store.RevokeSession(ctx, hashToken(token))
}

// List returns a user's live sessions (§11). The returned Sessions carry the stored
// token HASH, never a token: an admin reviewing "who is signed in" needs a stable handle
// to revoke by, and the hash is already that handle. Returning it preserves the schema's
// guarantee that a read never yields a usable cookie — SHA-256 is preimage-resistant, so
// the hash cannot be turned back into the cookie it authenticates.
func (m *Manager) List(ctx context.Context, userID string) ([]store.Session, error) {
	return m.store.ListSessionsForUser(ctx, userID, m.now())
}

// RevokeHash ends a single session addressed by its stored hash — the admin-facing
// counterpart to Revoke, which takes the plaintext token only the session's owner holds.
func (m *Manager) RevokeHash(ctx context.Context, tokenHash string) error {
	return m.store.RevokeSession(ctx, tokenHash)
}

// IsCurrent reports whether a stored hash belongs to the caller's own token. The UI uses
// it to mark "this device" so an admin does not sign themselves out by accident.
func IsCurrent(tokenHash, callerToken string) bool {
	return callerToken != "" && tokenHash == hashToken(callerToken)
}
