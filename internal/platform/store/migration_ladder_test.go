package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// The migration ladder gives a migration test a private SQLite file already migrated through the
// version it asks for, so the test seeds the rows the migration under test must carry and applies
// only that migration. Replaying the history from an empty file costs each such test tens of
// seconds under -race, and about forty tests did it (#2716). The ladder replays the history once
// per test process, keeps a closed copy of the file every ladderStep versions, and each test
// replays at most ladderStep-1 versions on top of its own copy.
//
// A rung is the file a fresh replay produces: TestMigrationLadderMatchesAFreshReplay holds that.
const ladderStep = 10

// sqliteConnections is how a migration test opens its database. The connection settings are part
// of a migration's history: a dropped table cascades only while foreign keys are enforced. So each
// kind climbs its own ladder, and a test's file has run its whole history under the settings the
// test itself opens it with.
type sqliteConnections string

const (
	// sql.Open("sqlite", "file:"+path): the driver's defaults, foreign keys not enforced.
	plainConnections sqliteConnections = "plain"
	// The driver's defaults with foreign keys enforced.
	foreignKeyConnections sqliteConnections = "foreign keys"
	// The store's own settings (sqliteDSN), which openSQLite uses.
	storeConnections sqliteConnections = "store"
)

// dsn is the database/sql data source name for a file opened with these settings.
func (c sqliteConnections) dsn(path string) string {
	switch c {
	case foreignKeyConnections:
		return "file:" + path + "?_pragma=foreign_keys(on)"
	case storeConnections:
		return sqliteDSN(path)
	default:
		return "file:" + path
	}
}

type migrationLadder struct {
	mu sync.Mutex
	// Closed database files by the version they are migrated through.
	rungs map[int64][]byte
	top   int64
}

var migrationLadders = struct {
	mu sync.Mutex
	by map[sqliteConnections]*migrationLadder
}{by: map[sqliteConnections]*migrationLadder{}}

// sqliteFileAt returns the path of a private SQLite file migrated through version over
// connections of the given kind. The caller opens it with the same kind.
func sqliteFileAt(t *testing.T, version int64, connections sqliteConnections) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "loomarr.db")
	rung := version - version%ladderStep
	if rung > 0 {
		if err := os.WriteFile(path, ladderFor(connections).rung(t, rung, connections), 0o600); err != nil {
			t.Fatalf("copy migration rung %d: %v", rung, err)
		}
	}
	if rung < version {
		if err := migrateSQLiteFile(path, version, connections); err != nil {
			t.Fatalf("migrate from rung %d through %d: %v", rung, version, err)
		}
	}
	return path
}

// sqliteDSNAt is sqliteFileAt as the data source name a test hands to sql.Open.
func sqliteDSNAt(t *testing.T, version int64, connections sqliteConnections) string {
	t.Helper()
	return connections.dsn(sqliteFileAt(t, version, connections))
}

func ladderFor(connections sqliteConnections) *migrationLadder {
	migrationLadders.mu.Lock()
	defer migrationLadders.mu.Unlock()
	ladder, ok := migrationLadders.by[connections]
	if !ok {
		ladder = &migrationLadder{rungs: map[int64][]byte{}}
		migrationLadders.by[connections] = ladder
	}
	return ladder
}

// rung returns the file migrated through version, a multiple of ladderStep, climbing from the
// highest rung built so far. One climber at a time: a second test waits for the rungs the first is
// building and never replays them itself.
func (l *migrationLadder) rung(t *testing.T, version int64, connections sqliteConnections) []byte {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if version > l.top {
		if err := l.climb(version, connections); err != nil {
			t.Fatalf("build the %s migration ladder through %d: %v", connections, version, err)
		}
	}
	return l.rungs[version]
}

func (l *migrationLadder) climb(version int64, connections sqliteConnections) error {
	dir, err := os.MkdirTemp("", "loomarr-migration-ladder-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, "ladder.db")
	if l.top > 0 {
		if err := os.WriteFile(path, l.rungs[l.top], 0o600); err != nil {
			return err
		}
	}
	for next := l.top + ladderStep; next <= version; next += ladderStep {
		if err := migrateSQLiteFile(path, next, connections); err != nil {
			return err
		}
		file, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		l.rungs[next] = file
		l.top = next
	}
	return nil
}

// migrateSQLiteFile applies every migration through version to the file and closes it, leaving
// one complete file. The store's settings keep a write-ahead log beside the database; it is folded
// into the main file before the close, and a log that still holds pages afterwards is an error,
// because a copy of the main file alone would be missing them.
func migrateSQLiteFile(path string, version int64, connections sqliteConnections) error {
	ctx := context.Background()
	db, err := sql.Open("sqlite", connections.dsn(path))
	if err != nil {
		return err
	}
	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		_ = db.Close()
		return err
	}
	if _, err := provider.UpTo(ctx, version); err != nil {
		_ = db.Close()
		return fmt.Errorf("migrate through %d: %w", version, err)
	}
	if _, err := db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		_ = db.Close()
		return fmt.Errorf("fold the write-ahead log at %d: %w", version, err)
	}
	if err := db.Close(); err != nil {
		return err
	}
	if wal, err := os.Stat(path + "-wal"); err == nil && wal.Size() > 0 {
		return fmt.Errorf("migrating through %d left a %d-byte write-ahead log beside %s", version, wal.Size(), path)
	}
	return nil
}

// A rung plus the few versions a test replays on top of it is the database a replay from an empty
// file produces, under each kind of connection. The version sits between two rungs and above
// several, so both the copy and the replay on top of it are exercised.
func TestMigrationLadderMatchesAFreshReplay(t *testing.T) {
	t.Parallel()
	const version = 3*ladderStep + 7
	for _, connections := range []sqliteConnections{plainConnections, foreignKeyConnections, storeConnections} {
		t.Run(string(connections), func(t *testing.T) {
			t.Parallel()
			fresh := filepath.Join(t.TempDir(), "fresh.db")
			if err := migrateSQLiteFile(fresh, version, connections); err != nil {
				t.Fatalf("replay from an empty file: %v", err)
			}
			want := fingerprintSQLiteFile(t, connections.dsn(fresh))
			if got := fingerprintSQLiteFile(t, sqliteDSNAt(t, version, connections)); got != want {
				t.Fatalf("the ladder's file drifted from a fresh replay\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func fingerprintSQLiteFile(t *testing.T, dsn string) string {
	t.Helper()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	return sqliteFingerprint(t, db)
}
