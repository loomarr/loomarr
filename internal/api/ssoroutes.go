package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/store"
)

// /v1/auth/sso/* — the OIDC credential path (§11 SSO, D-F, V8).
//
// ⚠ **Huma operations mounted via rawOp, not plain mux handlers.** Both routes are BROWSER
// REDIRECTS: start sends the person to their provider, and callback sends them back carrying a
// session cookie. A 302 with a Set-Cookie is not a typed JSON body — but that describes the
// RESPONSE, and it never described where the route belongs. rawOp keeps the raw `(w, r)` these
// handlers need while mounting them on the one Huma API, exactly as `/v1/backup` and the
// playout streams now are.
//
// ⚠ They ARE in `api/openapi.yaml` now, declared as 302s with a Location header. The previous
// text called their absence deliberate — "a redirect an operator's browser follows is not a
// client contract" — which reads fine until you notice the same argument would justify leaving
// any route out, and that the spec's job is to describe the served surface. The concern behind
// it was that an orval hook would invite a `fetch` that cannot work; that is answered by the
// declared 302 (there is no 2xx body to generate a useful call from) and by login.tsx driving
// `sso-start` through `window.location.assign`, which is the only thing that can work.
//
// ⚠ **Unauthenticated by necessity** — this is how you sign IN. Three things protect them:
// the state COOKIE (below) binds the callback to the browser that started the login, the
// state/nonce pair bounds it in time and to one use, and §11's allowlist decides access after
// the token verifies. Neither route can create a user.
//
// ⚠ **The cookie is the CSRF protection, not the state map.** An earlier version of this
// comment credited "the state/nonce pair", and that was wrong: `pending` lives in one process
// and answers "did a login start here?", never "did THIS browser start it?".
//
// ⚠ **Nor does the `X-Loomarr-Csrf` header help, and the reason CHANGED — read it before
// deleting the cookie check as redundant.** It used to be "these are plain-mux GETs registered
// outside that middleware". They are Huma operations now, inside the middleware, so that
// sentence would today read as "…therefore the header covers us". It does not: the header guard
// applies only to MUTATING methods (`isMutating`, titles.go), and a login redirect is a GET. It
// also could not apply — the callback arrives as a cross-site top-level navigation from the
// provider, which carries no custom headers at all. The cookie remains the only thing that
// binds a callback to the browser that started the login.

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
//
// ⚠ **rawOp, not the bare mux.** A 302 carrying Set-Cookie is not a typed JSON body, which is
// why these lived on the mux — but that reasoning confused the RESPONSE shape with where a
// route belongs. rawOp (rawop.go) keeps the raw `(w, r)` so the handlers still write their own
// redirect and cookie, while mounting on the same Huma API as everything else. Concretely that
// buys: `RolePublic` written down instead of inferred from an absence, coverage by the raw-mux
// guard, an entry in api/openapi.yaml, and — the one that actually bit before —
// TestRegisterListsMatchBetweenRouterAndExporter, which matches `srv.register*(humaAPI)` and
// was structurally blind to `registerSSO(mux)`.
//
// ⚠ The nil guard admits schemaOnly so the routes still reach the spec when the exporter runs
// without a provider wired. Dropping a route from openapi.yaml because a dependency happened to
// be nil at export time has already happened in this package once (see export.go).
func (s *Server) registerSSO(api huma.API) {
	if s.sso == nil && !s.schemaOnly {
		return
	}
	rawOp[struct{}](api, redirectResponse(huma.Operation{
		OperationID: "sso-start", Method: http.MethodGet, Path: "/v1/auth/sso/start",
		Summary: "Begin an SSO login",
		Description: "Public — this is how you sign in. Redirects to the identity provider and sets " +
			"a short-lived state cookie binding the callback to this browser. `next` may be a PATH " +
			"only; an absolute URL would make this an open redirector.",
		Tags: []string{"auth"},
	}, "Redirect to the identity provider."), RolePublic, s.ssoStart)

	rawOp[struct{}](api, redirectResponse(huma.Operation{
		OperationID: "sso-callback", Method: http.MethodGet, Path: "/v1/auth/sso/callback",
		Summary: "Complete an SSO login",
		Description: "Public. The provider redirects here; Loomarr verifies the state cookie and the " +
			"code, mints a session, and redirects onward. §11's allowlist decides access — this " +
			"route cannot create a user.",
		Tags: []string{"auth"},
	}, "Redirect back into the app, with a session cookie on success."), RolePublic, s.ssoCallback)
}

// redirectResponse declares a 302 for a route whose whole job is the Location header, so the
// spec does not imply a JSON body these never return.
func redirectResponse(op huma.Operation, description string) huma.Operation {
	if op.Responses == nil {
		op.Responses = map[string]*huma.Response{}
	}
	op.Responses["302"] = &huma.Response{
		Description: description,
		Headers: map[string]*huma.Param{
			"Location": {Description: "Where to go next", Schema: &huma.Schema{Type: huma.TypeString}},
		},
	}
	return op
}

// ssoStateCookie is the name of the short-lived cookie that binds a callback to the browser
// that started the login.
const ssoStateCookie = "loomarr_sso_state"

