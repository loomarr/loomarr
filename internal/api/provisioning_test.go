package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

type provisioningHarnessConfig struct {
	Users []testkit.MediaServerUser
}

// provisioningHarness hides the fixed bootstrap, import, login, and user-sync
// assembly behind the common API lifecycle. Tests vary only the media-server
// users visible to provisioning.
type provisioningHarness struct {
	*apiHarness
}

func newProvisioningHarness(t *testing.T, configs ...provisioningHarnessConfig) *provisioningHarness {
	t.Helper()
	if len(configs) > 1 {
		t.Fatalf("newProvisioningHarness accepts at most one configuration, got %d", len(configs))
	}
	config := provisioningHarnessConfig{}
	if len(configs) == 1 {
		config = configs[0]
	}

	ms := testkit.NewMediaServer(t)
	ms.Users = config.Users
	t.Cleanup(ms.Close)
	base := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
		mgr := auth.NewManager(defaults.Store, time.Hour, time.Now)
		n := 0
		newID := func() string { n++; return "local-" + string(rune('a'+n-1)) }

		return api.Router(defaults.Log, api.Options{
			Store:        defaults.Store,
			Auth:         api.NewSessionAuthorizer(mgr, adminToken),
			Log:          defaults.Log,
			Login:        auth.NewLoginService(lib, defaults.Store, mgr, nil, time.Now),
			Sessions:     mgr,
			UserSync:     auth.NewUserSync(lib, defaults.Store, time.Now),
			Provision:    auth.NewProvisioner(defaults.Store, lib, newID, time.Now),
			CookieSecure: "false",
		})
	})
	return &provisioningHarness{apiHarness: base}
}

// Bootstrap is UNAUTHENTICATED and succeeds once; a second call 409s (§11).
func TestBootstrap_OnceViaAPI(t *testing.T) {
	srv := newProvisioningHarness(t).Server

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
	srv := newProvisioningHarness(t).Server

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
	srv := newProvisioningHarness(t).Server
	resp := do(t, srv, http.MethodPost, "/v1/setup/bootstrap", "", `{"username":"","password":""}`)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("empty bootstrap → %d, want 422", resp.StatusCode)
	}
}

