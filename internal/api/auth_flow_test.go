package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

// authServer builds the full session-auth stack over a mock media server with an
// admin and a member account.
func authServer(t *testing.T) (*httptest.Server, store.Store, *testkit.MediaServer) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/auth.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	ms.Accounts = map[string]testkit.Account{
		"boss": {Password: "pw", ID: "u-boss", IsAdmin: true},
		"kid":  {Password: "pw", ID: "u-kid", IsAdmin: false},
	}
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
	mgr := auth.NewManager(st, time.Hour, time.Now)
	loginSvc := auth.NewLoginService(lib, st, mgr, nil, time.Now)
	userSync := auth.NewUserSync(lib, st, time.Now)

	// §11 rework: identity is the allowlist — users must be imported before they
	// can log in (no lazy self-provision). Seed boss (admin) + kid (member).
	seedImported(t, st, "u-boss", "boss", store.RoleAdmin)
	seedImported(t, st, "u-kid", "kid", store.RoleMember)

	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:        st,
		Auth:         api.NewSessionAuthorizer(mgr, "break-glass-token"),
		Log:          slog.New(slog.DiscardHandler),
		Login:        loginSvc,
		Sessions:     mgr,
		UserSync:     userSync,
		CookieSecure: "false",
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, st, ms
}

// seedImported allowlists a media-server user directly (§11: the store IS the
// allowlist), so the flow tests can log in as an imported user.
func seedImported(t *testing.T, st store.Store, id, name string, role store.Role) {
	t.Helper()
	u := store.User{ID: id, Name: name, Role: role, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := st.UpsertUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}

// login performs a login and returns the session cookie.
func login(t *testing.T, srv *httptest.Server, user, pass string) *http.Cookie {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": user, "password": pass})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/auth/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login %s → %d, want 200", user, resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName && c.Value != "" {
			return c
		}
	}
	t.Fatal("login set no session cookie")
	return nil
}

// authed builds a request carrying the session cookie (+ CSRF header for mutations).
func authed(t *testing.T, method, url string, cookie *http.Cookie, body string) *http.Response {
	t.Helper()
	var rdr *strings.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	} else {
		rdr = strings.NewReader("")
	}
	req, _ := http.NewRequest(method, url, rdr)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		req.Header.Set("X-Loomarr-Csrf", "1")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// The login cookie is HttpOnly + SameSite=Strict (§11).
func TestLoginCookieFlags(t *testing.T) {
	srv, _, _ := authServer(t)
	c := login(t, srv, "boss", "pw")
	if !c.HttpOnly {
		t.Error("session cookie not HttpOnly (§11)")
	}
	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("session cookie SameSite = %v, want Strict (§11)", c.SameSite)
	}
}

// /v1/auth/me reflects the signed-in user and role.
func TestMe(t *testing.T) {
	srv, _, _ := authServer(t)
	c := login(t, srv, "kid", "pw")
	resp := authed(t, http.MethodGet, srv.URL+"/v1/auth/me", c, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("me → %d", resp.StatusCode)
	}
	var body struct{ ID, Role string }
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.ID != "u-kid" || body.Role != "member" {
		t.Errorf("me = %+v, want u-kid/member", body)
	}
}

// THE GATE (§19 negative): a member is 403'd on admin routes.
func TestMemberForbiddenOnAdminRoutes(t *testing.T) {
	srv, _, _ := authServer(t)
	member := login(t, srv, "kid", "pw")

	cases := []struct{ method, path, body string }{
		{http.MethodPost, "/v1/titles", `{"mediaType":"movie","tmdbId":1}`},
		{http.MethodDelete, "/v1/titles/movie:tmdb:1", ""},
		{http.MethodGet, "/v1/users", ""},
		{http.MethodPatch, "/v1/users/u-boss", `{"role":"member"}`},
	}
	for _, tc := range cases {
		resp := authed(t, tc.method, srv.URL+tc.path, member, tc.body)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("member %s %s → %d, want 403 (§19)", tc.method, tc.path, resp.StatusCode)
		}
	}
}

