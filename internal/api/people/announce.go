package people

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// readOnlyPosts are the people routes that take a POST but store nothing an admin's list shows:
// a device asking whether it is approved yet, and the two previews of a link before it is used.
// A route missing from here costs a refetch, never a stale list.
var readOnlyPosts = map[string]bool{
	"device-pair-poll":          true,
	"preview-invitation":        true,
	"preview-password-recovery": true,
}

// announcing returns api with the access announcement on every route registered through it: once
// a write answers 2xx, Changed runs, so People reads its users, invitations and devices again
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
		if op.Method == http.MethodGet || op.Method == http.MethodHead || readOnlyPosts[op.OperationID] {
			return
		}
		if status := ctx.Status(); status >= 200 && status < 300 {
			s.changed()
		}
	})
	return group
}
