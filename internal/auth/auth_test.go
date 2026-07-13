package auth

import (
	"context"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
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

// First-admin bootstrap: a media-server admin becomes a Loomarr admin; a
// non-admin becomes a member.
func TestBootstrapRoles(t *testing.T) {
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

	if _, _, u, err := svc.Login(ctx, "boss", "pw", "ip|boss"); err != nil || u.Role != store.RoleAdmin {
		t.Errorf("media-server admin → %v (%v), want admin", u.Role, err)
	}
	if _, _, u, err := svc.Login(ctx, "kid", "pw", "ip|kid"); err != nil || u.Role != store.RoleMember {
		t.Errorf("non-admin → %v (%v), want member", u.Role, err)
	}
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
