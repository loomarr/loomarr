package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/recovery"
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
	value := recovery.Record{
		ID: "recovery-preview", UserID: "recover-local", Status: recovery.StatusPending,
		CreatedAt: now, ExpiresAt: now.Add(recovery.Expiry),
	}
	if err := s.CreatePasswordRecovery(ctx, value); err != nil {
		t.Fatal(err)
	}
	grant := recovery.Grant{
		TokenHash: recovery.HashGrant("preview-plaintext"), RecoveryID: value.ID,
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
	value := recovery.Record{
		ID: "recovery-atomic", UserID: user.ID, Status: recovery.StatusPending,
		CreatedAt: now, ExpiresAt: now.Add(recovery.Expiry),
	}
	if err := s.CreatePasswordRecovery(ctx, value); err != nil {
		t.Fatal(err)
	}
	winning := recovery.Grant{
		TokenHash: recovery.HashGrant("winning-plaintext"), RecoveryID: value.ID,
		CreatedAt: now, ExpiresAt: value.ExpiresAt,
	}
	sibling := recovery.Grant{
		TokenHash: recovery.HashGrant("sibling-plaintext"), RecoveryID: value.ID,
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
	if err != nil || got.Status != recovery.StatusRedeemed || !got.TerminalAt.Equal(redeemAt) {
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
	first := recovery.Record{
		ID: "recovery-first", UserID: "recover-replace", Status: recovery.StatusPending,
		CreatedAt: now, ExpiresAt: now.Add(recovery.Expiry),
	}
	if err := s.CreatePasswordRecovery(ctx, first); err != nil {
		t.Fatal(err)
	}
	firstGrant := recovery.Grant{
		TokenHash: recovery.HashGrant("first-plaintext"), RecoveryID: first.ID,
		CreatedAt: now, ExpiresAt: first.ExpiresAt,
	}
	if err := s.AddPasswordRecoveryGrant(ctx, first.ID, firstGrant, now); err != nil {
		t.Fatal(err)
	}

	secondAt := now.Add(time.Minute)
	second := recovery.Record{
		ID: "recovery-second", UserID: first.UserID, Status: recovery.StatusPending,
		CreatedAt: secondAt, ExpiresAt: secondAt.Add(recovery.Expiry),
	}
	if err := s.CreatePasswordRecovery(ctx, second); err != nil {
		t.Fatal(err)
	}
	old, err := s.GetPasswordRecovery(ctx, first.ID, secondAt)
	if err != nil || old.Status != recovery.StatusRevoked || !old.TerminalAt.Equal(secondAt) {
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
	oldCreated := now.Add(-recovery.Retention - recovery.Expiry - time.Hour)
	oldExpired := recovery.Record{
		ID: "recovery-old-expired", UserID: "retention-expired", Status: recovery.StatusPending,
		CreatedAt: oldCreated, ExpiresAt: oldCreated.Add(recovery.Expiry),
	}
	if err := s.CreatePasswordRecovery(ctx, oldExpired); err != nil {
		t.Fatal(err)
	}
	oldRevoked := recovery.Record{
		ID: "recovery-old-revoked", UserID: "retention-revoked", Status: recovery.StatusPending,
		CreatedAt: oldCreated, ExpiresAt: oldCreated.Add(recovery.Expiry),
	}
	if err := s.CreatePasswordRecovery(ctx, oldRevoked); err != nil {
		t.Fatal(err)
	}
	// Revoke the old row just before the retention horizon while leaving the replacement's
	// thirty-minute expiry just inside it. This pins terminal_at and expires_at independently.
	replacementAt := now.Add(-recovery.Retention - 15*time.Minute)
	replacement := recovery.Record{
		ID: "recovery-replacement", UserID: oldRevoked.UserID, Status: recovery.StatusPending,
		CreatedAt: replacementAt, ExpiresAt: replacementAt.Add(recovery.Expiry),
	}
	if err := s.CreatePasswordRecovery(ctx, replacement); err != nil {
		t.Fatal(err)
	}
	recent := recovery.Record{
		ID: "recovery-recent", UserID: "retention-recent", Status: recovery.StatusPending,
		CreatedAt: now.Add(-recovery.Expiry), ExpiresAt: now,
	}
	if err := s.CreatePasswordRecovery(ctx, recent); err != nil {
		t.Fatal(err)
	}

	purged, err := s.PurgeTerminalPasswordRecoveries(ctx, now.Add(-recovery.Retention))
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
	value := recovery.Record{
		ID: "recovery-disabled", UserID: user.ID, Status: recovery.StatusPending,
		CreatedAt: now, ExpiresAt: now.Add(recovery.Expiry),
	}
	if err := s.CreatePasswordRecovery(ctx, value); err != nil {
		t.Fatal(err)
	}
	grant := recovery.Grant{
		TokenHash: recovery.HashGrant("disabled-plaintext"), RecoveryID: value.ID,
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
	value := recovery.Record{
		ID: "recovery-concurrent", UserID: user.ID, Status: recovery.StatusPending,
		CreatedAt: now, ExpiresAt: now.Add(recovery.Expiry),
	}
	if err := s.CreatePasswordRecovery(ctx, value); err != nil {
		t.Fatal(err)
	}
	grant := recovery.Grant{
		TokenHash: recovery.HashGrant("concurrent-plaintext"), RecoveryID: value.ID,
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
