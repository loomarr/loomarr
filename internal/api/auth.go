package api

import "github.com/loomarr/loomarr/internal/api/httpcore"

// The caller's role and the authorizer seam live in internal/api/httpcore, where every area
// package can reach them. They are re-exported here by alias so callers of this package (the
// composition root, the tests) keep one name for them.
type (
	// Role is the caller's authorization level (httpcore.Role).
	Role = httpcore.Role
	// Authorizer resolves a request to a role (httpcore.Authorizer).
	Authorizer = httpcore.Authorizer
)

// The roles (httpcore).
const (
	RoleAnonymous = httpcore.RoleAnonymous
	RoleMember    = httpcore.RoleMember
	RoleAdmin     = httpcore.RoleAdmin
	// RolePublic marks a route reachable with no credential (httpcore.RolePublic).
	RolePublic = httpcore.RolePublic
)

// NewTokenAuthorizer returns an Authorizer that treats the API_TOKEN bearer as admin
// (httpcore.NewTokenAuthorizer).
func NewTokenAuthorizer(token string) Authorizer { return httpcore.NewTokenAuthorizer(token) }
