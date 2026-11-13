package store

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// RedeemDevicePairing consumes an approved pairing and issues the token in one step. It refuses a
// pairing that is pending, expired, unknown, already consumed or approved by a since-disabled user.
func testRedeemDevicePairing(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	for _, id := range []string{"u1", "u2"} {
		if err := s.UpsertUser(ctx, User{ID: id, Name: id, Role: RoleMember}); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []DevicePairing{
		{DeviceCodeHash: "approved", UserCode: "AAAA-AAAA", DeviceName: "Lounge TV", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{DeviceCodeHash: "pending", UserCode: "BBBB-BBBB", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{DeviceCodeHash: "expired", UserCode: "CCCC-CCCC", CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour)},
		{DeviceCodeHash: "disabled", UserCode: "DDDD-DDDD", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
	} {
		if err := s.CreateDevicePairing(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	for code, user := range map[string]string{"AAAA-AAAA": "u1", "CCCC-CCCC": "u1", "DDDD-DDDD": "u2"} {
		// The expired row cannot be approved through the store, which is the point: it stays pending.
		if _, err := s.ApproveDevicePairing(ctx, code, user, now.Add(-90*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DisableUser(ctx, "u2", now); err != nil {
		t.Fatal(err)
	}

	// Pending, expired, unknown and disabled-approver pairings issue nothing.
	for _, hash := range []string{"pending", "expired", "unknown", "disabled"} {
		if _, err := s.RedeemDevicePairing(ctx, hash, "tok-"+hash, now); !errors.Is(err, ErrNotFound) {
			t.Errorf("RedeemDevicePairing(%s) err = %v, want ErrNotFound", hash, err)
		}
	}
	if got, _ := s.ListDeviceTokensForUser(ctx, "u1"); len(got) != 0 {
		t.Fatalf("refused redeems issued %d tokens, want 0", len(got))
	}
	if _, err := s.GetDevicePairing(ctx, "pending", now); err != nil {
		t.Errorf("a pending pairing was consumed by a refused redeem: %v", err)
	}

	// An approved pairing yields the approver's token once, then is gone.
	got, err := s.RedeemDevicePairing(ctx, "approved", "tok-approved", now)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != "u1" || got.DeviceName != "Lounge TV" {
		t.Errorf("RedeemDevicePairing = %+v, want u1's Lounge TV pairing", got)
	}
	tokens, _ := s.ListDeviceTokensForUser(ctx, "u1")
	if len(tokens) != 1 || tokens[0].TokenHash != "tok-approved" || tokens[0].DeviceName != "Lounge TV" {
		t.Fatalf("tokens = %+v, want the one just issued", tokens)
	}
	if _, err := s.GetDevicePairing(ctx, "approved", now); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetDevicePairing after redeem err = %v, want ErrNotFound", err)
	}
	if _, err := s.RedeemDevicePairing(ctx, "approved", "tok-again", now); !errors.Is(err, ErrNotFound) {
		t.Errorf("second RedeemDevicePairing err = %v, want ErrNotFound", err)
	}
}

// Overlapping redeems of one approved pairing produce one token.
func testRedeemDevicePairingConcurrent(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_700_000_000, 0).UTC()
	if err := s.UpsertUser(ctx, User{ID: "u1", Name: "u1", Role: RoleMember}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateDevicePairing(ctx, DevicePairing{DeviceCodeHash: "code", UserCode: "AAAA-AAAA", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ApproveDevicePairing(ctx, "AAAA-AAAA", "u1", now); err != nil || !ok {
		t.Fatalf("ApproveDevicePairing = %v, %v", ok, err)
	}
	const racers = 8
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := range racers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = s.RedeemDevicePairing(ctx, "code", "tok-"+string(rune('a'+i)), now)
		}()
	}
	wg.Wait()
	wins := 0
	for _, err := range errs {
		switch {
		case err == nil:
			wins++
		case !errors.Is(err, ErrNotFound):
			t.Errorf("a racer failed with %v, want success or ErrNotFound", err)
		}
	}
	if wins != 1 {
		t.Errorf("%d redeems succeeded, want 1", wins)
	}
	if got, _ := s.ListDeviceTokensForUser(ctx, "u1"); len(got) != 1 {
		t.Errorf("%d tokens issued, want 1", len(got))
	}
}
