package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestFillerStorageGovernorMigrationSQLite(t *testing.T) {
	t.Run("moves a durable positive allowance", func(t *testing.T) {
		ctx := context.Background()
		s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "storage-governor.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.Close() })
		provider, err := newMigrationProvider(s.db, s.dialect, "migrations/sqlite")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := provider.UpTo(ctx, 111); err != nil {
			t.Fatalf("migrate through 111: %v", err)
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO settings
			(key, value, updated_at, updated_by, env_override) VALUES (?, ?, ?, ?, ?)`,
			"filler.fetch.max_disk_gb", "37", int64(1234), "admin-1", false); err != nil { // retired-ok: migration fixture
			t.Fatal(err)
		}

		if _, err := provider.UpTo(ctx, 112); err != nil {
			t.Fatalf("apply storage governor migration: %v", err)
		}
		var value, updatedBy string
		var updatedAt int64
		var envOverride bool
		if err := s.db.QueryRowContext(ctx, `SELECT value, updated_at, updated_by, env_override
			FROM settings WHERE key = ?`, "filler.storage.library_budget_gb").Scan(
			&value, &updatedAt, &updatedBy, &envOverride,
		); err != nil {
			t.Fatal(err)
		}
		if value != "37" || updatedAt != 1234 || updatedBy != "admin-1" || envOverride {
			t.Fatalf("migrated setting = value %q updated_at %d updated_by %q env_override %t", value, updatedAt, updatedBy, envOverride)
		}
		assertSettingAbsent(t, ctx, s, "filler.fetch.max_disk_gb") // retired-ok: migration assertion
	})

	t.Run("keeps an existing new allowance and removes the obsolete key", func(t *testing.T) {
		ctx := context.Background()
		s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "storage-governor-existing.db"))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = s.Close() })
		provider, err := newMigrationProvider(s.db, s.dialect, "migrations/sqlite")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := provider.UpTo(ctx, 111); err != nil {
			t.Fatalf("migrate through 111: %v", err)
		}
		for key, value := range map[string]string{
			"filler.fetch.max_disk_gb":         "37", // retired-ok: migration fixture
			"filler.storage.library_budget_gb": "9",
		} {
			if _, err := s.db.ExecContext(ctx, `INSERT INTO settings
				(key, value, updated_at, env_override) VALUES (?, ?, 0, 0)`, key, value); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := provider.UpTo(ctx, 112); err != nil {
			t.Fatalf("apply storage governor migration: %v", err)
		}
		var value string
		if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`,
			"filler.storage.library_budget_gb").Scan(&value); err != nil {
			t.Fatal(err)
		}
		if value != "9" {
			t.Fatalf("existing allowance = %q, want 9", value)
		}
		assertSettingAbsent(t, ctx, s, "filler.fetch.max_disk_gb") // retired-ok: migration assertion
	})
}

func assertSettingAbsent(t *testing.T, ctx context.Context, s *sqlStore, key string) {
	t.Helper()
	var count int
	query := `SELECT COUNT(*) FROM settings WHERE key = ?`
	if s.dialect == DialectPostgres {
		query = `SELECT COUNT(*) FROM settings WHERE key = $1`
	}
	if err := s.db.QueryRowContext(ctx, query, key).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("setting %q still exists", key)
	}
}
