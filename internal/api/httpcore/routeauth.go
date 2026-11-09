package httpcore

import (
	"github.com/danielgtaylor/huma/v2"
)

// Route authorization (§11): the role a route needs is part of REGISTERING it.
//
// ⚠ **The requirement lives on the operation, in ONE place.** A requirement split between an
// operation's `Description: "Admin only."` and a check at the top of a handler body has nothing
// binding the two, so it can be documented and not enforced — or neither. Probing once found 27
// of 83 operations consulting no role at all, including `POST /v1/proposals`, which returned 200
// to an anonymous caller and spent LLM tokens.
//
// ⚠ **Anonymous is denied by DEFAULT.** `roleForOperation` returns RoleAdmin — the most
// restrictive — for an operation that declared nothing, so forgetting the marker fails
// CLOSED. A route that should be reachable before a session exists must say `RolePublic`
// out loud, which also makes the public surface one greppable list instead of an absence
// you have to infer.
//
// TestEveryOperationDeclaresARole enumerates the registry and fails on an operation with no
// marker, so this cannot regress quietly.

// roleMetaKey is where the required role rides on huma.Operation.Metadata. Metadata is
// `yaml:"-"`, so this never reaches api/openapi.yaml — the authorization model is not a
// client contract, and publishing it would invite a frontend to enforce it.
const RoleMetaKey = "loomarr:role"

// RolePublic marks a route reachable with NO credential. It is a real value rather than the
// absence of one so that "public" is a decision someone wrote down.
//
// The whole list, and it should stay short: login, logout, bootstrap, setup-state, version.
// Each must work before a session can exist.
const RolePublic Role = "public"

// OperatorAPI marks an operation that no Loomarr client calls on purpose: it serves operators,
// scripts and support work, so the client-coverage audit (#1568) should not flag it.
const OperatorAPI = "Operator and automation API: no Loomarr client calls it."

// WithRole stamps the required role onto an operation. Every huma.Register call in the
// api packages goes through it (directly, or through RawOp).
func WithRole(op huma.Operation, role Role) huma.Operation {
	if op.Metadata == nil {
		op.Metadata = map[string]any{}
	}
	op.Metadata[RoleMetaKey] = role
	return op
}

// RoleForOperation reports the role an operation requires.
//
// ⚠ An operation with no marker requires ADMIN, not anonymous. That is the fail-closed
// default: a route added without thinking about authorization is unreachable rather than
// open, so the mistake surfaces as a 403 in development instead of an exposure in
// production.
func RoleForOperation(op *huma.Operation) Role {
	if op == nil {
		return RoleAdmin
	}
	if v, ok := op.Metadata[RoleMetaKey]; ok {
		if role, ok := v.(Role); ok {
			return role
		}
	}
	return RoleAdmin
}

// AuthorizeRole reports whether have satisfies want.
//
// Ordering is admin > member > public. Admin passes everything, since an admin holding a
// member's permissions is the point of the hierarchy.
func AuthorizeRole(want, have Role) bool {
	switch want {
	case RolePublic:
		return true
	case RoleMember:
		return have == RoleMember || have == RoleAdmin
	case RoleAdmin:
		return have == RoleAdmin
	default:
		// An unrecognised requirement denies. Same reasoning as the missing-marker default.
		return false
	}
}

// ⚠ **There is no second, hand-written role guard — do not add one.** A route that streams
// bytes, redirects, or takes multipart still belongs on the Huma API: use `RawOp` (rawop.go),
// which keeps the raw `(w, r)` signature and takes the required role as an argument. The role
// is enforced once, in the request middleware (internal/api/middleware.go), which ALSO applies
// the CSRF check and fails closed for an operation that declared nothing. Two hand-written guards
// once disagreed on what a nil authorizer means; a second authorization path cannot disagree
// with the first if it does not exist.
