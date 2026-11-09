package httpcore

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
)

// ConstantEq is a constant-time string equality (auth tokens).
func ConstantEq(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Role is the caller's authorization level (§11): anonymous, a session- or device-derived member,
// or admin (a session, a device, or the API_TOKEN bearer).
type Role string

const (
	RoleAnonymous Role = ""
	RoleMember    Role = "member"
	RoleAdmin     Role = "admin"
)

// Authorizer resolves a request to a role (§7 authorization model). Production uses the
// session-cookie authorizer in internal/api (sessionauth.go); NewTokenAuthorizer is the
// API_TOKEN-only form (§11 break-glass admin).
type Authorizer interface {
	Authorize(r *http.Request) Role
}

// tokenAuthorizer authenticates the static API_TOKEN as admin via
// `Authorization: Bearer <token>` (§11). Empty token ⇒ no Bearer auth.
type tokenAuthorizer struct{ token string }

// NewTokenAuthorizer returns an Authorizer that treats the API_TOKEN bearer as
// admin. If token is empty, every request is anonymous.
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

// ContextWithRole is ctx carrying the role the auth middleware resolved, for RoleFrom.
func ContextWithRole(ctx huma.Context, role Role) huma.Context {
	return huma.WithValue(ctx, roleCtxKey{}, role)
}

// RoleFrom reads the role resolved by the auth middleware.
func RoleFrom(ctx context.Context) Role {
	if v, ok := ctx.Value(roleCtxKey{}).(Role); ok {
		return v
	}
	return RoleAnonymous
}
