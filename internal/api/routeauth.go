package api

import (
	"github.com/danielgtaylor/huma/v2"
)

// Route authorization (§11): the role a route needs is part of REGISTERING it.
//
// ⚠ **The old shape put the requirement in two places that nothing connected.** Intent lived
// in an operation's `Description: "Admin only."`; enforcement lived in a `requireAdmin(ctx)`
// call at the top of a handler body, 250 lines away. Nothing bound them, so the requirement
// could be documented and not enforced — or, more often, neither. Probing found 27 of 83
// operations consulting no role at all, including `POST /v1/proposals`, which returned 200
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
const roleMetaKey = "loomarr:role"

// RolePublic marks a route reachable with NO credential. It is a real value rather than the
// absence of one so that "public" is a decision someone wrote down.
//
// The whole list, and it should stay short: login, logout, bootstrap, setup-state, version.
// Each must work before a session can exist.
const RolePublic Role = "public"

// withRole stamps the required role onto an operation. Every huma.Register call in this
// package goes through it.
func withRole(op huma.Operation, role Role) huma.Operation {
	if op.Metadata == nil {
		op.Metadata = map[string]any{}
	}
	op.Metadata[roleMetaKey] = role
	return op
}

// roleForOperation reports the role an operation requires.
//
// ⚠ An operation with no marker requires ADMIN, not anonymous. That is the fail-closed
// default: a route added without thinking about authorization is unreachable rather than
// open, so the mistake surfaces as a 403 in development instead of an exposure in
// production.
func roleForOperation(op *huma.Operation) Role {
	if op == nil {
		return RoleAdmin
	}
	if v, ok := op.Metadata[roleMetaKey]; ok {
		if role, ok := v.(Role); ok {
			return role
		}
	}
	return RoleAdmin
}

// authorizeRole reports whether have satisfies want.
//
// Ordering is admin > member > public. Admin passes everything, since an admin holding a
// member's permissions is the point of the hierarchy.
func authorizeRole(want, have Role) bool {
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

// ⚠ **`requireRole` USED TO LIVE HERE, AND ITS DELETION IS THE POINT OF THIS WORK.**
//
// It was the plain-mux twin of the middleware above: the same rule, written a second time, for
// the handlers Huma never saw. It existed because two hand-written guards had already diverged
// on what a nil authorizer means — `backupHandler` denied, `eventsHandler` allowed, one package
// apart — so consolidating them into one function was an improvement over three copies.
//
// One function was still one too many. Every route it guarded is now a Huma operation carrying
// its required role in Metadata, enforced once in registerMiddleware, which ALSO applies the
// CSRF check the raw handlers had to hand-roll and fails closed for an operation that declared
// nothing. When the SSE stream and the multipart icon upload moved, the last two callers went
// with them and the linter reported the function unused — which is the shape of this being
// finished rather than merely started.
//
// ⚠ **Do not reintroduce it.** A route that streams bytes, redirects, or takes multipart still
// belongs on the Huma API: use `rawOp` (rawop.go), which keeps the raw `(w, r)` signature and
// takes the required role as an argument. There is one authorization path now; a second one
// cannot disagree with the first if it does not exist.
