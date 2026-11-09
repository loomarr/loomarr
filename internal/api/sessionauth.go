package api

import (
	"context"
	"net/http"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

// sessionAuthorizer resolves a request to a role via the session cookie first
// (§11), falling back to the API_TOKEN Bearer (admin break-glass, §11). It is the
// production implementation of httpcore.Authorizer.
type sessionAuthorizer struct {
	mgr             *auth.Manager
	devices         *auth.DeviceManager
	apiToken        string
	apiTokenCurrent func(context.Context) (string, error)
}

// NewSessionAuthorizerCurrent builds the production session authorizer with a
// request-boundary API token resolver. Postgres uses it to observe a rotation
// performed by another replica; resolver errors fail only the bearer fallback
// closed and never interfere with a valid user session.
//
// devices may be nil, which disables the paired-device path entirely — the authorizer then behaves
// with sessions and the API_TOKEN only. It is a constructor parameter rather than a setter so a
// security-critical object cannot exist in a half-configured state, and so the one composition site
// that enables device auth is greppable.
func NewSessionAuthorizerCurrent(
	mgr *auth.Manager,
	devices *auth.DeviceManager,
	apiToken func(context.Context) (string, error),
) httpcore.Authorizer {
	return &sessionAuthorizer{mgr: mgr, devices: devices, apiTokenCurrent: apiToken}
}

// Authorize resolves the role AND the authenticated user (when a session cookie or a paired
// device identifies one). The API_TOKEN path has no user (machine/break-glass).
func (a *sessionAuthorizer) Authorize(r *http.Request) httpcore.Identity {
	// Session cookie (the normal human path).
	if c, err := r.Cookie(auth.CookieName); err == nil && c.Value != "" {
		if u, err := a.mgr.Resolve(r.Context(), c.Value, httpcore.ClientLabel(r.UserAgent())); err == nil {
			user := u
			return httpcore.Identity{Role: roleOf(u), User: &user}
		}
	}
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")

	// A paired device (§11, Shield P1) — checked BEFORE the API_TOKEN comparison, and returning on
	// a hit, because both credentials arrive in the same header. If API_TOKEN were tried first, a
	// device token would be compared against the household admin secret; ordering it this way means
	// the admin comparison only ever sees credentials that are not device tokens.
	//
	// A device acts AS the person who approved it, with their favourites, recents, audience and
	// role, mapped exactly as their session would be (ADR 0043). ResolveDevice reads the user row on
	// every request, so a demotion applies on the device's next request and a disabled user's
	// devices fail.
	if a.devices != nil && len(h) > len(prefix) && h[:len(prefix)] == prefix {
		if principal, err := a.devices.ResolveDevice(r.Context(), h[len(prefix):]); err == nil {
			user := principal.User
			return httpcore.Identity{DeviceID: principal.ID, Role: roleOf(user), User: &user}
		}
	}

	// API_TOKEN Bearer → admin (machine access + break-glass, §11). Resolve only
	// after session auth failed; a normal human request should not pay a durable
	// generated-secret read merely because Postgres replicas are enabled.
	if len(h) > len(prefix) && h[:len(prefix)] == prefix {
		apiToken := a.apiToken
		if a.apiTokenCurrent != nil {
			var err error
			apiToken, err = a.apiTokenCurrent(r.Context())
			if err != nil {
				apiToken = "" // credential state unavailable: fail bearer auth closed.
			}
		}
		if apiToken != "" && httpcore.ConstantEq(h[len(prefix):], apiToken) {
			return httpcore.Identity{Role: httpcore.RoleAdmin}
		}
	}
	return httpcore.Identity{Role: httpcore.RoleAnonymous}
}

// hasBearer reports whether a request carries an Authorization: Bearer credential.
//
// Used by the CSRF guard to exempt token callers: a browser cannot be tricked into attaching a
// bearer header cross-site the way it attaches a cookie, so such a request is not a CSRF vector.
// This does NOT assert the credential is valid — an invalid one fails authorization moments later,
// and a request with no user never reaches the guard at all.
func hasBearer(r *http.Request) bool {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	return len(h) > len(prefix) && h[:len(prefix)] == prefix
}

func roleOf(u identitystore.User) httpcore.Role {
	if u.Role == identitystore.RoleAdmin {
		return httpcore.RoleAdmin
	}
	return httpcore.RoleMember
}
