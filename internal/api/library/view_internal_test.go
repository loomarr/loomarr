package library

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/loomarr/loomarr/internal/api/httpcore"
)

// Only an admin reads the Library's admin-only facts: a member, an anonymous caller and a context
// the middleware never stamped all read the member's view, so a missing role fails closed.
func TestReadsAdminFactsOnlyForAnAdmin(t *testing.T) {
	stamped := func(role httpcore.Role) huma.Context {
		r := httptest.NewRequest(http.MethodGet, "/v1/library/items", nil)
		return httpcore.ContextWithRole(humatest.NewContext(&huma.Operation{}, r, httptest.NewRecorder()), role)
	}
	for _, tc := range []struct {
		name string
		ctx  huma.Context
		want bool
	}{
		{"admin", stamped(httpcore.RoleAdmin), true},
		{"member", stamped(httpcore.RoleMember), false},
		{"anonymous", stamped(httpcore.RoleAnonymous), false},
		{"public", stamped(httpcore.RolePublic), false},
		{"unstamped", humatest.NewContext(&huma.Operation{}, httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder()), false},
	} {
		if got := readsAdminFacts(tc.ctx.Context()); got != tc.want {
			t.Errorf("%s reads admin facts = %v, want %v", tc.name, got, tc.want)
		}
	}
}
