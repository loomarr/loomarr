package store

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/pressly/goose/v3"
)

// migrate runs forward-only migrations for the given dialect (§5, §16), after
// enforcing the startup downgrade guard: if the DB's applied schema version is
// NEWER than any migration this binary embeds, refuse to proceed (§16) — a
// rolled-back container must not limp into a schema it doesn't understand.
//
// dialect is Loomarr's ("sqlite" | "postgres"); dir is the embedded subdir.
func migrate(ctx context.Context, db *sql.DB, dialect, dir string) error {
	provider, err := newMigrationProvider(db, Dialect(dialect), dir)
	if err != nil {
		return err
	}

	maxEmbedded, err := highestMigration(dir)
	if err != nil {
		return err
	}

	// Downgrade guard (§16): compare applied version to what we embed.
	current, err := provider.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("read db version: %w", err)
	}
	if current > maxEmbedded {
		return fmt.Errorf(
			"downgrade guard: DB schema version %d is newer than this binary knows (max %d) — "+
				"restore your pre-upgrade backup or run a build with migration ≥ %d",
			current, maxEmbedded, current)
	}

	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}

// newMigrationProvider owns the complete goose configuration for one database.
// In particular, it avoids goose's legacy process-global dialect and filesystem
// state, because the live migration stepper can use SQLite and Postgres in the
// same process at the same time.
func newMigrationProvider(db *sql.DB, dialect Dialect, dir string) (*goose.Provider, error) {
	var gooseDialect goose.Dialect
	switch dialect {
	case DialectSQLite:
		// Loomarr calls this dialect "sqlite"; Provider uses goose's canonical
		// "sqlite3" spelling rather than the alias accepted by SetDialect.
		gooseDialect = goose.DialectSQLite3
	case DialectPostgres:
		gooseDialect = goose.DialectPostgres
	default:
		return nil, fmt.Errorf("goose dialect %q: unsupported dialect", dialect)
	}

	migrations, err := fs.Sub(migrationFS, dir)
	if err != nil {
		return nil, fmt.Errorf("migration filesystem %q: %w", dir, err)
	}
	provider, err := goose.NewProvider(
		gooseDialect,
		db,
		migrations,
		goose.WithLogger(goose.NopLogger()),
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return nil, fmt.Errorf("create migration provider for %q: %w", dialect, err)
	}
	return provider, nil
}

// highestMigration returns the largest numeric prefix among the .sql files in
// the embedded dir, i.e. the newest schema version this binary can apply.
func highestMigration(dir string) (int64, error) {
	entries, err := fs.ReadDir(migrationFS, dir)
	if err != nil {
		return 0, fmt.Errorf("read migration dir %q: %w", dir, err)
	}
	var versions []int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		prefix := strings.SplitN(path.Base(e.Name()), "_", 2)[0]
		v, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("migration %q has no numeric prefix: %w", e.Name(), err)
		}
		versions = append(versions, v)
	}
	if len(versions) == 0 {
		return 0, fmt.Errorf("no migrations found in %q", dir)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	return versions[len(versions)-1], nil
}

// SchemaVersion reports the migration version currently applied to the store's database
// — the number the About page shows and an operator quotes in a bug report (§16, V12).
//
// Read live rather than cached at boot: the migration stepper (§5, V11) can move an
// install onto a different database within one process lifetime, and a value captured at
// startup would then describe a database the app is no longer talking to.
//
// Returns 0 for a non-SQL store or an unreadable version table. 0 is "unknown", which the
// API omits rather than rendering as a real schema 0 — a wrong number in a bug report is
// worse than an absent one.
func SchemaVersion(st Store) int64 {
	s, ok := st.(*sqlStore)
	if !ok || s.db == nil {
		return 0
	}
	provider, err := newMigrationProvider(
		s.db,
		s.dialect,
		"migrations/"+string(s.dialect),
	)
	if err != nil {
		return 0
	}
	v, err := provider.GetDBVersion(context.Background())
	if err != nil {
		return 0
	}
	return v
}
