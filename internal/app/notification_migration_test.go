package app

import (
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestMigrateLegacySMTPProviderCreatesOneEncryptedAccountOnlyProvider(t *testing.T) {
	t.Parallel()
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
	if err := migrateLegacySMTPProvider(t.Context(), repository, set); err != nil {
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

	destination.Label = "Operator label"
	destination.UpdatedAt = time.Now().UTC().Add(time.Minute)
	if err := repository.SaveNotificationDestination(t.Context(), destination); err != nil {
		t.Fatal(err)
	}
	if err := migrateLegacySMTPProvider(t.Context(), repository, set); err != nil {
		t.Fatal(err)
	}
	unchanged, err := repository.ResolveNotificationDestination(t.Context(), destination.ID)
	if err != nil || unchanged.Label != "Operator label" {
		t.Fatalf("repeat migration overwrote provider = %+v, %v", unchanged.Summary(), err)
	}
}