// Import is admin-only: a non-admin (no token) → 403 (§11, §19).
func TestImport_RequiresAdmin(t *testing.T) {
	srv := newProvisioningHarness(t).Server
	resp := do(t, srv, http.MethodPost, "/v1/users/import", "", `{"ids":["00000000000000000000000000000007"]}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("member import → %d, want 401", resp.StatusCode)
	}
}

// Admin import creates allowlist rows; a subsequent login for an imported user
// works and an un-imported one is rejected (§11 end-to-end via the API).
func TestImport_AdminCreatesAllowlist(t *testing.T) {
	const (
		adminID  = "00000000000000000000000000000007" // admin in the fixture
		memberID = "00000000000000000000000000000002" // member
	)
	harness := newProvisioningHarness(t)
	srv, st := harness.Server, harness.Store

	resp := do(t, srv, http.MethodPost, "/v1/users/import", adminToken,
		`{"ids":["`+memberID+`"]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin import → %d, want 200", resp.StatusCode)
	}
	var body struct{ Imported int }
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Imported != 1 {
		t.Errorf("imported = %d, want 1", body.Imported)
	}
	// Fixture Member is now allowlisted; Fixture Admin (not imported) is not.
	if _, err := st.GetUser(context.Background(), memberID); err != nil {
		t.Errorf("imported user should have a row: %v", err)
	}
	if _, err := st.GetUser(context.Background(), adminID); err != store.ErrNotFound {
		t.Errorf("un-imported user must have NO row: %v", err)
	}
}

// A bulk import copies the source role on first import without asking each caller
// to remember a make-admin switch. The wizard posts exactly this ids-only shape;
// if the role decision lives in the UI, the same Emby admin becomes a member or
// admin depending on which import screen the operator happened to use.
func TestImport_BulkMapsSourceRoles(t *testing.T) {
	const (
		mattID     = "media-admin"
		chrisID    = "media-member"
		disabledID = "media-disabled"
	)
	harness := newProvisioningHarness(t, provisioningHarnessConfig{Users: []testkit.MediaServerUser{
		{ID: mattID, Name: "Matt", IsAdmin: true},
		{ID: chrisID, Name: "Chris"},
		{ID: disabledID, Name: "Disabled", Disabled: true},
	}})
	srv, st := harness.Server, harness.Store

	resp := do(t, srv, http.MethodPost, "/v1/users/import", adminToken,
		`{"ids":["`+mattID+`","`+chrisID+`","`+disabledID+`","`+chrisID+`","unknown"]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("bulk import → %d, want 200", resp.StatusCode)
	}
	var imported struct{ Imported int }
	if err := json.NewDecoder(resp.Body).Decode(&imported); err != nil {
		t.Fatalf("decode import: %v", err)
	}
	if imported.Imported != 3 {
		t.Fatalf("imported = %d, want 3 (duplicates deduplicated; unknown ids skipped)", imported.Imported)
	}

	// Loomarr-owned choices made after import survive a repeat import. The media
	// server remains authoritative only for identity and disabled state.
	member, err := st.GetUser(context.Background(), mattID)
	if err != nil {
		t.Fatalf("get imported admin: %v", err)
	}
	if member.Role != store.RoleAdmin {
		t.Fatalf("media-server admin initial role = %q, want admin", member.Role)
	}
	member.Role = store.RoleMember
	member.Quota = 7
	member.AutoApprove = true
	if err := st.UpsertUser(context.Background(), member); err != nil {
		t.Fatalf("customize imported admin: %v", err)
	}
	resp = do(t, srv, http.MethodPost, "/v1/users/import", adminToken,
		`{"ids":["`+mattID+`"]}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("repeat import → %d, want 200", resp.StatusCode)
	}
	preserved, err := st.GetUser(context.Background(), mattID)
	if err != nil {
		t.Fatalf("get re-imported admin: %v", err)
	}
	if preserved.Role != store.RoleMember || preserved.Quota != 7 || !preserved.AutoApprove {
		t.Errorf("re-import overwrote Loomarr choices: role=%q quota=%d autoApprove=%v",
			preserved.Role, preserved.Quota, preserved.AutoApprove)
	}

	resp = do(t, srv, http.MethodGet, "/v1/users", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list users → %d, want 200", resp.StatusCode)
	}
	var listed struct {
		Users []struct {
			ID       string `json:"id"`
			Role     string `json:"role"`
			Disabled bool   `json:"disabled"`
		} `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	roles := make(map[string]string, len(listed.Users))
	disabled := make(map[string]bool, len(listed.Users))
	for _, u := range listed.Users {
		roles[u.ID] = u.Role
		disabled[u.ID] = u.Disabled
	}
	if roles[mattID] != "member" {
		t.Errorf("re-imported customized admin role = %q, want preserved member", roles[mattID])
	}
	if roles[chrisID] != "member" {
		t.Errorf("media-server member role = %q, want member", roles[chrisID])
	}
	if !disabled[disabledID] {
		t.Error("disabled media-server account was not imported disabled")
	}
	if _, exists := roles["unknown"]; exists {
		t.Error("unknown media-server id was invented locally")
	}
}

func TestImport_RejectsAccountReservedByInvitation(t *testing.T) {
	const accountID = "media-invited"
	harness := newProvisioningHarness(t, provisioningHarnessConfig{Users: []testkit.MediaServerUser{{
		ID: accountID, Name: "Invited",
	}}})
	srv, st := harness.Server, harness.Store
	now := time.Unix(1_900_000_000, 0).UTC()
	if err := st.CreateInvitation(context.Background(), invitation.Invitation{
		ID: "pending-import", Kind: invitation.KindLibrary, LibraryUserID: accountID,
		DisplayName: "Invited", IdentityKey: accountID, Role: invitation.RoleMember,
		Status: invitation.StatusPending, CreatedAt: now, ExpiresAt: now.Add(invitation.Expiry),
	}, nil); err != nil {
		t.Fatal(err)
	}
	resp := do(t, srv, http.MethodPost, "/v1/users/import", adminToken,
		`{"ids":["`+accountID+`"]}`)
	if resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("import reserved account = %d %s, want 409", resp.StatusCode, body)
	}
	if _, err := st.GetUser(context.Background(), accountID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("reserved import created user: %v", err)
	}
}

// GET /v1/users/candidates is the read side of explicit import (§11): it lists the
// media server's accounts so an admin picks real names instead of raw ids, and marks
// the ones already allowlisted so the picker can show them as done.
func TestImportCandidates(t *testing.T) {
	const adminID = "00000000000000000000000000000007" // admin in the fixture
	srv := newProvisioningHarness(t).Server

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
	var admin api.ImportCandidate
	for _, c := range before {
		if c.ID == adminID {
			admin = c
		}
	}
	if admin.Name == "" {
		t.Fatalf("fixture admin missing from candidates: %+v", before)
	}
	if !admin.IsAdmin {
		t.Error("fixture admin should be flagged IsAdmin so import can map the role")
	}
	if admin.Imported {
		t.Error("nothing imported yet — Imported must be false")
	}

	// Importing flips the flag: the picker shows the account as done, not gone.
	do(t, srv, http.MethodPost, "/v1/users/import", adminToken, `{"ids":["`+adminID+`"]}`)
	for _, c := range decode() {
		if c.ID == adminID && !c.Imported {
			t.Error("after import, the candidate must be flagged Imported")
		}
	}
}
