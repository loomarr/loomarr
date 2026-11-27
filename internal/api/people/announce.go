package people

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// readOnlyPosts are the people routes that take a POST but store nothing an admin's list shows:
// the two previews of a link before it is used. A route missing from here costs a refetch, never
// a stale list. A device's pairing poll is not one: the poll that finds its pairing approved
// stores the device's token, and only a pending poll stores nothing, answering 428.
var readOnlyPosts = map[string]bool{
	"preview-invitation":        true,
	"preview-password-recovery": true,
}

// storingGets are the people routes that take a GET but store a change: the SSO callback mints a
// session and answers with a redirect onward.
var storingGets = map[string]bool{
	"sso-callback": true,
}

// announcing returns api with the access announcement on every route registered through it: once
// a write answers, Changed runs, so People reads its users, invitations and devices again
// (#3352). The write is stored by then, so the read sees it. Announcing from the route rather than
// from each handler keeps a new route from being forgotten.
func (s *Handlers) announcing(api huma.API) huma.API {
	if s.changed == nil {
		return api
	}
	group := huma.NewGroup(api)
	group.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(ctx)
		op := ctx.Operation()
		status := ctx.Status()
		switch {
		case storingGets[op.OperationID]:
			// A redirect is how the callback answers, whether or not it signed anyone in; a
			// refused sign-in costs a refetch.
			if status >= 200 && status < 400 {
				s.changed()
			}
		case op.Method == http.MethodGet || op.Method == http.MethodHead || readOnlyPosts[op.OperationID]:
		case status >= 200 && status < 300:
			s.changed()
		}
	})
	return group
}
