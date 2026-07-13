package api

import (
	"context"
	"net/http"

	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/store"
)

// sessionAuthorizer resolves a request to a role via the session cookie first
// (§11), falling back to the API_TOKEN Bearer (admin break-glass, §11). This is
// the Phase-9 fill of the Phase-8 Authorizer seam.
type sessionAuthorizer struct {
	mgr      *auth.Manager
	apiToken string
}

// NewSessionAuthorizer builds the session+token authorizer.
func NewSessionAuthorizer(mgr *auth.Manager, apiToken string) Authorizer {
	return &sessionAuthorizer{mgr: mgr, apiToken: apiToken}
}

func (a *sessionAuthorizer) Authorize(r *http.Request) Role {
	role, _ := a.AuthorizeUser(r)
	return role
}

// AuthorizeUser resolves the role AND the authenticated user (when a session
// cookie identifies one). The API_TOKEN path has no user (machine/break-glass).
func (a *sessionAuthorizer) AuthorizeUser(r *http.Request) (Role, *store.User) {
	// Session cookie (the normal human path).
	if c, err := r.Cookie(auth.CookieName); err == nil && c.Value != "" {
		if u, err := a.mgr.Resolve(r.Context(), c.Value); err == nil {
			user := u
			return roleOf(u), &user
		}
	}
	// API_TOKEN Bearer → admin (machine access + break-glass, §11).
	if a.apiToken != "" {
		const prefix = "Bearer "
		h := r.Header.Get("Authorization")
		if len(h) > len(prefix) && h[:len(prefix)] == prefix &&
			constantEq(h[len(prefix):], a.apiToken) {
			return RoleAdmin, nil
		}
	}
	return RoleAnonymous, nil
}

// UserAuthorizer is implemented by authorizers that can also return the
// authenticated user (the session authorizer). The middleware uses it to thread
// identity for /v1/auth/me etc.
type UserAuthorizer interface {
	Authorizer
	AuthorizeUser(r *http.Request) (Role, *store.User)
}

func roleOf(u store.User) Role {
	if u.Role == store.RoleAdmin {
		return RoleAdmin
	}
	return RoleMember
}

// userCtxKey threads the authenticated store.User for handlers that need
// identity (e.g. /v1/auth/me). It is stored by the middleware via huma.WithValue.
type userCtxKey struct{}

func userFrom(ctx context.Context) (store.User, bool) {
	u, ok := ctx.Value(userCtxKey{}).(store.User)
	return u, ok
}

// reqCtxKey threads the raw *http.Request so handlers can read the client IP and
// set cookies (login/logout).
type reqCtxKey struct{}

func requestFrom(ctx context.Context) *http.Request {
	r, _ := ctx.Value(reqCtxKey{}).(*http.Request)
	return r
}
