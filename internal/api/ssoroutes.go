package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/metrics"
	"github.com/mantonx/loomarr/internal/store"
)

// /v1/auth/sso/* — the OIDC credential path (§11 SSO, D-F, V8).
//
// ⚠ **Plain mux handlers, not Huma operations.** Both routes are BROWSER REDIRECTS: start
// sends the person to their provider, and callback sends them back into the app carrying a
// session cookie. Huma models typed JSON bodies, and a 302 with a Set-Cookie is neither —
// the same reason `/v1/backup` and the playout routes are mounted directly.
//
// They are also deliberately absent from `api/openapi.yaml`: a redirect an operator's
// browser follows is not a client contract, and generating an orval hook for it would invite
// a fetch that cannot work.
//
// ⚠ **Unauthenticated by necessity** — this is how you sign IN. What protects them is the
// state/nonce pair (a callback that did not come from our redirect is refused) and, after
// verification, §11's allowlist. Neither route can create a user.

// SSOService is the credential path the routes drive. nil ⇒ the routes are not mounted at
// all, which is the honest posture for an unconfigured provider: a Sign-in-with button that
// 501s is worse than one that is not offered.
type SSOService interface {
	// Available reports whether SSO is configured well enough to attempt.
	Available() bool
	// Start returns the provider URL to redirect to, plus the state it minted.
	Start(ctx context.Context, returnTo string) (authURL, state string, err error)
	// Exchange completes the flow. Claims come back even on failure, so a refusal can be
	// explained rather than merely reported.
	Exchange(ctx context.Context, state, code string) (token string, expires time.Time, u store.User, claims auth.SSOClaims, returnTo string, err error)
}

// registerSSO mounts the two redirect routes when a provider is wired.
func (s *Server) registerSSO(mux *http.ServeMux) {
	if s.sso == nil {
		return
	}
	mux.HandleFunc("GET /v1/auth/sso/start", s.ssoStart)
	mux.HandleFunc("GET /v1/auth/sso/callback", s.ssoCallback)
}

// ssoStart redirects to the provider.
func (s *Server) ssoStart(w http.ResponseWriter, r *http.Request) {
	// `next` is where to land after signing in, so a deep link survives the round trip.
	// ⚠ Only a PATH is accepted. An absolute URL here would make this an open redirector:
	// an attacker could send someone a Loomarr link that bounces them to a look-alike login
	// page, and the domain in the address bar would have been ours right up to the hop.
	next := safeReturnPath(r.URL.Query().Get("next"))

	authURL, _, err := s.sso.Start(r.Context(), next)
	if err != nil {
		if errors.Is(err, auth.ErrSSONotConfigured) {
			s.writeProblem(w, r, http.StatusNotImplemented, "Single sign-on isn't set up",
				"Ask an admin to finish the identity-provider settings.")
			return
		}
		// A provider that cannot be reached is the operator's problem to see, not a blank page.
		s.log.Error("sso: could not start login", "err", err)
		s.writeProblem(w, r, http.StatusBadGateway, "Couldn't reach your identity provider",
			"Loomarr couldn't load the provider's configuration. Check the issuer address in Settings.")
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

// ssoCallback completes the flow and lands the person in the app.
func (s *Server) ssoCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	// A provider can refuse before we ever see a code (the person cancelled, or the client
	// is misconfigured on their side). Say so rather than reporting a state mismatch.
	if providerErr := q.Get("error"); providerErr != "" {
		s.log.Info("sso: provider refused the login", "error", providerErr, "description", q.Get("error_description"))
		s.redirectToLogin(w, r, "sso_provider_error")
		return
	}

	token, expires, u, claims, returnTo, err := s.sso.Exchange(r.Context(), q.Get("state"), q.Get("code"))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrSSONotConfigured):
			s.redirectToLogin(w, r, "sso_unavailable")
		case errors.Is(err, auth.ErrSSOStateMismatch):
			// Did not come from our redirect, or a token replayed from another login.
			s.log.Warn("sso: callback did not match a pending login")
			s.redirectToLogin(w, r, "sso_expired")
		case errors.Is(err, auth.ErrInvalidCredentials):
			// ⚠ THE ALLOWLIST refusing a verified identity (§11). Logged WITH the claims —
			// the operator's question is "which name arrived, and why is there no row for
			// it?", and they cannot answer it from a redirect.
			metrics.LoginResult(false)
			s.log.Info("sso: no account for this identity",
				"matched_on", claims.MatchName(), "sub", claims.Subject, "email", claims.Email)
			s.redirectToLogin(w, r, "sso_no_account")
		default:
			s.log.Error("sso: callback failed", "err", err)
			s.redirectToLogin(w, r, "sso_failed")
		}
		return
	}

	metrics.LoginResult(true)
	cookie := s.sessionCookie(r, token, expires)
	http.SetCookie(w, &cookie)
	s.log.Info("sso login", "user", u.ID, "matched_on", claims.MatchName())

	// Back to wherever they started, or the app root.
	dest := returnTo
	if dest == "" {
		dest = "/"
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// redirectToLogin sends the browser to the sign-in screen with a reason code the UI turns
// into copy.
//
// ⚠ Note for anyone hardening this path: it is NOT the early `return` on each refusal branch
// that stops a session being issued. `http.Redirect` writes the status line and headers
// immediately, so a later `http.SetCookie` is discarded by net/http — verified by sabotage
// (removing the return, and even forging a non-empty token, still yields no usable cookie).
// The guarantee is structural. Do not "tidy" this into a single exit that redirects last,
// because then the return WOULD be the only thing holding it.
//
// ⚠ A reason CODE, never a message. Reflecting server text into a URL the browser renders is
// how a redirect becomes a phishing surface; a fixed vocabulary the frontend owns cannot be
// used to put an attacker's words on our login page.
func (s *Server) redirectToLogin(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, "/login?sso="+url.QueryEscape(reason), http.StatusFound)
}

// safeReturnPath keeps only a same-app path, so `next` can never become an open redirect.
//
// Rejects anything with a scheme or host, and `//evil.test` — which browsers treat as
// protocol-relative and would follow off-site despite looking like a path.
func safeReturnPath(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return ""
	}
	if strings.Contains(next, "://") {
		return ""
	}
	return next
}
