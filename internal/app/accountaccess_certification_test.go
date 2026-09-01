package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/recovery"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

// TestAccountAccessLifecycleCertification follows one approved person across the module seams that
// slice-level tests cannot join: administrator invitation, deterministic email delivery, atomic
// activation, verified-contact transfer, later recovery delivery, credential replacement, and
// session revocation. Provider activation and QR/copy presentation remain independently certified
// by their shared adapter and browser suites; this test pins the durable account lifecycle they join.
func TestAccountAccessLifecycleCertification(t *testing.T) {
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	now := time.Date(2030, time.March, 17, 12, 0, 0, 0, time.UTC)
	const (
		invitationBearer  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		recoveryBearer    = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		initialPassword   = "correct horse battery staple"
		recoveredPassword = "a new correct horse battery staple"
	)

	invitationService := invitation.NewService(
		st,
		nil,
		func() string { return "certification-invitation" },
		func() (string, error) { return invitationBearer, nil },
		func() time.Time { return now },
	)
	recoveryService := auth.NewPasswordRecoveryService(
		st,
		func() string { return "certification-recovery" },
		func() (string, error) { return recoveryBearer, nil },
		func() time.Time { return now },
	)
	config := func() notifications.EmailConfig {
		return notifications.EmailConfig{
			Enabled: true, Host: "smtp.example.com", Port: 587,
			Security: notifications.EmailSecuritySTARTTLS, FromAddress: "loomarr@example.com",
		}
	}
	sender := &sequenceEmailSender{results: []notifications.EmailTransmission{
		{State: notifications.EmailAccepted, ProviderMessageID: "certification-invitation-message"},
		{State: notifications.EmailAccepted, ProviderMessageID: "certification-recovery-message"},
	}}
	adapter := notifications.NewEmailAdapter(config, invitationEmailMaterializer{
		invitations: invitationService,
		recovery:    recoveryService,
		publicURL:   func() string { return "https://loomarr.example" },
	}, sender)
	sequence := 0
	notificationService := notifications.NewService(notificationRepositoryForTest(t, st), invitationEmailRouter{
		invitations: invitationService,
		recovery:    recoveryService,
		config:      config,
	}, []notifications.Adapter{adapter}, func() string {
		sequence++
		return fmt.Sprintf("certification-notification-%d", sequence)
	}, func() time.Time { return now })
	delivery := &invitationDeliveryCoordinator{
		invitations: invitationService, notifications: notificationService,
	}
	recoveryCoordinator := &passwordRecoveryCoordinator{
		recovery: recoveryService, notifications: notificationService,
	}

	invited, err := invitationService.Create(ctx, invitation.CreateCommand{
		Kind: invitation.KindLocal, Username: "Ada", ContactEmail: "ada@example.com",
		Role: invitation.RoleAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := delivery.SendEmail(ctx, invited.ID, "certification-send"); err != nil {
		t.Fatal(err)
	}
	if ran, err := notificationService.RunOne(ctx, "certification-worker"); err != nil || !ran {
		t.Fatalf("invitation delivery = %t, %v", ran, err)
	}

	mgr := auth.NewManager(st, time.Hour, func() time.Time { return now })
	redemption := auth.NewInvitationRedemptionService(
		st, nil, mgr, func() string { return "certification-user" }, func() time.Time { return now },
	)
	activationToken, _, activated, err := redemption.RedeemLocal(ctx, invitationBearer, initialPassword)
	if err != nil {
		t.Fatal(err)
	}
	if activated.Role != store.RoleAdmin || activated.MediaServerLinked ||
		!strings.HasPrefix(activated.PasswordHash, "$argon2id$") {
		t.Fatalf("activated account did not preserve local admin credentials: %+v", activated)
	}
	if _, err := mgr.Resolve(ctx, activationToken); err != nil {
		t.Fatalf("activation session is unusable before recovery: %v", err)
	}
	addresses, err := st.GetContactAddresses(ctx, activated.ID)
	if err != nil || addresses.Verified == nil || addresses.Verified.Email != "ada@example.com" {
		t.Fatalf("emailed invitation did not transfer a verified recovery contact: %+v, %v", addresses, err)
	}

	now = now.Add(time.Minute)
	if err := recoveryCoordinator.Request(ctx, activated.Name, "127.0.0.1|ada"); err != nil {
		t.Fatal(err)
	}
	if ran, err := notificationService.RunOne(ctx, "certification-worker"); err != nil || !ran {
		t.Fatalf("recovery delivery = %t, %v", ran, err)
	}
	if err := recoveryCoordinator.Redeem(ctx, recoveryBearer, recoveredPassword, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Resolve(ctx, activationToken); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("activation session survived recovery: %v", err)
	}

	login := auth.NewLoginService(nil, st, mgr, nil, func() time.Time { return now })
	if _, _, _, err := login.Login(ctx, activated.Name, initialPassword, "old-password"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("old credential still authenticates after recovery: %v", err)
	}
	if _, _, recovered, err := login.Login(ctx, activated.Name, recoveredPassword, "new-password"); err != nil || recovered.Role != store.RoleAdmin {
		t.Fatalf("recovered credential or role = %+v, %v", recovered, err)
	}

	invitationGrants, err := st.ListInvitationGrants(ctx, invited.ID)
	if err != nil || len(invitationGrants) != 1 || invitationGrants[0].ConsumedAt.IsZero() {
		t.Fatalf("invitation grant lifecycle = %+v, %v", invitationGrants, err)
	}
	recoveryGrants, err := st.ListPasswordRecoveryGrants(ctx, "certification-recovery")
	if err != nil || len(recoveryGrants) != 1 || recoveryGrants[0].ConsumedAt.IsZero() {
		t.Fatalf("recovery grant lifecycle = %+v, %v", recoveryGrants, err)
	}
	invitationIntents, err := st.ListNotificationIntentsByReference(
		ctx, notifications.ReferenceInvitation, invited.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	recoveryIntents, err := st.ListNotificationIntentsByReference(
		ctx, notifications.ReferenceRecovery, "certification-recovery",
	)
	if err != nil {
		t.Fatal(err)
	}
	durable := fmt.Sprintf("%+v %+v %+v %+v", invitationIntents, recoveryIntents,
		invitationGrants, recoveryGrants)
	for _, secret := range []string{invitationBearer, recoveryBearer, initialPassword, recoveredPassword, "#grant="} {
		if strings.Contains(durable, secret) {
			t.Fatalf("secret %q crossed a durable boundary: %s", secret, durable)
		}
	}
	if len(sender.messages) != 2 ||
		!strings.Contains(sender.messages[0].TextBody, "/join#grant="+invitationBearer) ||
		!strings.Contains(sender.messages[1].TextBody, "/reset-password#grant="+recoveryBearer) {
		t.Fatalf("deterministic account messages = %+v", sender.messages)
	}
	if recoveryGrants[0].TokenHash != recovery.HashGrant(recoveryBearer) {
		t.Fatalf("recovery persisted anything other than the expected one-way hash: %+v", recoveryGrants[0])
	}
}
