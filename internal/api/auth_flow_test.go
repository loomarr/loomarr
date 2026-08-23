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

	"golang.org/x/crypto/bcrypt"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

// authServer builds the full session-auth stack over a mock media server with an
// admin and a member account.
func authServer(t *testing.T) (*httptest.Server, store.Store, *testkit.MediaServer) {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/auth.db")
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
		Passwords:    auth.NewPasswordService(st, func() string { return "u-new" }, time.Now),
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

// --- local account management (§11, V7) -----------------------------------------
//
// The split between these routes IS the authorization model, so the negatives are
// the gate: a member must not reach the admin paths, and the self path must not be
// aimable at anyone else (it takes no target id, by construction).

// seedLocalUser writes a user WITH a bcrypt hash — the credential-path discriminator
// (§11). The harness otherwise seeds imported users, so a local one is explicit.
func seedLocalUser(t *testing.T, st store.Store, id, name, password string, role store.Role) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	u := store.User{
		ID: id, Name: name, Role: role, PasswordHash: string(hash),
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := st.UpsertUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}

// §19: a member gets 403 on both admin password routes. Creating an account and
// resetting someone else's credential are admin actions.
func TestMemberForbiddenOnPasswordAdminRoutes(t *testing.T) {
	srv, _, _ := authServer(t)
	member := login(t, srv, "kid", "pw")

	cases := []struct{ method, path, body string }{
		{http.MethodPost, "/v1/users", `{"username":"sneaky","password":"a-good-password"}`},
		{http.MethodPost, "/v1/users/u-boss/password", `{"next":"a-good-password"}`},
	}
	for _, tc := range cases {
		resp := authed(t, tc.method, srv.URL+tc.path, member, tc.body)
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("member %s %s → %d, want 403 (§19)", tc.method, tc.path, resp.StatusCode)
		}
	}
}

