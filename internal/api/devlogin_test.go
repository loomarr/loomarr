package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	peopleapi "github.com/loomarr/loomarr/internal/api/people"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/testkit"
)

type devAccessHarnessConfig struct {
	DevLogin bool
	Pprof    bool
	Demo     peopleapi.DemoAccount
	Seed     func(records.Store)
}

// newDevAccessHarness owns the auth stack for opt-in development route gates.
// Negative and positive cases differ only by the named gate behavior (§11/§19).
func newDevAccessHarness(t *testing.T, config devAccessHarnessConfig) *apiHarness {
	t.Helper()
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	return startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		lib := mediaserver.New(mediaserver.Emby, ms.URL, ms.AdminToken, "dev")
		mgr := auth.NewManager(defaults.Store, time.Hour, time.Now)
		if config.Seed != nil {
			config.Seed(defaults.Store)
		}
		n := 0
		return api.Router(defaults.Log, api.Options{
			Store:    defaults.Store,
			Auth:     sessionAuth(mgr, "break-glass-token"),
			Log:      defaults.Log,
			Login:    auth.NewLoginService(lib, defaults.Store, mgr, nil, time.Now),
			Sessions: mgr,
			// Provision mounts /v1/setup/state so the off case cannot pass by
			// decoding a 404 response into a zero-valued state.
			Provision:    auth.NewProvisioner(defaults.Store, lib, func() string { n++; return "local-" + string(rune('a'+n-1)) }, time.Now),
			CookieSecure: "false",
			DevLogin:     config.DevLogin,
			Pprof:        config.Pprof,
			Demo:         config.Demo,
		})
	})
}

func newDevLoginHarness(t *testing.T, devLogin bool, seed func(records.Store)) *apiHarness {
	t.Helper()
	return newDevAccessHarness(t, devAccessHarnessConfig{DevLogin: devLogin, Seed: seed})
}

func seedAdmin(t *testing.T, st records.Store, id, name string, role identitystore.Role) {
	t.Helper()
	u := identitystore.User{ID: id, Name: name, Role: role, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := st.UpsertUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}

// THE negative that matters (§19): with the flag unset — the default, and what every
// shipped install runs — the route is not registered at all, so it 404s. This is the
// whole security property of the feature; if it ever regresses, a production binary
// grows a credential-free admin door.
func TestDevLoginAbsentByDefault(t *testing.T) {
	srv := newDevLoginHarness(t, false, func(st records.Store) {
		seedAdmin(t, st, "u-boss", "boss", identitystore.RoleAdmin)
	}).Server

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
	srv := newDevLoginHarness(t, true, func(st records.Store) {
		seedAdmin(t, st, "u-boss", "boss", identitystore.RoleAdmin)
	}).Server

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
	srv := newDevLoginHarness(t, true, func(st records.Store) {
		// A member exists, but no admin.
		seedAdmin(t, st, "u-kid", "kid", identitystore.RoleMember)
	}).Server

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
	srv := newDevLoginHarness(t, true, func(st records.Store) {
		u := identitystore.User{ID: "u-off", Name: "off", Role: identitystore.RoleAdmin, Disabled: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}
		if err := st.UpsertUser(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}).Server

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
			srv := newDevLoginHarness(t, tc.on, func(st records.Store) {
				seedAdmin(t, st, "u-boss", "boss", identitystore.RoleAdmin)
			}).Server
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

// setup/state carries demo mode to the login screen before anyone signs in (#2268). The object is
// always on the wire, so a client never meets null, and only a demo site names an account.
func TestSetupStateReportsDemoMode(t *testing.T) {
	for _, tc := range []struct {
		name    string
		account peopleapi.DemoAccount
		want    string
	}{
		{"off", peopleapi.DemoAccount{}, `{"enabled":false,"username":"","password":""}`},
		{"on", peopleapi.DemoAccount{Username: "guest", Password: "loomarr-guest"}, `{"enabled":true,"username":"guest","password":"loomarr-guest"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := newDevAccessHarness(t, devAccessHarnessConfig{Demo: tc.account, Seed: func(st records.Store) {
				seedAdmin(t, st, "u-boss", "boss", identitystore.RoleAdmin)
			}}).Server
			res, err := http.Get(srv.URL + "/v1/setup/state")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = res.Body.Close() }()
			if res.StatusCode != http.StatusOK {
				t.Fatalf("GET /v1/setup/state = %d, want 200", res.StatusCode)
			}
			var body struct {
				Demo json.RawMessage `json:"demo"`
			}
			if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if string(body.Demo) != tc.want {
				t.Fatalf("setup/state demo = %s, want %s", body.Demo, tc.want)
			}
		})
	}
}

// pprof is gated exactly like dev-login (§7): mounted only when the server was started with
// LOOMARR_PPROF=1, and simply ABSENT otherwise. The negative is the one that matters — these
// handlers are unauthenticated by nature and expose stack traces and memory contents, so a
// shipped install must not serve them.
func TestPprofAbsentByDefault(t *testing.T) {
	srv := newDevLoginHarness(t, false, nil).Server

	// Both the canonical /v1 paths and the bare aliases: the profiler moved under /v1 like the
	// rest of the ops surface, and an install without the flag must serve NEITHER. Checking only
	// one would leave the other reachable on a change that missed a registration.
	for _, p := range []string{
		"/v1/debug/pprof/", "/v1/debug/pprof/profile", "/v1/debug/pprof/heap", "/v1/debug/pprof/goroutineleak",
		"/debug/pprof/", "/debug/pprof/profile", "/debug/pprof/heap", "/debug/pprof/goroutineleak",
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

func TestPprofGoroutineLeakProfileWhenEnabled(t *testing.T) {
	srv := newDevAccessHarness(t, devAccessHarnessConfig{Pprof: true}).Server

	for _, p := range []string{"/v1/debug/pprof/goroutineleak", "/debug/pprof/goroutineleak"} {
		res, err := http.Get(srv.URL + p + "?debug=1")
		if err != nil {
			t.Fatal(err)
		}
		body, readErr := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s with LOOMARR_PPROF enabled = %d, want 200", p, res.StatusCode)
		}
		if !strings.Contains(string(body), "goroutineleak profile: total ") {
			t.Fatalf("%s served the wrong profile: %q", p, body)
		}
	}
}
