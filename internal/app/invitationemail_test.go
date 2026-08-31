package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/contact"
	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/recovery"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestInvitationEmailMintsBearerOnlyInsideClaimedAttemptsAndRotatesItBeforeRetry(t *testing.T) {
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	tokens := []string{strings.Repeat("a", 64), strings.Repeat("b", 64)}
	tokenIndex := 0
	invitationService := invitation.NewService(
		st, nil, func() string { return "invitation-1" },
		func() (string, error) {
			token := tokens[tokenIndex]
			tokenIndex++
			return token, nil
		},
		func() time.Time { return now },
	)
	if _, err := invitationService.Create(ctx, invitation.CreateCommand{
		Kind: invitation.KindLocal, Username: "Ada", ContactEmail: "ada@example.com",
	}); err != nil {
		t.Fatal(err)
	}
	config := func() notifications.EmailConfig {
		return notifications.EmailConfig{
			Enabled: true, Host: "smtp.example.com", Port: 587,
			Security: notifications.EmailSecuritySTARTTLS, FromAddress: "loomarr@example.com",
		}
	}
	sender := &sequenceEmailSender{results: []notifications.EmailTransmission{
		{State: notifications.EmailTransientPreAcceptance},
		{State: notifications.EmailAccepted, ProviderMessageID: "safe-message-42"},
	}}
	adapter := notifications.NewEmailAdapter(config, invitationEmailMaterializer{
		invitations: invitationService, publicURL: func() string { return "https://loomarr.example" },
	}, sender)
	ids := 0
	notificationService := notifications.NewService(st, invitationEmailRouter{
		invitations: invitationService, config: config,
	}, []notifications.Adapter{adapter}, func() string {
		ids++
		return fmt.Sprintf("notification-%d", ids)
	}, func() time.Time { return now })
	coordinator := &invitationDeliveryCoordinator{
		invitations: invitationService, notifications: notificationService,
	}

	queued, err := coordinator.SendEmail(ctx, "invitation-1", "operator-action-42")
	if err != nil || queued.Status != notifications.StatusQueued {
		t.Fatalf("send = %+v, %v", queued, err)
	}
	if _, err := coordinator.SendEmail(ctx, "invitation-1", "operator-action-42"); err != nil {
		t.Fatal(err)
	}
	intents, err := st.ListNotificationIntentsByReference(ctx, notifications.ReferenceInvitation, "invitation-1")
	if err != nil || len(intents) != 1 {
		t.Fatalf("idempotent intents = %+v, %v", intents, err)
	}
	grants, err := st.ListInvitationGrants(ctx, "invitation-1")
	if err != nil || len(grants) != 0 {
		t.Fatalf("grant existed before worker claim = %+v, %v", grants, err)
	}

	if ran, err := notificationService.RunOne(ctx, "worker-1"); err != nil || !ran {
		t.Fatalf("first run = %t, %v", ran, err)
	}
	grants, err = st.ListInvitationGrants(ctx, "invitation-1")
	if err != nil || len(grants) != 1 || grants[0].RevokedAt.IsZero() ||
		grants[0].TokenHash != invitation.HashGrant(tokens[0]) {
		t.Fatalf("pre-acceptance grant = %+v, %v", grants, err)
	}
	attempts, err := st.ListNotificationAttempts(ctx, intents[0].ID)
	if err != nil || len(attempts) != 2 || attempts[1].Status != notifications.StatusQueued {
		t.Fatalf("retry attempts = %+v, %v", attempts, err)
	}
	now = attempts[1].AvailableAt
	if ran, err := notificationService.RunOne(ctx, "worker-1"); err != nil || !ran {
		t.Fatalf("second run = %t, %v", ran, err)
	}
	grants, err = st.ListInvitationGrants(ctx, "invitation-1")
	if err != nil || len(grants) != 2 || grants[1].TokenHash != invitation.HashGrant(tokens[1]) ||
		!grants[1].RevokedAt.IsZero() {
		t.Fatalf("accepted grant = %+v, %v", grants, err)
	}
	delivered, err := coordinator.LatestEmail(ctx, "invitation-1")
	if err != nil || delivered.Status != notifications.StatusDelivered || delivered.AttemptNumber != 2 {
		t.Fatalf("delivered = %+v, %v", delivered, err)
	}
	if len(sender.messages) != 2 || !strings.Contains(sender.messages[0].TextBody, tokens[0]) ||
		!strings.Contains(sender.messages[1].HTMLBody, tokens[1]) {
		t.Fatalf("materialized messages did not carry their one in-memory grants")
	}
	durable := fmt.Sprintf("%+v %+v", intents, attempts)
	if strings.Contains(durable, tokens[0]) || strings.Contains(durable, tokens[1]) || strings.Contains(durable, "#grant=") {
		t.Fatalf("plaintext bearer crossed durable notification boundary: %s", durable)
	}
}

type failedRecoveryNotifier struct{ err error }

func (n failedRecoveryNotifier) Publish(
	context.Context,
	notifications.PublishCommand,
) (notifications.Intent, bool, error) {
	return notifications.Intent{}, false, n.err
}

