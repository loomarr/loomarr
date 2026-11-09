package httpcore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// Request correlation (§7). Every request carries an id — taken from an inbound
// X-Request-Id when a proxy already set one, else generated here — that we (1) echo
// on the response header, (2) stash in the request context, and (3) stamp onto every
// error problem as its RFC 7807 `instance` while logging the full cause under the same
// id. So a user only ever sees a friendly message plus a short id; a developer greps
// the logs by that id to get the endpoint, cause, and stack — no debugging detail leaks
// to the client, and no client `?debug=` switch (an information-disclosure anti-pattern)
// is ever needed.
const requestIDHeader = "X-Request-Id"

type ctxKeyRequestID struct{}

// NewRequestID mints a short, log-greppable id ("req_" + 8 random bytes hex), matching
// the house id style (see internal/app/ids.go). crypto/rand can't
// fail in practice here; on the vanishingly unlikely error we fall back to a fixed marker
// rather than panic — a correlation id is a convenience, never a correctness dependency.
func NewRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req_unknown"
	}
	return "req_" + hex.EncodeToString(b[:])
}

// RequestIDFrom returns the correlation id carried on ctx, or "" if none (e.g. a code path
// that never passed through the middleware, such as a unit test calling a handler directly).
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(ctxKeyRequestID{}).(string)
	return id
}

// WithRequestID is the outermost app middleware: it establishes the correlation id for the
// request and echoes it back so a caller can quote it in a bug report. A well-formed inbound
// X-Request-Id (from a trusted reverse proxy) is honored so the id spans hops; anything
// unreasonable is replaced, so a client can't inject arbitrary log content via the header.
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := sanitizeRequestID(r.Header.Get(requestIDHeader))
		if id == "" {
			id = NewRequestID()
		}
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKeyRequestID{}, id)))
	})
}

// sanitizeRequestID accepts an inbound id only if it's a short run of unambiguous id
// characters — so a proxy-set id propagates, but a client can't smuggle newlines/control
// chars into our logs (log-injection) or send a megabyte header. Returns "" to reject.
func sanitizeRequestID(s string) string {
	if s == "" || len(s) > 128 {
		return ""
	}
	for _, c := range s {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.'
		if !ok {
			return ""
		}
	}
	return s
}
