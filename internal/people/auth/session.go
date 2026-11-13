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
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

// CookieName is the session cookie (§11).
const CookieName = "loomarr_session"

// SessionStore is the exact persistence role session issuance and validation
// require. Keeping this seam local prevents auth from learning the unrelated
// acquisition, channel, filler, and scheduler surfaces on store.Store.
type SessionStore interface {
	CreateSession(ctx context.Context, sess identitystore.Session) error
	GetSession(ctx context.Context, tokenHash string, now time.Time) (identitystore.Session, error)
	GetUser(ctx context.Context, id string) (identitystore.User, error)
	TouchSession(ctx context.Context, tokenHash string, seen identitystore.SessionSeen) error
	RevokeSession(ctx context.Context, tokenHash string) error
	ListSessionsForUser(ctx context.Context, userID string, now time.Time) ([]identitystore.Session, error)
}

// Manager issues, validates, and revokes sessions.
type Manager struct {
	store SessionStore
	ttl   time.Duration // sliding SESSION_TTL (§11)
	now   func() time.Time
	touch touchWarner
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
// (stored only as its hash). Caller sets the cookie. A user disabled (or removed) since the caller
// checked is refused with ErrInvalidCredentials, like every other disabled login.
func (m *Manager) Issue(ctx context.Context, userID string) (token string, expires time.Time, err error) {
	token, session, err := m.prepare(userID)
	if err != nil {
		return "", time.Time{}, err
	}
	err = m.store.CreateSession(ctx, session)
	if errors.Is(err, store.ErrNotFound) {
		return "", time.Time{}, ErrInvalidCredentials
	}
	if err != nil {
		return "", time.Time{}, err
	}
	return token, session.ExpiresAt, nil
}

// prepare creates the plaintext/stored halves of a session without persisting
// either. Invitation redemption uses this to commit the account and session in
// the same database transaction; ordinary login persists it immediately above.
func (m *Manager) prepare(userID string) (string, identitystore.Session, error) {
	token, err := newToken()
	if err != nil {
		return "", identitystore.Session{}, err
	}
	now := m.now()
	return token, identitystore.Session{
		TokenHash: hashToken(token), UserID: userID, CreatedAt: now, ExpiresAt: now.Add(m.ttl),
	}, nil
}

// Resolve validates a cookie token → the user, sliding the session's expiry. It
// returns store.ErrNotFound for an unknown/expired token, and enforces that the
// user still exists and is not disabled (§11: a disabled user's sessions must
// not authenticate — belt-and-suspenders with immediate revocation on disable).
// clientLabel names the requesting client ("Firefox on macOS", "" if unknown); the
// session keeps it with the time of this use for "Where you're signed in" (#1667).
func (m *Manager) Resolve(ctx context.Context, token, clientLabel string) (identitystore.User, error) {
	now := m.now()
	sess, err := m.store.GetSession(ctx, hashToken(token), now)
	if err != nil {
		return identitystore.User{}, err
	}
	u, err := m.store.GetUser(ctx, sess.UserID)
	if err != nil {
		return identitystore.User{}, err
	}
	if u.Disabled {
		// Defense in depth: even if revocation missed a session, a disabled user
		// never authenticates.
		return identitystore.User{}, store.ErrNotFound
	}
	// Slide the expiry (§11 sliding TTL) and record the use, at most once per touchGranularity.
	if touchDue(sess.LastSeenAt, now) || (clientLabel != "" && clientLabel != sess.ClientLabel) {
		// Best-effort: a failed write must not fail the request that carried a valid cookie, but it
		// is reported, since a store that keeps failing stops the expiry from sliding.
		if err := m.store.TouchSession(ctx, hashToken(token), identitystore.SessionSeen{At: now, ExpiresAt: now.Add(m.ttl), ClientLabel: clientLabel}); err != nil {
			m.touch.warn(ctx, "session", now, err)
		}
	}
	return u, nil
}

// touchGranularity is how stale a recorded use may get before a request records a new one (#2446).
// Writing the row on every authenticated request would cost dozens of write transactions per page
// load, each waiting for SQLite's one write lock. "Last seen" reads "now" or "2h ago", and a sliding expiry measured in weeks loses
// nothing by moving a minute late, so a minute is plenty — the same granularity the person-level
// last_seen_at already uses.
const touchGranularity = time.Minute

// touchDue reports whether a credential last used at lastSeen should record a use at now.
func touchDue(lastSeen, now time.Time) bool {
	return now.Sub(lastSeen) >= touchGranularity
}

// touchWarner reports a failed use-recording write without flooding the log: a failing store fails
// every authenticated request, so it warns at most once per touchGranularity. The zero value is
// ready to use.
type touchWarner struct {
	last atomic.Int64 // unix nanoseconds of the last warning
}

// warn logs err for the named credential kind when no warning has been logged within
// touchGranularity of now.
func (w *touchWarner) warn(ctx context.Context, kind string, now time.Time, err error) {
	prev := w.last.Load()
	if prev != 0 && now.Sub(time.Unix(0, prev)) < touchGranularity {
		return
	}
	if !w.last.CompareAndSwap(prev, now.UnixNano()) {
		return
	}
	slog.WarnContext(ctx, "could not record a credential use; its last-seen time and sliding expiry are not advancing",
		"event", "auth.touch_failed", "subsystem", "auth", "credential", kind, "err", err)
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
func (m *Manager) List(ctx context.Context, userID string) ([]identitystore.Session, error) {
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
