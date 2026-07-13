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
