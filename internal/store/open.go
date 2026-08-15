package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/taxonomy"
)

// seedAfterMigrate runs idempotent boot seeds that must live in Go rather than SQL (the taxonomy, so
// its graph and taxonomy.SeedForest() cannot drift — §10 V45a). Each is guarded to no-op when already
// present, so it is safe on every open including test stores.
func seedAfterMigrate(ctx context.Context, s *sqlStore) error {
	now := time.Now()
	if err := s.SeedTaxonomy(ctx, taxonomy.SeedForest(), now); err != nil {
		return err
	}
	// ⚠ Rebuild the closure on EVERY open, unlike the seed's empty-guard. The closure is a derived
	// cache of `taxa` that a fresh migration creates empty; an operator-edited graph needs its closure
	// present just as a seeded one does. It is a full idempotent replace over ~55 taxa (a few hundred
	// rows), so running it unconditionally at boot keeps the closure in sync with whatever graph is
	// live — seeded default or operator's — at negligible cost. RebuildRollups is NOT run here: clip
	// rollups only go stale on a GRAPH EDIT (the CRUD path rebuilds them) or a re-tag (SetClipTags),
	// not on a plain re-open where the graph is unchanged.
	taxa, err := s.ListTaxa(ctx)
	if err != nil {
		return err
	}
	return s.RebuildClosure(ctx, taxonomy.New(taxa), now)
}

// Open selects and opens a backend from the DATABASE_URL scheme (§5) and, when
// autoMigrate is set, runs forward-only migrations (with the downgrade guard).
// Fails fast on an unknown scheme.
//
//	sqlite:///data/loomarr.db   -> SQLite (modernc, WAL)
//	postgres://user:pass@host/db -> Postgres (Phase 4)
func Open(ctx context.Context, databaseURL string, autoMigrate bool) (Store, error) {
	switch {
	case strings.HasPrefix(databaseURL, "sqlite://"):
		path := strings.TrimPrefix(databaseURL, "sqlite://")
		// sqlite:///data/x.db -> /data/x.db ; sqlite://./x.db -> ./x.db
		s, err := openSQLite(ctx, path)
		if err != nil {
			return nil, err
		}
		if autoMigrate {
			if err := migrate(ctx, s.db, "sqlite", "migrations/sqlite"); err != nil {
				_ = s.Close()
				return nil, err
			}
			if err := seedAfterMigrate(ctx, s); err != nil {
				_ = s.Close()
				return nil, err
			}
		}
		return s, nil

	case strings.HasPrefix(databaseURL, "postgres://"), strings.HasPrefix(databaseURL, "postgresql://"):
		s, err := openPostgres(ctx, databaseURL)
		if err != nil {
			return nil, err
		}
		if autoMigrate {
			if err := migrate(ctx, s.db, "postgres", "migrations/postgres"); err != nil {
				_ = s.Close()
				return nil, err
			}
			if err := seedAfterMigrate(ctx, s); err != nil {
				_ = s.Close()
				return nil, err
			}
		}
		return s, nil

	default:
		scheme := databaseURL
		if i := strings.Index(databaseURL, "://"); i >= 0 {
			scheme = databaseURL[:i]
		}
		return nil, fmt.Errorf("unknown DATABASE_URL scheme %q (want sqlite:// or postgres://)", scheme)
	}
}

// openPostgresForDataMigration builds only the SQL schema. Unlike a normal boot it
// deliberately does not run seedAfterMigrate: the SQLite source owns every application
// row, including the taxonomy, and seeding Go-owned rows into the target would make the
// target non-empty before the copy begins.
func openPostgresForDataMigration(ctx context.Context, dsn string) (*sqlStore, error) {
	s, err := openPostgres(ctx, dsn)
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, s.db, "postgres", "migrations/postgres"); err != nil {
		_ = s.Close()
		return nil, err
	}
	return s, nil
}
