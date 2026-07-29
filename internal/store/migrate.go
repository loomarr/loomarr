package store

import (
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
// dialect is goose's ("sqlite" | "postgres"); dir is the embedded subdir.
func migrate(db *sql.DB, dialect, dir string) error {
	goose.SetBaseFS(migrationFS)
	goose.SetLogger(goose.NopLogger())
	if err := goose.SetDialect(dialect); err != nil {
		return fmt.Errorf("goose dialect %q: %w", dialect, err)
	}

	maxEmbedded, err := highestMigration(dir)
	if err != nil {
		return err
	}

	// Downgrade guard (§16): compare applied version to what we embed.
	current, err := goose.GetDBVersion(db)
	if err != nil {
		return fmt.Errorf("read db version: %w", err)
	}
	if current > maxEmbedded {
		return fmt.Errorf(
			"downgrade guard: DB schema version %d is newer than this binary knows (max %d) — "+
				"restore your pre-upgrade backup or run a build with migration ≥ %d",
			current, maxEmbedded, current)
	}

	if err := goose.Up(db, dir); err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
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
	// goose keeps global dialect state, set during migrate(). Re-assert it: this may run
	// long after boot, and on a mixed process (the V11 stepper touches both) the last
	// dialect set is not necessarily this store's.
	if err := goose.SetDialect(string(s.dialect)); err != nil {
		return 0
	}
	v, err := goose.GetDBVersion(s.db)
	if err != nil {
		return 0
	}
	return v
}