func TestPasswordRecoveryRequestDoesNotEnumerateNotificationQueueFailure(t *testing.T) {
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	if err := st.UpsertUser(ctx, store.User{
		ID: "eligible", Name: "Eligible", Role: store.RoleMember, PasswordHash: "$argon2id$existing",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	email, normalized, err := contact.Normalize("eligible@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutPendingContactAddress(ctx, contact.Address{
		OwnerKind: contact.OwnerUser, OwnerID: "eligible", Email: email, Normalized: normalized,
		Status: contact.StatusPending, Provenance: contact.ProvenanceAdmin, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.VerifyPendingContactAddress(ctx, "eligible", normalized, now); err != nil {
		t.Fatal(err)
	}
	recoveryService := auth.NewPasswordRecoveryService(st, func() string { return "recovery-enumeration" },
		func() (string, error) { return strings.Repeat("e", 64), nil }, func() time.Time { return now })
	coordinator := &passwordRecoveryCoordinator{
		recovery:      recoveryService,
		notifications: failedRecoveryNotifier{err: errors.New("notification store unavailable")},
	}
	if err := coordinator.Request(ctx, "Eligible", "ip|eligible"); err != nil {
		t.Fatalf("eligible queue failure escaped public response: %v", err)
	}
	if err := coordinator.Request(ctx, "Unknown", "ip|unknown"); err != nil {
		t.Fatalf("unknown request response differs: %v", err)
	}
}

func TestPasswordRecoveryEmailUsesSharedWorkerAndKeepsBearerEphemeral(t *testing.T) {
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	now := time.Unix(1_900_000_000, 0).UTC()
	if err := st.UpsertUser(ctx, store.User{
		ID: "person-1", Name: "Ada", Role: store.RoleMember, PasswordHash: "$argon2id$existing",
		CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	email, normalized, err := contact.Normalize("ada@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutPendingContactAddress(ctx, contact.Address{
		OwnerKind: contact.OwnerUser, OwnerID: "person-1", Email: email, Normalized: normalized,
		Status: contact.StatusPending, Provenance: contact.ProvenanceAdmin, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.VerifyPendingContactAddress(ctx, "person-1", normalized, now); err != nil {
		t.Fatal(err)
	}
	const bearer = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	recoveryService := auth.NewPasswordRecoveryService(st, func() string { return "recovery-1" },
		func() (string, error) { return bearer, nil }, func() time.Time { return now })
	config := func() notifications.EmailConfig {
		return notifications.EmailConfig{
			Enabled: true, Host: "smtp.example.com", Port: 587,
			Security: notifications.EmailSecuritySTARTTLS, FromAddress: "loomarr@example.com",
		}
	}
	sender := &sequenceEmailSender{results: []notifications.EmailTransmission{{
		State: notifications.EmailAccepted, ProviderMessageID: "recovery-message-1",
	}}}
	adapter := notifications.NewEmailAdapter(config, invitationEmailMaterializer{
		recovery: recoveryService, publicURL: func() string { return "https://loomarr.example" },
	}, sender)
	ids := 0
	notificationService := notifications.NewService(st, invitationEmailRouter{
		recovery: recoveryService, config: config,
	}, []notifications.Adapter{adapter}, func() string {
		ids++
		return fmt.Sprintf("recovery-notification-%d", ids)
	}, func() time.Time { return now })
	coordinator := &passwordRecoveryCoordinator{recovery: recoveryService, notifications: notificationService}
	if err := coordinator.Request(ctx, "Ada", "127.0.0.1|ada"); err != nil {
		t.Fatal(err)
	}
	intents, err := st.ListNotificationIntentsByReference(ctx, notifications.ReferenceRecovery, "recovery-1")
	if err != nil || len(intents) != 1 || intents[0].Topic != notifications.TopicLocalPasswordRecovery {
		t.Fatalf("recovery intents = %+v, %v", intents, err)
	}
	grants, err := st.ListPasswordRecoveryGrants(ctx, "recovery-1")
	if err != nil || len(grants) != 0 {
		t.Fatalf("recovery grant existed before worker claim = %+v, %v", grants, err)
	}
	if ran, err := notificationService.RunOne(ctx, "worker-1"); err != nil || !ran {
		t.Fatalf("recovery delivery = %t, %v", ran, err)
	}
	grants, err = st.ListPasswordRecoveryGrants(ctx, "recovery-1")
	if err != nil || len(grants) != 1 || grants[0].TokenHash != recovery.HashGrant(bearer) {
		t.Fatalf("recovery grants = %+v, %v", grants, err)
	}
	if len(sender.messages) != 1 || !strings.Contains(sender.messages[0].TextBody,
		"https://loomarr.example/reset-password#grant="+bearer) {
		t.Fatalf("recovery message = %+v", sender.messages)
	}
	durable := fmt.Sprintf("%+v", intents)
	if strings.Contains(durable, bearer) || strings.Contains(durable, "#grant=") {
		t.Fatalf("recovery bearer crossed durable notification boundary: %s", durable)
	}
}

type sequenceEmailSender struct {
	results  []notifications.EmailTransmission
	messages []notifications.EmailMessage
}

func (s *sequenceEmailSender) Send(
	_ context.Context,
	_ notifications.EmailConfig,
	message notifications.EmailMessage,
) notifications.EmailTransmission {
	s.messages = append(s.messages, message)
	result := s.results[0]
	s.results = s.results[1:]
	return result
}
