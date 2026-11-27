package httpcore_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/loomarr/loomarr/internal/api/httpcore"
)

// A raw handler reads the identity the auth middleware resolved from r.Context(), exactly as a
// typed handler reads it from its ctx. That holds only because humago's context rebuilds its
// request on huma.WithValue; a huma upgrade or another adapter that kept the original request
// would make every raw handler see an anonymous caller (the image route's visibility check and the
// MCP endpoint's tool set both read it), so it is pinned here rather than assumed.
func TestRawOpHandlerSeesTheResolvedRole(t *testing.T) {
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("test", "1"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		next(httpcore.ContextWithRole(ctx, httpcore.RoleMember))
	})
	var got httpcore.Role
	httpcore.RawOp[struct{}](api, huma.Operation{OperationID: "raw", Method: http.MethodGet, Path: "/raw"},
		httpcore.RoleMember, func(w http.ResponseWriter, r *http.Request) {
			got = httpcore.RoleFrom(r.Context())
			w.WriteHeader(http.StatusNoContent)
		})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/raw", nil))

	if got != httpcore.RoleMember {
		t.Fatalf("raw handler saw role %q, want %q", got, httpcore.RoleMember)
	}
}
