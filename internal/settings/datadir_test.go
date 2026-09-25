package settings

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/config"
)

// storageKeys are the directories whose defaults live under the database's data directory.
var storageKeys = map[string]string{
	"playout.prepared_dir": "prepared",
	"backup.dir":           "backups",
	"images.dir":           "images",
	"filler.dir":           "filler",
	"diagnostics.dir":      "diagnostics",
}

func newDataDirService(t *testing.T, databaseURL string, env, db map[string]string) *Service {
	t.Helper()
	s, err := New(context.Background(), NewRegistry(), fakeLoader{m: db}, nil,
		WithDataDir(config.DataDirFor(databaseURL)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.env = func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	return s
}

// A bare-metal install (#1461): with no storage settings every directory resolves beside the
// SQLite file instead of the container's /data, which an unprivileged user cannot create.
func TestStorageDefaults_FollowTheSQLiteDataDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "x")
	s := newDataDirService(t, "sqlite://"+filepath.Join(dir, "loomarr.db"), nil, nil)
	for key, sub := range storageKeys {
		r := s.Resolve(key)
		if want := filepath.Join(dir, sub); r.Value != want || r.Provenance != ProvenanceDefault {
			t.Errorf("%s = %v (%s), want %s (default)", key, r.Value, r.Provenance, want)
		}
	}
}

// The container layout and every Postgres install keep exactly today's /data paths.
func TestStorageDefaults_ContainerLayoutIsUnchanged(t *testing.T) {
	for _, url := range []string{"sqlite:///data/loomarr.db", "postgres://u:p@db/loomarr"} {
		s := newDataDirService(t, url, nil, nil)
		for key, sub := range storageKeys {
			if got, want := s.String(key), "/data/"+sub; got != want {
				t.Errorf("%s: %s = %q, want %q", url, key, got, want)
			}
		}
	}
	// A Service built without a data dir (every existing caller) is the container layout too.
	s, err := New(context.Background(), NewRegistry(), fakeLoader{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.String("backup.dir"); got != "/data/backups" {
		t.Errorf("no data dir: backup.dir = %q, want /data/backups", got)
	}
}

// Deriving the default must not disturb precedence: env > db > default.
func TestStorageDefaults_ExplicitSettingsStillWin(t *testing.T) {
	url := "sqlite://" + filepath.Join(t.TempDir(), "loomarr.db")
	env := map[string]string{"BACKUP_DIR": "/srv/env-backups"}
	db := map[string]string{"backup.dir": "/srv/db-backups", "images.dir": "/srv/db-images"}
	s := newDataDirService(t, url, env, db)
	if r := s.Resolve("backup.dir"); r.Value != "/srv/env-backups" || r.Provenance != ProvenanceEnv {
		t.Errorf("backup.dir = %v (%s), want env value", r.Value, r.Provenance)
	}
	if r := s.Resolve("images.dir"); r.Value != "/srv/db-images" || r.Provenance != ProvenanceDB {
		t.Errorf("images.dir = %v (%s), want db value", r.Value, r.Provenance)
	}
}

// The generated reference must say where the default resolves, not print one machine's path.
func TestStorageDefaults_DocsDescribeTheDerivation(t *testing.T) {
	for key, sub := range storageKeys {
		set, ok := NewRegistry().Get(key)
		if !ok {
			t.Fatalf("no setting %s", key)
		}
		if got, want := defaultCell(set), "`<data dir>/"+sub+"`"; got != want {
			t.Errorf("%s docs default = %s, want %s", key, got, want)
		}
	}
}
