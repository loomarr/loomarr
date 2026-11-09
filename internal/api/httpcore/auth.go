package httpcore

import (
	"context"
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
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

// Identity is who a request acts as: the role it is authorized under and, when a session or a
// paired device identifies a person, that user. The API_TOKEN bearer is an admin with no user.
// DeviceID names the paired device when a device credential authenticated the request.
type Identity struct {
	Role     Role
	User     *identitystore.User
	DeviceID string
}

// Authorizer resolves a request to its Identity (§7 authorization model). Production uses the
// session-cookie authorizer in internal/api (sessionauth.go); NewTokenAuthorizer is the
// API_TOKEN-only form (§11 break-glass admin).
type Authorizer interface {
	Authorize(r *http.Request) Identity
}

// tokenAuthorizer authenticates the static API_TOKEN as admin via
// `Authorization: Bearer <token>` (§11). Empty token ⇒ no Bearer auth.
type tokenAuthorizer struct{ token string }

// NewTokenAuthorizer returns an Authorizer that treats the API_TOKEN bearer as
// admin. If token is empty, every request is anonymous.
func NewTokenAuthorizer(token string) Authorizer { return tokenAuthorizer{token: token} }

func (a tokenAuthorizer) Authorize(r *http.Request) Identity {
	if a.token == "" {
		return Identity{Role: RoleAnonymous}
	}
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return Identity{Role: RoleAnonymous}
	}
	got := strings.TrimPrefix(h, prefix)
	if subtle.ConstantTimeCompare([]byte(got), []byte(a.token)) == 1 {
		return Identity{Role: RoleAdmin}
	}
	return Identity{Role: RoleAnonymous}
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
