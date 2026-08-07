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
	return s.SeedTaxonomy(ctx, taxonomy.SeedForest(), time.Now())
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
			if err := migrate(s.db, "sqlite", "migrations/sqlite"); err != nil {
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
			if err := migrate(s.db, "postgres", "migrations/postgres"); err != nil {
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
