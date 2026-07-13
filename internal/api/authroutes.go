package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/auth"
)

// registerAuth mounts /v1/auth/* (§11). login and logout manage the session
// cookie; me reports the current user. Absent if no LoginService is configured.
func (s *Server) registerAuth(api huma.API) {
	if s.login == nil {
		return
	}
	huma.Register(api, huma.Operation{
		OperationID: "login", Method: http.MethodPost, Path: "/v1/auth/login",
		Summary: "Sign in with media-server credentials", Tags: []string{"auth"},
	}, s.handleLogin)

	huma.Register(api, huma.Operation{
		OperationID: "logout", Method: http.MethodPost, Path: "/v1/auth/logout",
		Summary: "End the current session", Tags: []string{"auth"},
		DefaultStatus: http.StatusNoContent,
	}, s.handleLogout)

	huma.Register(api, huma.Operation{
		OperationID: "me", Method: http.MethodGet, Path: "/v1/auth/me",
		Summary: "Current user, role, and quotas", Tags: []string{"auth"},
	}, s.handleMe)
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
			return nil, huma.Error429TooManyRequests("too many login attempts")
		case errors.Is(err, auth.ErrInvalidCredentials):
			return nil, huma.Error401Unauthorized("invalid username or password")
		default:
			return nil, err
		}
	}
	out := &meOutput{
		SetCookie: s.sessionCookie(r, token, expires),
		Body:      meBody{ID: u.ID, Name: u.Name, Role: string(u.Role), Disabled: u.Disabled, Quota: u.Quota, AutoApprove: u.AutoApprove},
	}
	return out, nil
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
		return nil, huma.Error401Unauthorized("not signed in")
	}
	return &meOnlyOutput{Body: meBody{
		ID: u.ID, Name: u.Name, Role: string(u.Role), Disabled: u.Disabled, Quota: u.Quota, AutoApprove: u.AutoApprove,
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
