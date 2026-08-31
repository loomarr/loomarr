package store

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/contact"
	"github.com/loomarr/loomarr/internal/invitation"
)

func testInvitationReserveAndList(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	if err := s.UpsertUser(ctx, User{
		ID: "existing-local", Name: "Ada", Role: RoleMember, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	conflict := localInvitationFixture("conflict", "ada", now)
	if err := s.CreateInvitation(ctx, conflict, nil); !errors.Is(err, ErrInvitationIdentityConflict) {
		t.Fatalf("reserve existing local identity = %v, want ErrInvitationIdentityConflict", err)
	}

	created := localInvitationFixture("grace", "Grace", now)
	if err := s.CreateInvitation(ctx, created, nil); err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	got, err := s.GetInvitation(ctx, created.ID, now)
	if err != nil || got != created {
		t.Fatalf("get invitation = %+v, %v; want %+v", got, err, created)
	}
	duplicate := localInvitationFixture("another", "GRACE", now)
	if err := s.CreateInvitation(ctx, duplicate, nil); !errors.Is(err, ErrInvitationIdentityConflict) {
		t.Fatalf("duplicate pending identity = %v, want ErrInvitationIdentityConflict", err)
	}

	values, err := s.ListInvitations(ctx, now)
	if err != nil || len(values) != 1 || values[0].ID != created.ID {
		t.Fatalf("list invitations = %+v, %v", values, err)
	}
	if _, err := s.GetInvitation(ctx, "missing", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing invitation = %v, want ErrNotFound", err)
	}
}

func testInvitationContactIsAtomicAndGloballyUnique(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	if err := s.UpsertUser(ctx, User{
		ID: "existing", Name: "Existing", Role: RoleMember, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	email, normalized, err := contact.Normalize("shared@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.PutPendingContactAddress(ctx, contact.Address{
		OwnerKind: contact.OwnerUser, OwnerID: "existing", Email: email, Normalized: normalized,
		Status: contact.StatusPending, Provenance: contact.ProvenanceAdmin, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	conflict := localInvitationFixture("contact-conflict", "Grace", now)
	conflictAddress := &contact.Address{
		OwnerKind: contact.OwnerInvitation, OwnerID: conflict.ID, Email: email, Normalized: normalized,
		Status: contact.StatusPending, Provenance: contact.ProvenanceAdmin, CreatedAt: now,
	}
	if err := s.CreateInvitation(ctx, conflict, conflictAddress); !errors.Is(err, ErrContactAddressConflict) {
		t.Fatalf("duplicate invitation contact = %v, want ErrContactAddressConflict", err)
	}
	if _, err := s.GetInvitation(ctx, conflict.ID, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("contact failure left an invitation reservation: %v", err)
	}

	created := localInvitationFixture("with-contact", "Katherine", now)
	email, normalized, err = contact.Normalize("katherine@example.com")
	if err != nil {
		t.Fatal(err)
	}
	address := &contact.Address{
		OwnerKind: contact.OwnerInvitation, OwnerID: created.ID, Email: email, Normalized: normalized,
		Status: contact.StatusPending, Provenance: contact.ProvenanceAdmin, CreatedAt: now,
	}
	if err := s.CreateInvitation(ctx, created, address); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetInvitationContactAddress(ctx, created.ID)
	if err != nil || got != *address {
		t.Fatalf("invitation contact = %+v, %v; want %+v", got, err, *address)
	}
}

func testInvitationRegenerateAndRevoke(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	value := localInvitationFixture("grants", "Dorothy", now)
	if err := s.CreateInvitation(ctx, value, nil); err != nil {
		t.Fatal(err)
	}
	first := invitation.Grant{
		TokenHash: invitation.HashGrant("first-plaintext"), InvitationID: value.ID,
		Kind: invitation.GrantActivation, Conveyance: invitation.ConveyanceQR,
		CreatedAt: now, ExpiresAt: now.Add(invitation.Expiry),
	}
	if err := s.ReplaceInvitationGrant(ctx, value.ID, first, now); err != nil {
		t.Fatal(err)
	}
	secondAt := now.Add(time.Minute)
	second := invitation.Grant{
		TokenHash: invitation.HashGrant("second-plaintext"), InvitationID: value.ID,
		Kind: invitation.GrantActivation, Conveyance: invitation.ConveyanceCopy,
		CreatedAt: secondAt, ExpiresAt: value.ExpiresAt,
	}
	if err := s.ReplaceInvitationGrant(ctx, value.ID, second, secondAt); err != nil {
		t.Fatal(err)
	}
	grants, err := s.ListInvitationGrants(ctx, value.ID)
	if err != nil || len(grants) != 2 {
		t.Fatalf("grants after regeneration = %+v, %v", grants, err)
	}
	if !grants[0].RevokedAt.Equal(secondAt) || !grants[1].RevokedAt.IsZero() {
		t.Fatalf("regeneration did not revoke only the superseded grant: %+v", grants)
	}

	revokedAt := now.Add(2 * time.Minute)
	if err := s.RevokeInvitation(ctx, value.ID, revokedAt); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetInvitation(ctx, value.ID, revokedAt)
	if err != nil || got.Status != invitation.StatusRevoked || !got.TerminalAt.Equal(revokedAt) {
		t.Fatalf("revoked invitation = %+v, %v", got, err)
	}
	grants, err = s.ListInvitationGrants(ctx, value.ID)
	if err != nil || grants[1].RevokedAt.IsZero() {
		t.Fatalf("revoke did not invalidate every outstanding grant: %+v, %v", grants, err)
	}
	third := invitation.Grant{
		TokenHash: invitation.HashGrant("third-plaintext"), InvitationID: value.ID,
		Kind: invitation.GrantActivation, Conveyance: invitation.ConveyanceCopy,
		CreatedAt: revokedAt, ExpiresAt: value.ExpiresAt,
	}
	if err := s.ReplaceInvitationGrant(ctx, value.ID, third, revokedAt); !errors.Is(err, ErrNotFound) {
		t.Fatalf("grant for revoked invitation = %v, want ErrNotFound", err)
	}
}

func testInvitationConcurrentRedemption(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	value := localInvitationFixture("redeem", "Mary", now)
	email, normalized, err := contact.Normalize("mary@example.com")
	if err != nil {
		t.Fatal(err)
	}
	address := &contact.Address{
		OwnerKind: contact.OwnerInvitation, OwnerID: value.ID, Email: email, Normalized: normalized,
		Status: contact.StatusPending, Provenance: contact.ProvenanceAdmin, CreatedAt: now,
	}
	if err := s.CreateInvitation(ctx, value, address); err != nil {
		t.Fatal(err)
	}
	grant := invitation.Grant{
		TokenHash: invitation.HashGrant("emailed-plaintext"), InvitationID: value.ID,
		Kind: invitation.GrantActivation, Conveyance: invitation.ConveyanceEmail,
		CreatedAt: now, ExpiresAt: value.ExpiresAt,
	}
	if err := s.ReplaceInvitationGrant(ctx, value.ID, grant, now); err != nil {
		t.Fatal(err)
	}

	user := User{
		ID: "redeemed-user", Name: "Mary", Role: RoleMember, PasswordHash: "$argon2id$prepared",
		CreatedAt: now.Add(time.Minute), UpdatedAt: now.Add(time.Minute),
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for contender := 0; contender < 2; contender++ {
		wg.Add(1)
		go func(contender int) {
			defer wg.Done()
			_, err := s.RedeemInvitation(ctx, grant.TokenHash, user, Session{
				TokenHash: fmt.Sprintf("session-hash-%d", contender), UserID: user.ID,
				CreatedAt: now.Add(time.Minute), ExpiresAt: now.Add(31 * 24 * time.Hour),
			}, now.Add(time.Minute))
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
			t.Fatalf("redemption loser = %v, want safe ErrNotFound", result)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent redemption successes = %d, want exactly 1", successes)
	}

	storedUser, err := s.GetUser(ctx, user.ID)
	if err != nil || storedUser.Role != RoleMember || storedUser.PasswordHash != user.PasswordHash {
		t.Fatalf("redeemed user = %+v, %v", storedUser, err)
	}
	sessions, err := s.ListSessionsForUser(ctx, user.ID, now.Add(time.Minute))
	if err != nil || len(sessions) != 1 {
		t.Fatalf("redeemed sessions = %+v, %v", sessions, err)
	}
	contacts, err := s.GetContactAddresses(ctx, user.ID)
	if err != nil || contacts.Verified == nil || contacts.Verified.Normalized != normalized || contacts.Pending != nil {
		t.Fatalf("email-conveyed redemption contact = %+v, %v", contacts, err)
	}
	grants, err := s.ListInvitationGrants(ctx, value.ID)
	if err != nil || len(grants) != 1 || grants[0].ConsumedAt.IsZero() || !grants[0].RevokedAt.IsZero() {
		t.Fatalf("winning grant = %+v, %v", grants, err)
	}
	got, err := s.GetInvitation(ctx, value.ID, now.Add(time.Minute))
	if err != nil || got.Status != invitation.StatusRedeemed || got.RedeemedBy != user.ID {
		t.Fatalf("redeemed invitation = %+v, %v", got, err)
	}
}

func testInvitationSiblingGrantLifecycle(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	value := localInvitationFixture("siblings", "Frances", now)
	if err := s.CreateInvitation(ctx, value, nil); err != nil {
		t.Fatal(err)
	}
	copyGrant := invitation.Grant{
		TokenHash: invitation.HashGrant("copied-grant"), InvitationID: value.ID,
		Kind: invitation.GrantActivation, Conveyance: invitation.ConveyanceCopy,
		CreatedAt: now, ExpiresAt: value.ExpiresAt,
	}
	if err := s.ReplaceInvitationGrant(ctx, value.ID, copyGrant, now); err != nil {
		t.Fatal(err)
	}
	emailAt := now.Add(time.Minute)
	emailGrant := invitation.Grant{
		TokenHash: invitation.HashGrant("email-grant"), InvitationID: value.ID,
		Kind: invitation.GrantActivation, Conveyance: invitation.ConveyanceEmail,
		CreatedAt: emailAt, ExpiresAt: value.ExpiresAt,
	}
	if err := s.AddInvitationGrant(ctx, value.ID, emailGrant, emailAt); err != nil {
		t.Fatal(err)
	}
	grants, err := s.ListInvitationGrants(ctx, value.ID)
	if err != nil || len(grants) != 2 || !grants[0].RevokedAt.IsZero() || !grants[1].RevokedAt.IsZero() {
		t.Fatalf("explicit sibling grant replaced existing delivery: %+v, %v", grants, err)
	}

	revokedAt := now.Add(2 * time.Minute)
	if err := s.RevokeInvitationGrant(ctx, emailGrant.TokenHash, revokedAt); err != nil {
		t.Fatal(err)
	}
	grants, err = s.ListInvitationGrants(ctx, value.ID)
	if err != nil || !grants[0].RevokedAt.IsZero() || !grants[1].RevokedAt.Equal(revokedAt) {
		t.Fatalf("single-grant revocation affected a sibling: %+v, %v", grants, err)
	}
}

func testInvitationRetention(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	oldCreated := now.Add(-invitation.Retention - invitation.Expiry - time.Hour)
	oldExpired := localInvitationFixture("old-expired", "Old Expired", oldCreated)
	if err := s.CreateInvitation(ctx, oldExpired, nil); err != nil {
		t.Fatal(err)
	}
	oldRevoked := localInvitationFixture("old-revoked", "Old Revoked", oldCreated)
	if err := s.CreateInvitation(ctx, oldRevoked, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.RevokeInvitation(ctx, oldRevoked.ID, oldCreated.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	recentExpired := localInvitationFixture("recent-expired", "Recent Expired", now.Add(-invitation.Expiry))
	if err := s.CreateInvitation(ctx, recentExpired, nil); err != nil {
		t.Fatal(err)
	}
	active := localInvitationFixture("active", "Active", now)
	if err := s.CreateInvitation(ctx, active, nil); err != nil {
		t.Fatal(err)
	}

	purged, err := s.PurgeTerminalInvitations(ctx, now.Add(-invitation.Retention))
	if err != nil || purged != 2 {
		t.Fatalf("purge invitations = %d, %v; want two old terminal decisions", purged, err)
	}
	for _, id := range []string{oldExpired.ID, oldRevoked.ID} {
		if _, err := s.GetInvitation(ctx, id, now); !errors.Is(err, ErrNotFound) {
			t.Fatalf("old terminal invitation %s survived: %v", id, err)
		}
	}
	for _, id := range []string{recentExpired.ID, active.ID} {
		if _, err := s.GetInvitation(ctx, id, now); err != nil {
			t.Fatalf("retained invitation %s was purged: %v", id, err)
		}
	}
}

func localInvitationFixture(id, username string, now time.Time) invitation.Invitation {
	return invitation.Invitation{
		ID: "invitation-" + id, Kind: invitation.KindLocal, Username: username,
		IdentityKey: invitation.NormalizeLocalIdentity(username), Role: invitation.RoleMember,
		Status: invitation.StatusPending, CreatedAt: now, ExpiresAt: now.Add(invitation.Expiry),
	}
}
