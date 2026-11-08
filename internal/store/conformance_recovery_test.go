package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/people/access"
)

func testPasswordRecoveryPreviewDoesNotConsume(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	if err := s.UpsertUser(ctx, User{
		ID: "recover-local", Name: "Ada", Role: RoleMember, PasswordHash: "$argon2id$existing",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	value := access.RecoveryRecord{
		ID: "recovery-preview", UserID: "recover-local", Status: access.RecoveryPending,
		CreatedAt: now, ExpiresAt: now.Add(access.RecoveryExpiry),
	}
	if err := s.CreatePasswordRecovery(ctx, value); err != nil {
		t.Fatal(err)
	}
	grant := access.RecoveryGrant{
		TokenHash: access.HashGrant("preview-plaintext"), RecoveryID: value.ID,
		CreatedAt: now, ExpiresAt: value.ExpiresAt,
	}
	if err := s.AddPasswordRecoveryGrant(ctx, value.ID, grant, now); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetPasswordRecoveryByGrant(ctx, grant.TokenHash, now.Add(time.Minute))
	if err != nil || got != value {
		t.Fatalf("preview recovery = %+v, %v; want %+v", got, err, value)
	}
	grants, err := s.ListPasswordRecoveryGrants(ctx, value.ID)
	if err != nil || len(grants) != 1 || !grants[0].ConsumedAt.IsZero() || !grants[0].RevokedAt.IsZero() {
		t.Fatalf("preview mutated recovery grant = %+v, %v", grants, err)
	}
}

func testPasswordRecoveryRedemptionIsAtomic(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	user := User{
		ID: "recover-atomic", Name: "Grace", Role: RoleAdmin, PasswordHash: "$argon2id$old",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.UpsertUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	for _, tokenHash := range []string{"session-one", "session-two"} {
		if err := s.CreateSession(ctx, Session{
			TokenHash: tokenHash, UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		}); err != nil {
			t.Fatal(err)
		}
	}
	value := access.RecoveryRecord{
		ID: "recovery-atomic", UserID: user.ID, Status: access.RecoveryPending,
		CreatedAt: now, ExpiresAt: now.Add(access.RecoveryExpiry),
	}
	if err := s.CreatePasswordRecovery(ctx, value); err != nil {
		t.Fatal(err)
	}
	winning := access.RecoveryGrant{
		TokenHash: access.HashGrant("winning-plaintext"), RecoveryID: value.ID,
		CreatedAt: now, ExpiresAt: value.ExpiresAt,
	}
	sibling := access.RecoveryGrant{
		TokenHash: access.HashGrant("sibling-plaintext"), RecoveryID: value.ID,
		CreatedAt: now.Add(time.Second), ExpiresAt: value.ExpiresAt,
	}
	if err := s.AddPasswordRecoveryGrant(ctx, value.ID, winning, now); err != nil {
		t.Fatal(err)
	}
	if err := s.AddPasswordRecoveryGrant(ctx, value.ID, sibling, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}

	redeemAt := now.Add(time.Minute)
	newHash := "$argon2id$v=19$m=65536,t=3,p=4$new$safeverifier"
	got, err := s.RedeemPasswordRecovery(ctx, winning.TokenHash, newHash, redeemAt)
	if err != nil || got.Status != access.RecoveryRedeemed || !got.TerminalAt.Equal(redeemAt) {
		t.Fatalf("redeem recovery = %+v, %v", got, err)
	}
	stored, err := s.GetUser(ctx, user.ID)
	if err != nil || stored.PasswordHash != newHash || !stored.UpdatedAt.Equal(redeemAt) {
		t.Fatalf("recovered person = %+v, %v", stored, err)
	}
	sessions, err := s.ListSessionsForUser(ctx, user.ID, redeemAt)
	if err != nil || len(sessions) != 0 {
		t.Fatalf("sessions survived recovery = %+v, %v", sessions, err)
	}
	grants, err := s.ListPasswordRecoveryGrants(ctx, value.ID)
	if err != nil || len(grants) != 2 || grants[0].ConsumedAt.IsZero() || grants[1].RevokedAt.IsZero() {
		t.Fatalf("recovery grant finalization = %+v, %v", grants, err)
	}
	if _, err := s.RedeemPasswordRecovery(ctx, sibling.TokenHash, "$argon2id$attacker", redeemAt.Add(time.Second)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("redeem sibling after completion = %v, want ErrNotFound", err)
	}
	stored, err = s.GetUser(ctx, user.ID)
	if err != nil || stored.PasswordHash != newHash {
		t.Fatalf("failed reuse changed verifier = %+v, %v", stored, err)
	}
}

func testPasswordRecoveryNewRequestSupersedesOld(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	if err := s.UpsertUser(ctx, User{
		ID: "recover-replace", Name: "Katherine", Role: RoleMember, PasswordHash: "$argon2id$old",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	first := access.RecoveryRecord{
		ID: "recovery-first", UserID: "recover-replace", Status: access.RecoveryPending,
		CreatedAt: now, ExpiresAt: now.Add(access.RecoveryExpiry),
	}
	if err := s.CreatePasswordRecovery(ctx, first); err != nil {
		t.Fatal(err)
	}
	firstGrant := access.RecoveryGrant{
		TokenHash: access.HashGrant("first-plaintext"), RecoveryID: first.ID,
		CreatedAt: now, ExpiresAt: first.ExpiresAt,
	}
	if err := s.AddPasswordRecoveryGrant(ctx, first.ID, firstGrant, now); err != nil {
		t.Fatal(err)
	}

	secondAt := now.Add(time.Minute)
	second := access.RecoveryRecord{
		ID: "recovery-second", UserID: first.UserID, Status: access.RecoveryPending,
		CreatedAt: secondAt, ExpiresAt: secondAt.Add(access.RecoveryExpiry),
	}
	if err := s.CreatePasswordRecovery(ctx, second); err != nil {
		t.Fatal(err)
	}
	old, err := s.GetPasswordRecovery(ctx, first.ID, secondAt)
	if err != nil || old.Status != access.RecoveryRevoked || !old.TerminalAt.Equal(secondAt) {
		t.Fatalf("superseded recovery = %+v, %v", old, err)
	}
	if _, err := s.GetPasswordRecoveryByGrant(ctx, firstGrant.TokenHash, secondAt); !errors.Is(err, ErrNotFound) {
		t.Fatalf("superseded grant preview = %v, want ErrNotFound", err)
	}
	grants, err := s.ListPasswordRecoveryGrants(ctx, first.ID)
	if err != nil || len(grants) != 1 || !grants[0].RevokedAt.Equal(secondAt) {
		t.Fatalf("superseded recovery grants = %+v, %v", grants, err)
	}
}

func testPasswordRecoveryRetention(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	seed := func(id string) {
		t.Helper()
		if err := s.UpsertUser(ctx, User{
			ID: id, Name: id, Role: RoleMember, PasswordHash: "$argon2id$existing",
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
	}
	seed("retention-expired")
	seed("retention-revoked")
	seed("retention-recent")
	oldCreated := now.Add(-access.RecoveryRetention - access.RecoveryExpiry - time.Hour)
	oldExpired := access.RecoveryRecord{
		ID: "recovery-old-expired", UserID: "retention-expired", Status: access.RecoveryPending,
		CreatedAt: oldCreated, ExpiresAt: oldCreated.Add(access.RecoveryExpiry),
	}
	if err := s.CreatePasswordRecovery(ctx, oldExpired); err != nil {
		t.Fatal(err)
	}
	oldRevoked := access.RecoveryRecord{
		ID: "recovery-old-revoked", UserID: "retention-revoked", Status: access.RecoveryPending,
		CreatedAt: oldCreated, ExpiresAt: oldCreated.Add(access.RecoveryExpiry),
	}
	if err := s.CreatePasswordRecovery(ctx, oldRevoked); err != nil {
		t.Fatal(err)
	}
	// Revoke the old row just before the retention horizon while leaving the replacement's
	// thirty-minute expiry just inside it. This pins terminal_at and expires_at independently.
	replacementAt := now.Add(-access.RecoveryRetention - 15*time.Minute)
	replacement := access.RecoveryRecord{
		ID: "recovery-replacement", UserID: oldRevoked.UserID, Status: access.RecoveryPending,
		CreatedAt: replacementAt, ExpiresAt: replacementAt.Add(access.RecoveryExpiry),
	}
	if err := s.CreatePasswordRecovery(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	recent := access.RecoveryRecord{
		ID: "recovery-recent", UserID: "retention-recent", Status: access.RecoveryPending,
		CreatedAt: now.Add(-access.RecoveryExpiry), ExpiresAt: now,
	}
	if err := s.CreatePasswordRecovery(ctx, recent); err != nil {
		t.Fatal(err)
	}

	purged, err := s.PurgeTerminalPasswordRecoveries(ctx, now.Add(-access.RecoveryRetention))
	if err != nil || purged != 2 {
		t.Fatalf("purge password recoveries = %d, %v; want two old records", purged, err)
	}
	for _, id := range []string{oldExpired.ID, oldRevoked.ID} {
		if _, err := s.GetPasswordRecovery(ctx, id, now); !errors.Is(err, ErrNotFound) {
			t.Fatalf("old password recovery %s survived: %v", id, err)
		}
	}
	for _, id := range []string{replacement.ID, recent.ID} {
		if _, err := s.GetPasswordRecovery(ctx, id, now); err != nil {
			t.Fatalf("retained password recovery %s was purged: %v", id, err)
		}
	}
}

func testPasswordRecoveryDisabledAfterIssueIsUnusable(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	user := User{
		ID: "recover-disabled", Name: "Dorothy", Role: RoleMember, PasswordHash: "$argon2id$old",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.UpsertUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	value := access.RecoveryRecord{
		ID: "recovery-disabled", UserID: user.ID, Status: access.RecoveryPending,
		CreatedAt: now, ExpiresAt: now.Add(access.RecoveryExpiry),
	}
	if err := s.CreatePasswordRecovery(ctx, value); err != nil {
		t.Fatal(err)
	}
	grant := access.RecoveryGrant{
		TokenHash: access.HashGrant("disabled-plaintext"), RecoveryID: value.ID,
		CreatedAt: now, ExpiresAt: value.ExpiresAt,
	}
	if err := s.AddPasswordRecoveryGrant(ctx, value.ID, grant, now); err != nil {
		t.Fatal(err)
	}
	user.Disabled = true
	user.UpdatedAt = now.Add(time.Minute)
	if err := s.UpsertUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPasswordRecoveryByGrant(ctx, grant.TokenHash, now.Add(time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled recovery preview = %v, want ErrNotFound", err)
	}
	if _, err := s.RedeemPasswordRecovery(ctx, grant.TokenHash, "$argon2id$new", now.Add(time.Minute)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("disabled recovery redemption = %v, want ErrNotFound", err)
	}
	stored, err := s.GetUser(ctx, user.ID)
	if err != nil || stored.PasswordHash != "$argon2id$old" {
		t.Fatalf("disabled redemption changed verifier = %+v, %v", stored, err)
	}
}

func testPasswordRecoveryConcurrentRedemption(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	user := User{
		ID: "recover-concurrent", Name: "Margaret", Role: RoleMember, PasswordHash: "$argon2id$old",
		CreatedAt: now, UpdatedAt: now,
	}
	if err := s.UpsertUser(ctx, user); err != nil {
		t.Fatal(err)
	}
	value := access.RecoveryRecord{
		ID: "recovery-concurrent", UserID: user.ID, Status: access.RecoveryPending,
		CreatedAt: now, ExpiresAt: now.Add(access.RecoveryExpiry),
	}
	if err := s.CreatePasswordRecovery(ctx, value); err != nil {
		t.Fatal(err)
	}
	grant := access.RecoveryGrant{
		TokenHash: access.HashGrant("concurrent-plaintext"), RecoveryID: value.ID,
		CreatedAt: now, ExpiresAt: value.ExpiresAt,
	}
	if err := s.AddPasswordRecoveryGrant(ctx, value.ID, grant, now); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for contender := range 2 {
		wg.Add(1)
		go func(contender int) {
			defer wg.Done()
			_, err := s.RedeemPasswordRecovery(ctx, grant.TokenHash,
				fmt.Sprintf("$argon2id$winner-%d", contender), now.Add(time.Minute))
			results <- err
		}(contender)
	}
	wg.Wait()
	close(results)
	var successes int
	for result := range results {
		if result == nil {
			successes++
		} else if !errors.Is(result, ErrNotFound) {
			t.Fatalf("concurrent recovery loser = %v, want ErrNotFound", result)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent recovery successes = %d, want exactly one", successes)
	}
	stored, err := s.GetUser(ctx, user.ID)
	if err != nil || stored.PasswordHash == "$argon2id$old" {
		t.Fatalf("concurrent recovery did not store winning verifier = %+v, %v", stored, err)
	}
}
