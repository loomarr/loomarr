package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

// registerMiddleware resolves each request's role + user (§7) before the
// operation runs and stores them on the context for per-op authorization.
func (s *Server) registerMiddleware(api huma.API) {
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		r, _ := humago.Unwrap(ctx)
		role := httpcore.RoleAnonymous
		var user *identitystore.User
		var deviceID string
		if s.auth != nil {
			if ia, ok := s.auth.(identityAuthorizer); ok {
				identity := ia.AuthorizeIdentity(r)
				role, user, deviceID = identity.role, identity.user, identity.deviceID
			} else if ua, ok := s.auth.(UserAuthorizer); ok {
				role, user = ua.AuthorizeUser(r)
			} else {
				role = s.auth.Authorize(r)
			}
		}
		// CSRF (§11): mutating requests authenticated by a *cookie* must carry
		// X-Loomarr-Csrf: 1. Combined with SameSite=Strict this closes form-based
		// CSRF cheaply. Bearer-token (machine) callers are exempt — they don't send
		// cookies, so they aren't a CSRF vector. Login is exempt (no session yet).
		//
		// ⚠ The exemption tests for a BEARER credential, not for the absence of a user. Those were
		// the same thing while API_TOKEN was the only bearer path (it resolves to admin with no
		// user), but a paired device (§11, Shield P1) authenticates by bearer AND carries identity.
		// Keying on `user != nil` would demand a CSRF header from a client that cannot be
		// cross-site-forged, so the condition now says what the paragraph above always meant.
		if isMutating(r.Method) && user != nil && !hasBearer(r) && r.URL.Path != "/v1/auth/login" &&
			r.Header.Get("X-Loomarr-Csrf") != "1" {
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "missing X-Loomarr-Csrf header")
			return
		}

		// ⚠ **AUTHORIZATION, from the route's own declaration** (§11, routeauth.go). The
		// role a route needs rides on its operation, so this enforces it for every route at
		// once — rather than depending on each handler body remembering to ask. An
		// operation that declared nothing requires admin, so forgetting fails CLOSED.
		//
		// Runs after CSRF: a cookie-authenticated request missing the header is refused as
		// CSRF regardless of role, which keeps that refusal from reading as "wrong role".
		if want := httpcore.RoleForOperation(ctx.Operation()); !httpcore.AuthorizeRole(want, role) {
			// Goes through WriteErr so the request-id stamping and cause logging in
			// installUserFacingErrors applies here too — a refusal an operator cannot
			// correlate to a request is a refusal they cannot debug.
			if role == httpcore.RoleAnonymous {
				_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "You need to sign in to do this.")
			} else {
				_ = huma.WriteErr(api, ctx, http.StatusForbidden, "This action needs an admin account.")
			}
			return
		}

		c := httpcore.ContextWithRole(ctx, role)
		c = httpcore.ContextWithRequest(c, r) // raw request for handlers (login IP, cookies)
		if user != nil {
			c = httpcore.ContextWithUser(c, *user)
		}
		if deviceID != "" {
			c = httpcore.ContextWithDevice(c, deviceID)
		}
		next(c)
	})
}

// isMutating reports whether a method changes state (needs CSRF, §11).
func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// (`requireAdmin` lived in this package until 2026-08-10. It returned a 403 unless the caller
// resolved to admin, and 80 handler bodies opened by calling it — every one of them inside an
// operation that ALREADY declared RoleAdmin, which `registerMiddleware` above enforces for every
// route at once. Deleting the function rather than leaving it unused is the point: while it exists,
// the next handler can call it and re-create the two-places-nothing-connects shape routeauth.go
// documents, where intent lived in a Description and enforcement 250 lines away. The role now
// rides the operation, and an operation that declares nothing fails CLOSED.)