// A member CAN change their own password — the self route is not admin-gated, and
// must not be. It simply has no way to name a different target.
func TestChangeOwnPassword_MemberAllowed(t *testing.T) {
	srv, st, _ := authServer(t)
	seedLocalUser(t, st, "u-local", "localkid", "original-pw", store.RoleMember)
	sess := login(t, srv, "localkid", "original-pw")

	resp := authed(t, http.MethodPost, srv.URL+"/v1/auth/password", sess,
		`{"current":"original-pw","next":"a-longer-new-pw"}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("self change → %d, want 204", resp.StatusCode)
	}
	// The new password works…
	if c := login(t, srv, "localkid", "a-longer-new-pw"); c == nil {
		t.Error("cannot sign in with the new password")
	}
	// …and the change revoked the session that made it (every session dies, so a
	// compromise-driven change actually evicts the intruder).
	if resp := authed(t, http.MethodGet, srv.URL+"/v1/auth/me", sess, ""); resp.StatusCode == http.StatusOK {
		t.Error("the session that changed the password survived — an attacker's would too")
	}
}

// The wrong current password is refused, and the stored credential is untouched.
func TestChangeOwnPassword_WrongCurrentRejected(t *testing.T) {
	srv, st, _ := authServer(t)
	seedLocalUser(t, st, "u-local", "localkid", "original-pw", store.RoleMember)
	sess := login(t, srv, "localkid", "original-pw")

	resp := authed(t, http.MethodPost, srv.URL+"/v1/auth/password", sess,
		`{"current":"not-it","next":"a-longer-new-pw"}`)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong current → %d, want 401", resp.StatusCode)
	}
	if c := login(t, srv, "localkid", "original-pw"); c == nil {
		t.Error("original password stopped working after a REJECTED change")
	}
}

// §19: an imported media-server user has no Loomarr-side password. The route says so
// rather than pretending to succeed — a silent 204 would imply Loomarr had changed
// their media-server password, which it cannot do.
func TestChangeOwnPassword_ImportedUserConflicts(t *testing.T) {
	srv, _, _ := authServer(t)
	member := login(t, srv, "kid", "pw") // seeded as IMPORTED — no hash

	resp := authed(t, http.MethodPost, srv.URL+"/v1/auth/password", member,
		`{"current":"pw","next":"a-longer-new-pw"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("imported self-change → %d, want 409", resp.StatusCode)
	}
}

// An admin mints a local account, and it can sign in immediately — the install is no
// longer stuck with the single bootstrap admin.
func TestCreateLocalUser_AdminCanMintAnAccount(t *testing.T) {
	srv, _, _ := authServer(t)
	admin := login(t, srv, "boss", "pw")

	resp := authed(t, http.MethodPost, srv.URL+"/v1/users", admin,
		`{"username":"newcomer","password":"a-good-password","role":"member","quota":3}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create → %d, want 200", resp.StatusCode)
	}
	if c := login(t, srv, "newcomer", "a-good-password"); c == nil {
		t.Error("the new local account cannot sign in")
	}
}

// Role defaults to member: minting an admin must be deliberate, never what happens
// when a field is omitted (§11 — roles gate the actions that spend real resources).
func TestCreateLocalUser_DefaultsToMember(t *testing.T) {
	srv, st, _ := authServer(t)
	admin := login(t, srv, "boss", "pw")

	resp := authed(t, http.MethodPost, srv.URL+"/v1/users", admin,
		`{"username":"newcomer","password":"a-good-password"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create → %d", resp.StatusCode)
	}
	u, err := st.GetUserByName(context.Background(), "newcomer")
	if err != nil {
		t.Fatal(err)
	}
	if u.Role != store.RoleMember {
		t.Errorf("role = %q, want member when the field is omitted", u.Role)
	}
}

// An admin resets a local user's password without knowing the current one — the
// "someone forgot theirs" path — and the target's sessions die.
func TestResetUserPassword_AdminPath(t *testing.T) {
	srv, st, _ := authServer(t)
	seedLocalUser(t, st, "u-local", "localkid", "original-pw", store.RoleMember)
	admin := login(t, srv, "boss", "pw")
	victim := login(t, srv, "localkid", "original-pw")

	resp := authed(t, http.MethodPost, srv.URL+"/v1/users/u-local/password", admin,
		`{"next":"an-admin-set-pw"}`)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reset → %d, want 204", resp.StatusCode)
	}
	if c := login(t, srv, "localkid", "an-admin-set-pw"); c == nil {
		t.Error("cannot sign in with the admin-set password")
	}
	if resp := authed(t, http.MethodGet, srv.URL+"/v1/auth/me", victim, ""); resp.StatusCode == http.StatusOK {
		t.Error("target's session survived an admin password reset")
	}
}

// §19: resetting an imported user's password conflicts — Loomarr never held it.
func TestResetUserPassword_ImportedUserConflicts(t *testing.T) {
	srv, _, _ := authServer(t)
	admin := login(t, srv, "boss", "pw")

	resp := authed(t, http.MethodPost, srv.URL+"/v1/users/u-kid/password", admin,
		`{"next":"an-admin-set-pw"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("reset imported → %d, want 409", resp.StatusCode)
	}
}

// --- V26 "My requests": per-user proposal scoping -------------------------------

// seedProposalFor writes a proposal owned by a specific user, with the approval
// provenance V25/V25b persist so the read path can be asserted end to end.
func seedProposalFor(t *testing.T, st store.Store, id, createdBy, status string, p store.Proposal) {
	t.Helper()
	p.ID, p.CreatedBy, p.Status = id, createdBy, status
	p.JobID = "job-" + id
	if p.ProposalJSON == "" {
		p.ProposalJSON = `{"acquisitions":[{"mediaType":"movie","tmdbId":100,"name":"Speed","year":1994}]}`
	}
	if err := st.CreateProposal(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func proposalIDs(t *testing.T, resp *http.Response) []string {
	t.Helper()
	var body struct {
		Proposals []struct {
			ID         string `json:"id"`
			ModSummary string `json:"modSummary"`
			Note       string `json:"note"`
			DenyReason string `json:"denyReason"`
		} `json:"proposals"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(body.Proposals))
	for _, p := range body.Proposals {
		ids = append(ids, p.ID)
	}
	return ids
}

// THE scoping test. `mine=true` resolves the owner from the SESSION, never from a
// parameter — the design sketch read `ListProposals(status[, user])`, and a
// client-supplied id would let any member read another's requests by editing a URL.
func TestListProposals_MineScopesToTheCaller(t *testing.T) {
	srv, st, _ := authServer(t)
	seedProposalFor(t, st, "p-kid", "u-kid", "submitted", store.Proposal{})
	seedProposalFor(t, st, "p-boss", "u-boss", "submitted", store.Proposal{})

	kid := login(t, srv, "kid", "pw")
	resp := authed(t, http.MethodGet, srv.URL+"/v1/proposals?status=submitted&mine=true", kid, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("mine=true → %d, want 200", resp.StatusCode)
	}
	got := proposalIDs(t, resp)
	if len(got) != 1 || got[0] != "p-kid" {
		t.Errorf("mine=true returned %v, want only [p-kid] — a member must not see another's requests", got)
	}
}

// Without `mine`, the list is the unscoped queue it has always been. Read visibility
// is global for authenticated users (§342), so this is not a leak — but it must stay
// a DELIBERATE choice rather than something `mine` accidentally changed.
func TestListProposals_WithoutMineIsUnscoped(t *testing.T) {
	srv, st, _ := authServer(t)
	seedProposalFor(t, st, "p-kid", "u-kid", "submitted", store.Proposal{})
	seedProposalFor(t, st, "p-boss", "u-boss", "submitted", store.Proposal{})

	kid := login(t, srv, "kid", "pw")
	resp := authed(t, http.MethodGet, srv.URL+"/v1/proposals?status=submitted", kid, "")
	defer func() { _ = resp.Body.Close() }()
	if got := proposalIDs(t, resp); len(got) != 2 {
		t.Errorf("unscoped list returned %v, want both proposals", got)
	}
}

// `mine` spans statuses: "what have I asked for?" includes the denied ones. The status
// filter still applies, so each tab asks for one status at a time.
func TestListProposals_MineHonoursTheStatusFilter(t *testing.T) {
	srv, st, _ := authServer(t)
	seedProposalFor(t, st, "p-open", "u-kid", "submitted", store.Proposal{})
	seedProposalFor(t, st, "p-denied", "u-kid", "denied", store.Proposal{DenyReason: "over the cap"})

	kid := login(t, srv, "kid", "pw")
	resp := authed(t, http.MethodGet, srv.URL+"/v1/proposals?status=denied&mine=true", kid, "")
	defer func() { _ = resp.Body.Close() }()
	if got := proposalIDs(t, resp); len(got) != 1 || got[0] != "p-denied" {
		t.Errorf("status=denied&mine=true returned %v, want only [p-denied]", got)
	}
}

// A break-glass API_TOKEN caller has no user record, so its id is "". The property that
// matters is that this cannot WIDEN: it must never fall through to every member's
// requests. (Submit stamps the same "" for a token-submitted proposal, so "mine" for a
// token means "what this token submitted" — usually nothing.)
func TestListProposals_MineOnTokenDoesNotSeeEveryone(t *testing.T) {
	srv, st, _ := authServer(t)
	seedProposalFor(t, st, "p-kid", "u-kid", "submitted", store.Proposal{})
	seedProposalFor(t, st, "p-boss", "u-boss", "submitted", store.Proposal{})

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/proposals?status=submitted&mine=true", nil)
	req.Header.Set("Authorization", "Bearer break-glass-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if got := proposalIDs(t, resp); len(got) != 0 {
		t.Errorf("mine=true on a token returned %v, want none — a token must not read members' requests", got)
	}
}

// The approval provenance (§7, D-K) has been PERSISTED since V25 and left the server
// nowhere: the note an approver wrote to explain an edited request reached the database
// and nothing could display it. This asserts the read path carries all three.
func TestListProposals_CarriesApprovalProvenance(t *testing.T) {
	srv, st, _ := authServer(t)
	seedProposalFor(t, st, "p-edited", "u-kid", "approved", store.Proposal{
		ApprovedBy: "u-boss",
		ModSummary: "dropped 2, added 1",
		Note:       "swapped Con Air for Face/Off — we already have that one",
	})

	kid := login(t, srv, "kid", "pw")
	resp := authed(t, http.MethodGet, srv.URL+"/v1/proposals?status=approved&mine=true", kid, "")
	defer func() { _ = resp.Body.Close() }()

	var body struct {
		Proposals []struct {
			ApprovedBy string `json:"approvedBy"`
			ModSummary string `json:"modSummary"`
			Note       string `json:"note"`
		} `json:"proposals"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Proposals) != 1 {
		t.Fatalf("got %d proposals, want 1", len(body.Proposals))
	}
	p := body.Proposals[0]
	if p.ApprovedBy != "u-boss" {
		t.Errorf("approvedBy = %q, want u-boss", p.ApprovedBy)
	}
	if p.ModSummary != "dropped 2, added 1" {
		t.Errorf("modSummary = %q — the server-generated record of what changed must reach the requester", p.ModSummary)
	}
	if p.Note == "" {
		t.Error("note is empty — V25b captured it and V26 must surface it, or an altered request stays unexplained")
	}
}
