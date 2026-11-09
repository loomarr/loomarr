package testkit

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// MediaServer is the shared Emby/Jellyfin test double (AGENTS.md: one shared
// mock per service, both flavors). It serves the pinned Phase-0 fixtures so the
// library adapter is tested against ground truth, never the network. It also
// records the auth headers it received, so flavor-specific header construction
// can be asserted.
type MediaServer struct {
	*httptest.Server
	// mu guards the mutable fields below against the httptest handler goroutine racing a
	// test goroutine — needed once a background job (e.g. the library scan) issues requests
	// concurrently with the test. Header captures take it.
	mu sync.RWMutex
	// LastAuthHeader / LastEmbyToken / LastEmbyAuthz capture what the adapter sent
	// on the most recent request, for header-shape assertions.
	LastAuthHeader string
	LastEmbyToken  string
	LastEmbyAuthz  string
	requests       []MediaServerRequest
	// AdminToken is the only token the mock accepts for /Users, /Items and
	// /Library/VirtualFolders; any other request is answered 401, as Emby does.
	AdminToken string
	// GoodUser/GoodPass authenticate successfully via /Users/AuthenticateByName.
	GoodUser string
	GoodPass string
	// Accounts maps username→(password, isAdmin, disabled) for AuthenticateByName.
	// If nil, GoodUser/GoodPass authenticate as an admin. Lets auth tests model
	// admin vs member vs disabled logins.
	Accounts map[string]Account
	// AuthStatus, when non-zero, forces AuthenticateByName to return that status.
	// It lets auth tests distinguish authoritative rejection from provider outage.
	AuthStatus int
	// Users, when set, is the account list returned by GET /Users. Nil keeps the
	// pinned Emby fixture; a non-nil slice lets provisioning tests cover mixed
	// roles and disabled accounts through the real library adapter.
	Users []MediaServerUser
}

// MediaServerRequest is one captured call to the shared media-server double. Tests that
// exercise live configuration use the per-request record instead of racing on the legacy
// last-header fields or mistaking a background scan for the operation under assertion.
type MediaServerRequest struct {
	Method        string
	Path          string
	RawQuery      string
	Authorization string
	EmbyToken     string
	EmbyAuthz     string
}

// Requests returns a copy of every captured media-server request in arrival order.
func (ms *MediaServer) Requests() []MediaServerRequest {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return append([]MediaServerRequest(nil), ms.requests...)
}

// Account is a media-server login the mock accepts.
type Account struct {
	Password string
	ID       string
	IsAdmin  bool
	Disabled bool
}

// MediaServerUser is one account returned by the mock's GET /Users endpoint.
type MediaServerUser struct {
	ID       string
	Name     string
	IsAdmin  bool
	Disabled bool
}

// NewMediaServer starts a mock media server serving the pinned fixtures.
func NewMediaServer(t testing.TB) *MediaServer {
	t.Helper()
	ms := &MediaServer{AdminToken: "test-admin-token", GoodUser: "Fixture Admin", GoodPass: "correct-horse"}
	mux := http.NewServeMux()

	// /Library/VirtualFolders — library enumeration, for resolving a filler library's NAME to the
	// item id `ParentId` needs (§10 V38c).
	//
	// ⚠ Serves a BARE ARRAY, because that is what Emby 4.10.0.22 returns — the one endpoint here
	// without the `{"Items": […]}` envelope. Captured 2026-08-02; see fixtures/emby/FINDINGS.md.
	// Wrapping it for consistency would make every test agree with a parser that cannot read the
	// real server.
	mux.HandleFunc("GET /Library/VirtualFolders", func(w http.ResponseWriter, r *http.Request) {
		ms.capture(r)
		if !ms.authorized(w, r) {
			return
		}
		_, _ = w.Write(Fixture(t, "emby/virtual_folders.json"))
	})

	// /Items — a ParentId-scoped read (§10 filler) answers the pinned filler fixture; anything
	// else (the cutover id map, collections) an empty list. Loomarr reads its catalog from its
	// own Library, never from here (#2151 step 16).
	mux.HandleFunc("GET /Items", func(w http.ResponseWriter, r *http.Request) {
		ms.capture(r)
		if !ms.authorized(w, r) {
			return
		}
		if pid := r.URL.Query().Get("ParentId"); pid != "" {
			_, _ = w.Write(Fixture(t, "emby/filler_library.json"))
			return
		}
		_, _ = w.Write([]byte(`{"Items":[],"TotalRecordCount":0}`))
	})

	// /Users — list for import/sync (§11).
	mux.HandleFunc("GET /Users", func(w http.ResponseWriter, r *http.Request) {
		ms.capture(r)
		if !ms.authorized(w, r) {
			return
		}
		if ms.Users != nil {
			type policy struct {
				IsAdministrator bool `json:"IsAdministrator"`
				IsDisabled      bool `json:"IsDisabled"`
			}
			type user struct {
				ID     string `json:"Id"`
				Name   string `json:"Name"`
				Policy policy `json:"Policy"`
			}
			users := make([]user, 0, len(ms.Users))
			for _, u := range ms.Users {
				users = append(users, user{
					ID: u.ID, Name: u.Name,
					Policy: policy{IsAdministrator: u.IsAdmin, IsDisabled: u.Disabled},
				})
			}
			_ = json.NewEncoder(w).Encode(users)
			return
		}
		_, _ = w.Write(Fixture(t, "emby/users_list.json"))
	})

	// /Users/AuthenticateByName — credential check (§11).
	mux.HandleFunc("POST /Users/AuthenticateByName", func(w http.ResponseWriter, r *http.Request) {
		ms.capture(r)
		var body struct{ Username, Pw string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		if ms.AuthStatus != 0 {
			w.WriteHeader(ms.AuthStatus)
			return
		}

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
					"Id": "00000000000000000000000000000007", "Name": ms.GoodUser,
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

// authorized reports whether the request carries AdminToken in one of the forms Emby and
// Jellyfin accept; otherwise it answers 401, as Emby does.
func (ms *MediaServer) authorized(w http.ResponseWriter, r *http.Request) bool {
	ms.mu.RLock()
	token := ms.AdminToken
	ms.mu.RUnlock()
	for _, v := range []string{r.Header.Get("X-Emby-Token"), r.Header.Get("X-MediaBrowser-Token"), r.URL.Query().Get("api_key")} {
		if v != "" && v == token {
			return true
		}
	}
	for _, h := range []string{r.Header.Get("Authorization"), r.Header.Get("X-Emby-Authorization")} {
		if token != "" && strings.Contains(h, `Token="`+token+`"`) {
			return true
		}
	}
	http.Error(w, "Access token is invalid or expired.", http.StatusUnauthorized)
	return false
}

// SetAdminToken rotates the accepted API key under the lock, as an operator rotating the media
// server's key would, for a test that reconfigures the connection while requests are in flight.
func (ms *MediaServer) SetAdminToken(token string) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.AdminToken = token
}

func (ms *MediaServer) capture(r *http.Request) {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	ms.LastAuthHeader = r.Header.Get("Authorization")
	ms.LastEmbyToken = r.Header.Get("X-Emby-Token")
	ms.LastEmbyAuthz = r.Header.Get("X-Emby-Authorization")
	ms.requests = append(ms.requests, MediaServerRequest{
		Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery,
		Authorization: ms.LastAuthHeader, EmbyToken: ms.LastEmbyToken, EmbyAuthz: ms.LastEmbyAuthz,
	})
}
