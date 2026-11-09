package httpcore

import "net/http"

const FrameAncestors = "frame-ancestors 'self'"

// WithSecurityHeaders sets the headers every response carries, API and SPA alike.
//
// nosniff pins every response to its declared type, so no served byte (a clip, an image, a help
// diagram, a problem body) can be re-read by a browser as HTML or script. same-origin keeps the
// app's own URLs, which can carry ids and share tokens, out of the Referer of every cross-origin
// request; an element that needs a referrer, like the YouTube preview frame, sets its own policy.
// frame-ancestors 'self' lets only Loomarr frame Loomarr: the session is a cookie, so a page that
// framed it would frame a signed-in household and could trick a click. A homelab dashboard that
// embeds apps shows a blocked frame and links out instead. A handler that sets its own policy
// carries the directive too (serveDiagram).
func WithSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", FrameAncestors)
		next.ServeHTTP(w, r)
	})
}
