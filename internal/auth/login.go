package auth

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/store"
)

// ErrRateLimited is returned when login attempts exceed the per-IP+username rate.
var ErrRateLimited = errors.New("too many login attempts")

// ErrInvalidCredentials is returned for a bad username/password.
var ErrInvalidCredentials = errors.New("invalid credentials")

// Authenticator verifies media-server credentials (the Phase-5 library adapter).
type Authenticator interface {
	AuthenticateByName(ctx context.Context, username, password string) (library.User, error)
}

// Limiter gates login attempts (per IP+username). Implemented over
// golang.org/x/time/rate in ratelimit.go.
type Limiter interface {
	Allow(key string) bool
}

// LoginService ties credential verification, user upsert + bootstrap, and
// session issuance together (§11).
type LoginService struct {
	lib     Authenticator
	store   store.Store
	mgr     *Manager
	limiter Limiter
	now     func() time.Time
}

// NewLoginService builds the login flow.
func NewLoginService(lib Authenticator, st store.Store, mgr *Manager, limiter Limiter, now func() time.Time) *LoginService {
	if now == nil {
		now = time.Now
	}
	return &LoginService{lib: lib, store: st, mgr: mgr, limiter: limiter, now: now}
}

// Login authenticates a user and issues a session (§11). Loomarr owns identity:
// the DB is the allowlist, so a user unknown to Loomarr is rejected regardless of
// media-server credentials — there is NO lazy self-provision. Two credential
// paths land on the one identity:
//
//   - LOCAL user (a row whose PasswordHash is set): verify the password against
//     that bcrypt hash, entirely in-app.
//   - IMPORTED media-server user (an allowlisted row with no hash): verify the
//     password against the media server, then confirm the returned id is
//     allowlisted.
//
// Every failure — unknown user, wrong password, not-imported, disabled — returns
// the same ErrInvalidCredentials, so login never reveals which users exist.
// rateKey is IP+username. Returns the cookie token, its expiry, and the user.
func (s *LoginService) Login(ctx context.Context, username, password, rateKey string) (token string, expires time.Time, u store.User, err error) {
	if s.limiter != nil && !s.limiter.Allow(rateKey) {
		return "", time.Time{}, store.User{}, ErrRateLimited
	}

	u, err = s.authenticate(ctx, username, password)
	if err != nil {
		return "", time.Time{}, store.User{}, err
	}
	if u.Disabled {
		return "", time.Time{}, store.User{}, ErrInvalidCredentials
	}

	token, expires, err = s.mgr.Issue(ctx, u.ID)
	if err != nil {
		return "", time.Time{}, store.User{}, err
	}
	return token, expires, u, nil
}

// authenticate resolves a username to an allowlisted user and verifies the
// password on the appropriate credential path (§11). A local match (has a hash)
// verifies in-app; otherwise it delegates to the media server AND confirms the
// resulting id is allowlisted. Returns ErrInvalidCredentials for every failure.
func (s *LoginService) authenticate(ctx context.Context, username, password string) (store.User, error) {
	// Local path: a row with this name that carries a password hash.
	if local, err := s.store.GetUserByName(ctx, username); err == nil && local.PasswordHash != "" {
		if bcrypt.CompareHashAndPassword([]byte(local.PasswordHash), []byte(password)) != nil {
			return store.User{}, ErrInvalidCredentials
		}
		return s.refreshOnLogin(ctx, local, local.Name, local.Disabled)
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.User{}, err
	}

	// Imported path: verify against the media server, then enforce the allowlist.
	if s.lib == nil {
		return store.User{}, ErrInvalidCredentials // no media server configured
	}
	msUser, err := s.lib.AuthenticateByName(ctx, username, password)
	if err != nil {
		return store.User{}, ErrInvalidCredentials
	}
	existing, err := s.store.GetUser(ctx, msUser.ID)
	if errors.Is(err, store.ErrNotFound) {
		// Valid media-server credentials, but NOT imported → denied (the allowlist).
		return store.User{}, ErrInvalidCredentials
	}
	if err != nil {
		return store.User{}, err
	}
	// Refresh name + disabled from the source of truth on each login (a
	// server-side disable takes effect at once); role/quota stay Loomarr-owned.
	return s.refreshOnLogin(ctx, existing, msUser.Name, msUser.Disabled)
}

// refreshOnLogin persists name/disabled updates observed at login and returns the
// updated user. Role, quota, and the password hash are preserved (UpsertUser
// COALESCEs an empty hash), so a media-server refresh never wipes local state.
func (s *LoginService) refreshOnLogin(ctx context.Context, u store.User, name string, disabled bool) (store.User, error) {
	if u.Name == name && u.Disabled == disabled {
		return u, nil // no change — skip the write
	}
	u.Name = name
	u.Disabled = disabled
	u.UpdatedAt = s.now()
	if err := s.store.UpsertUser(ctx, u); err != nil {
		return store.User{}, err
	}
	return u, nil
}

// Disable disables a user and immediately revokes their sessions (§11).
func (s *LoginService) Disable(ctx context.Context, userID string) error {
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	u.Disabled = true
	u.UpdatedAt = s.now()
	if err := s.store.UpsertUser(ctx, u); err != nil {
		return err
	}
	return s.store.RevokeSessionsForUser(ctx, userID)
}
