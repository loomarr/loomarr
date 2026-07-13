package api

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"
)

// constantEq is a constant-time string equality (auth tokens).
func constantEq(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Role is the caller's authorization level (§11). Phase 8 recognizes admin (via
// API_TOKEN) and anonymous; Phase 9 adds session-derived member/admin.
type Role string

const (
	RoleAnonymous Role = ""
	RoleMember    Role = "member"
	RoleAdmin     Role = "admin"
)

// Authorizer resolves a request to a role (§7 authorization model). It is the
// seam Phase 9 fills with session-cookie auth; Phase 8 ships an API_TOKEN Bearer
// authorizer so /v1 routes aren't wide open before sessions exist (§11
// break-glass admin).
type Authorizer interface {
	Authorize(r *http.Request) Role
}

// tokenAuthorizer authenticates the static API_TOKEN as admin via
// `Authorization: Bearer <token>` (§11). Empty token ⇒ no Bearer auth (sessions
// only, once Phase 9 lands).
type tokenAuthorizer struct{ token string }

// NewTokenAuthorizer returns an Authorizer that treats the API_TOKEN bearer as
// admin. If token is empty, every request is anonymous (until Phase 9).
func NewTokenAuthorizer(token string) Authorizer { return tokenAuthorizer{token: token} }

func (a tokenAuthorizer) Authorize(r *http.Request) Role {
	if a.token == "" {
		return RoleAnonymous
	}
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return RoleAnonymous
	}
	got := strings.TrimPrefix(h, prefix)
	if subtle.ConstantTimeCompare([]byte(got), []byte(a.token)) == 1 {
		return RoleAdmin
	}
	return RoleAnonymous
}

type roleCtxKey struct{}

// roleFrom reads the role resolved by the auth middleware.
func roleFrom(ctx context.Context) Role {
	if v, ok := ctx.Value(roleCtxKey{}).(Role); ok {
		return v
	}
	return RoleAnonymous
}

// roleFromHuma reads the role a Huma handler's context carries. Huma threads the
// middleware's context value into the standard context.Context passed to
// handlers, so this is the same lookup as roleFrom.
func roleFromHuma(ctx context.Context) Role { return roleFrom(ctx) }
