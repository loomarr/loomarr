package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/contact"
	"github.com/loomarr/loomarr/internal/recovery"
	"github.com/loomarr/loomarr/internal/store"
)

const recoveryToken = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestPasswordRecoveryRequest_OnlyEligibleLocalPersonCreatesRecord(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	seedLocal(t, st, "local", "Ada", "original-password", store.RoleMember)
	seedLocal(t, st, "disabled", "Disabled", "original-password", store.RoleMember)
	disabled, _ := st.GetUser(ctx, "disabled")
	disabled.Disabled = true
	if err := st.UpsertUser(ctx, disabled); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertUser(ctx, store.User{
		ID: "imported", Name: "Imported", Role: store.RoleMember, MediaServerLinked: true,
		PasswordHash: "$argon2id$offline", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	seedLocal(t, st, "contactless", "Contactless", "original-password", store.RoleMember)
	verifyRecoveryContact(t, st, "local", "ada@example.com")

	n := 0
	svc := NewPasswordRecoveryService(st, func() string {
		n++
		return "recovery-id"
	}, func() (string, error) { return recoveryToken, nil }, func() time.Time { return now })
	for _, username := range []string{"Unknown", "Disabled", "Imported", "Contactless"} {
		result, err := svc.Request(ctx, username)
		if err != nil || result != nil {
			t.Fatalf("Request(%q) = %+v, %v; want indistinguishable no-op", username, result, err)
		}
	}
	result, err := svc.Request(ctx, "Ada")
	if err != nil || result == nil || result.Recovery.UserID != "local" ||
		!result.Recovery.ExpiresAt.Equal(now.Add(recovery.Expiry)) {
		t.Fatalf("eligible Request = %+v, %v", result, err)
	}
	if n != 1 {
		t.Fatalf("recovery ids minted = %d, want only eligible request", n)
	}
}

func TestPasswordRecoveryGrant_RedeemsArgon2idAndCannotBeReused(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	seedLocal(t, st, "local", "Ada", "original-password", store.RoleMember)
	verifyRecoveryContact(t, st, "local", "ada@example.com")
	svc := NewPasswordRecoveryService(st, func() string { return "recovery-id" },
		func() (string, error) { return recoveryToken, nil }, func() time.Time { return now })
	request, err := svc.Request(ctx, "Ada")
	if err != nil {
		t.Fatal(err)
	}
	issued, err := svc.IssueGrant(ctx, request.Recovery.ID)
	if err != nil || issued.Plaintext != recoveryToken {
		t.Fatalf("IssueGrant = %+v, %v", issued, err)
	}
	if _, err := svc.Preview(ctx, issued.Plaintext); err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if err := svc.Redeem(ctx, issued.Plaintext, "replacement-password"); err != nil {
		t.Fatalf("Redeem: %v", err)
	}
	user, err := st.GetUser(ctx, "local")
	if err != nil || !verifyPassword(user.PasswordHash, "replacement-password") ||
		verifyPassword(user.PasswordHash, "original-password") {
		t.Fatalf("stored recovered verifier did not replace original: %+v, %v", user, err)
	}
	if err := svc.Redeem(ctx, issued.Plaintext, "attacker-password"); !errors.Is(err, ErrInvalidPasswordRecovery) {
		t.Fatalf("reused recovery = %v, want ErrInvalidPasswordRecovery", err)
	}
}

func TestPasswordRecoveryGrant_WeakPasswordDoesNotConsume(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	seedLocal(t, st, "local", "Ada", "original-password", store.RoleMember)
	verifyRecoveryContact(t, st, "local", "ada@example.com")
	svc := NewPasswordRecoveryService(st, func() string { return "recovery-id" },
		func() (string, error) { return recoveryToken, nil }, func() time.Time { return now })
	request, _ := svc.Request(ctx, "Ada")
	issued, _ := svc.IssueGrant(ctx, request.Recovery.ID)
	if err := svc.Redeem(ctx, issued.Plaintext, "short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("weak recovery password = %v, want ErrWeakPassword", err)
	}
	if _, err := svc.Preview(ctx, issued.Plaintext); err != nil {
		t.Fatalf("weak password consumed grant: %v", err)
	}
}

func verifyRecoveryContact(t *testing.T, st store.Store, userID, raw string) {
	t.Helper()
	email, normalized, err := contact.Normalize(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutPendingContactAddress(context.Background(), contact.Address{
		OwnerKind: contact.OwnerUser, OwnerID: userID, Email: email, Normalized: normalized,
		Status: contact.StatusPending, Provenance: contact.ProvenanceAdmin, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.VerifyPendingContactAddress(context.Background(), userID, normalized, now); err != nil {
		t.Fatal(err)
	}
}
