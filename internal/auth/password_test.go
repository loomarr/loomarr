package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/mantonx/loomarr/internal/store"
)

// Local account management (§11). These cover the two operations the identity model
// implied but never had — a local user changing their own password, and an admin
// minting a second local account — plus the negatives that keep them from becoming
// a way around the allowlist.

func newPasswordService(t *testing.T) (*PasswordService, store.Store) {
	t.Helper()
	st := newStore(t)
	n := 0
	return NewPasswordService(st, func() string { n++; return "gen-" + string(rune('a'+n-1)) }, func() time.Time { return now }), st
}

// seedLocal writes a local user with a known password.
func seedLocal(t *testing.T, st store.Store, id, name, password string, role store.Role) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertUser(context.Background(), store.User{
		ID: id, Name: name, Role: role, PasswordHash: string(hash), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
}

// The bootstrap admin can change the password set in wizard step 1. Before this,
// that password was permanent for the life of the install (defect S2).
func TestChangePassword_UpdatesHashAndVerifies(t *testing.T) {
	svc, st := newPasswordService(t)
	ctx := context.Background()
	seedLocal(t, st, "u1", "owner", "original-pw", store.RoleAdmin)

	if err := svc.ChangePassword(ctx, "u1", "original-pw", "a-longer-new-pw"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	u, err := st.GetUser(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("a-longer-new-pw")) != nil {
		t.Error("new password does not verify against the stored hash")
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("original-pw")) == nil {
		t.Error("OLD password still verifies — the hash was not replaced")
	}
}

// Knowing the current password is required even though the caller already holds a
// session: a session proves "this browser was logged in", which is also what an
// unattended laptop looks like.
func TestChangePassword_RequiresCurrentPassword(t *testing.T) {
	svc, st := newPasswordService(t)
	ctx := context.Background()
	seedLocal(t, st, "u1", "owner", "original-pw", store.RoleAdmin)

	err := svc.ChangePassword(ctx, "u1", "not-the-password", "a-longer-new-pw")
	if !errors.Is(err, ErrWrongPassword) {
		t.Fatalf("err = %v, want ErrWrongPassword", err)
	}
	// And the stored hash must be untouched by the failed attempt.
	u, _ := st.GetUser(ctx, "u1")
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("original-pw")) != nil {
		t.Error("a FAILED change still altered the stored hash")
	}
}

// §19 negative: an imported media-server user has no Loomarr-side password. Changing
// it must error rather than silently no-op — a no-op would imply to the caller that
// their media-server password had been changed, which Loomarr cannot do.
func TestChangePassword_RejectsImportedUser(t *testing.T) {
	svc, st := newPasswordService(t)
	ctx := context.Background()
	// No PasswordHash ⇒ imported (§11's credential-path discriminator).
	if err := st.UpsertUser(ctx, store.User{ID: "u2", Name: "emby-user", Role: store.RoleMember}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangePassword(ctx, "u2", "anything", "a-longer-new-pw"); !errors.Is(err, ErrNotLocalUser) {
		t.Fatalf("err = %v, want ErrNotLocalUser", err)
	}
}

func TestChangePassword_RejectsShortPassword(t *testing.T) {
	svc, st := newPasswordService(t)
	seedLocal(t, st, "u1", "owner", "original-pw", store.RoleAdmin)
	if err := svc.ChangePassword(context.Background(), "u1", "original-pw", "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("err = %v, want ErrWeakPassword", err)
	}
}

// A password change ends EVERY session, including the caller's. A change made
// because of a suspected compromise has to actually evict the intruder — keeping
// "the current session" would mean trusting the request that might be theirs.
func TestChangePassword_RevokesAllSessions(t *testing.T) {
	svc, st := newPasswordService(t)
	ctx := context.Background()
	seedLocal(t, st, "u1", "owner", "original-pw", store.RoleAdmin)
	mgr := NewManager(st, time.Hour, func() time.Time { return now })

	tokenA, _, err := mgr.Issue(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	tokenB, _, err := mgr.Issue(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangePassword(ctx, "u1", "original-pw", "a-longer-new-pw"); err != nil {
		t.Fatal(err)
	}
	for name, tok := range map[string]string{"first": tokenA, "second": tokenB} {
		if _, err := st.GetSession(ctx, hashToken(tok), now); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("%s session survived a password change (err = %v)", name, err)
		}
	}
}

// An admin can mint a second local account — the install is no longer stuck with
// exactly one, which is what bootstrap-once left it as.
func TestCreateLocal_AddsAnAllowlistRowWithAHash(t *testing.T) {
	svc, st := newPasswordService(t)
	ctx := context.Background()

	u, err := svc.CreateLocal(ctx, "second-admin", "a-good-password", store.RoleMember, 5)
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	got, err := st.GetUser(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PasswordHash == "" {
		t.Error("created user has no password hash — it would read as an IMPORTED account (§11)")
	}
	if bcrypt.CompareHashAndPassword([]byte(got.PasswordHash), []byte("a-good-password")) != nil {
		t.Error("stored hash does not verify against the given password")
	}
	if got.Role != store.RoleMember || got.Quota != 5 {
		t.Errorf("role/quota not persisted: %+v", got)
	}
}

// Two rows with the same name would look identical in the users list and resolve
// ambiguously at login, which matches by name.
func TestCreateLocal_RejectsDuplicateUsername(t *testing.T) {
	svc, st := newPasswordService(t)
	ctx := context.Background()
	seedLocal(t, st, "u1", "owner", "original-pw", store.RoleAdmin)

	if _, err := svc.CreateLocal(ctx, "owner", "a-good-password", store.RoleMember, 0); !errors.Is(err, ErrDuplicateUsername) {
		t.Fatalf("err = %v, want ErrDuplicateUsername", err)
	}
	// Case differences are still the same name to a human and to login.
	if _, err := svc.CreateLocal(ctx, "OWNER", "a-good-password", store.RoleMember, 0); !errors.Is(err, ErrDuplicateUsername) {
		t.Fatalf("case-different duplicate: err = %v, want ErrDuplicateUsername", err)
	}
}

func TestCreateLocal_RejectsShortPassword(t *testing.T) {
	svc, _ := newPasswordService(t)
	if _, err := svc.CreateLocal(context.Background(), "newbie", "short", store.RoleMember, 0); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("err = %v, want ErrWeakPassword", err)
	}
}

// The admin reset path: no current password required (that's the point), but it is
// still local-accounts-only and still evicts the target's sessions.
func TestResetPassword_SetsHashAndRevokesTargetSessions(t *testing.T) {
	svc, st := newPasswordService(t)
	ctx := context.Background()
	seedLocal(t, st, "u1", "forgetful", "lost-to-time", store.RoleMember)
	mgr := NewManager(st, time.Hour, func() time.Time { return now })
	tok, _, err := mgr.Issue(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.ResetPassword(ctx, "u1", "an-admin-set-pw"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	u, _ := st.GetUser(ctx, "u1")
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte("an-admin-set-pw")) != nil {
		t.Error("reset password does not verify")
	}
	if _, err := st.GetSession(ctx, hashToken(tok), now); !errors.Is(err, store.ErrNotFound) {
		t.Error("target's session survived an admin password reset")
	}
}

// §19 negative: an admin cannot reset an imported user's password either — Loomarr
// never held that credential, so there is nothing to reset.
func TestResetPassword_RejectsImportedUser(t *testing.T) {
	svc, st := newPasswordService(t)
	ctx := context.Background()
	if err := st.UpsertUser(ctx, store.User{ID: "u2", Name: "emby-user", Role: store.RoleMember}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResetPassword(ctx, "u2", "an-admin-set-pw"); !errors.Is(err, ErrNotLocalUser) {
		t.Fatalf("err = %v, want ErrNotLocalUser", err)
	}
}

// A password change must not turn a member into an admin, or otherwise disturb the
// fields the identity model owns.
func TestChangePassword_LeavesRoleAndQuotaAlone(t *testing.T) {
	svc, st := newPasswordService(t)
	ctx := context.Background()
	hash, _ := bcrypt.GenerateFromPassword([]byte("original-pw"), bcrypt.MinCost)
	if err := st.UpsertUser(ctx, store.User{
		ID: "u1", Name: "member", Role: store.RoleMember, Quota: 3, AutoApprove: true,
		PasswordHash: string(hash), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangePassword(ctx, "u1", "original-pw", "a-longer-new-pw"); err != nil {
		t.Fatal(err)
	}
	u, _ := st.GetUser(ctx, "u1")
	if u.Role != store.RoleMember || u.Quota != 3 || !u.AutoApprove {
		t.Errorf("password change disturbed identity fields: %+v", u)
	}
}
