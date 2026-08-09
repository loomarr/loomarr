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

// provServer builds the provisioning stack (bootstrap + import + login) over a
// mock media server, with NO users seeded (so bootstrap is open).
func provServer(t *testing.T) (*httptest.Server, store.Store) {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/prov.db")
	t.Cleanup(func() { _ = st.Close() })

	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
	mgr := auth.NewManager(st, time.Hour, time.Now)
	n := 0
	newID := func() string { n++; return "local-" + string(rune('a'+n-1)) }

	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:        st,
		Auth:         api.NewSessionAuthorizer(mgr, adminToken),
		Log:          slog.New(slog.DiscardHandler),
		Login:        auth.NewLoginService(lib, st, mgr, nil, time.Now),
		Sessions:     mgr,
		UserSync:     auth.NewUserSync(lib, st, time.Now),
		Provision:    auth.NewProvisioner(st, lib, newID, time.Now),
		CookieSecure: "false",
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, st
}

// Bootstrap is UNAUTHENTICATED and succeeds once; a second call 409s (§11).
func TestBootstrap_OnceViaAPI(t *testing.T) {
	srv, _ := provServer(t)

	// First bootstrap: no auth needed, creates the owning admin.
	resp := do(t, srv, http.MethodPost, "/v1/setup/bootstrap", "", `{"username":"owner","password":"s3cret-pw"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("first bootstrap → %d, want 200", resp.StatusCode)
	}
	var body struct{ Role string }
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Role != "admin" {
		t.Errorf("bootstrap role = %q, want admin", body.Role)
	}

	// Second bootstrap → 409 (the door is closed).
	resp = do(t, srv, http.MethodPost, "/v1/setup/bootstrap", "", `{"username":"other","password":"pw2"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("second bootstrap → %d, want 409", resp.StatusCode)
	}
}

// GET /v1/setup/state is unauthenticated and flips exactly once bootstrap lands (§7).
//
// The regression it guards: the frontend is a static bundle, so with no unauthenticated
// signal every entry point resolves to /login — and a brand-new install has no account
// to log in with. The maintainer smoke walked into that dead end (FINDING 1); only an
// operator who guessed the /wizard URL could escape it.
func TestSetupState_UnauthenticatedAndFlipsOnBootstrap(t *testing.T) {
	srv, _ := provServer(t)

	state := func() bool {
		t.Helper()
		// The empty token argument is the assertion that matters: no session, no
		// API_TOKEN. A 401 here would restore the dead end.
		resp := do(t, srv, http.MethodGet, "/v1/setup/state", "", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /v1/setup/state (no auth) → %d, want 200", resp.StatusCode)
		}
		var body struct {
			Bootstrapped bool `json:"bootstrapped"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return body.Bootstrapped
	}

	if state() {
		t.Error("a fresh install reports bootstrapped=true — the wizard would be skipped")
	}

	resp := do(t, srv, http.MethodPost, "/v1/setup/bootstrap", "", `{"username":"owner","password":"s3cret-pw"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bootstrap → %d, want 200", resp.StatusCode)
	}

	if !state() {
		t.Error("still bootstrapped=false after bootstrap — every visitor would be sent back to the wizard")
	}
}

// Empty username/password → 422 (§11).
func TestBootstrap_Invalid(t *testing.T) {
	srv, _ := provServer(t)
	resp := do(t, srv, http.MethodPost, "/v1/setup/bootstrap", "", `{"username":"","password":""}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("empty bootstrap → %d, want 422", resp.StatusCode)
	}
}

// Import is admin-only: a non-admin (no token) → 403 (§11, §19).
func TestImport_RequiresAdmin(t *testing.T) {
	srv, _ := provServer(t)
	resp := do(t, srv, http.MethodPost, "/v1/users/import", "", `{"ids":["c9c1815f5b7e46308169209bf320e196"]}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("member import → %d, want 401", resp.StatusCode)
	}
}

// Admin import creates allowlist rows; a subsequent login for an imported user
// works and an un-imported one is rejected (§11 end-to-end via the API).
func TestImport_AdminCreatesAllowlist(t *testing.T) {
	const (
		mattID  = "c9c1815f5b7e46308169209bf320e196" // admin in the fixture
		chrisID = "b1df9e921c8f4ddb85f5b032f93ebdf4" // member
	)
	srv, st := provServer(t)

	resp := do(t, srv, http.MethodPost, "/v1/users/import", adminToken,
		`{"ids":["`+chrisID+`"],"makeAdmin":true}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin import → %d, want 200", resp.StatusCode)
	}
	var body struct{ Imported int }
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Imported != 1 {
		t.Errorf("imported = %d, want 1", body.Imported)
	}
	// Chris is now allowlisted; Matt (not imported) is not.
	if _, err := st.GetUser(context.Background(), chrisID); err != nil {
		t.Errorf("imported user should have a row: %v", err)
	}
	if _, err := st.GetUser(context.Background(), mattID); err != store.ErrNotFound {
		t.Errorf("un-imported user must have NO row: %v", err)
	}
}

// GET /v1/users/candidates is the read side of explicit import (§11): it lists the
// media server's accounts so an admin picks real names instead of raw ids, and marks
// the ones already allowlisted so the picker can show them as done.
func TestImportCandidates(t *testing.T) {
	const mattID = "c9c1815f5b7e46308169209bf320e196" // admin in the fixture
	srv, _ := provServer(t)

	// Admin-only (§19).
	if resp := do(t, srv, http.MethodGet, "/v1/users/candidates", "", ""); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("candidates without admin → %d, want 401", resp.StatusCode)
	}

	decode := func() []api.ImportCandidate {
		t.Helper()
		resp := do(t, srv, http.MethodGet, "/v1/users/candidates", adminToken, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("candidates → %d", resp.StatusCode)
		}
		var body struct {
			Candidates []api.ImportCandidate `json:"candidates"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return body.Candidates
	}

	before := decode()
	if len(before) == 0 {
		t.Fatal("expected the media server's accounts to be listed")
	}
	var matt api.ImportCandidate
	for _, c := range before {
		if c.ID == mattID {
			matt = c
		}
	}
	if matt.Name == "" {
		t.Fatalf("fixture admin missing from candidates: %+v", before)
	}
	if !matt.IsAdmin {
		t.Error("fixture admin should be flagged IsAdmin so import can map the role")
	}
	if matt.Imported {
		t.Error("nothing imported yet — Imported must be false")
	}

	// Importing flips the flag: the picker shows the account as done, not gone.
	do(t, srv, http.MethodPost, "/v1/users/import", adminToken, `{"ids":["`+mattID+`"]}`)
	for _, c := range decode() {
		if c.ID == mattID && !c.Imported {
			t.Error("after import, the candidate must be flagged Imported")
		}
	}
}
