package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/people/keyrotation"
	"github.com/loomarr/loomarr/internal/people/notifications"
	"github.com/loomarr/loomarr/internal/people/protectedsettings"
	"github.com/loomarr/loomarr/internal/people/secretprotection"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestBootSettingsGeneratesOnlyOperationalTokensAndRedactsThem(t *testing.T) {
	t.Setenv("API_TOKEN", "")
	t.Setenv("PLAYOUT_TOKEN", "")
	st := testkit.MigratedSQLiteStore(t)

	var logs bytes.Buffer
	base := slog.New(slog.NewTextHandler(&logs, nil))
	protection := testSecretProtection(t, st)
	_, secrets, _, log, err := bootSettings(context.Background(), st, protection, base, "")
	if err != nil {
		t.Fatal(err)
	}
	webPushIdentity, err := json.Marshal(secrets.WebPushIdentity())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"secret.api_token":               secrets.Value(settings.SecretAPI),
		"secret.playout_token":           secrets.Value(settings.SecretPlayout),
		"secret.web_push_vapid_identity": string(webPushIdentity),
		"secret.arr_webhook":             secrets.ArrWebhookSecret(),
	}
	rows, err := st.ListSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	generated := map[string]string{}
	for _, row := range rows {
		if strings.HasPrefix(row.Key, "secret.") {
			generated[row.Key] = row.Value
		}
	}
	if len(generated) != len(want) {
		t.Fatalf("generated token rows = %v, want %v", generated, want)
	}
	for key, value := range want {
		if value == "" || generated[key] == value || !secretprotection.IsEnvelope(generated[key]) {
			t.Fatalf("generated %s is not encrypted at rest", key)
		}
		log.Info("credential", "value", value)
	}
	if strings.Contains(logs.String(), want["secret.api_token"]) ||
		strings.Contains(logs.String(), want["secret.playout_token"]) ||
		strings.Contains(logs.String(), secrets.WebPushIdentity().PrivateKey) ||
		strings.Contains(logs.String(), secrets.ArrWebhookSecret()) {
		t.Fatalf("generated token leaked through boot logger: %s", logs.String())
	}
}

func TestBootSettingsRedactsSMTPPasswordFromApplicationLogs(t *testing.T) {
	t.Setenv("NOTIFICATIONS_SMTP_PASSWORD", "")
	st := testkit.MigratedSQLiteStore(t)
	const password = "smtp-password-that-must-stay-secret"
	protection := testSecretProtection(t, st)
	if err := protectedsettings.Write(context.Background(), st, protection, "notifications.smtp.password", password); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	base := slog.New(slog.NewTextHandler(&logs, nil))
	set, _, _, log, err := bootSettings(context.Background(), st, protection, base, "")
	if err != nil {
		t.Fatal(err)
	}
	log.Error("delivery failed", "credential", password)
	if strings.Contains(logs.String(), password) {
		t.Fatalf("SMTP password leaked through application logger: %s", logs.String())
	}
	// A MigrationOnly key has no handle, so the test reads it by key.
	if got, _ := set.Value("notifications.smtp.password").(string); got != password {
		t.Fatalf("resolved SMTP password = %q, want plaintext in memory", got)
	}
	stored, err := st.GetSetting(context.Background(), "notifications.smtp.password")
	if err != nil {
		t.Fatal(err)
	}
	if stored == password || !secretprotection.IsEnvelope(stored) {
		t.Fatalf("SMTP password was not encrypted at rest: %q", stored)
	}
}

func TestBootSettingsOpensNamespacedLLMKeys(t *testing.T) {
	t.Setenv("API_TOKEN", "test-api-token")
	t.Setenv("PLAYOUT_TOKEN", "test-playout-token")
	st := testkit.MigratedSQLiteStore(t)
	const key = "llm.api_key.openrouter"
	const value = "namespaced-provider-key"
	protection := testSecretProtection(t, st)
	if err := protectedsettings.Write(context.Background(), st, protection, key, value); err != nil {
		t.Fatal(err)
	}
	set, _, _, _, err := bootSettings(context.Background(), st, protection, slog.New(slog.DiscardHandler), "")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := set.svc.LoadRaw(key); err != nil || got != value {
		t.Fatalf("in-memory namespaced key = (%q, %v), want plaintext", got, err)
	}
	stored, err := st.GetSetting(context.Background(), key)
	if err != nil || stored == value || !secretprotection.IsEnvelope(stored) {
		t.Fatalf("stored namespaced key = (%q, %v), want envelope", stored, err)
	}
}

