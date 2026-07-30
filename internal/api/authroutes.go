package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/metrics"
)

// registerAuth mounts /v1/auth/* (§11). login and logout manage the session
// cookie; me reports the current user. Absent if no LoginService is configured.
func (s *Server) registerAuth(api huma.API) {
	if s.login == nil && !s.schemaOnly {
		return
	}
	huma.Register(api, withRole(huma.Operation{
		OperationID: "login", Method: http.MethodPost, Path: "/v1/auth/login",
		Summary: "Sign in with media-server credentials", Tags: []string{"auth"},
	}, RolePublic), s.handleLogin)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "logout", Method: http.MethodPost, Path: "/v1/auth/logout",
		Summary: "End the current session", Tags: []string{"auth"},
		DefaultStatus: http.StatusNoContent,
	}, RolePublic), s.handleLogout)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "me", Method: http.MethodGet, Path: "/v1/auth/me",
		Summary: "Current user, role, and quotas", Tags: []string{"auth"},
	}, RoleMember), s.handleMe)

	// Dev login (§11) — mounted ONLY when the server was started with
	// LOOMARR_DEV_LOGIN=1. Not registering is the gate: an install without the flag
	// serves 404 because the route genuinely does not exist, not because a handler
	// decided to refuse. There is nothing to misconfigure into an open state.
	//
	// Deliberately excluded from schemaOnly, so it never reaches api/openapi.yaml:
	// the exported spec is the product's contract, and a development bypass is not
	// part of it. That also keeps orval from generating a client for an endpoint
	// that is absent in every shipped install.
	if s.devLogin && !s.schemaOnly {
		// RolePublic because it IS the credential — requiring a session to reach the route
		// that issues one would make it useless. The gate is that it is not registered at
		// all unless LOOMARR_DEV_LOGIN=1 (§11), which is a stronger guarantee than a role.
		huma.Register(api, withRole(huma.Operation{
			OperationID: "dev-login", Method: http.MethodPost, Path: "/v1/auth/dev-login",
			Summary: "Sign in as an admin without a credential (development only)", Tags: []string{"auth"},
		}, RolePublic), s.handleDevLogin)
	}
}

type loginInput struct {
	Body struct {
		Username string `json:"username"`
		Password string `json:"password" format:"password"`
	}
}
type meBody struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role" enum:"admin,member"`
	Disabled    bool   `json:"disabled"`
	Quota       int    `json:"quota"`
	AutoApprove bool   `json:"autoApprove"`
	// Local reports the caller's OWN credential path (§11), the same field `userBody`
	// exposes for other people. Without it the app could label everyone else's
	// account as local-or-media-server but not yours — exactly backwards for the
	// Account screen, where the only account that matters is your own. It decides
	// whether to offer a change-password form or explain that the media server owns
	// that credential. The hash itself is never exposed, only whether one exists.
	Local bool `json:"local"`
}
type meOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      meBody
}

func (s *Server) handleLogin(ctx context.Context, in *loginInput) (*meOutput, error) {
	r := requestFrom(ctx)
	rateKey := clientIP(r) + "|" + in.Body.Username
	token, expires, u, err := s.login.Login(ctx, in.Body.Username, in.Body.Password, rateKey)
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrRateLimited):
			// Rate-limited attempts are their own signal (HTTP 429); don't fold
			// them into the credential success/failure ratio.
			return nil, errTooManyRequests("Too many attempts", "You've tried to sign in too many times. Wait a moment and try again.")
		case errors.Is(err, auth.ErrInvalidCredentials):
			metrics.LoginResult(false)
			return nil, errUnauthorized("Sign-in failed", "That username and password don't match. Check them and try again.")
		default:
			return nil, err
		}
	}
	metrics.LoginResult(true)
	out := &meOutput{
		SetCookie: s.sessionCookie(r, token, expires),
		Body:      meBody{ID: u.ID, Name: u.Name, Role: string(u.Role), Disabled: u.Disabled, Quota: u.Quota, AutoApprove: u.AutoApprove, Local: u.PasswordHash != ""},
	}
	return out, nil
}

// handleDevLogin issues an admin session with no credential (§11). Reaching this
// handler already required LOOMARR_DEV_LOGIN=1 at boot — registerAuth does not mount
// the route otherwise — so it performs no second gate check of its own.
//
// It is NOT rate-limited, on purpose: rate limiting exists to slow credential
// guessing, and there is no credential here to guess. The protection is that the
// route does not exist unless an operator deliberately turned it on.
func (s *Server) handleDevLogin(ctx context.Context, _ *struct{}) (*meOutput, error) {
	r := requestFrom(ctx)
	token, expires, u, err := s.login.DevLogin(ctx)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			// No admin row to borrow. Dev login never CREATES one (§11) — that would
			// turn a credential shortcut into a provisioning path.
			return nil, errUnauthorized("No admin to sign in as",
				"Dev login borrows an existing admin account, and this install has none. Create one through the setup wizard first.")
		}
		return nil, err
	}
	// Counted as a successful login: it really did mint a session, and a metric that
	// quietly omits some sign-ins is worse than one that includes an unusual path.
	metrics.LoginResult(true)
	slog.WarnContext(ctx, "dev login used — credential check bypassed", "user", u.Name, "id", u.ID)
	return &meOutput{
		SetCookie: s.sessionCookie(r, token, expires),
		Body:      meBody{ID: u.ID, Name: u.Name, Role: string(u.Role), Disabled: u.Disabled, Quota: u.Quota, AutoApprove: u.AutoApprove, Local: u.PasswordHash != ""},
	}, nil
}

type logoutOutput struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
}

func (s *Server) handleLogout(ctx context.Context, _ *struct{}) (*logoutOutput, error) {
	r := requestFrom(ctx)
	if c, err := r.Cookie(auth.CookieName); err == nil && c.Value != "" && s.sessions != nil {
		_ = s.sessions.Revoke(ctx, c.Value)
	}
	// Clear the cookie.
	clear := s.sessionCookie(r, "", time.Unix(0, 0))
	clear.MaxAge = -1
	return &logoutOutput{SetCookie: clear}, nil
}

type meOnlyOutput struct{ Body meBody }

func (s *Server) handleMe(ctx context.Context, _ *struct{}) (*meOnlyOutput, error) {
	u, ok := userFrom(ctx)
	if !ok {
		return nil, errUnauthorized("Not signed in", "You need to sign in to view this.")
	}
	return &meOnlyOutput{Body: meBody{
		ID: u.ID, Name: u.Name, Role: string(u.Role), Disabled: u.Disabled, Quota: u.Quota, AutoApprove: u.AutoApprove,
		Local: u.PasswordHash != "",
	}}, nil
}

// sessionCookie builds the session cookie: HttpOnly, SameSite=Strict, Secure per
// COOKIE_SECURE (§11).
func (s *Server) sessionCookie(r *http.Request, token string, expires time.Time) http.Cookie {
	return http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.secureCookie(r),
	}
}

// secureCookie decides the Secure flag from COOKIE_SECURE (§11): auto honors
// direct TLS or X-Forwarded-Proto=https; true/false force it.
func (s *Server) secureCookie(r *http.Request) bool {
	switch strings.ToLower(s.cookieSecure) {
	case "true":
		return true
	case "false":
		return false
	default: // auto
		return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
	}
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	return host
}
