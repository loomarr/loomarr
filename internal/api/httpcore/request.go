package httpcore

import (
	"context"
	"net/http"
	"net/netip"
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
	// TrustedProxies is `security.trusted_proxies`: the address ranges a reverse proxy in front of
	// Loomarr connects from. A request whose socket peer is inside one has its X-Forwarded-For,
	// X-Forwarded-Proto and geography headers honored; from any other peer they are the client's own
	// claim and are ignored. Empty trusts no peer, loopback included.
	TrustedProxies []netip.Prefix
}

// FromTrustedProxy reports whether the request's socket peer is one of the configured trusted
// proxies. It is the single question behind every forwarded-header decision (client address,
// protocol, geography), so none of them can trust a peer the others refuse.
func (o Origin) FromTrustedProxy(r *http.Request) bool {
	peer, ok := peerAddr(r.RemoteAddr)
	if !ok {
		return false
	}
	for _, prefix := range o.TrustedProxies {
		if prefix.Contains(peer) {
			return true
		}
	}
	return false
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
		// X-Forwarded-Proto is honored only from a trusted proxy (§11). A direct client
		// could otherwise send `X-Forwarded-Proto: http` to keep its OWN cookie non-Secure;
		// that only weakens the sender's session, but the header is not trustworthy input
		// unless a proxy sets it, so it rides the same gate as the rate-limit IP.
		if o.FromTrustedProxy(r) && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
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

// ClientIP returns the address the login rate-limiter keys on, in a normalized form so one address
// has one key. X-Forwarded-For is read only when the socket peer is inside `security.trusted_proxies`
// (§11, FromTrustedProxy): otherwise the header is the client's own claim, and honoring it would
// let a client rotate it per request and defeat the per-IP throttle.
//
// A proxy appends the address it saw to the header, so only the right-most hop is the trusted
// proxy's own observation; everything to its left was written by whoever sent the request. That one
// hop is the client. A hop further left is never taken, even when the right-most one is a private
// address: a private address cannot be told apart from a LAN client, and taking the hop to its left
// would let that client pick its own key. Behind a chain of proxies every client therefore shares
// the inner proxy's key, a stricter throttle rather than a forgeable one. A right-most hop that is
// not an address falls back to the peer.
func (o Origin) ClientIP(r *http.Request) string {
	peer, ok := peerAddr(r.RemoteAddr)
	if !ok {
		return r.RemoteAddr
	}
	if !o.FromTrustedProxy(r) {
		return peer.String()
	}
	values := r.Header.Values("X-Forwarded-For")
	if len(values) == 0 {
		return peer.String()
	}
	last := values[len(values)-1]
	if i := strings.LastIndexByte(last, ','); i >= 0 {
		last = last[i+1:]
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(last))
	if err != nil {
		return peer.String()
	}
	return addr.WithZone("").Unmap().String()
}

// peerAddr parses a socket address ("host:port", IPv6 bracketed) into a normalized address.
func peerAddr(remote string) (netip.Addr, bool) {
	if ap, err := netip.ParseAddrPort(remote); err == nil {
		return ap.Addr().WithZone("").Unmap(), true
	}
	if addr, err := netip.ParseAddr(remote); err == nil {
		return addr.WithZone("").Unmap(), true
	}
	return netip.Addr{}, false
}
