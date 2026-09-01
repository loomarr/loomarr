package notifications_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/secretprotection"
	"github.com/loomarr/loomarr/internal/store"
)

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
	opened, err := repository.GetNotificationDestination(ctx, destination.ID)
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
	if _, err := repository.GetNotificationDestination(ctx, destination.ID); err != nil {
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
	if _, err := repository.GetNotificationDestination(ctx, raw.ID); err == nil {
		t.Fatal("credential envelope opened after substitution onto another destination")
	}
}
