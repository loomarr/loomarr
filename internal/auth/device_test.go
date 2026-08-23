package auth

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/store"
)

func deviceFixture(t *testing.T) (*DeviceManager, store.Store, func(time.Duration)) {
	t.Helper()
	st := newStore(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	clock := &now
	mgr := NewDeviceManager(st, func() time.Time { return *clock })
	if err := st.UpsertUser(t.Context(), store.User{
		ID: "u-member", Name: "member", Role: store.RoleMember,
	}); err != nil {
		t.Fatal(err)
	}
	return mgr, st, func(d time.Duration) { *clock = clock.Add(d) }
}

// The happy path: a device pairs, a human approves, the device redeems exactly one credential.
func TestDevicePairingRoundTrip(t *testing.T) {
	t.Parallel()
	mgr, _, _ := deviceFixture(t)

	deviceCode, userCode, _, err := mgr.StartPairing(t.Context(), "Living Room Shield")
	if err != nil {
		t.Fatal(err)
	}

	// While pending, the device must be told to keep waiting — not that its code is wrong.
	if _, _, err := mgr.Redeem(t.Context(), deviceCode); err != ErrPairingNotApproved {
		t.Fatalf("Redeem before approval = %v, want ErrPairingNotApproved", err)
	}

	if err := mgr.Approve(t.Context(), userCode, "u-member"); err != nil {
		t.Fatal(err)
	}
	token, name, err := mgr.Redeem(t.Context(), deviceCode)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || name != "Living Room Shield" {
		t.Fatalf("Redeem = (%q, %q), want a token and the device name", token, name)
	}

	user, err := mgr.ResolveDevice(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "u-member" {
		t.Errorf("device resolved to %q, want the approving user", user.ID)
	}
}

// ⚠ The security property this whole feature exists for: a device inherits the APPROVER's role, so
// a member's TV must never come back as admin.
func TestDeviceInheritsApproverRoleAndNeverEscalates(t *testing.T) {
	t.Parallel()
	mgr, _, _ := deviceFixture(t)
	deviceCode, userCode, _, err := mgr.StartPairing(t.Context(), "TV")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Approve(t.Context(), userCode, "u-member"); err != nil {
		t.Fatal(err)
	}
	token, _, err := mgr.Redeem(t.Context(), deviceCode)
	if err != nil {
		t.Fatal(err)
	}
	user, err := mgr.ResolveDevice(t.Context(), token)
	if err != nil {
		t.Fatal(err)
	}
	if user.Role == "admin" {
		t.Fatalf("device escalated to admin; role = %q", user.Role)
	}
	if user.Role != "member" {
		t.Errorf("device role = %q, want member", user.Role)
	}
}

// A pairing is single-use: redeeming twice must not mint a second credential.
func TestDeviceCodeCannotBeRedeemedTwice(t *testing.T) {
	t.Parallel()
	mgr, _, _ := deviceFixture(t)
	deviceCode, userCode, _, err := mgr.StartPairing(t.Context(), "TV")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Approve(t.Context(), userCode, "u-member"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := mgr.Redeem(t.Context(), deviceCode); err != nil {
		t.Fatal(err)
	}
	if _, _, err := mgr.Redeem(t.Context(), deviceCode); err == nil {
		t.Fatal("second Redeem succeeded; the pairing must be consumed")
	}
}

// Approving twice must also be single-use — the guard is the store's conditional update, so this
// covers a double click as well as two racing humans.
func TestDevicePairingApprovesOnce(t *testing.T) {
	t.Parallel()
	mgr, _, _ := deviceFixture(t)
	_, userCode, _, err := mgr.StartPairing(t.Context(), "TV")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Approve(t.Context(), userCode, "u-member"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Approve(t.Context(), userCode, "u-member"); err == nil {
		t.Fatal("second Approve succeeded; approval must be single-use")
	}
}

// An abandoned code must stop working on its own.
func TestDevicePairingExpires(t *testing.T) {
	t.Parallel()
	mgr, _, advance := deviceFixture(t)
	deviceCode, userCode, _, err := mgr.StartPairing(t.Context(), "TV")
	if err != nil {
		t.Fatal(err)
	}
	advance(PairingTTL + time.Minute)
	if err := mgr.Approve(t.Context(), userCode, "u-member"); err == nil {
		t.Error("approved an expired pairing")
	}
	if _, _, err := mgr.Redeem(t.Context(), deviceCode); err == nil {
		t.Error("redeemed an expired pairing")
	}
}

// Disabling a user must take effect on their TV immediately, the same way it kills their sessions.
func TestDisabledUserLosesDeviceAccess(t *testing.T) {
	t.Parallel()
	mgr, st, _ := deviceFixture(t)
	deviceCode, userCode, _, err := mgr.StartPairing(t.Context(), "TV")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Approve(t.Context(), userCode, "u-member"); err != nil {
		t.Fatal(err)
	}
	token, _, err := mgr.Redeem(t.Context(), deviceCode)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertUser(t.Context(), store.User{
		ID: "u-member", Name: "member", Role: store.RoleMember, Disabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.ResolveDevice(t.Context(), token); err == nil {
		t.Fatal("a disabled user's device still authenticates")
	}
}

// A revoked device stops working while its owner's other devices keep going — the reason per-device
// tokens exist rather than one shared TV credential.
func TestRevokingOneDeviceLeavesOthers(t *testing.T) {
	t.Parallel()
	mgr, st, _ := deviceFixture(t)
	tokens := make([]string, 0, 2)
	for _, name := range []string{"Living Room", "Bedroom"} {
		deviceCode, userCode, _, err := mgr.StartPairing(t.Context(), name)
		if err != nil {
			t.Fatal(err)
		}
		if err := mgr.Approve(t.Context(), userCode, "u-member"); err != nil {
			t.Fatal(err)
		}
		token, _, err := mgr.Redeem(t.Context(), deviceCode)
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}

	list, err := st.ListDeviceTokensForUser(t.Context(), "u-member")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("paired devices = %d, want 2", len(list))
	}

	revoked, err := st.DeleteDeviceToken(t.Context(), list[0].TokenHash, "u-member")
	if err != nil || !revoked {
		t.Fatalf("DeleteDeviceToken = (%v, %v), want revoked", revoked, err)
	}
	remaining := 0
	for _, token := range tokens {
		if _, err := mgr.ResolveDevice(t.Context(), token); err == nil {
			remaining++
		}
	}
	if remaining != 1 {
		t.Errorf("%d devices still authenticate, want exactly 1", remaining)
	}
}

// A device may only be revoked by its owner — the authorisation is in the WHERE clause.
func TestDeviceRevocationIsScopedToItsOwner(t *testing.T) {
	t.Parallel()
	mgr, st, _ := deviceFixture(t)
	if err := st.UpsertUser(t.Context(), store.User{ID: "u-other", Name: "other", Role: store.RoleMember}); err != nil {
		t.Fatal(err)
	}
	deviceCode, userCode, _, err := mgr.StartPairing(t.Context(), "TV")
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Approve(t.Context(), userCode, "u-member"); err != nil {
		t.Fatal(err)
	}
	token, _, err := mgr.Redeem(t.Context(), deviceCode)
	if err != nil {
		t.Fatal(err)
	}
	list, err := st.ListDeviceTokensForUser(t.Context(), "u-member")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListDeviceTokensForUser = (%d, %v)", len(list), err)
	}
	revoked, err := st.DeleteDeviceToken(t.Context(), list[0].TokenHash, "u-other")
	if err != nil {
		t.Fatal(err)
	}
	if revoked {
		t.Fatal("another user revoked a device they do not own")
	}
	if _, err := mgr.ResolveDevice(t.Context(), token); err != nil {
		t.Error("the owner's device stopped working after a foreign revoke attempt")
	}
}

// The displayed code must be readable across a room: no look-alike glyphs, and no accidental words.
func TestUserCodeAvoidsAmbiguousGlyphs(t *testing.T) {
	t.Parallel()
	mgr, _, _ := deviceFixture(t)
	for i := 0; i < 200; i++ {
		_, userCode, _, err := mgr.StartPairing(t.Context(), "TV")
		if err != nil {
			t.Fatal(err)
		}
		if strings.ContainsAny(userCode, "IO01S5AEU") {
			t.Fatalf("user code %q contains an ambiguous or vowel glyph", userCode)
		}
		if len(userCode) != 9 || userCode[4] != '-' {
			t.Fatalf("user code %q is not two groups of four", userCode)
		}
	}
}

// A human retyping the code must not be defeated by spacing or case.
func TestNormalizeUserCodeAcceptsHumanTyping(t *testing.T) {
	t.Parallel()
	want := "BCDF-GHJK"
	for _, raw := range []string{"BCDF-GHJK", "bcdf-ghjk", "bcdfghjk", " BCDF GHJK ", "bcdf ghjk"} {
		if got := NormalizeUserCode(raw); got != want {
			t.Errorf("NormalizeUserCode(%q) = %q, want %q", raw, got, want)
		}
	}
}
