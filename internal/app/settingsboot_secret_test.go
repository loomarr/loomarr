package app

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/settings"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestBootSettingsGeneratesOnlyOperationalTokensAndRedactsThem(t *testing.T) {
	t.Setenv("API_TOKEN", "")
	t.Setenv("PLAYOUT_TOKEN", "")
	st := testkit.MigratedSQLiteStore(t)
	legacyKey := "secret." + "session_" + "secret"
	const legacyValue = "legacy-inert-value-should-not-be-live"
	if err := st.SetSetting(context.Background(), legacyKey, legacyValue); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer
	base := slog.New(slog.NewTextHandler(&logs, nil))
	_, secrets, _, log, err := bootSettings(context.Background(), st, base)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"secret.api_token":     secrets.Value(settings.SecretAPI),
		"secret.playout_token": secrets.Value(settings.SecretPlayout),
	}
	rows, err := st.ListSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	generated := map[string]string{}
	for _, row := range rows {
		if strings.HasPrefix(row.Key, "secret.") && row.Key != legacyKey {
			generated[row.Key] = row.Value
		}
	}
	if len(generated) != len(want) {
		t.Fatalf("generated token rows = %v, want %v", generated, want)
	}
	for key, value := range want {
		if value == "" || generated[key] != value {
			t.Fatalf("generated %s = %q, live = %q", key, generated[key], value)
		}
		log.Info("credential", "value", value)
	}
	if strings.Contains(logs.String(), want["secret.api_token"]) ||
		strings.Contains(logs.String(), want["secret.playout_token"]) {
		t.Fatalf("generated token leaked through boot logger: %s", logs.String())
	}

	// An existing legacy row is preserved for forward-only compatibility but never loaded into
	// the generated-token cache or redactor. It has no authentication semantics.
	if stored, err := st.GetSetting(context.Background(), legacyKey); err != nil || stored != legacyValue {
		t.Fatalf("legacy row = (%q, %v), want preserved inert value", stored, err)
	}
	if strings.Contains(strings.Join(secrets.RedactionValues(), "\n"), legacyValue) {
		t.Fatal("legacy inert row was loaded as a live generated credential")
	}
}
