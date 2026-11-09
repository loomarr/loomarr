package secretprotection

import "testing"

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
