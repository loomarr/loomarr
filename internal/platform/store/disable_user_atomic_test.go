package store

import (
	"context"
	"testing"
	"time"
)

// TestDisableUserIsAtomic proves the flag and the revocations commit together (#1861). A delete that
// fails partway leaves the user enabled with every credential intact, never disabled with leftovers
// that come back when an admin re-enables them. A trigger makes the last delete fail.
func TestDisableUserIsAtomic(t *testing.T) {
	t.Parallel()
	s := newSQLiteStore(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	if err := s.UpsertUser(ctx, User{ID: "u1", Name: "Ada", Role: RoleMember}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSession(ctx, Session{TokenHash: "s1", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateDeviceToken(ctx, DeviceToken{TokenHash: "d1", UserID: "u1", DeviceName: "TV", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateDevicePairing(ctx, DevicePairing{DeviceCodeHash: "p1", UserCode: "AAAA-AAAA", CreatedAt: now, ExpiresAt: now.Add(10 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ApproveDevicePairing(ctx, "AAAA-AAAA", "u1", now); err != nil || !ok {
		t.Fatalf("ApproveDevicePairing = %v, %v", ok, err)
	}
	if _, err := HandleOf(s).ExecContext(ctx,
		`CREATE TRIGGER fail_session_delete BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}

	if err := s.DisableUser(ctx, "u1", now); err == nil {
		t.Fatal("DisableUser succeeded with a failing session delete")
	}

	if u, err := s.GetUser(ctx, "u1"); err != nil || u.Disabled {
		t.Errorf("after a failed DisableUser: u1 = %+v (err %v), want still enabled", u, err)
	}
	if _, err := s.GetDeviceToken(ctx, "d1"); err != nil {
		t.Errorf("a failed DisableUser deleted the device token: %v", err)
	}
	if _, err := s.GetDevicePairing(ctx, "p1", now); err != nil {
		t.Errorf("a failed DisableUser deleted the pairing: %v", err)
	}
	if _, err := s.GetSession(ctx, "s1", now); err != nil {
		t.Errorf("a failed DisableUser deleted the session: %v", err)
	}
}
