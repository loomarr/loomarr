package httpcore

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

// The request the auth middleware resolved, threaded to handlers through the huma context. The
// middleware sets these once per request; a handler reads them and never re-parses credentials.

type userCtxKey struct{}

// ContextWithUser is ctx carrying the authenticated user, for UserFrom.
func ContextWithUser(ctx huma.Context, u identitystore.User) huma.Context {
	return huma.WithValue(ctx, userCtxKey{}, u)
}

// WithUser is the plain-context form of ContextWithUser, for code that holds a context.Context.
func WithUser(ctx context.Context, u identitystore.User) context.Context {
	return context.WithValue(ctx, userCtxKey{}, u)
}

// UserFrom reads the authenticated user (a session or a paired device's person); ok is false for the
// API_TOKEN break-glass caller, which has none.
func UserFrom(ctx context.Context) (identitystore.User, bool) {
	u, ok := ctx.Value(userCtxKey{}).(identitystore.User)
	return u, ok
}

// UserIDFrom returns the authenticated user's id, or "" for a token (break-glass) caller with no
// user record. It is what stamps created_by and decided_by.
func UserIDFrom(ctx context.Context) string {
	if u, ok := UserFrom(ctx); ok {
		return u.ID
	}
	return ""
}

type deviceCtxKey struct{}

// ContextWithDevice is ctx carrying the safe token-hash identity of an authenticated paired device.
// Only the authorizer that validated the bearer credential sets it.
func ContextWithDevice(ctx huma.Context, id string) huma.Context {
	return huma.WithValue(ctx, deviceCtxKey{}, id)
}

// DeviceIDFrom reads the paired device's identity; ok is false for any other caller.
func DeviceIDFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(deviceCtxKey{}).(string)
	return id, ok && id != ""
}

type reqCtxKey struct{}

// ContextWithRequest is ctx carrying the raw *http.Request, so handlers can read the client IP and
// set cookies (login and logout).
func ContextWithRequest(ctx huma.Context, r *http.Request) huma.Context {
	return huma.WithValue(ctx, reqCtxKey{}, r)
}

// RequestFrom reads the raw request threaded by ContextWithRequest, or nil.
func RequestFrom(ctx context.Context) *http.Request {
	r, _ := ctx.Value(reqCtxKey{}).(*http.Request)
	return r
}

// Origin is how this server decides where a request really came from and how to mark the cookies
// it sets: the two settings that both depend on whether a trusted proxy sits in front (§11).
type Origin struct {
	// CookieSecure is COOKIE_SECURE: "auto", "true" or "false" (anything else reads as auto).
	CookieSecure string
	// TrustProxy is TRUST_PROXY: X-Forwarded-For and X-Forwarded-Proto are honored only when true.
	TrustProxy bool
}

// SecureCookie decides a cookie's Secure flag from COOKIE_SECURE (§11): auto honors direct TLS or
// X-Forwarded-Proto=https; true and false force it.
func (o Origin) SecureCookie(r *http.Request) bool {
	switch strings.ToLower(o.CookieSecure) {
	case "true":
		return true
	case "false":
		return false
	default: // auto
		// X-Forwarded-Proto is honored only behind a trusted proxy (§11). A direct client
		// could otherwise send `X-Forwarded-Proto: http` to keep its OWN cookie non-Secure;
		// that only weakens the sender's session, but the header is not trustworthy input
		// unless a proxy sets it, so it rides the same gate as the rate-limit IP.
		if o.TrustProxy && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			return true
		}
		return r.TLS != nil
	}
}

// Cookie builds an HttpOnly cookie scoped to path, with the given SameSite policy and a Secure flag
// from SecureCookie.
func (o Origin) Cookie(r *http.Request, name, value, path string, sameSite http.SameSite, expires time.Time) http.Cookie {
	return http.Cookie{
		Name:     name,
		Value:    value,
		Path:     path,
		Expires:  expires,
		HttpOnly: true,
		SameSite: sameSite,
		Secure:   o.SecureCookie(r),
	}
}

// ClientIP returns the address the login rate-limiter keys on. X-Forwarded-For is honored only when
// `security.trust_proxy` is set (§11): without a proxy, the socket peer is the real client and the
// header is attacker-controlled, so trusting it would let a direct client rotate XFF per request
// and defeat the per-IP throttle. Behind a proxy the peer is the proxy, so the first forwarded hop
// is the real client and trusting it is correct — and correct only there.
func (o Origin) ClientIP(r *http.Request) string {
	if o.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if i := strings.IndexByte(xff, ','); i >= 0 {
				return strings.TrimSpace(xff[:i])
			}
			return strings.TrimSpace(xff)
		}
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	return host
}
