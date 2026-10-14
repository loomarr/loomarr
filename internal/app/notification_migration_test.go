package app

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestMigrateLegacySMTPProviderCreatesOneEncryptedAccountOnlyProvider(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	protection := testSecretProtection(t, st)
	repository := notifications.NewProtectedDestinationRepository(st, protection)
	set := visionSet(t, map[string]string{
		"notifications.email.enabled":      "true",
		"notifications.email.from_address": "loomarr@example.test",
		"notifications.email.from_name":    "Loomarr",
		"notifications.smtp.host":          "smtp.example.test",
		"notifications.smtp.port":          "587",
		"notifications.smtp.security":      "starttls",
		"notifications.smtp.username":      "mailer",
		"notifications.smtp.password":      "smtp-migration-secret",
	})
	if err := migrateLegacySMTPProvider(t.Context(), st, repository, set); err != nil {
		t.Fatal(err)
	}
	destination, err := repository.ResolveNotificationDestination(t.Context(), "smtp-legacy")
	if err != nil {
		t.Fatal(err)
	}
	if !destination.Enabled || len(destination.Topics) != 0 ||
		destination.Configuration["host"] != "smtp.example.test" ||
		destination.Credentials["password"] != "smtp-migration-secret" {
		t.Fatalf("migrated SMTP provider = %+v", destination.Summary())
	}
	raw, err := st.GetNotificationDestinationRecord(t.Context(), destination.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw.CredentialsEncrypted, "smtp-migration-secret") {
		t.Fatal("SMTP migration persisted its password in plaintext")
	}
	if marker, err := st.GetSetting(t.Context(), "migration.notifications.smtp_provider.v1"); err != nil || marker != "true" {
		t.Fatalf("migration marker = %q, %v", marker, err)
	}

	destination.Label = "Operator label"
	destination.UpdatedAt = time.Now().UTC().Add(time.Minute)
	if err := repository.SaveNotificationDestination(t.Context(), destination); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacySMTPProvider(t.Context(), st, repository, set); err != nil {
		t.Fatal(err)
	}
	unchanged, err := repository.ResolveNotificationDestination(t.Context(), destination.ID)
	if err != nil || unchanged.Label != "Operator label" {
		t.Fatalf("repeat migration overwrote provider = %+v, %v", unchanged.Summary(), err)
	}
	if err := repository.DeleteNotificationDestination(t.Context(), destination.ID); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacySMTPProvider(t.Context(), st, repository, set); err != nil {
		t.Fatal(err)
	}
	if destinations, err := repository.ListNotificationDestinationMetadata(t.Context()); err != nil || len(destinations) != 0 {
		t.Fatalf("deleted migrated provider was recreated = %+v, %v", destinations, err)
	}
}

func TestMigrateLegacySMTPProviderLeavesAnExistingProviderAuthoritative(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	repository := notifications.NewProtectedDestinationRepository(st, testSecretProtection(t, st))
	now := time.Unix(1_900_000_000, 0).UTC()
	existing := notifications.Destination{
		ID: "smtp-current", Means: notifications.MeansEmail, Label: "Current SMTP",
		Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
		Enabled: true,
		Configuration: map[string]string{
			"host": "current.example.test", "port": "465", "security": "tls",
			"fromAddress": "current@example.test", "fromName": "Current",
		},
		CreatedAt: now, UpdatedAt: now,
	}
	if err := repository.SaveNotificationDestination(t.Context(), existing); err != nil {
		t.Fatal(err)
	}
	set := visionSet(t, map[string]string{
		"notifications.email.enabled":      "true",
		"notifications.email.from_address": "legacy@example.test",
		"notifications.smtp.host":          "legacy.example.test",
	})
	if err := migrateLegacySMTPProvider(t.Context(), st, repository, set); err != nil {
		t.Fatal(err)
	}
	destinations, err := repository.ListNotificationDestinationMetadata(t.Context())
	if err != nil || len(destinations) != 1 || destinations[0].ID != existing.ID {
		t.Fatalf("destinations = %+v, %v", destinations, err)
	}
}

func TestCompletedSMTPMigrationNeverReadsStaleLegacyInput(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	if err := st.SetSetting(t.Context(), "migration.notifications.smtp_provider.v1", "true"); err != nil {
		t.Fatal(err)
	}
	repository := notifications.NewProtectedDestinationRepository(st, testSecretProtection(t, st))
	set := visionSet(t, map[string]string{"notifications.smtp.port": "not-a-port"})
	if err := migrateLegacySMTPProvider(t.Context(), st, repository, set); err != nil {
		t.Fatalf("completed migration consulted stale legacy input: %v", err)
	}
}

func TestPendingSMTPMigrationRejectsMalformedLegacyInput(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	repository := notifications.NewProtectedDestinationRepository(st, testSecretProtection(t, st))
	set := visionSet(t, map[string]string{"notifications.smtp.port": "not-a-port"})
	if err := migrateLegacySMTPProvider(t.Context(), st, repository, set); err == nil {
		t.Fatal("pending migration accepted malformed legacy input")
	}
	if _, err := st.GetSetting(t.Context(), "migration.notifications.smtp_provider.v1"); err == nil {
		t.Fatal("failed migration recorded completion")
	}
}
