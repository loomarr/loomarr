package app

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit"
)

// Boot says so when the generated key lands beside the SQLite file, the default layout.
func TestBootWarnsWhenTheKeySitsBesideTheDatabase(t *testing.T) {
	t.Setenv("LOOMARR_ENCRYPTION_KEY", "")
	t.Setenv("LOOMARR_ENCRYPTION_KEY_FILE", "")
	t.Setenv("LOOMARR_ENCRYPTION_KEY_PREVIOUS", "")
	t.Setenv("LOOMARR_ENCRYPTION_KEY_PREVIOUS_FILE", "")
	st := testkit.MigratedSQLiteStore(t)
	var logs bytes.Buffer
	if _, err := buildSecretProtection(context.Background(), st, "", slog.New(slog.NewTextHandler(&logs, nil))); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(logs.String(), "encryption key is stored beside the database") {
		t.Fatalf("boot log = %q, want the key-placement warning", logs.String())
	}
}
