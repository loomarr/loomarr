package invitation_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/contact"
	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestServiceCreatesLocalInvitationAndReturnsBearerOnlyFromRegeneration(t *testing.T) {
	ctx := context.Background()
	repository := testkit.MigratedSQLiteStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	bearer := strings.Repeat("a", 64)
	service := invitation.NewService(
		repository, nil, sequentialIDs(), func() (string, error) { return bearer, nil },
		func() time.Time { return now },
	)

	created, err := service.Create(ctx, invitation.CreateCommand{
		Kind: invitation.KindLocal, Username: "  Ada  ", ContactEmail: "Ada@Example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Username != "Ada" || created.IdentityKey != "ada" || created.Role != invitation.RoleMember ||
		created.Status != invitation.StatusPending || !created.ExpiresAt.Equal(now.Add(invitation.Expiry)) {
		t.Fatalf("created invitation = %+v", created)
	}
	address, err := repository.GetInvitationContactAddress(ctx, created.ID)
	if err != nil || address.OwnerKind != contact.OwnerInvitation || address.Normalized != "ada@example.com" ||
		address.Status != contact.StatusPending {
		t.Fatalf("created contact = %+v, %v", address, err)
	}

	issued, err := service.Regenerate(ctx, created.ID, invitation.ConveyanceQR)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Plaintext != bearer || issued.Invitation.ID != created.ID ||
		issued.ExpiresAt != created.ExpiresAt {
		t.Fatalf("issued grant = %+v", issued)
	}
	grants, err := repository.ListInvitationGrants(ctx, created.ID)
	if err != nil || len(grants) != 1 || grants[0].TokenHash != invitation.HashGrant(bearer) {
		t.Fatalf("durable grants = %+v, %v", grants, err)
	}
	if grants[0].TokenHash == issued.Plaintext {
		t.Fatal("plaintext bearer crossed into persistence")
	}
}

func TestServicePinsAnExplicitEnabledLibraryAccount(t *testing.T) {
	ctx := context.Background()
	repository := testkit.MigratedSQLiteStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	resolver := testkit.LibraryAccountResolver{Accounts: map[string]invitation.LibraryAccount{
		"library-42": {ID: "library-42", Name: "Grace"},
		"disabled":   {ID: "disabled", Name: "Disabled", Disabled: true},
	}}
	service := invitation.NewService(
		repository, resolver, sequentialIDs(), nil, func() time.Time { return now },
	)
	created, err := service.Create(ctx, invitation.CreateCommand{
		Kind: invitation.KindLibrary, LibraryUserID: "library-42", Role: invitation.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.LibraryUserID != "library-42" || created.IdentityKey != "library-42" ||
		created.DisplayName != "Grace" || created.Role != invitation.RoleAdmin {
		t.Fatalf("created Library invitation = %+v", created)
	}
	if _, err := service.Create(ctx, invitation.CreateCommand{
		Kind: invitation.KindLibrary, LibraryUserID: "unknown",
	}); err == nil {
		t.Fatal("an account absent from the Library was invited")
	}
	if _, err := service.Create(ctx, invitation.CreateCommand{
		Kind: invitation.KindLibrary, LibraryUserID: "disabled",
	}); err == nil {
		t.Fatal("a disabled Library account was invited")
	}
	values, err := repository.ListInvitations(ctx, now)
	if err != nil || len(values) != 1 {
		t.Fatalf("failed Library selections persisted reservations: %+v, %v", values, err)
	}
}

func TestServiceAddsSiblingDeliveryGrantAndRevokesTheAdmissionDecision(t *testing.T) {
	ctx := context.Background()
	repository := testkit.MigratedSQLiteStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	tokens := []string{strings.Repeat("a", 64), strings.Repeat("b", 64)}
	service := invitation.NewService(
		repository, nil, sequentialIDs(), func() (string, error) {
			token := tokens[0]
			tokens = tokens[1:]
			return token, nil
		}, func() time.Time { return now },
	)
	created, err := service.Create(ctx, invitation.CreateCommand{Kind: invitation.KindLocal, Username: "Alan"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Regenerate(ctx, created.ID, invitation.ConveyanceCopy); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	if _, err := service.IssueSibling(ctx, created.ID, invitation.ConveyanceEmail); err != nil {
		t.Fatal(err)
	}
	grants, err := repository.ListInvitationGrants(ctx, created.ID)
	if err != nil || len(grants) != 2 || !grants[0].RevokedAt.IsZero() || !grants[1].RevokedAt.IsZero() {
		t.Fatalf("sibling delivery grants = %+v, %v", grants, err)
	}

	now = now.Add(time.Minute)
	if err := service.Revoke(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	listed, err := service.List(ctx)
	if err != nil || len(listed) != 1 || listed[0].Status != invitation.StatusRevoked {
		t.Fatalf("listed revoked invitation = %+v, %v", listed, err)
	}
	if _, err := service.Regenerate(ctx, created.ID, invitation.ConveyanceQR); err == nil {
		t.Fatal("revoked invitation issued another grant")
	}
}

func TestServiceRevokesOneIssuedGrantAfterDefiniteDeliveryFailure(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	repository := testkit.MigratedSQLiteStore(t)
	service := invitation.NewService(repository, nil, sequentialIDs(),
		func() (string, error) { return strings.Repeat("ab", 32), nil }, func() time.Time { return now })
	created, err := service.Create(t.Context(), invitation.CreateCommand{Kind: invitation.KindLocal, Username: "Ada"})
	if err != nil {
		t.Fatal(err)
	}
	issued, err := service.IssueSibling(t.Context(), created.ID, invitation.ConveyanceEmail)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeIssuedGrant(t.Context(), issued.Plaintext); err != nil {
		t.Fatal(err)
	}
	grants, err := repository.ListInvitationGrants(t.Context(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(grants) != 1 || grants[0].RevokedAt.IsZero() || grants[0].TokenHash != invitation.HashGrant(issued.Plaintext) {
		t.Fatalf("revoked grant = %+v", grants)
	}
}

func sequentialIDs() func() string {
	n := 0
	return func() string {
		n++
		return "invitation-id"
	}
}
