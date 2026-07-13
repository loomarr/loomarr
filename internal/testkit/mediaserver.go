package testkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// MediaServer is the shared Emby/Jellyfin test double (CLAUDE.md: one shared
// mock per service, both flavors). It serves the pinned Phase-0 fixtures so the
// library adapter is tested against ground truth, never the network. It also
// records the auth headers it received, so flavor-specific header construction
// can be asserted.
type MediaServer struct {
	*httptest.Server
	// LastAuthHeader / LastEmbyToken / LastEmbyAuthz capture what the adapter sent
	// on the most recent request, for header-shape assertions.
	LastAuthHeader string
	LastEmbyToken  string
	LastEmbyAuthz  string
	// AdminToken is the token the mock accepts for /Users and /Items.
	AdminToken string
	// GoodUser/GoodPass authenticate successfully via /Users/AuthenticateByName.
	GoodUser string
	GoodPass string
	// PresentTMDB, if set, is an additional tmdb id the mock reports present
	// (beyond the pinned 16153) — lets ingest tests flip a title to confirmed.
	PresentTMDB string
	// Accounts maps username→(password, isAdmin, disabled) for AuthenticateByName.
	// If nil, GoodUser/GoodPass authenticate as an admin. Lets auth tests model
	// admin vs member vs disabled logins.
	Accounts map[string]Account
}

// Account is a media-server login the mock accepts.
type Account struct {
	Password string
	ID       string
	IsAdmin  bool
	Disabled bool
}

// NewMediaServer starts a mock media server serving the pinned fixtures.
func NewMediaServer(t testing.TB) *MediaServer {
	t.Helper()
	ms := &MediaServer{AdminToken: "test-admin-token", GoodUser: "Matt", GoodPass: "correct-horse"}
	mux := http.NewServeMux()

	// /Items — presence lookup (§6). AnyProviderIdEquals decides present/absent.
	mux.HandleFunc("GET /Items", func(w http.ResponseWriter, r *http.Request) {
		ms.capture(r)
		prov := r.URL.Query().Get("AnyProviderIdEquals")
		// The pinned present fixture is tmdb.16153 (Phase 0); a test may add one
		// more present id via PresentTMDB. Anything else is absent.
		present := strings.EqualFold(prov, "tmdb.16153") ||
			(ms.PresentTMDB != "" && strings.EqualFold(prov, "tmdb."+ms.PresentTMDB))
		if present {
			_, _ = w.Write(Fixture(t, "emby/lookup_present.json"))
			return
		}
		_, _ = w.Write(Fixture(t, "emby/lookup_absent.json"))
	})

	// /Users — list for import/sync (§11).
	mux.HandleFunc("GET /Users", func(w http.ResponseWriter, r *http.Request) {
		ms.capture(r)
		_, _ = w.Write(Fixture(t, "emby/users_list.json"))
	})

	// /Users/AuthenticateByName — credential check (§11).
	mux.HandleFunc("POST /Users/AuthenticateByName", func(w http.ResponseWriter, r *http.Request) {
		ms.capture(r)
		var body struct{ Username, Pw string }
		_ = json.NewDecoder(r.Body).Decode(&body)

		// Configurable accounts take precedence; otherwise the default admin.
		if ms.Accounts != nil {
			if acct, ok := ms.Accounts[body.Username]; ok && acct.Password == body.Pw {
				id := acct.ID
				if id == "" {
					id = "user-" + body.Username
				}
				resp := map[string]any{
					"AccessToken": "ms-session-token",
					"User": map[string]any{
						"Id": id, "Name": body.Username,
						"Policy": map[string]any{"IsAdministrator": acct.IsAdmin, "IsDisabled": acct.Disabled},
					},
				}
				_ = json.NewEncoder(w).Encode(resp)
				return
			}
		} else if body.Username == ms.GoodUser && body.Pw == ms.GoodPass {
			resp := map[string]any{
				"AccessToken": "ms-session-token",
				"User": map[string]any{
					"Id": "b07b7a15f38343c39f0da4283e896acc", "Name": ms.GoodUser,
					"Policy": map[string]any{"IsAdministrator": true, "IsDisabled": false},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		// Bad creds: the real Emby returns 401 (Phase 0 pinned this).
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write(Fixture(t, "emby/auth_badpw_response.json"))
	})

	// /Sessions/Logout — best-effort token discard (§11).
	mux.HandleFunc("POST /Sessions/Logout", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	ms.Server = httptest.NewServer(mux)
	return ms
}

func (ms *MediaServer) capture(r *http.Request) {
	ms.LastAuthHeader = r.Header.Get("Authorization")
	ms.LastEmbyToken = r.Header.Get("X-Emby-Token")
	ms.LastEmbyAuthz = r.Header.Get("X-Emby-Authorization")
}
