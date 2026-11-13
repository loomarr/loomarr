package api_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/testkit"
)

// revokeFailingStore fails every session revoke, as a locked or unreachable database does.
type revokeFailingStore struct {
	store.Store
}

func (revokeFailingStore) RevokeSession(context.Context, string) error {
	return errors.New("database is locked")
}

// A logout whose revoke failed must not say it worked. The session is still valid server-side, so
// the response is a 500, it keeps the cookie (the person can retry), and the session still resolves.
func TestLogoutReportsAFailedRevoke(t *testing.T) {
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	ms.Accounts = map[string]testkit.Account{"boss": {Password: "pw", ID: "u-boss", IsAdmin: true}}
	harness := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		lib := mediaserver.New(mediaserver.Emby, ms.URL, ms.AdminToken, "dev")
		mgr := auth.NewManager(defaults.Store, time.Hour, time.Now)
		seedImported(t, defaults.Store, "u-boss", "boss", identitystore.RoleAdmin)
		return api.Router(defaults.Log, api.Options{
			Store:        defaults.Store,
			Auth:         sessionAuth(mgr, "break-glass-token"),
			Log:          defaults.Log,
			CookieSecure: "false",
			People: api.PeopleOptions{
				Login:    auth.NewLoginService(lib, defaults.Store, mgr, nil, time.Now),
				Sessions: auth.NewManager(revokeFailingStore{defaults.Store}, time.Hour, time.Now),
			},
		})
	})
	c := login(t, harness.Server, "boss", "pw")

	resp := authed(t, http.MethodPost, harness.Server.URL+"/v1/auth/logout", c, "")
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("logout with a failing revoke → %d, want 500", resp.StatusCode)
	}
	for _, set := range resp.Cookies() {
		if set.Name == auth.CookieName {
			t.Errorf("a failed logout still set the session cookie %+v; the person must keep it to retry", set)
		}
	}
	if resp := authed(t, http.MethodGet, harness.Server.URL+"/v1/auth/me", c, ""); resp.StatusCode != http.StatusOK {
		t.Errorf("the session after a failed logout → %d, want 200 (it is still live)", resp.StatusCode)
	}
}
