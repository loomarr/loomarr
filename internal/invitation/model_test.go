package invitation

import (
	"testing"
	"time"
)

func TestInvitationValidationPinsReservedIdentityAndRole(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	local := Invitation{
		ID: "invitation-local", Kind: KindLocal, Username: "Ada", IdentityKey: "ada",
		Role: RoleMember, Status: StatusPending, CreatedAt: now, ExpiresAt: now.Add(Expiry),
	}
	if err := local.Validate(); err != nil {
		t.Fatalf("valid local invitation: %v", err)
	}

	imported := Invitation{
		ID: "invitation-library", Kind: KindLibrary, LibraryUserID: "library-user-42",
		DisplayName: "Grace", IdentityKey: "library-user-42", Role: RoleAdmin,
		Status: StatusPending, CreatedAt: now, ExpiresAt: now.Add(Expiry),
	}
	if err := imported.Validate(); err != nil {
		t.Fatalf("valid Library invitation: %v", err)
	}

	for name, mutate := range map[string]func(*Invitation){
		"local cannot carry Library identity": func(i *Invitation) { i.LibraryUserID = "other" },
		"local key is normalized username":    func(i *Invitation) { i.IdentityKey = "Ada" },
		"unknown role":                        func(i *Invitation) { i.Role = "owner" },
		"unknown kind":                        func(i *Invitation) { i.Kind = "sso" },
		"expiry is fixed":                     func(i *Invitation) { i.ExpiresAt = now.Add(time.Hour) },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := local
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatal("expected validation failure")
			}
		})
	}
}

func TestPendingInvitationBecomesEffectivelyExpiredAtDeadline(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	invitation := Invitation{Status: StatusPending, ExpiresAt: now}
	if got := invitation.EffectiveStatus(now.Add(-time.Second)); got != StatusPending {
		t.Fatalf("before deadline = %q, want pending", got)
	}
	if got := invitation.EffectiveStatus(now); got != StatusExpired {
		t.Fatalf("at deadline = %q, want expired", got)
	}
	invitation.Status = StatusRevoked
	if got := invitation.EffectiveStatus(now.Add(time.Hour)); got != StatusRevoked {
		t.Fatalf("revoked invitation became %q", got)
	}
}

func TestInvitationGrantStoresOnlyAHashAndBoundedConveyance(t *testing.T) {
	now := time.Unix(1_900_000_000, 0)
	if got, want := HashGrant("grant-secret"), "f4e068bcabeaf80bca2c3709413c36ef78c1f7a6f40fe284aecf91ce183fa91e"; got != want {
		t.Fatalf("grant hash = %q, want known SHA-256 digest %q", got, want)
	}
	grant := Grant{
		TokenHash: HashGrant("grant-secret"), InvitationID: "invitation-1",
		Kind: GrantActivation, Conveyance: ConveyanceQR,
		CreatedAt: now, ExpiresAt: now.Add(Expiry),
	}
	if err := grant.Validate(); err != nil {
		t.Fatalf("valid grant: %v", err)
	}
	grant.Conveyance = "sms"
	if err := grant.Validate(); err == nil {
		t.Fatal("unknown conveyance was accepted")
	}
	grant.Conveyance = ConveyanceCopy
	grant.TokenHash = "plaintext-grant-secret"
	if err := grant.Validate(); err == nil {
		t.Fatal("plaintext grant was accepted as a durable hash")
	}
}
