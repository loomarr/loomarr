package auth

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

var now = time.Date(2026, 7, 13, 20, 0, 0, 0, time.UTC)

func newStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/a.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// Session tokens are stored SHA-256-hashed: the DB never holds the raw cookie.
func TestTokenHashedAtRest(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	_ = st.UpsertUser(ctx, store.User{ID: "u1", Name: "A", Role: store.RoleMember})
	mgr := NewManager(st, time.Hour, func() time.Time { return now })

	token, _, err := mgr.Issue(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	// The stored session must be keyed by the hash, not the token.
	if _, err := st.GetSession(ctx, token, now); err != store.ErrNotFound {
		t.Error("session is retrievable by the RAW token — token not hashed at rest (§11)")
	}
	if _, err := st.GetSession(ctx, hashToken(token), now); err != nil {
		t.Errorf("session not found by hash: %v", err)
	}
}

// Resolve returns the user for a valid token and slides expiry.
func TestResolveValid(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	_ = st.UpsertUser(ctx, store.User{ID: "u1", Name: "A", Role: store.RoleAdmin})
	mgr := NewManager(st, time.Hour, func() time.Time { return now })
	token, _, _ := mgr.Issue(ctx, "u1")

	u, err := mgr.Resolve(ctx, token)
	if err != nil || u.ID != "u1" || u.Role != store.RoleAdmin {
		t.Fatalf("Resolve = %+v, %v; want admin u1", u, err)
	}
}

// A disabled user's session must not authenticate (defense in depth).
func TestResolveDisabledUser(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	_ = st.UpsertUser(ctx, store.User{ID: "u1", Name: "A", Role: store.RoleMember})
	mgr := NewManager(st, time.Hour, func() time.Time { return now })
	token, _, _ := mgr.Issue(ctx, "u1")

	// Disable the user directly (simulating a race where the session outlived it).
	u, _ := st.GetUser(ctx, "u1")
	u.Disabled = true
	_ = st.UpsertUser(ctx, u)

	if _, err := mgr.Resolve(ctx, token); err == nil {
		t.Error("disabled user's session still resolved (§11 — sessions die on disable)")
	}
}

// Disabling via the login service revokes all the user's sessions immediately.
func TestDisableRevokesSessions(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	ms.Accounts = map[string]testkit.Account{"bob": {Password: "pw", ID: "u-bob"}}
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
	mgr := NewManager(st, time.Hour, func() time.Time { return now })
	svc := NewLoginService(lib, st, mgr, nil, func() time.Time { return now })

	// §11 rework: bob must be imported (allowlisted) before login works.
	importOne(t, st, "u-bob", "bob", false)

	token, _, _, err := svc.Login(ctx, "bob", "pw", "ip|bob")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Resolve(ctx, token); err != nil {
		t.Fatalf("fresh session should resolve: %v", err)
	}

	if err := svc.Disable(ctx, "u-bob"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Resolve(ctx, token); err == nil {
		t.Error("session survived Disable (§11 — must be revoked immediately)")
	}
}

// §11 rework: an IMPORTED media-server user can log in; an UN-IMPORTED one is
// rejected even with valid credentials (the allowlist — no lazy self-provision).
func TestImportedUserLogin_AllowlistEnforced(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	ms.Accounts = map[string]testkit.Account{
		"boss": {Password: "pw", ID: "u-boss", IsAdmin: true},
		"kid":  {Password: "pw", ID: "u-kid", IsAdmin: false},
	}
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
	mgr := NewManager(st, time.Hour, func() time.Time { return now })
	svc := NewLoginService(lib, st, mgr, nil, func() time.Time { return now })

	// boss is imported (as admin); kid is NOT imported.
	importOne(t, st, "u-boss", "boss", true)

	if _, _, u, err := svc.Login(ctx, "boss", "pw", "ip|boss"); err != nil || u.Role != store.RoleAdmin {
		t.Errorf("imported admin → %v (%v), want admin login", u.Role, err)
	}
	// kid has VALID media-server creds but is not on the allowlist → denied.
	if _, _, _, err := svc.Login(ctx, "kid", "pw", "ip|kid"); err != ErrInvalidCredentials {
		t.Errorf("un-imported user login → %v, want ErrInvalidCredentials", err)
	}
	// And no row was created for kid (no lazy self-provision).
	if _, err := st.GetUser(ctx, "u-kid"); err != store.ErrNotFound {
		t.Errorf("un-imported login must not create a row: %v", err)
	}
}

// Import assigns admin to media-server admins only when makeAdmin is set (§11).
// Uses the pinned users_list fixture ids: Fixture Admin (admin) + Fixture Member (member).
func TestImport_RoleAssignment(t *testing.T) {
	const (
		adminID  = "00000000000000000000000000000007" // IsAdministrator: true
		memberID = "00000000000000000000000000000002" // member
	)
	st := newStore(t)
	ctx := context.Background()
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
	prov := NewProvisioner(st, lib, seqID(), func() time.Time { return now })

	n, err := prov.Import(ctx, []string{adminID, memberID}, true)
	if err != nil || n != 2 {
		t.Fatalf("import → %d,%v want 2,nil", n, err)
	}
	if u, _ := st.GetUser(ctx, adminID); u.Role != store.RoleAdmin {
		t.Errorf("media-server admin imported with makeAdmin → %v, want admin", u.Role)
	}
	if u, _ := st.GetUser(ctx, memberID); u.Role != store.RoleMember {
		t.Errorf("non-admin imported → %v, want member", u.Role)
	}
	// An id the server doesn't list is skipped, never invented.
	n2, _ := prov.Import(ctx, []string{"ghost-id"}, true)
	if n2 != 0 {
		t.Errorf("importing an unknown id → %d, want 0 (never invent)", n2)
	}
}

// Bootstrap creates the first local admin, once (§11).
func TestBootstrap_OnceOnly(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	prov := NewProvisioner(st, nil, seqID(), func() time.Time { return now })

	u, err := prov.Bootstrap(ctx, "owner", "s3cret-pw")
	if err != nil || u.Role != store.RoleAdmin || u.PasswordHash == "" {
		t.Fatalf("bootstrap → %+v, %v (want admin with a hash)", u, err)
	}
	// A local admin can now log in against the bcrypt hash (no media server).
	mgr := NewManager(st, time.Hour, func() time.Time { return now })
	svc := NewLoginService(nil, st, mgr, nil, func() time.Time { return now })
	if _, _, lu, lerr := svc.Login(ctx, "owner", "s3cret-pw", "ip|owner"); lerr != nil || lu.ID != u.ID {
		t.Errorf("local admin login → %v (%v), want success", lu.ID, lerr)
	}
	// A wrong password is rejected.
	if _, _, _, lerr := svc.Login(ctx, "owner", "wrong", "ip|owner2"); lerr != ErrInvalidCredentials {
		t.Errorf("wrong local password → %v, want ErrInvalidCredentials", lerr)
	}
	// A second bootstrap is closed.
	if _, err := prov.Bootstrap(ctx, "other", "pw2"); err != ErrBootstrapClosed {
		t.Errorf("second bootstrap → %v, want ErrBootstrapClosed", err)
	}
}

// importOne allowlists a single media-server user id for the login tests. It
// writes the row directly (the store IS the allowlist) — equivalent to what
// Provisioner.Import does, without needing the media-server list fixture.
func importOne(t *testing.T, st store.Store, id, name string, admin bool) {
	t.Helper()
	role := store.RoleMember
	if admin {
		role = store.RoleAdmin
	}
	u := store.User{ID: id, Name: name, Role: role, CreatedAt: now, UpdatedAt: now}
	if err := st.UpsertUser(context.Background(), u); err != nil {
		t.Fatal(err)
	}
}

// seqID returns a deterministic id generator for tests (local-user ids).
func seqID() IDGen {
	n := 0
	return func() string { n++; return "local-" + string(rune('a'+n-1)) }
}

// Bad credentials → ErrInvalidCredentials (§11 negative path).
func TestLoginBadPassword(t *testing.T) {
	st := newStore(t)
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	ms.Accounts = map[string]testkit.Account{"bob": {Password: "right", ID: "u-bob"}}
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
	mgr := NewManager(st, time.Hour, func() time.Time { return now })
	svc := NewLoginService(lib, st, mgr, nil, func() time.Time { return now })

	if _, _, _, err := svc.Login(context.Background(), "bob", "wrong", "ip|bob"); err != ErrInvalidCredentials {
		t.Errorf("bad password → %v, want ErrInvalidCredentials", err)
	}
}

// The rate limiter blocks sustained attempts for the same key.
func TestRateLimit(t *testing.T) {
	rl := NewRateLimiter(0, 3) // 3 burst, no refill
	key := "1.2.3.4|bob"
	for i := 0; i < 3; i++ {
		if !rl.Allow(key) {
			t.Fatalf("attempt %d should be allowed within burst", i)
		}
	}
	if rl.Allow(key) {
		t.Error("4th attempt should be rate-limited")
	}
	// A different key is independent.
	if !rl.Allow("5.6.7.8|bob") {
		t.Error("different IP should not be limited")
	}
}

// Login returns ErrRateLimited when the limiter blocks.
func TestLoginRateLimited(t *testing.T) {
	st := newStore(t)
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	ms.Accounts = map[string]testkit.Account{"bob": {Password: "pw", ID: "u-bob"}}
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
	mgr := NewManager(st, time.Hour, func() time.Time { return now })
	svc := NewLoginService(lib, st, mgr, NewRateLimiter(0, 0), func() time.Time { return now }) // 0 burst

	if _, _, _, err := svc.Login(context.Background(), "bob", "pw", "ip|bob"); err != ErrRateLimited {
		t.Errorf("login with exhausted limiter → %v, want ErrRateLimited", err)
	}
}