// ssoStateTTL bounds the cookie's life. Matches the service's pendingTTL: two different
// answers to "how long may a login take" would mean the shorter one silently governs, and a
// mismatch would read as a flaky provider rather than a deliberate limit.
const ssoStateTTL = 10 * time.Minute

// stateCookie binds a pending login to this browser.
//
// ⚠ **`SameSite=Lax` is load-bearing, and `Strict` would break every SSO login.** The callback
// arrives as a cross-site TOP-LEVEL NAVIGATION from the provider, and a Strict cookie is not
// sent on those — so Strict does not harden this, it silently guarantees the check can never
// pass. Verified against real Authelia and Authentik, which is the only place this shows up:
// no stub IdP performs a real cross-site redirect.
//
// Scoped to /v1/auth/sso so it is never attached to ordinary app requests, and Secure follows
// the same COOKIE_SECURE policy as the session cookie (§11) — a plain-HTTP LAN install must
// still be able to sign in.
func (s *Server) stateCookie(r *http.Request, state string, expires time.Time) http.Cookie {
	return http.Cookie{
		Name:     ssoStateCookie,
		Value:    state,
		Path:     "/v1/auth/sso",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.secureCookie(r),
	}
}

// clearStateCookie expires the state cookie. Called on every callback outcome, so one
// browser round trip can never satisfy two callbacks.
//
// Attributes must MATCH the ones it was set with — a browser keys a cookie by name+domain+
// path, so an expiry with the wrong Path deletes nothing and leaves the original in place.
func (s *Server) clearStateCookie(w http.ResponseWriter, r *http.Request) {
	cookie := s.stateCookie(r, "", time.Unix(0, 0))
	cookie.MaxAge = -1
	http.SetCookie(w, &cookie)
}

// ssoStart redirects to the provider.
func (s *Server) ssoStart(w http.ResponseWriter, r *http.Request) {
	// `next` is where to land after signing in, so a deep link survives the round trip.
	// ⚠ Only a PATH is accepted. An absolute URL here would make this an open redirector:
	// an attacker could send someone a Loomarr link that bounces them to a look-alike login
	// page, and the domain in the address bar would have been ours right up to the hop.
	next := safeReturnPath(r.URL.Query().Get("next"))

	authURL, state, err := s.sso.Start(r.Context(), next)
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

	// ⚠ Set BEFORE the redirect. `http.Redirect` writes the status line and headers
	// immediately, so a Set-Cookie after it is discarded by net/http — the same structural
	// property `redirectToLogin` relies on to guarantee no session escapes a refusal. Here it
	// cuts the other way: get the order wrong and the cookie is silently never sent, which
	// presents as "SSO always says the login expired".
	cookie := s.stateCookie(r, state, time.Now().Add(ssoStateTTL))
	http.SetCookie(w, &cookie)
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

	// ⚠ **THE CSRF CHECK, and it must run BEFORE the exchange.** The state cookie is the only
	// thing proving this browser is the one that started the login — the pending map proves
	// only that SOME login started on this server. Without it, someone the provider
	// authenticates can capture their own ?state=&code= redirect and get a victim's browser
	// to follow it, landing the victim in the app signed in AS THEM.
	//
	// Before the exchange, not after: an unbound callback should never spend a code or reach
	// the provider at all.
	state := q.Get("state")
	sc, err := r.Cookie(ssoStateCookie)
	if err != nil || sc.Value == "" || subtle.ConstantTimeCompare([]byte(sc.Value), []byte(state)) != 1 {
		s.clearStateCookie(w, r)
		s.log.Warn("sso: callback did not carry the state cookie from this browser")
		s.redirectToLogin(w, r, "sso_expired")
		return
	}
	// Single use, whatever happens next.
	s.clearStateCookie(w, r)

	token, expires, u, claims, returnTo, err := s.sso.Exchange(r.Context(), state, q.Get("code"))
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
	//
	// ⚠ Re-validated HERE, not merely at /start. This value crossed a package boundary and a
	// map that outlives the request, so the header defends itself rather than trusting a gate
	// three hops upstream — the check that matters is the one next to the Location it writes.
	dest := safeReturnPath(returnTo)
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
// ⚠ **PARSED, not prefix-matched, and the backslash is why.** An earlier version checked
// HasPrefix("/") + !HasPrefix("//") + !Contains("://"), which reads as thorough and let
// `/\evil.test` straight through: browsers resolving a special-scheme URL treat `\` as `/`
// (WHATWG URL), so that Location navigates to https://evil.test while looking like a path.
// Go does not normalise it away either — `path.Clean` treats `\` as an ordinary character and
// `http.Redirect` emits the value verbatim.
//
// The backslash is rejected explicitly BEFORE parsing, because url.Parse does not apply the
// WHATWG backslash rule — it would report an empty Host and wave `/\evil.test` through. A
// narrower fix (rejecting only a leading `/\`) still admits `/\/evil.test`, which is why this
// rejects the character anywhere in the value.
func safeReturnPath(next string) string {
	if next == "" || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") {
		return ""
	}
	if strings.Contains(next, `\`) {
		return ""
	}
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" ||
		!strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") {
		return ""
	}
	return u.RequestURI()
}
