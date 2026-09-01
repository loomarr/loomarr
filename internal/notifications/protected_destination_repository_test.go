package notifications_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/secretprotection"
	"github.com/loomarr/loomarr/internal/store"
)

type metadataDestinationTester struct {
	destination notifications.DestinationMetadata
}

func (t *metadataDestinationTester) PublishDestinationTest(
	_ context.Context,
	destination notifications.DestinationMetadata,
	_ string,
) (notifications.DestinationTestResult, error) {
	t.destination = destination
	return notifications.DestinationTestResult{IntentID: "test-intent", Created: true}, nil
}

func TestProtectedDestinationRepositoryNeverGivesPlaintextCredentialsToStorage(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "notifications.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	manager, err := secretprotection.NewManager(ctx, st, secretprotection.ManagerOptions{
		InstallationKey: secretprotection.InstallationKey{0x42},
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := notifications.NewProtectedDestinationRepository(st, manager)
	now := time.Unix(1_900_000_000, 0).UTC()
	destination := notifications.Destination{
		ID: "slack-main", Means: notifications.MeansSlack, Label: "Main Slack",
		Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
		Topics: []notifications.Topic{notifications.TopicChannelDegraded}, Enabled: true,
		Configuration: map[string]string{"displayName": "Loomarr"},
		Credentials:   map[string]string{"webhookUrl": "https://hooks.slack.test/plaintext-token"},
		CreatedAt:     now, UpdatedAt: now,
	}
	if err := repository.SaveNotificationDestination(ctx, destination); err != nil {
		t.Fatal(err)
	}
	raw, err := st.GetNotificationDestinationRecord(ctx, destination.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !secretprotection.IsEnvelope(raw.CredentialsEncrypted) ||
		strings.Contains(raw.CredentialsEncrypted, "plaintext-token") {
		t.Fatalf("stored credentials are not an opaque encryption envelope: %q", raw.CredentialsEncrypted)
	}
	var databaseValue string
	if err := store.PoolOf(st).QueryRowContext(
		ctx, `SELECT credentials_encrypted FROM notification_destinations WHERE id = ?`, destination.ID,
	).Scan(&databaseValue); err != nil {
		t.Fatal(err)
	}
	if databaseValue != raw.CredentialsEncrypted || strings.Contains(databaseValue, "plaintext-token") {
		t.Fatalf("database credential column = %q", databaseValue)
	}
	opened, err := repository.ResolveNotificationDestination(ctx, destination.ID)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Credentials["webhookUrl"] != destination.Credentials["webhookUrl"] {
		t.Fatalf("opened credential = %q", opened.Credentials["webhookUrl"])
	}

	before := raw.CredentialsEncrypted
	if err := manager.RotateDataKey(ctx); err != nil {
		t.Fatal(err)
	}
	if err := repository.ReencryptAll(ctx); err != nil {
		t.Fatal(err)
	}
	raw, err = st.GetNotificationDestinationRecord(ctx, destination.ID)
	if err != nil {
		t.Fatal(err)
	}
	if raw.CredentialsEncrypted == before {
		t.Fatal("data-key rotation did not reseal destination credentials")
	}
	if _, err := repository.ResolveNotificationDestination(ctx, destination.ID); err != nil {
		t.Fatalf("read after data-key rotation: %v", err)
	}
}

func TestProtectedDestinationRepositoryBindsCredentialsToDestinationIdentity(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "notifications.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	manager, err := secretprotection.NewManager(ctx, st, secretprotection.ManagerOptions{
		InstallationKey: secretprotection.InstallationKey{0x71},
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := notifications.NewProtectedDestinationRepository(st, manager)
	now := time.Unix(1_900_000_000, 0).UTC()
	destination := notifications.Destination{
		ID: "discord-one", Means: notifications.MeansDiscord, Label: "Discord",
		Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
		Topics: []notifications.Topic{notifications.TopicChannelDegraded}, Enabled: false,
		Credentials: map[string]string{"webhookUrl": "https://discord.test/secret"},
		CreatedAt:   now, UpdatedAt: now,
	}
	if err := repository.SaveNotificationDestination(ctx, destination); err != nil {
		t.Fatal(err)
	}
	raw, err := st.GetNotificationDestinationRecord(ctx, destination.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw.ID = "discord-two"
	if err := st.SaveNotificationDestinationRecord(ctx, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ResolveNotificationDestination(ctx, raw.ID); err == nil {
		t.Fatal("credential envelope opened after substitution onto another destination")
	}
}

func TestProtectedDestinationRepositoryListsMetadataWithoutOpeningCredentials(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "notifications.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	manager, err := secretprotection.NewManager(ctx, st, secretprotection.ManagerOptions{
		InstallationKey: secretprotection.InstallationKey{0x63},
	})
	if err != nil {
		t.Fatal(err)
	}
	repository := notifications.NewProtectedDestinationRepository(st, manager)
	now := time.Unix(1_900_000_000, 0).UTC()
	destination := notifications.Destination{
		ID: "webhook-main", Means: notifications.MeansWebhook, Label: "Main webhook",
		Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
		Topics: []notifications.Topic{notifications.TopicChannelDegraded}, Enabled: true,
		Credentials: map[string]string{
			"url":        "https://hooks.example.test/private",
			"hmacSecret": "signing-secret",
		},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.SaveNotificationDestination(ctx, destination); err != nil {
		t.Fatal(err)
	}
	raw, err := st.GetNotificationDestinationRecord(ctx, destination.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw.CredentialsEncrypted = "corrupt-envelope-that-must-not-be-opened"
	if err := st.SaveNotificationDestinationRecord(ctx, raw); err != nil {
		t.Fatal(err)
	}

	metadata, err := repository.ListNotificationDestinationMetadata(ctx)
	if err != nil {
		t.Fatalf("list redacted metadata: %v", err)
	}
	if len(metadata) != 1 {
		t.Fatalf("metadata count = %d, want 1", len(metadata))
	}
	summary := metadata[0].Summary()
	if !summary.CredentialsConfigured {
		t.Fatal("credentials reported as unconfigured")
	}
	configured := make(map[string]bool, len(summary.Settings))
	for _, state := range summary.Settings {
		configured[state.Key] = state.SecretConfigured
	}
	if !configured["url"] || !configured["hmacSecret"] {
		t.Fatalf("redacted secret state = %+v", configured)
	}
	tester := &metadataDestinationTester{}
	managerAPI := notifications.NewDestinationManager(repository, nil, func() string { return "unused" }, time.Now).WithTester(tester)
	summaries, err := managerAPI.List(ctx, notifications.Principal{PersonID: "admin", Administrator: true})
	if err != nil || len(summaries) != 1 {
		t.Fatalf("list destination management summaries = %+v, %v", summaries, err)
	}
	if _, err := managerAPI.Update(ctx, notifications.Principal{PersonID: "member"}, destination.ID, notifications.DestinationUpdateCommand{}); !errors.Is(err, notifications.ErrForbidden) {
		t.Fatalf("unauthorized update opened credentials: %v", err)
	}
	router := notifications.NewDestinationRouter(repository, nil)
	routes, err := router.Routes(ctx, notifications.Intent{
		ID: "intent-1", Topic: notifications.TopicChannelDegraded,
		RecipientKind: notifications.RecipientOperators, RecipientID: "operators",
		ReferenceKind: notifications.ReferenceChannel, ReferenceID: "channel-1",
		Policy: notifications.PolicyConfigurable, Template: notifications.TemplateData{SubjectName: "Channel"},
		IdempotencyKey: "channel-degraded:channel-1", CreatedAt: now,
	})
	if err != nil || len(routes) != 1 || routes[0].DestinationRef != destination.ID {
		t.Fatalf("route from credential-free metadata = %+v, %v", routes, err)
	}
	result, err := managerAPI.Test(ctx, notifications.Principal{PersonID: "admin", Administrator: true}, destination.ID, "request-1")
	if err != nil || !result.Created || tester.destination.ID != destination.ID {
		t.Fatalf("queue test from credential-free metadata = %+v, %v", result, err)
	}
	if err := repository.RetireNotificationDestination(ctx, destination.ID); err != nil {
		t.Fatalf("retire without opening credentials: %v", err)
	}
	retired, err := repository.GetNotificationDestinationMetadata(ctx, destination.ID)
	if err != nil || retired.Enabled {
		t.Fatalf("retired metadata = %+v, %v", retired, err)
	}
	if err := managerAPI.Delete(ctx, notifications.Principal{PersonID: "admin", Administrator: true}, destination.ID); err != nil {
		t.Fatalf("delete from credential-free metadata: %v", err)
	}
}