func TestStorePersisterEncryptsOnlySecretSettings(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	protection := testSecretProtection(t, st)
	persister := storePersister{st: st, protection: protection}
	if err := persister.Apply(context.Background(), settings.PersistenceBatch{Upserts: []settings.PersistedSetting{
		{Key: "notifications.smtp.password", Value: "smtp-secret"},
		{Key: "notifications.smtp.host", Value: "mail.example.com"},
	}}); err != nil {
		t.Fatal(err)
	}
	secret, err := st.GetSetting(context.Background(), "notifications.smtp.password")
	if err != nil || secret == "smtp-secret" || !secretprotection.IsEnvelope(secret) {
		t.Fatalf("stored password = (%q, %v), want envelope", secret, err)
	}
	host, err := st.GetSetting(context.Background(), "notifications.smtp.host")
	if err != nil || host != "mail.example.com" {
		t.Fatalf("stored host = (%q, %v), want plaintext ordinary setting", host, err)
	}
}

func TestBootSettingsRejectsCorruptProtectedCiphertext(t *testing.T) {
	t.Setenv("API_TOKEN", "test-api-token")
	t.Setenv("PLAYOUT_TOKEN", "test-playout-token")
	st := testkit.MigratedSQLiteStore(t)
	if err := st.SetSetting(context.Background(), "notifications.smtp.password", "loomarr-secret:v1:missing:bad"); err != nil {
		t.Fatal(err)
	}
	protection := testSecretProtection(t, st)
	if _, _, _, _, err := bootSettings(context.Background(), st, protection, slog.New(slog.DiscardHandler), ""); err == nil {
		t.Fatal("boot accepted corrupt protected setting")
	}
}

// A protected setting stored as plaintext predates encrypted settings. Boot names every such
// key with the remedy, never echoes the value, and leaves the row untouched.
func TestBootSettingsFailsClosedOnPlaintextProtectedSettings(t *testing.T) {
	t.Setenv("API_TOKEN", "test-api-token")
	t.Setenv("PLAYOUT_TOKEN", "test-playout-token")
	st := testkit.MigratedSQLiteStore(t)
	const tokenValue, passwordValue = "plain-library-token-value", "plain-smtp-password-value"
	if err := st.SetSetting(context.Background(), "library.token", tokenValue); err != nil {
		t.Fatal(err)
	}
	if err := st.SetSetting(context.Background(), "notifications.smtp.password", passwordValue); err != nil {
		t.Fatal(err)
	}
	protection := testSecretProtection(t, st)
	_, _, _, _, err := bootSettings(context.Background(), st, protection, slog.New(slog.DiscardHandler), "")
	if err == nil {
		t.Fatal("boot accepted plaintext protected settings")
	}
	msg := err.Error()
	for _, want := range []string{"library.token", "notifications.smtp.password", "v0.2.0-beta.1", "re-enter"} {
		if !strings.Contains(msg, want) {
			t.Errorf("boot error %q lacks %q", msg, want)
		}
	}
	if strings.Contains(msg, tokenValue) || strings.Contains(msg, passwordValue) {
		t.Fatalf("boot error leaked a setting value: %q", msg)
	}
	if got, err := st.GetSetting(context.Background(), "library.token"); err != nil || got != tokenValue {
		t.Fatalf("failed boot changed plaintext row = (%q, %v)", got, err)
	}
}

