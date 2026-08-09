package api_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

// devLoginServer builds the auth stack with the dev-login gate in a chosen state, so
// the negative and positive cases below differ ONLY by that flag (§11/§19).
func devLoginServer(t *testing.T, devLogin bool, seed func(store.Store)) *httptest.Server {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/devlogin.db")
	t.Cleanup(func() { _ = st.Close() })

	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
	mgr := auth.NewManager(st, time.Hour, time.Now)

	if seed != nil {
		seed(st)
	}

	n := 0
	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:    st,
		Auth:     api.NewSessionAuthorizer(mgr, "break-glass-token"),
		Log:      slog.New(slog.DiscardHandler),
		Login:    auth.NewLoginService(lib, st, mgr, nil, time.Now),
		Sessions: mgr,
		// Provision is what mounts /v1/setup/state — without it that route is absent
		// and TestSetupStateReportsDevLoginFlag would decode a 404 body into a zero
		// struct, "passing" the off case for entirely the wrong reason.
		Provision:    auth.NewProvisioner(st, lib, func() string { n++; return "local-" + string(rune('a'+n-1)) }, time.Now),
		CookieSecure: "false",
		DevLogin:     devLogin,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func seedAdmin(t *testing.T, st store.Store, id, name string, role store.Role) {
	t.Helper()
	u := store.User{ID: id, Name: name, Role: role, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := st.UpsertUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}

// THE negative that matters (§19): with the flag unset — the default, and what every
// shipped install runs — the route is not registered at all, so it 404s. This is the
// whole security property of the feature; if it ever regresses, a production binary
// grows a credential-free admin door.
func TestDevLoginAbsentByDefault(t *testing.T) {
	srv := devLoginServer(t, false, func(st store.Store) {
		seedAdmin(t, st, "u-boss", "boss", store.RoleAdmin)
	})

	res, err := http.Post(srv.URL+"/v1/auth/dev-login", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("dev-login with LOOMARR_DEV_LOGIN unset = %d, want 404 (the route must not exist)", res.StatusCode)
	}
	// No session may be handed out on the way to that 404.
	if c := res.Cookies(); len(c) != 0 {
		t.Fatalf("dev-login 404 set %d cookie(s), want none", len(c))
	}
}

// The positive case, through the SAME composition root. Without it the 404 above
// would pass vacuously — a typo'd path or a server that registered no routes at all
// would satisfy it just as well.
func TestDevLoginIssuesAdminSessionWhenEnabled(t *testing.T) {
	srv := devLoginServer(t, true, func(st store.Store) {
		seedAdmin(t, st, "u-boss", "boss", store.RoleAdmin)
	})

	res, err := http.Post(srv.URL+"/v1/auth/dev-login", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("dev-login enabled = %d, want 200", res.StatusCode)
	}
	var body struct {
		ID   string `json:"id"`
		Role string `json:"role"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ID != "u-boss" || body.Role != "admin" {
		t.Fatalf("dev-login returned %+v, want the seeded admin u-boss", body)
	}

	// It must issue a REAL session cookie — one that actually authenticates a
	// subsequent request, not just a 200 that looks like success.
	var session *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == auth.CookieName {
			session = c
		}
	}
	if session == nil {
		t.Fatal("dev-login issued no session cookie")
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/auth/me", nil)
	req.AddCookie(session)
	me, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = me.Body.Close() }()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("session from dev-login failed on /v1/auth/me: %d", me.StatusCode)
	}
}

// Dev login borrows an admin; it never mints one (§11's allowlist invariant). An
// install with no admin row must be refused, or the bypass would quietly become a
// provisioning path around the wizard.
func TestDevLoginRefusesWhenNoAdminExists(t *testing.T) {
	srv := devLoginServer(t, true, func(st store.Store) {
		// A member exists, but no admin.
		seedAdmin(t, st, "u-kid", "kid", store.RoleMember)
	})

	res, err := http.Post(srv.URL+"/v1/auth/dev-login", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("dev-login with no admin = %d, want 401", res.StatusCode)
	}
	if c := res.Cookies(); len(c) != 0 {
		t.Fatalf("refused dev-login set %d cookie(s), want none", len(c))
	}
}

// A disabled admin is not a usable identity — Login rejects one, and dev login must
// not become the way around that.
func TestDevLoginSkipsDisabledAdmin(t *testing.T) {
	srv := devLoginServer(t, true, func(st store.Store) {
		u := store.User{ID: "u-off", Name: "off", Role: store.RoleAdmin, Disabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := st.UpsertUser(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	})

	res, err := http.Post(srv.URL+"/v1/auth/dev-login", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("dev-login with only a DISABLED admin = %d, want 401", res.StatusCode)
	}
}

// setup/state must report exactly what was mounted, or the login screen offers a link
// that 404s (or hides one that works).
func TestSetupStateReportsDevLoginFlag(t *testing.T) {
	for _, tc := range []struct {
		name string
		on   bool
	}{{"off", false}, {"on", true}} {
		t.Run(tc.name, func(t *testing.T) {
			srv := devLoginServer(t, tc.on, func(st store.Store) {
				seedAdmin(t, st, "u-boss", "boss", store.RoleAdmin)
			})
			res, err := http.Get(srv.URL + "/v1/setup/state")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = res.Body.Close() }()
			// Assert the status BEFORE decoding: a 404 decodes cleanly into a zero
			// struct, which would report devLogin=false and silently pass the off case.
			if res.StatusCode != http.StatusOK {
				t.Fatalf("GET /v1/setup/state = %d, want 200", res.StatusCode)
			}
			var body struct {
				DevLogin bool `json:"devLogin"`
			}
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.DevLogin != tc.on {
				t.Fatalf("setup/state devLogin = %v, want %v", body.DevLogin, tc.on)
			}
		})
	}
}

// pprof is gated exactly like dev-login (§7): mounted only when the server was started with
// LOOMARR_PPROF=1, and simply ABSENT otherwise. The negative is the one that matters — these
// handlers are unauthenticated by nature and expose stack traces and memory contents, so a
// shipped install must not serve them.
func TestPprofAbsentByDefault(t *testing.T) {
	srv := devLoginServer(t, false, nil)

	// Both the canonical /v1 paths and the bare aliases: the profiler moved under /v1 like the
	// rest of the ops surface, and an install without the flag must serve NEITHER. Checking only
	// one would leave the other reachable on a change that missed a registration.
	for _, p := range []string{
		"/v1/debug/pprof/", "/v1/debug/pprof/profile", "/v1/debug/pprof/heap",
		"/debug/pprof/", "/debug/pprof/profile", "/debug/pprof/heap",
	} {
		res, err := http.Get(srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		_ = res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("%s with LOOMARR_PPROF unset = %d, want 404 (the route must not exist)", p, res.StatusCode)
		}
	}
}
