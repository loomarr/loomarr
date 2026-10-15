package store

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// Migration 00134 backfills a clip's name provenance from what its row already records (#1988):
// a name that is the path's stem, a hash, or a segment's filename fallback is 'filename'; a name
// that is exactly a grounded rendering of the row's own facts is 'grounded'; everything else, a
// spaced stem taken from a sidecar's originalName included, stays 'unknown' rather than guessed.
func TestClipNameSourceMigrationSQLite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, err := openSQLite(ctx, filepath.Join(t.TempDir(), "clip-name-source.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	testClipNameSourceMigration(t, s, "migrations/sqlite")
}

func testClipNameSourceMigration(t *testing.T, s *sqlStore, migrationDir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(s.db, s.dialect, migrationDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 133); err != nil {
		t.Fatalf("migrate through 133: %v", err)
	}

	hashA, hashB := strings.Repeat("a1", 32), strings.Repeat("b2", 32)
	type row struct {
		hash, path, name, kind, brand, category, parent string
		era                                             int
		want                                            string
	}
	rows := []row{
		{hash: "stem", path: "ads/Widgetmax Ad 1991.mp4", name: "Widgetmax Ad 1991", want: "filename"},
		{hash: "stem-root", path: "Widgetmax_Spot.mkv", name: "Widgetmax_Spot", want: "filename"},
		{hash: hashA, path: "a1/a1/" + hashA + ".mp4", name: hashA, want: "filename"},
		{hash: "fallback", path: "c3/c3/fallback.mp4", name: "Clip 2 from widgetmax tape", parent: "tape", want: "filename"},
		{hash: "brand", path: "d4/d4/brand.mp4", name: "Fizzbolt — 1993", brand: "Fizzbolt", era: 1993, want: "grounded"},
		{hash: "brand-bare", path: "d4/d4/brand-bare.mp4", name: "Fizzbolt", brand: "Fizzbolt", want: "grounded"},
		{hash: "quote", path: "e5/e5/quote.mp4", name: "“True tales of the canyon patrol”", want: "grounded"},
		{hash: "quote-era", path: "e5/e5/quote-era.mp4", name: "“Zap goes the cola” — 1988", era: 1988, want: "grounded"},
		{hash: "category", path: "f6/f6/category.mp4", name: "Soft drink commercial — 1991", category: "soft_drink",
			era: 1991, want: "grounded"},
		{hash: "era", path: "07/07/era.mp4", name: "1994 station id", kind: "station_id", era: 1994, want: "grounded"},
		{hash: "untitled", path: "08/08/untitled.mp4", name: "Untitled commercial", want: "grounded"},
		// Intake filed this one as its hash; its filename lived only in the sidecar's originalName.
		{hash: hashB, path: "b2/b2/" + hashB + ".mp4", name: "Widgetmax Ad 1991", want: "unknown"},
		{hash: "title", path: "09/09/title.mp4", name: "Zap Cola Summer", want: "unknown"},
		// Exact comparison on both dialects: SQLite's LIKE would have matched this case-blind.
		{hash: "case", path: "ads/widgetmax ad 1991.mp4", name: "Widgetmax Ad 1991", want: "unknown"},
		{hash: "not-a-segment", path: "10/10/not-a-segment.mp4", name: "Clip 2 from widgetmax tape", want: "unknown"},
	}
	at := time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC).Unix()
	for _, r := range rows {
		kind := r.kind
		if kind == "" {
			kind = string(filler.Commercial)
		}
		// Seeded through the schema as it stood at 00133: current UpsertClip writes name_source.
		if _, err := s.db.ExecContext(ctx, s.ph(`INSERT INTO clips
			(hash, path, name, kind, brand, category, era, parent_hash, duration_ms, held, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
			r.hash, r.path, r.name, kind, r.brand, r.category, r.era, r.parent, 30_000, false, at); err != nil {
			t.Fatalf("seed %s: %v", r.hash, err)
		}
	}

	if _, err := provider.UpTo(ctx, 134); err != nil {
		t.Fatalf("apply clip name source: %v", err)
	}
	for _, r := range rows {
		var got string
		if err := s.db.QueryRowContext(ctx, s.ph(`SELECT name_source FROM clips WHERE hash = ?`), r.hash).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != r.want {
			t.Errorf("%s (%q at %q) name_source = %q, want %q", r.hash, r.name, r.path, got, r.want)
		}
	}
}