func TestEncryptionServiceRotatesAndReencryptsStoredSettings(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	protection := testSecretProtection(t, st)
	persister := storePersister{st: st, protection: protection}
	if err := persister.Apply(context.Background(), settings.PersistenceBatch{Upserts: []settings.PersistedSetting{
		{Key: "notifications.smtp.password", Value: "smtp-before-rotation"},
	}}); err != nil {
		t.Fatal(err)
	}
	before, err := st.GetSetting(context.Background(), "notifications.smtp.password")
	if err != nil {
		t.Fatal(err)
	}
	destinations := notifications.NewProtectedRepository(st, protection)
	now := time.Unix(1_900_000_000, 0).UTC()
	if err := destinations.SaveNotificationDestination(context.Background(), notifications.Destination{
		ID: "slack-rotation", Means: notifications.MeansSlack, Label: "Slack",
		Scope: notifications.ScopeInstallation, Audience: notifications.RecipientOperators,
		Topics: []notifications.Topic{notifications.TopicChannelDegraded}, Enabled: false,
		Credentials: map[string]string{"webhookUrl": "https://hooks.slack.test/rotation-secret"},
		CreatedAt:   now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	destinationBefore, err := st.GetNotificationDestinationRecord(context.Background(), "slack-rotation")
	if err != nil {
		t.Fatal(err)
	}
	service := keyrotation.New(protection, st, destinations)
	if err := service.RotateDataKey(context.Background()); err != nil {
		t.Fatal(err)
	}
	after, err := st.GetSetting(context.Background(), "notifications.smtp.password")
	if err != nil {
		t.Fatal(err)
	}
	beforeParts, afterParts := strings.Split(before, ":"), strings.Split(after, ":")
	if len(beforeParts) < 4 || len(afterParts) < 4 || beforeParts[2] == afterParts[2] {
		t.Fatalf("setting did not move to rotated key: before=%q after=%q", before, after)
	}
	plain, err := protection.Open(protectedsettings.Record("notifications.smtp.password"), after)
	if err != nil || string(plain) != "smtp-before-rotation" {
		t.Fatalf("rotated setting = (%q, %v)", plain, err)
	}
	destinationAfter, err := st.GetNotificationDestinationRecord(context.Background(), "slack-rotation")
	if err != nil {
		t.Fatal(err)
	}
	destinationBeforeParts := strings.Split(destinationBefore.CredentialsEncrypted, ":")
	destinationAfterParts := strings.Split(destinationAfter.CredentialsEncrypted, ":")
	if len(destinationBeforeParts) < 4 || len(destinationAfterParts) < 4 ||
		destinationBeforeParts[2] == destinationAfterParts[2] {
		t.Fatalf("destination did not move to rotated key: before=%q after=%q",
			destinationBefore.CredentialsEncrypted, destinationAfter.CredentialsEncrypted)
	}
	opened, err := destinations.ResolveNotificationDestination(context.Background(), "slack-rotation")
	if err != nil || opened.Credentials["webhookUrl"] != "https://hooks.slack.test/rotation-secret" {
		t.Fatalf("rotated destination = (%q, %v)", opened.Credentials["webhookUrl"], err)
	}
}

func TestBuildSecretProtectionRewrapsFromOneBootPreviousKey(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	oldRaw := bytes.Repeat([]byte{0x81}, 32)
	newRaw := bytes.Repeat([]byte{0x91}, 32)
	oldEncoded := base64.RawURLEncoding.EncodeToString(oldRaw)
	newEncoded := base64.RawURLEncoding.EncodeToString(newRaw)
	t.Setenv("LOOMARR_ENCRYPTION_KEY", oldEncoded)
	t.Setenv("LOOMARR_ENCRYPTION_KEY_FILE", "")
	t.Setenv("LOOMARR_ENCRYPTION_KEY_PREVIOUS", "")
	t.Setenv("LOOMARR_ENCRYPTION_KEY_PREVIOUS_FILE", "")
	first, err := buildSecretProtection(context.Background(), st, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	record := protectedsettings.Record("library.token")
	envelope, err := first.Seal(record, []byte("survives-installation-key-replacement"))
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("LOOMARR_ENCRYPTION_KEY", newEncoded)
	t.Setenv("LOOMARR_ENCRYPTION_KEY_PREVIOUS", oldEncoded)
	replaced, err := buildSecretProtection(context.Background(), st, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Fingerprint() == first.Fingerprint() {
		t.Fatal("installation-key fingerprint did not change")
	}
	plain, err := replaced.Open(record, envelope)
	if err != nil || string(plain) != "survives-installation-key-replacement" {
		t.Fatalf("open after replacement = (%q, %v)", plain, err)
	}

	t.Setenv("LOOMARR_ENCRYPTION_KEY_PREVIOUS", "")
	if _, err := buildSecretProtection(context.Background(), st, t.TempDir(), nil); err != nil {
		t.Fatalf("restart using only replacement key: %v", err)
	}
}

func testSecretProtection(t testing.TB, st store.Store) *secretprotection.Manager {
	t.Helper()
	manager, err := secretprotection.NewManager(context.Background(), st, secretprotection.ManagerOptions{
		InstallationKey: secretprotection.InstallationKey{0xa1, 0xb2, 0xc3, 0xd4},
	})
	if err != nil {
		t.Fatalf("build test secret protection: %v", err)
	}
	return manager
}
