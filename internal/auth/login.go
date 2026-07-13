package auth

import (
	"context"
	"errors"
	"time"

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

// Login verifies credentials against the media server, syncs the user locally
// (with first-admin bootstrap), and issues a session. rateKey is IP+username.
// Returns the cookie token, its expiry, and the local user.
func (s *LoginService) Login(ctx context.Context, username, password, rateKey string) (token string, expires time.Time, u store.User, err error) {
	if s.limiter != nil && !s.limiter.Allow(rateKey) {
		return "", time.Time{}, store.User{}, ErrRateLimited
	}

	msUser, err := s.lib.AuthenticateByName(ctx, username, password)
	if err != nil {
		return "", time.Time{}, store.User{}, ErrInvalidCredentials
	}

	u, err = s.syncUser(ctx, msUser)
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

// syncUser upserts a media-server user locally, applying the role policy (§11):
// a media-server administrator maps to admin. First-admin bootstrap — the first
// media-server admin to sign in claims the instance; a non-admin who signs in
// before any admin exists is created as a member and sees "waiting for admin".
func (s *LoginService) syncUser(ctx context.Context, ms library.User) (store.User, error) {
	now := s.now()
	existing, err := s.store.GetUser(ctx, ms.ID)
	switch {
	case err == nil:
		// Keep the locally-managed role/quota; refresh name + disabled from source.
		existing.Name = ms.Name
		existing.Disabled = ms.Disabled
		existing.UpdatedAt = now
		if err := s.store.UpsertUser(ctx, existing); err != nil {
			return store.User{}, err
		}
		return existing, nil
	case errors.Is(err, store.ErrNotFound):
		role := store.RoleMember
		if ms.IsAdmin {
			role = store.RoleAdmin // media-server admins map to admin (§11)
		}
		u := store.User{
			ID: ms.ID, Name: ms.Name, Role: role, Disabled: ms.Disabled,
			CreatedAt: now, UpdatedAt: now,
		}
		if err := s.store.UpsertUser(ctx, u); err != nil {
			return store.User{}, err
		}
		return u, nil
	default:
		return store.User{}, err
	}
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
