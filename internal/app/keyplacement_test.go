package app

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit"
)

func TestKeyBesideDatabase(t *testing.T) {
	for _, tc := range []struct {
		name, key, database string
		want                bool
	}{
		{"generated beside the SQLite file", "/data/encryption.key", "/data/loomarr.db", true},
		{"same folder written differently", "/data/./encryption.key", "/data/loomarr.db", true},
		{"mounted from elsewhere", "/run/secrets/loomarr-key", "/data/loomarr.db", false},
		{"key from the environment", "", "/data/loomarr.db", false},
		{"Postgres has no database file", "/data/encryption.key", "", false},
	} {
		if got := keyBesideDatabase(tc.key, tc.database); got != tc.want {
			t.Errorf("%s: keyBesideDatabase(%q, %q) = %v, want %v", tc.name, tc.key, tc.database, got, tc.want)
		}
	}
}

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
