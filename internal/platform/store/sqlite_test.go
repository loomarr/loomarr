package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/people/notifications"
	"github.com/loomarr/loomarr/internal/platform/store/storetest"
)

// newSQLiteStore gives each test a private, migrated, boot-seeded SQLite store in a temp file.
// A file (not :memory:) is used because WAL + the single-conn model is what production runs;
// t.TempDir cleans it up.
//
// The store is a byte-for-byte copy of one template migrated once per test process: under -race
// every replay of the migration history costs ~3s, and ~40 behaviour tests used to pay it each
// (#1570). TestSQLiteStoreCloneMatchesFreshReplay proves a copy equals a fresh replay. Tests of
// migration, boot seeding, or startup healing must call Open(..., true) themselves.
func newSQLiteStore(t *testing.T) Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	if err := os.WriteFile(path, sharedSQLiteTemplate(t), 0o600); err != nil {
		t.Fatalf("clone sqlite template: %v", err)
	}
	s, err := Open(context.Background(), "sqlite://"+path, false)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

var (
	sqliteTemplateOnce   sync.Once
	sqliteTemplateBytes  []byte
	sqliteTemplateErr    error
	sqliteTemplateBuilds atomic.Int64
)

func sharedSQLiteTemplate(t *testing.T) []byte {
	t.Helper()
	sqliteTemplateOnce.Do(func() {
		sqliteTemplateBuilds.Add(1)
		dir, err := os.MkdirTemp("", "loomarr-sqlite-template-")
		if err != nil {
			sqliteTemplateErr = err
			return
		}
		defer func() { _ = os.RemoveAll(dir) }()
		sqliteTemplateBytes, sqliteTemplateErr = buildSQLiteTemplate(context.Background(), Open, filepath.Join(dir, "template.db"))
	})
	if sqliteTemplateErr != nil {
		t.Fatalf("create migrated sqlite template: %v", sqliteTemplateErr)
	}
	return sqliteTemplateBytes
}

// buildSQLiteTemplate migrates and boot-seeds one database, folds its WAL into the main file, and
// returns the file's bytes: a complete, closed database that any number of copies can start from.
func buildSQLiteTemplate(ctx context.Context, open sqliteConformanceOpenFunc, path string) ([]byte, error) {
	return storetest.SQLiteTemplate(ctx, storetest.OpenFunc[Store](open), checkpointSQLite, path)
}

// checkpointSQLite folds the WAL into the main file so the template's bytes are the whole database.
func checkpointSQLite(s Store) error {
	_, err := HandleOf(s).ExecContext(context.Background(), "PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

type sqliteConformanceOpenFunc func(context.Context, string, bool) (Store, error)

// newSQLiteConformanceStoreFactoryWithOpen gives every conformance assertion a private copy of one
// migrated, boot-seeded SQLite database (storetest.SQLiteClones), each opened with open.
func newSQLiteConformanceStoreFactoryWithOpen(t *testing.T, open sqliteConformanceOpenFunc) NewStoreFunc {
	t.Helper()
	return storetest.SQLiteClones(t, storetest.OpenFunc[Store](open), checkpointSQLite)
}

// TestSQLiteConformance runs the shared suite against SQLite, then proves every statement it made
// the store run finds its rows through an index (checkQueryPlans, #2793).
func TestSQLiteConformance(t *testing.T) {
	t.Parallel()
	t.Cleanup(func() { checkQueryPlans(t) })
	RunConformance(t, newSQLiteConformanceStoreFactoryWithOpen(t, openRecordingSQLite))
}

func TestNotificationWorkSurvivesStoreRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "notification-restart.db")
	dsn := "sqlite://" + path
	now := time.Unix(1_900_000_000, 0)
	st, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	intent, attempts := notificationFixture("restart", now, notifications.StatusQueued)
	if _, _, err := st.CreateNotificationIntent(ctx, intent, attempts); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	got, err := reopened.GetNotificationIntent(ctx, intent.ID)
	if err != nil || got.IdempotencyKey != intent.IdempotencyKey {
		t.Fatalf("intent after restart = %+v, %v", got, err)
	}
	gotAttempts, err := reopened.ListNotificationAttempts(ctx, intent.ID)
	if err != nil || len(gotAttempts) != 1 || gotAttempts[0].Status != notifications.StatusQueued {
		t.Fatalf("attempt after restart = %+v, %v", gotAttempts, err)
	}
}

func TestMigrationSourceIsStructurallyReadOnly(t *testing.T) {
	ctx := context.Background()
	live := newSQLiteStore(t)
	if err := live.SetSetting(ctx, "migration.probe", "before"); err != nil {
		t.Fatal(err)
	}

	snapshot, err := openSQLiteReadOnly(ctx, SQLitePath(live))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = snapshot.Close() }()

	if err := snapshot.SetSetting(ctx, "migration.probe", "after"); err == nil {
		t.Fatal("read-only migration source accepted a write")
	}
	if got, err := snapshot.GetSetting(ctx, "migration.probe"); err != nil || got != "before" {
		t.Fatalf("read-only source value = %q (err %v), want before", got, err)
	}
}

func TestOpenHealsPreAtomicTaxonomyProjections(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "taxonomy-upgrade.db")
	dsn := "sqlite://" + path
	s, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	clip := Clip{}
	clip.Hash, clip.Path, clip.TunarrProgramID = "upgrade-clip", "p/upgrade-clip.mp4", "tun-upgrade-clip"
	clip.Name, clip.Role, clip.Era, clip.Audience = "upgrade.mp4", clips.Commercial, 1994, clips.General
	clip.DurationMs, clip.Source, clip.UpdatedAt = 30000, "archive", time.Unix(1_700_000_000, 0).UTC()
	if err := s.UpsertClip(ctx, clip); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipTags(ctx, clip.Hash, []string{"cereal"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Simulate the state an old disabled repair job or a manual restore could leave behind: the
	// asserted leaf is trustworthy, while its rollup and closure are stale. Category is derived on
	// read (#2323), so there is no stored shadow left to go stale.
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `DELETE FROM clip_tags WHERE leaf = FALSE;
		DELETE FROM taxa_closure WHERE ancestor = 'food' AND descendant = 'cereal'`); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	got, err := reopened.GetClip(ctx, clip.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if got.Category != "cereal" {
		t.Errorf("healed category = %q, want cereal", got.Category)
	}
	if tags := slices.Sorted(slices.Values(got.Tags)); !slices.Equal(tags, []string{"cereal", "food"}) {
		t.Errorf("healed tags = %v, want [cereal food] (set)", got.Tags)
	}
}

func TestUnknownSchemeFailsFast(t *testing.T) {
	_, err := Open(context.Background(), "mysql://nope", false)
	if err == nil {
		t.Fatal("expected error for unknown DATABASE_URL scheme")
	}
}