// An admin passes the same routes.
func TestAdminAllowedOnAdminRoutes(t *testing.T) {
	srv, _, _ := authServer(t)
	admin := login(t, srv, "boss", "pw")
	resp := authed(t, http.MethodGet, srv.URL+"/v1/users", admin, "")
	if resp.StatusCode != http.StatusOK {
		t.Errorf("admin GET /v1/users → %d, want 200", resp.StatusCode)
	}
	resp = authed(t, http.MethodPost, srv.URL+"/v1/titles", admin, `{"mediaType":"movie","tmdbId":42}`)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("admin POST /v1/titles → %d, want 200", resp.StatusCode)
	}
}

// THE GATE (§19): disabling a user kills their session immediately — the next
// request with the old cookie is unauthenticated.
func TestSessionDiesOnDisable(t *testing.T) {
	srv, _, _ := authServer(t)
	admin := login(t, srv, "boss", "pw")
	member := login(t, srv, "kid", "pw")

	// Member works before disable.
	if resp := authed(t, http.MethodGet, srv.URL+"/v1/auth/me", member, ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("member me before disable → %d", resp.StatusCode)
	}
	// Admin disables the member.
	resp := authed(t, http.MethodPatch, srv.URL+"/v1/users/u-kid", admin, `{"disabled":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable → %d", resp.StatusCode)
	}
	// The member's session no longer authenticates.
	resp = authed(t, http.MethodGet, srv.URL+"/v1/auth/me", member, "")
	if resp.StatusCode == http.StatusOK {
		t.Error("disabled user's session still works (§19 — sessions die on disable)")
	}
}

// CSRF: a cookie-authenticated mutation without X-Loomarr-Csrf is rejected (§11).
func TestCSRFRequiredForCookieMutations(t *testing.T) {
	srv, _, _ := authServer(t)
	admin := login(t, srv, "boss", "pw")
	// POST with cookie but NO csrf header.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/titles", strings.NewReader(`{"mediaType":"movie","tmdbId":9}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(admin)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("cookie mutation without CSRF → %d, want 403 (§11)", resp.StatusCode)
	}
}

// POST /v1/users/sync imports media-server users (admin only); the mock /Users
// fixture has 10 users, so a synced admin can then list them.
// §11 rework: sync REFRESHES imported users but NEVER adds. The fixture lists 10
// media-server users, but only the 2 seeded (imported) ones exist locally; sync
// must leave the count at 2, not balloon it to 10.
func TestUserSyncAdmin(t *testing.T) {
	srv, st, _ := authServer(t)
	before, _ := st.ListUsers(context.Background())
	admin := login(t, srv, "boss", "pw")
	resp := authed(t, http.MethodPost, srv.URL+"/v1/users/sync", admin, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sync → %d, want 200", resp.StatusCode)
	}
	after, _ := st.ListUsers(context.Background())
	if len(after) != len(before) {
		t.Errorf("sync added users (%d → %d); it must refresh imported users only, never add", len(before), len(after))
	}
	if len(after) != 2 {
		t.Errorf("expected exactly the 2 seeded users, got %d", len(after))
	}
}

// A member cannot trigger user sync.
func TestUserSyncMemberForbidden(t *testing.T) {
	srv, _, _ := authServer(t)
	member := login(t, srv, "kid", "pw")
	resp := authed(t, http.MethodPost, srv.URL+"/v1/users/sync", member, "")
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("member sync → %d, want 403", resp.StatusCode)
	}
}

// Logout revokes the session.
func TestLogout(t *testing.T) {
	srv, _, _ := authServer(t)
	c := login(t, srv, "boss", "pw")
	if resp := authed(t, http.MethodPost, srv.URL+"/v1/auth/logout", c, ""); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout → %d, want 204", resp.StatusCode)
	}
	if resp := authed(t, http.MethodGet, srv.URL+"/v1/auth/me", c, ""); resp.StatusCode == http.StatusOK {
		t.Error("session still works after logout")
	}
}

// The API_TOKEN Bearer still works as admin break-glass, and is CSRF-exempt.
func TestBreakGlassToken(t *testing.T) {
	srv, _, _ := authServer(t)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/titles", strings.NewReader(`{"mediaType":"movie","tmdbId":77}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer break-glass-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("break-glass token POST → %d, want 200 (no CSRF needed for Bearer)", resp.StatusCode)
	}
}
