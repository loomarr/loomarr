package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/contact"
	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/provision"
)

// Operational tables: the settings KV, session lifecycle, the /metrics count queries,
// the activity feed, and the retention purge that sweeps the first three.

func testUserContactAddresses(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0)

	email, normalized, err := contact.Normalize("Ada@Example.COM")
	if err != nil {
		t.Fatal(err)
	}
	pending := contact.Address{
		OwnerKind: contact.OwnerUser, OwnerID: "u1", Email: email, Normalized: normalized,
		Status: contact.StatusPending, Provenance: contact.ProvenanceAdmin, CreatedAt: now,
	}
	if err := s.PutPendingContactAddress(ctx, pending); !errors.Is(err, ErrNotFound) {
		t.Fatalf("contact for missing user = %v, want ErrNotFound", err)
	}
	if err := s.UpsertUser(ctx, User{ID: "u1", Name: "Ada", Role: RoleAdmin, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUser(ctx, User{ID: "u2", Name: "Grace", Role: RoleMember, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutPendingContactAddress(ctx, pending); err != nil {
		t.Fatal(err)
	}
	set, err := s.GetContactAddresses(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if set.Verified != nil || set.Pending == nil || set.Pending.Normalized != normalized {
		t.Fatalf("initial contact set = %+v, want pending %q only", set, normalized)
	}
	if _, err := s.VerifyPendingContactAddress(ctx, "u1", "stale@example.com", now.Add(time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale verification = %v, want ErrNotFound", err)
	}
	verifiedAt := now.Add(time.Minute)
	verified, err := s.VerifyPendingContactAddress(ctx, "u1", normalized, verifiedAt)
	if err != nil {
		t.Fatal(err)
	}
	if verified.Status != contact.StatusVerified || !verified.VerifiedAt.Equal(verifiedAt) {
		t.Fatalf("verified contact = %+v", verified)
	}
	byAddress, err := s.GetVerifiedContactAddressByNormalized(ctx, normalized)
	if err != nil || byAddress.OwnerID != "u1" {
		t.Fatalf("verified lookup = %+v, %v", byAddress, err)
	}

	replacementEmail, replacementKey, err := contact.Normalize("ada.new@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PutPendingContactAddress(ctx, contact.Address{
		OwnerKind: contact.OwnerUser, OwnerID: "u1", Email: replacementEmail, Normalized: replacementKey,
		Status: contact.StatusPending, Provenance: contact.ProvenanceSelf, CreatedAt: now.Add(2 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	set, err = s.GetContactAddresses(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if set.Verified == nil || set.Verified.Normalized != normalized || set.Pending == nil || set.Pending.Normalized != replacementKey {
		t.Fatalf("replacement state = %+v; old verified address must remain recovery-capable", set)
	}
	if err := s.PutPendingContactAddress(ctx, contact.Address{
		OwnerKind: contact.OwnerUser, OwnerID: "u2", Email: replacementEmail, Normalized: replacementKey,
		Status: contact.StatusPending, Provenance: contact.ProvenanceAdmin, CreatedAt: now,
	}); !errors.Is(err, ErrContactAddressConflict) {
		t.Fatalf("duplicate normalized address = %v, want ErrContactAddressConflict", err)
	}

	if _, err := s.VerifyPendingContactAddress(ctx, "u1", replacementKey, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetVerifiedContactAddressByNormalized(ctx, normalized); !errors.Is(err, ErrNotFound) {
		t.Fatalf("retired verified lookup = %v, want ErrNotFound", err)
	}
	if err := s.PutPendingContactAddress(ctx, contact.Address{
		OwnerKind: contact.OwnerUser, OwnerID: "u1", Email: "ADA.NEW@example.com", Normalized: replacementKey,
		Status: contact.StatusPending, Provenance: contact.ProvenanceAdmin, CreatedAt: now.Add(4 * time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	set, err = s.GetContactAddresses(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if set.Pending != nil || set.Verified == nil || set.Verified.Email != "ADA.NEW@example.com" {
		t.Fatalf("case-only correction = %+v; must preserve verification and cancel replacement", set)
	}

	all, err := s.ListContactAddresses(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("ListContactAddresses = %+v, %v; want one verified row", all, err)
	}
	if err := s.DeleteContactAddresses(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	set, err = s.GetContactAddresses(ctx, "u1")
	if err != nil || set.Verified != nil || set.Pending != nil {
		t.Fatalf("deleted contact set = %+v, %v", set, err)
	}
}

func testUserCredentialCapabilities(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	want := User{
		ID: "linked", Name: "Linked", Role: RoleMember, MediaServerLinked: true,
		PasswordHash: "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$dGFn", CreatedAt: now, UpdatedAt: now,
	}
	if err := s.UpsertUser(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetUser(ctx, want.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.MediaServerLinked || got.PasswordHash != want.PasswordHash {
		t.Fatalf("linked+verifier did not round-trip: %+v", got)
	}
}

func testSettings(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.GetSetting(ctx, "instance_id"); err != ErrNotFound {
		t.Errorf("GetSetting(missing) = %v, want ErrNotFound", err)
	}
	if err := s.SetSetting(ctx, "instance_id", "abc123"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(ctx, "instance_id", "def456"); err != nil {
		t.Fatal(err) // upsert
	}
	v, err := s.GetSetting(ctx, "instance_id")
	if err != nil || v != "def456" {
		t.Errorf("GetSetting = %q,%v want def456,nil", v, err)
	}

	// --- audited overrides (config-design §3): ListSettings + UpsertSetting +
	// DeleteSetting round-trip with audit metadata, one suite both backends. ---
	when := time.Unix(1_700_000_000, 0).UTC()
	if err := s.UpsertSetting(ctx, SettingRow{Key: "library.url", Value: "http://emby:8096", UpdatedAt: when, UpdatedBy: "matt"}); err != nil {
		t.Fatal(err)
	}
	// A system write (SetSetting) leaves updated_by NULL — surfaced as "".
	if err := s.SetSetting(ctx, "llm.model", "qwen3:8b"); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]SettingRow{}
	for _, r := range rows {
		got[r.Key] = r
	}
	if r := got["library.url"]; r.Value != "http://emby:8096" || r.UpdatedBy != "matt" || !r.UpdatedAt.Equal(when) {
		t.Errorf("audited override: %+v want value/matt/%v", r, when)
	}
	if r := got["llm.model"]; r.Value != "qwen3:8b" || r.UpdatedBy != "" {
		t.Errorf("system write should have empty updated_by: %+v", r)
	}
	// Upsert overwrites value + audit.
	later := time.Unix(1_700_000_100, 0).UTC()
	if err := s.UpsertSetting(ctx, SettingRow{Key: "library.url", Value: "http://emby:9096", UpdatedAt: later, UpdatedBy: "ana"}); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ListSettings(ctx)
	for _, r := range rows {
		if r.Key == "library.url" && (r.Value != "http://emby:9096" || r.UpdatedBy != "ana") {
			t.Errorf("upsert did not overwrite audit: %+v", r)
		}
	}

	// A settings PATCH is one durable commit, including a mix of upserts and clears.
	// The same suite pins this contract for SQLite and Postgres.
	batchAt := time.Unix(1_700_000_200, 0).UTC()
	if err := s.ApplySettingBatch(ctx, SettingBatch{
		Upserts: []SettingMutation{
			{Key: "library.flavor", Value: "emby"},
			{Key: "library.url", Value: "http://batch:8096"},
			{Key: "library.token", Value: "batch-token"},
		},
		Deletes:   []string{"llm.model"},
		UpdatedAt: batchAt,
		UpdatedBy: "grace",
	}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.ListSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got = map[string]SettingRow{}
	for _, r := range rows {
		got[r.Key] = r
	}
	for _, key := range []string{"library.flavor", "library.url", "library.token"} {
		if r := got[key]; r.UpdatedBy != "grace" || !r.UpdatedAt.Equal(batchAt) {
			t.Errorf("batch audit for %s: %+v want grace/%v", key, r, batchAt)
		}
	}
	if got["library.url"].Value != "http://batch:8096" {
		t.Errorf("batch url = %q, want http://batch:8096", got["library.url"].Value)
	}
	if _, ok := got["llm.model"]; ok {
		t.Error("batch delete left llm.model present")
	}

	// Ambiguous batches fail before opening a transaction or changing any row.
	if err := s.ApplySettingBatch(ctx, SettingBatch{
		Upserts: []SettingMutation{{Key: "library.url", Value: "http://must-not-save:8096"}},
		Deletes: []string{"library.url"},
	}); err == nil {
		t.Fatal("duplicate setting key in batch succeeded")
	}
	if v, err := s.GetSetting(ctx, "library.url"); err != nil || v != "http://batch:8096" {
		t.Errorf("rejected batch mutated url: %q %v", v, err)
	}
	// --- the env-override claim (config-design §3.1), one suite both backends ---
	//
	// ⚠ The bug this guards: env_override must NOT be in UpsertSetting's DO UPDATE list.
	// If it were, an ordinary save would silently re-lock a key the operator had just
	// unlocked — at the exact moment they are certain to be editing it.
	if err := s.SetSettingEnvOverride(ctx, "library.url", true, "http://seed:8096", "matt"); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ListSettings(ctx)
	got = map[string]SettingRow{}
	for _, r := range rows {
		got[r.Key] = r
	}
	if r := got["library.url"]; !r.EnvOverride {
		t.Errorf("claim not persisted: %+v", r)
	}
	// A plain value save must leave the claim standing.
	if err := s.UpsertSetting(ctx, SettingRow{Key: "library.url", Value: "http://edited:8096", UpdatedAt: later, UpdatedBy: "ana"}); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ListSettings(ctx)
	for _, r := range rows {
		if r.Key == "library.url" {
			if !r.EnvOverride {
				t.Error("an ordinary save cleared the env-override claim — the key silently re-locked")
			}
			if r.Value != "http://edited:8096" {
				t.Errorf("value did not save alongside the claim: %+v", r)
			}
		}
	}
	// Seeding only applies when the row is absent; an existing value is never clobbered.
	if err := s.SetSettingEnvOverride(ctx, "library.url", true, "http://should-not-apply:8096", "matt"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.GetSetting(ctx, "library.url"); v != "http://edited:8096" {
		t.Errorf("re-claiming overwrote the stored value with the seed: %q", v)
	}
	// Handing the key back keeps the value, so the round trip loses nothing.
	if err := s.SetSettingEnvOverride(ctx, "library.url", false, "", "matt"); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ListSettings(ctx)
	for _, r := range rows {
		if r.Key == "library.url" {
			if r.EnvOverride {
				t.Error("claim not cleared on re-lock")
			}
			if r.Value != "http://edited:8096" {
				t.Errorf("re-lock discarded the stored value: %+v", r)
			}
		}
	}
	// Claiming a key with NO existing row creates one carrying the seed — the ordinary
	// unlock case, since an env-pinned key usually has nothing stored.
	if err := s.SetSettingEnvOverride(ctx, "seerr.url", true, "http://seerr:5055", "matt"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.GetSetting(ctx, "seerr.url"); err != nil || v != "http://seerr:5055" {
		t.Errorf("unlock did not seed a new row: %q %v", v, err)
	}

	// Delete reverts the key (config-design §9).
	if err := s.DeleteSetting(ctx, "library.url"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSetting(ctx, "library.url"); err != ErrNotFound {
		t.Errorf("after delete, GetSetting = %v want ErrNotFound", err)
	}
}

// testSessionLifecycle covers the §11 session rules that authentication depends on:
// sessions are revocable rows, expiry is immediate for reads even though purging is
// eventual, and disabling a user kills every session at once. None of this had store
// conformance coverage before — it is the one area where a dialect difference would be
// a security bug rather than a correctness bug, so it belongs in the shared suite.
// testLastSeen (#1667): a session remembers when it was last used and from what client, and a
// person's last-seen follows both their browser sessions and their paired TV (a paired TV acts
// as the person who paired it).
func testLastSeen(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	t0 := time.Unix(1_800_000_000, 0).UTC()
	for _, id := range []string{"u-seen", "u-never"} {
		if err := s.UpsertUser(ctx, User{ID: id, Name: id, Role: RoleMember}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateSession(ctx, Session{TokenHash: "h1", UserID: "u-seen", CreatedAt: t0, ExpiresAt: t0.Add(24 * time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateDeviceToken(ctx, DeviceToken{TokenHash: "d1", UserID: "u-seen", DeviceName: "Lounge TV", CreatedAt: t0}); err != nil {
		t.Fatal(err)
	}
	touch := func(at time.Time, label string) {
		t.Helper()
		if err := s.TouchSession(ctx, "h1", SessionSeen{At: at, ExpiresAt: at.Add(24 * time.Hour), ClientLabel: label}); err != nil {
			t.Fatal(err)
		}
	}
	touch(t0.Add(time.Hour), "Firefox on macOS")
	touch(t0.Add(2*time.Hour), "") // a request without a recognisable client keeps the label
	sessions, err := s.ListSessionsForUser(ctx, "u-seen", t0.Add(2*time.Hour))
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions = %+v (err %v), want one", sessions, err)
	}
	if got := sessions[0]; !got.LastSeenAt.Equal(t0.Add(2*time.Hour)) || got.ClientLabel != "Firefox on macOS" {
		t.Errorf("session seen %v from %q, want %v from Firefox on macOS", got.LastSeenAt, got.ClientLabel, t0.Add(2*time.Hour))
	}
	lastSeen := func() string {
		t.Helper()
		seen, err := s.UserLastSeen(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(seen)
	}
	if got, want := lastSeen(), fmt.Sprint(map[string]time.Time{"u-seen": t0.Add(2 * time.Hour)}); got != want {
		t.Errorf("after browser use: last seen %s, want %s (never-seen users absent)", got, want)
	}
	if err := s.TouchDeviceToken(ctx, "d1", t0.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, want := lastSeen(), fmt.Sprint(map[string]time.Time{"u-seen": t0.Add(3 * time.Hour)}); got != want {
		t.Errorf("after TV use: last seen %s, want %s", got, want)
	}
}

func testSessionLifecycle(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	if err := s.UpsertUser(ctx, User{ID: "u1", Name: "Ada", Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUser(ctx, User{ID: "u2", Name: "Grace", Role: RoleMember}); err != nil {
		t.Fatal(err)
	}

	live := Session{TokenHash: "h-live", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	older := Session{TokenHash: "h-older", UserID: "u1", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}
	dead := Session{TokenHash: "h-dead", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(-time.Minute)}
	other := Session{TokenHash: "h-other", UserID: "u2", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	for _, sess := range []Session{live, older, dead, other} {
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListSessionsForUser(ctx, "u1", now)
	if err != nil {
		t.Fatal(err)
	}
	// Expired excluded, another user's session excluded, newest first.
	if len(got) != 2 {
		t.Fatalf("ListSessionsForUser = %d sessions, want 2 (expired and other-user excluded)", len(got))
	}
	if got[0].TokenHash != "h-live" || got[1].TokenHash != "h-older" {
		t.Errorf("order = [%s %s], want newest first [h-live h-older]", got[0].TokenHash, got[1].TokenHash)
	}
	if !got[0].CreatedAt.Equal(now) || !got[0].ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Errorf("timestamps did not round-trip: created=%v expires=%v", got[0].CreatedAt, got[0].ExpiresAt)
	}

	// Revoking one leaves the rest — the admin "sign out this device" path.
	if err := s.RevokeSession(ctx, "h-live"); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.ListSessionsForUser(ctx, "u1", now); len(got) != 1 {
		t.Errorf("after RevokeSession: %d sessions, want 1", len(got))
	}

	// Disabling a user kills their sessions immediately (§11) — and only theirs.
	if err := s.RevokeSessionsForUser(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.ListSessionsForUser(ctx, "u1", now); len(got) != 0 {
		t.Errorf("after RevokeSessionsForUser: %d sessions, want 0", len(got))
	}
	if got, _ = s.ListSessionsForUser(ctx, "u2", now); len(got) != 1 {
		t.Errorf("RevokeSessionsForUser hit another user's sessions: u2 has %d, want 1", len(got))
	}

	// …and their paired devices, for good (ADR 0043) — and only theirs.
	for _, d := range []DeviceToken{
		{TokenHash: "d-tv", UserID: "u1", DeviceName: "Lounge TV", CreatedAt: now},
		{TokenHash: "d-phone", UserID: "u1", DeviceName: "Phone", CreatedAt: now},
		{TokenHash: "d-other", UserID: "u2", DeviceName: "Kitchen TV", CreatedAt: now},
	} {
		if err := s.CreateDeviceToken(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RevokeDeviceTokensForUser(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if devices, err := s.ListDeviceTokensForUser(ctx, "u1"); err != nil || len(devices) != 0 {
		t.Errorf("after RevokeDeviceTokensForUser: u1 has %d devices (err %v), want 0", len(devices), err)
	}
	if _, err := s.GetDeviceToken(ctx, "d-tv"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetDeviceToken(revoked) err = %v, want ErrNotFound", err)
	}
	if devices, _ := s.ListDeviceTokensForUser(ctx, "u2"); len(devices) != 1 {
		t.Errorf("RevokeDeviceTokensForUser hit another user's devices: u2 has %d, want 1", len(devices))
	}

	// …and the pairings they approved but no device redeemed yet — and only theirs; a pending
	// pairing nobody approved is not anyone's to revoke.
	for _, p := range []DevicePairing{
		{DeviceCodeHash: "p-u1", UserCode: "AAAA-AAAA"},
		{DeviceCodeHash: "p-u2", UserCode: "BBBB-BBBB"},
		{DeviceCodeHash: "p-pending", UserCode: "CCCC-CCCC"},
	} {
		p.CreatedAt, p.ExpiresAt = now, now.Add(10*time.Minute)
		if err := s.CreateDevicePairing(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	for code, user := range map[string]string{"AAAA-AAAA": "u1", "BBBB-BBBB": "u2"} {
		if ok, err := s.ApproveDevicePairing(ctx, code, user, now); err != nil || !ok {
			t.Fatalf("ApproveDevicePairing(%s, %s) = %v, %v", code, user, ok, err)
		}
	}
	if err := s.RevokeDevicePairingsForUser(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetDevicePairing(ctx, "p-u1", now); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetDevicePairing(revoked) err = %v, want ErrNotFound", err)
	}
	for _, code := range []string{"p-u2", "p-pending"} {
		if _, err := s.GetDevicePairing(ctx, code, now); err != nil {
			t.Errorf("RevokeDevicePairingsForUser hit %s: %v", code, err)
		}
	}

	// A disabled user approves nothing and is issued nothing, so no credential can be minted after
	// the revocation above (ADR 0043). Unknown users likewise.
	if err := s.UpsertUser(ctx, User{ID: "u1", Name: "Ada", Role: RoleAdmin, Disabled: true}); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ApproveDevicePairing(ctx, "CCCC-CCCC", "u1", now); err != nil || ok {
		t.Errorf("ApproveDevicePairing by a disabled user = %v, %v; want false, nil", ok, err)
	}
	if ok, err := s.ApproveDevicePairing(ctx, "CCCC-CCCC", "u-ghost", now); err != nil || ok {
		t.Errorf("ApproveDevicePairing by an unknown user = %v, %v; want false, nil", ok, err)
	}
	for _, user := range []string{"u1", "u-ghost"} {
		err := s.CreateDeviceToken(ctx, DeviceToken{TokenHash: "d-late-" + user, UserID: user, DeviceName: "TV", CreatedAt: now})
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("CreateDeviceToken for %s err = %v, want ErrNotFound", user, err)
		}
	}
	if devices, err := s.ListDeviceTokensForUser(ctx, "u1"); err != nil || len(devices) != 0 {
		t.Errorf("disabled u1 was issued %d devices (err %v), want 0", len(devices), err)
	}
	// Re-enabled, the same user approves again: the guard is the flag, not a tombstone.
	if err := s.UpsertUser(ctx, User{ID: "u1", Name: "Ada", Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ApproveDevicePairing(ctx, "CCCC-CCCC", "u1", now); err != nil || !ok {
		t.Errorf("ApproveDevicePairing by re-enabled u1 = %v, %v; want true, nil", ok, err)
	}
}

// testCounts covers the §17 observability gauges: grouped counts must reflect
// the rows present, and (for sessions) honor the same expiry predicate the read
// path uses. Same assertions on both backends (one suite, two dialects).
func testCounts(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	// Titles across three states: two requested, one available.
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:20", provision.Requested, time.Time{}))
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:21", provision.Requested, time.Time{}))
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:22", provision.Available, time.Time{}))

	titles, err := s.CountTitlesByState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if titles[provision.Requested] != 2 || titles[provision.Available] != 1 {
		t.Errorf("CountTitlesByState = %v, want requested:2 available:1", titles)
	}
	// A state with no rows is absent, not zero — the collector zero-fills it.
	if _, ok := titles[provision.Downloading]; ok {
		t.Errorf("CountTitlesByState included an empty state: %v", titles)
	}

	// Jobs: two queued (sampleJob defaults to queued), one flipped to running.
	_ = s.CreateJob(ctx, sampleJob("job-1", "h1", now, now.Add(-5*time.Minute)))
	_ = s.CreateJob(ctx, sampleJob("job-2", "h2", now, now))
	running := sampleJob("job-3", "h3", now, now)
	running.Status = "running"
	_ = s.CreateJob(ctx, running)
	failed := sampleJob("job-failed", "h4", now, now)
	failed.Status, failed.FailureCode = "failed", "generation_failed"
	_ = s.CreateJob(ctx, failed)
	unknownFailure := sampleJob("job-unknown", "h5", now, now)
	unknownFailure.Status, unknownFailure.FailureCode = "failed", "provider_secret"
	_ = s.CreateJob(ctx, unknownFailure)
	maintenance := sampleJob("recurate-job", "h6", now, now.Add(-time.Hour))
	maintenance.Kind = "recurate"
	_ = s.CreateJob(ctx, maintenance)

	jobs, err := s.CountJobsByStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if jobs["queued"] != 3 || jobs["running"] != 1 {
		t.Errorf("CountJobsByStatus = %v, want queued:3 running:1", jobs)
	}
	oldest, err := s.OldestProposalJobsByStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !oldest["queued"].Equal(now.Add(-5*time.Minute)) || !oldest["running"].Equal(now) {
		t.Errorf("OldestProposalJobsByStatus = %v", oldest)
	}
	if _, included := oldest["failed"]; included {
		t.Errorf("terminal Proposal Job included in oldest ages: %v", oldest)
	}
	failures, err := s.CountFailedProposalJobsByCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if failures["generation_failed"] != 1 || failures["provider_secret"] != 1 {
		t.Errorf("CountFailedProposalJobsByCode = %v", failures)
	}

	// Create one succeeded and one failed terminal Attempt through the guarded
	// transition methods so the aggregate observes the real durable history.
	for i, outcome := range []string{"succeeded", "failed"} {
		job := sampleJob(fmt.Sprintf("attempt-%d", i), fmt.Sprintf("ah-%d", i), now.Add(-2*time.Hour), now)
		if err := s.CreateJob(ctx, job); err != nil {
			t.Fatal(err)
		}
		claimed, err := s.ClaimDueJobs(ctx, now, time.Minute, 1)
		if err != nil || len(claimed) != 1 {
			t.Fatalf("claim metric Attempt %d = %+v, %v", i, claimed, err)
		}
		claimedJob := claimed[0]
		if outcome == "succeeded" {
			proposal := Proposal{ID: "metric-proposal", JobID: claimedJob.ID, Status: "submitted",
				CreatedBy: claimedJob.CreatedBy, ProposalJSON: `{}`, CreatedAt: now, UpdatedAt: now}
			if err := s.CommitSuggestionSuccess(ctx, claimedJob.ID, claimedJob.Attempts, proposal, now); err != nil {
				t.Fatal(err)
			}
		} else if err := s.CommitSuggestionFailure(ctx, claimedJob.ID, claimedJob.Attempts,
			"private", "generation_failed", "", now); err != nil {
			t.Fatal(err)
		}
	}
	attempts, err := s.CountProposalJobAttemptsByStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if attempts["succeeded"] != 1 || attempts["failed"] != 1 {
		t.Errorf("CountProposalJobAttemptsByStatus = %v", attempts)
	}

	// Sessions: two live, one expired — only the live ones count as of now.
	if err := s.UpsertUser(ctx, User{ID: "u1", Name: "Ada", Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	for _, sess := range []Session{
		{TokenHash: "c-live1", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{TokenHash: "c-live2", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{TokenHash: "c-dead", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(-time.Minute)},
	} {
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
	}
	active, err := s.CountActiveSessions(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if active != 2 {
		t.Errorf("CountActiveSessions = %d, want 2 (expired excluded)", active)
	}
}

// testActivityFeed covers the Dashboard feed's three operations on both backends (§5, V32):
// append, newest-first read, and age-based purge.
func testActivityFeed(t *testing.T, newStore func(t *testing.T) Store) {
	t.Helper()
	st := newStore(t)
	ctx := context.Background()

	base := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	for i, tc := range []struct {
		text  string
		level string
		age   time.Duration
	}{
		{"oldest — CH 12 reconciled", ActivityInfo, 90 * time.Minute},
		{"middle — Seerr timed out, retrying", ActivityWarn, 60 * time.Minute},
		{"newest — Darkwing Duck landed", ActivityInfo, 30 * time.Minute},
	} {
		if err := st.RecordActivity(ctx, Activity{
			At: base.Add(-tc.age).Unix(), Kind: ActivityKindTitle, Level: tc.level,
			Text: tc.text, SubjectID: fmt.Sprintf("subj-%d", i),
		}); err != nil {
			t.Fatalf("record activity %d: %v", i, err)
		}
	}

	// Newest first — the ONLY ordering the feed has.
	got, err := st.ListActivity(ctx, 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("listed %d rows, want 3", len(got))
	}
	if !strings.HasPrefix(got[0].Text, "newest") || !strings.HasPrefix(got[2].Text, "oldest") {
		t.Errorf("wrong order: %q ... %q", got[0].Text, got[2].Text)
	}
	if got[1].Level != ActivityWarn {
		t.Errorf("level = %q, want warn — it drives the UI dot", got[1].Level)
	}
	if got[0].ID == "" {
		t.Error("row has no id; the store must mint one")
	}

	// The limit is honoured, or a dashboard panel would render the whole table.
	if two, err := st.ListActivity(ctx, 2); err != nil || len(two) != 2 {
		t.Errorf("ListActivity(2) = %d rows, %v; want 2", len(two), err)
	}

	// An unrecognised level is normalised on the way in — the UI has no colour for it, so
	// storing it would render an invisible dot.
	if err := st.RecordActivity(ctx, Activity{Text: "odd", Level: "banana"}); err != nil {
		t.Fatalf("record odd level: %v", err)
	}
	after, _ := st.ListActivity(ctx, 1)
	if len(after) != 1 || after[0].Level != ActivityInfo {
		t.Errorf("level %q was stored as-is; want normalisation to info", after[0].Level)
	}

	// An empty text is dropped rather than stored: a blank row occupies a slot in a list
	// the operator is scanning.
	before, _ := st.ListActivity(ctx, 100)
	if err := st.RecordActivity(ctx, Activity{Text: ""}); err != nil {
		t.Fatalf("record empty: %v", err)
	}
	if all, _ := st.ListActivity(ctx, 100); len(all) != len(before) {
		t.Errorf("an empty-text row was stored (%d -> %d)", len(before), len(all))
	}

	// Purge is by AGE — the feed is the one append-only table here, so it is the one that
	// would otherwise grow without bound.
	n, err := st.PurgeActivity(ctx, base.Add(-45*time.Minute))
	if err != nil {
		t.Fatalf("purge activity: %v", err)
	}
	if n != 2 {
		t.Errorf("purged %d rows, want 2 (the two older than the horizon)", n)
	}
	left, _ := st.ListActivity(ctx, 100)
	for _, a := range left {
		if strings.HasPrefix(a.Text, "oldest") || strings.HasPrefix(a.Text, "middle") {
			t.Errorf("row %q survived a purge that should have removed it", a.Text)
		}
	}
}

// testDiagnostics pins the normalized-event batch, Process-run lifecycle, and conservative
// retention contract against both SQLite and Postgres (§5, §17).
func testDiagnostics(t *testing.T, newStore func(t *testing.T) Store) {
	t.Helper()
	st := newStore(t)
	ctx := context.Background()
	base := time.Date(2026, 8, 23, 16, 0, 0, 0, time.UTC)

	oldEvent := diagnostics.Record{
		ID: "diag-old", OccurredAt: base.Add(-2 * time.Hour).UnixMilli(), ReceivedAt: base.UnixMilli(),
		Level: diagnostics.LevelInfo, Source: diagnostics.SourceServer, Subsystem: "store",
		Event: "store.old", AttributesJSON: `{}`, SizeBytes: 20,
	}
	newEvent := diagnostics.Record{
		ID: "diag-new", OccurredAt: base.Add(-time.Minute).UnixMilli(), ReceivedAt: base.UnixMilli(),
		Level: diagnostics.LevelError, Source: diagnostics.SourceWeb, Subsystem: "player",
		Event: "player.failed", RequestID: "req-1", ChannelID: "channel-1",
		AttributesJSON: `{"code":"MEDIA_ERR"}`, SizeBytes: 40,
	}
	if err := st.AppendDiagnosticEvents(ctx, []diagnostics.Record{oldEvent, newEvent}); err != nil {
		t.Fatal(err)
	}
	events, err := st.ListDiagnosticEvents(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0] != newEvent || events[1] != oldEvent {
		t.Fatalf("diagnostic events = %+v, want newest then oldest", events)
	}
	filtered, err := st.QueryDiagnosticEvents(ctx, diagnostics.EventStoreQuery{
		From: base.Add(-3 * time.Hour).UnixMilli(), To: base.UnixMilli(), Limit: 10,
		Level: diagnostics.LevelError, Source: diagnostics.SourceWeb, Subsystem: "player",
		Event: "player.failed", RequestID: "req-1", ChannelID: "channel-1", Text: "media_err",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 1 || filtered[0] != newEvent {
		t.Fatalf("filtered diagnostic events = %+v, want correlated new event", filtered)
	}
	literalWildcard, err := st.QueryDiagnosticEvents(ctx, diagnostics.EventStoreQuery{
		From: base.Add(-3 * time.Hour).UnixMilli(), To: base.UnixMilli(), Limit: 10, Text: "%",
	})
	if err != nil || len(literalWildcard) != 0 {
		t.Fatalf("literal wildcard text = %+v, %v; percent must not widen the search", literalWildcard, err)
	}
	first, err := st.QueryDiagnosticEvents(ctx, diagnostics.EventStoreQuery{
		From: base.Add(-3 * time.Hour).UnixMilli(), To: base.UnixMilli(), Limit: 1,
	})
	if err != nil || len(first) != 1 || first[0].ID != newEvent.ID {
		t.Fatalf("first diagnostic page = %+v, %v", first, err)
	}
	second, err := st.QueryDiagnosticEvents(ctx, diagnostics.EventStoreQuery{
		From: base.Add(-3 * time.Hour).UnixMilli(), To: base.UnixMilli(), Limit: 1,
		CursorOccurredAt: first[0].OccurredAt, CursorID: first[0].ID,
	})
	if err != nil || len(second) != 1 || second[0].ID != oldEvent.ID {
		t.Fatalf("second diagnostic page = %+v, %v", second, err)
	}
	oldestFirst, err := st.QueryDiagnosticEvents(ctx, diagnostics.EventStoreQuery{
		From: base.Add(-3 * time.Hour).UnixMilli(), To: base.UnixMilli(), Limit: 1,
		Order: diagnostics.EventOrderOldest,
	})
	if err != nil || len(oldestFirst) != 1 || oldestFirst[0].ID != oldEvent.ID {
		t.Fatalf("oldest-first diagnostic page = %+v, %v", oldestFirst, err)
	}
	newerFromOldest, err := st.QueryDiagnosticEvents(ctx, diagnostics.EventStoreQuery{
		From: base.Add(-3 * time.Hour).UnixMilli(), To: base.UnixMilli(), Limit: 1,
		Order:            diagnostics.EventOrderOldest,
		CursorOccurredAt: oldestFirst[0].OccurredAt, CursorID: oldestFirst[0].ID,
	})
	if err != nil || len(newerFromOldest) != 1 || newerFromOldest[0].ID != newEvent.ID {
		t.Fatalf("next oldest-first diagnostic page = %+v, %v", newerFromOldest, err)
	}
	if _, err := st.QueryDiagnosticEvents(ctx, diagnostics.EventStoreQuery{Limit: 1000}); err == nil {
		t.Fatal("unbounded diagnostic store query succeeded")
	}

	// A batch is one commit: invalid input after a valid row cannot leave the valid prefix behind.
	if err := st.AppendDiagnosticEvents(ctx, []diagnostics.Record{
		{ID: "diag-rolled-back", OccurredAt: base.UnixMilli(), ReceivedAt: base.UnixMilli(),
			Level: diagnostics.LevelInfo, Source: diagnostics.SourceServer, Event: "would.persist",
			AttributesJSON: `{}`, SizeBytes: 10},
		{ID: "diag-invalid", OccurredAt: base.UnixMilli(), ReceivedAt: base.UnixMilli(),
			Level: diagnostics.LevelInfo, Source: diagnostics.SourceServer, AttributesJSON: `{}`},
	}); err == nil {
		t.Fatal("invalid diagnostic batch succeeded")
	}
	events, _ = st.ListDiagnosticEvents(ctx, 100)
	if len(events) != 2 {
		t.Fatalf("failed batch left %d total events, want original 2", len(events))
	}

	running := diagnostics.ProcessRun{
		ID: "process-running", Purpose: "playout_parent", InstanceID: "instance-1",
		ChannelID: "channel-1", StartedAt: base.Add(-24 * time.Hour).UnixMilli(),
		Status: diagnostics.ProcessRunning, UpdatedAt: base.UnixMilli(), SizeBytes: 70,
	}
	if err := st.UpsertDiagnosticProcessRun(ctx, running); err != nil {
		t.Fatal(err)
	}
	exitCode := 7
	finished := diagnostics.ProcessRun{
		ID: "process-finished", Purpose: "program_encode", ParentRunID: running.ID,
		InstanceID: "instance-1", ChannelID: "channel-1", ScheduleBlockID: "block-1",
		Executable: "ffmpeg", ExecutableVersion: "n9.0", CommandSummary: "ffmpeg [redacted]",
		StartedAt: base.Add(-3 * time.Hour).UnixMilli(), EndedAt: base.Add(-2 * time.Hour).UnixMilli(),
		Status: diagnostics.ProcessFailed, ExitCode: &exitCode, LastError: "encoder failed",
		UpdatedAt: base.Add(-2 * time.Hour).UnixMilli(), SizeBytes: 80,
	}
	if err := st.UpsertDiagnosticProcessRun(ctx, finished); err != nil {
		t.Fatal(err)
	}
	gotRun, err := st.GetDiagnosticProcessRun(ctx, finished.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotRun.ExitCode == nil || *gotRun.ExitCode != exitCode || gotRun.Status != diagnostics.ProcessFailed || gotRun.ParentRunID != running.ID {
		t.Fatalf("process run round trip = %+v", gotRun)
	}
	if _, err := st.GetDiagnosticProcessRun(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("missing process run = %v, want ErrNotFound", err)
	}
	if foundRun, found, err := st.FindDiagnosticProcessRun(ctx, finished.ID); err != nil || !found || foundRun.ID != finished.ID {
		t.Fatalf("find process run = (%+v, %v, %v)", foundRun, found, err)
	}
	if _, found, err := st.FindDiagnosticProcessRun(ctx, "missing"); err != nil || found {
		t.Fatalf("find missing process run = (%v, %v), want false nil", found, err)
	}
	processes, err := st.QueryDiagnosticProcessRuns(ctx, diagnostics.ProcessStoreQuery{
		From: base.Add(-4 * time.Hour).UnixMilli(), To: base.UnixMilli(), Limit: 2,
		Status: diagnostics.ProcessFailed, Purpose: finished.Purpose, ChannelID: finished.ChannelID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(processes) != 1 || processes[0].ID != finished.ID {
		t.Fatalf("filtered process query = %+v", processes)
	}
	if _, err := st.QueryDiagnosticProcessRuns(ctx, diagnostics.ProcessStoreQuery{Limit: 1000}); err == nil {
		t.Fatal("unbounded diagnostic process query succeeded")
	}
	fileBacked := finished
	fileBacked.ID = "process-file-backed"
	fileBacked.EndedAt = base.Add(-4 * time.Hour).UnixMilli()
	fileBacked.OutputRef = "process-file-backed.log"
	fileBacked.SizeBytes = 120
	if err := st.UpsertDiagnosticProcessRun(ctx, fileBacked); err != nil {
		t.Fatal(err)
	}
	candidates, err := st.ListDiagnosticRetentionCandidates(ctx, base.Add(-90*time.Minute), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 3 || candidates[0].ID != fileBacked.ID || candidates[0].OutputRef != fileBacked.OutputRef {
		t.Fatalf("retention candidates = %+v, want file-backed run then old event/run", candidates)
	}
	if removed, err := st.DeleteDiagnosticProcessRun(ctx, running.ID); err != nil || removed {
		t.Fatalf("active process delete = (%v, %v), want guarded false", removed, err)
	}

	// Age removes the old event and completed run, but NEVER the older active run.
	purged, err := st.PurgeDiagnostics(ctx, base.Add(-90*time.Minute), 0)
	if err != nil {
		t.Fatal(err)
	}
	if purged.Events != 1 || purged.ProcessRuns != 1 {
		t.Fatalf("age purge = %+v, want 1 event and 1 completed process", purged)
	}
	if _, err := st.GetDiagnosticProcessRun(ctx, running.ID); err != nil {
		t.Fatalf("active process was purged by age: %v", err)
	}
	if _, err := st.GetDiagnosticProcessRun(ctx, fileBacked.ID); err != nil {
		t.Fatalf("store-only purge orphaned file-backed process metadata: %v", err)
	}
	if removed, err := st.DeleteDiagnosticProcessRun(ctx, fileBacked.ID); err != nil || !removed {
		t.Fatalf("completed process delete = (%v, %v), want true", removed, err)
	}

	// The logical-byte budget removes the oldest eligible evidence. The active run remains even
	// when it alone exceeds the budget: storage pressure is not authority to delete live state.
	if err := st.AppendDiagnosticEvents(ctx, []diagnostics.Record{
		{ID: "diag-budget-old", OccurredAt: base.UnixMilli(), ReceivedAt: base.UnixMilli(), Level: diagnostics.LevelInfo, Source: diagnostics.SourceServer, Event: "budget.old", AttributesJSON: `{}`, SizeBytes: 30},
		{ID: "diag-budget-new", OccurredAt: base.Add(time.Minute).UnixMilli(), ReceivedAt: base.UnixMilli(), Level: diagnostics.LevelInfo, Source: diagnostics.SourceServer, Event: "budget.new", AttributesJSON: `{}`, SizeBytes: 30},
	}); err != nil {
		t.Fatal(err)
	}
	purged, err = st.PurgeDiagnostics(ctx, base.Add(-24*time.Hour), 110)
	if err != nil {
		t.Fatal(err)
	}
	if purged.RetainedBytes > 110 {
		t.Fatalf("retained bytes = %d, want <= 110", purged.RetainedBytes)
	}
	events, _ = st.ListDiagnosticEvents(ctx, 100)
	for _, event := range events {
		if event.ID == "diag-new" || event.ID == "diag-budget-old" {
			t.Errorf("old budget candidate %s survived before newer evidence", event.ID)
		}
	}
}

// testRetentionPurge covers §5's retention on both backends — and, more importantly, what it
// must NOT remove.
func testRetentionPurge(t *testing.T, newStore func(t *testing.T) Store) {
	t.Helper()
	st := newStore(t)
	ctx := context.Background()

	old := time.Now().Add(-100 * 24 * time.Hour)
	recent := time.Now().Add(-time.Hour)

	// Jobs: one of each status, all OLD, so status is the only thing distinguishing them.
	for _, j := range []Job{
		{ID: "j-done", Kind: "suggest", Status: "done", UpdatedAt: old},
		{ID: "j-failed", Kind: "suggest", Status: "failed", UpdatedAt: old},
		{ID: "j-queued", Kind: "suggest", Status: "queued", UpdatedAt: old},
		{ID: "j-running", Kind: "suggest", Status: "running", UpdatedAt: old},
		{ID: "j-fresh", Kind: "suggest", Status: "done", UpdatedAt: recent},
	} {
		if err := st.CreateJob(ctx, j); err != nil {
			t.Fatalf("seed job %s: %v", j.ID, err)
		}
	}

	n, err := st.PurgeFinishedJobs(ctx, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("purge jobs: %v", err)
	}
	if n != 2 {
		t.Errorf("purged %d jobs, want 2 (done + failed, both old)", n)
	}
	// ⚠ The invariants: in-flight work survives regardless of age, and a recent finished job
	// is inside the window.
	for _, id := range []string{"j-queued", "j-running", "j-fresh"} {
		if _, err := st.GetJob(ctx, id); err != nil {
			t.Errorf("job %s was purged; only finished jobs past the window may go (%v)", id, err)
		}
	}
	for _, id := range []string{"j-done", "j-failed"} {
		if _, err := st.GetJob(ctx, id); err == nil {
			t.Errorf("job %s survived a purge that should have removed it", id)
		}
	}

	// Proposals: one of each status, all OLD.
	for _, p := range []Proposal{
		{ID: "p-denied", Status: "denied", UpdatedAt: old},
		{ID: "p-approved", Status: "approved", UpdatedAt: old},
		{ID: "p-submitted", Status: "submitted", UpdatedAt: old},
		{ID: "p-fresh-denied", Status: "denied", UpdatedAt: recent},
	} {
		if err := st.CreateProposal(ctx, p); err != nil {
			t.Fatalf("seed proposal %s: %v", p.ID, err)
		}
	}

	n, err = st.PurgeDeniedProposals(ctx, time.Now().Add(-90*24*time.Hour))
	if err != nil {
		t.Fatalf("purge proposals: %v", err)
	}
	if n != 1 {
		t.Errorf("purged %d proposals, want 1 (the old denied one)", n)
	}
	// ⚠ THE AUDIT TRAIL. An approved proposal is the record that someone authorized spending
	// real resources (§7); a submitted one is a member still waiting for an answer. Neither
	// may be aged out.
	for _, id := range []string{"p-approved", "p-submitted", "p-fresh-denied"} {
		if _, err := st.GetProposal(ctx, id); err != nil {
			t.Errorf("proposal %s was purged; only OLD DENIED proposals may go (%v)", id, err)
		}
	}
	if _, err := st.GetProposal(ctx, "p-denied"); err == nil {
		t.Error("the old denied proposal survived a purge that should have removed it")
	}
}

// Both retention page modes share terminal eligibility and stable ending-time ordering.
func testDiagnosticRetentionOrder(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	st := newStore(t)
	ctx := context.Background()
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		id     string
		ended  time.Time
		status diagnostics.ProcessStatus
		output string
	}{
		{"old-z", base.Add(-2 * time.Hour), diagnostics.ProcessSucceeded, ""},
		{"old-a", base.Add(-2 * time.Hour), diagnostics.ProcessFailed, ""},
		{"oldest", base.Add(-3 * time.Hour), diagnostics.ProcessSucceeded, "file.log"},
		{"new", base.Add(-30 * time.Minute), diagnostics.ProcessSucceeded, ""},
		{"active", base.Add(-4 * time.Hour), diagnostics.ProcessRunning, ""},
		{"zero-end", time.UnixMilli(0), diagnostics.ProcessSucceeded, ""},
	} {
		row := diagnostics.ProcessRun{ID: tc.id, Purpose: "program_encode", Status: tc.status,
			StartedAt: base.Add(-5 * time.Hour).UnixMilli(), EndedAt: tc.ended.UnixMilli(), UpdatedAt: base.UnixMilli(), OutputRef: tc.output}
		if err := st.UpsertDiagnosticProcessRun(ctx, row); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		name   string
		before time.Time
		want   []string
	}{
		{"expiry", base.Add(-time.Hour), []string{"oldest", "old-a", "old-z"}},
		{"budget", time.Time{}, []string{"oldest", "old-a", "old-z", "new"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidates, err := st.ListDiagnosticRetentionCandidates(ctx, tc.before, 10)
			if err != nil {
				t.Fatal(err)
			}
			var ids []string
			for _, c := range candidates {
				ids = append(ids, c.ID)
			}
			if fmt.Sprint(ids) != fmt.Sprint(tc.want) {
				t.Fatalf("retention order=%v, want %v", ids, tc.want)
			}
			if candidates[0].OutputRef != "file.log" {
				t.Fatal("file-backed candidate lost its cleanup reference")
			}
		})
	}
}
