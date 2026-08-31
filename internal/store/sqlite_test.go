package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
)

type cachedEpisodeAvailability []schedule.ResolvedProgram

func (a cachedEpisodeAvailability) Resolve(provision.Key) (string, int64, bool) {
	return "", 0, false
}

func (a cachedEpisodeAvailability) ResolveEpisodes(provision.Key) schedule.EpisodeResolution {
	return schedule.EpisodeResolution{Programs: a}
}

// newSQLiteStore builds a fresh migrated SQLite store in a temp file per test.
// A file (not :memory:) is used because WAL + the single-conn model is what
// production runs; t.TempDir cleans it up.
func newSQLiteStore(t *testing.T) Store {
	t.Helper()
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "test.db")
	s, err := Open(context.Background(), dsn, true)
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestSQLiteConformance runs the shared suite against SQLite. Phase 4 adds the
// identical call for Postgres.
func TestSQLiteConformance(t *testing.T) {
	RunConformance(t, newSQLiteStore)
}

func TestNotificationWorkSurvivesStoreRestart(t *testing.T) {
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

func TestLegacySeriesEpisodeEvidenceIsSanitizedBeforeScheduling(t *testing.T) {
	ctx := context.Background()
	st := newSQLiteStore(t).(*sqlStore)
	tags := make([]any, 16)
	for i := range tags {
		tags[i] = fmt.Sprintf("ordinary-%d", i)
	}
	tags = append(tags, "christmas")
	legacy := []map[string]any{
		{
			"LibraryItemID": "legacy-1", "Title": "Ordinary One", "DurationMs": 1,
			"Season": 1, "Episode": 1, "EpisodeEnd": 0, "CommunityRating": "not-a-number",
			"Overview": strings.Repeat("x", 2048) + " christmas", "Tags": tags,
			"UnknownEvidence": "christmas",
		},
		{
			"LibraryItemID": "legacy-2", "Title": "Ordinary Two", "DurationMs": 1,
			"Season": 1, "Episode": 2, "EpisodeEnd": 0, "CommunityRating": 99,
			"Overview": 42, "Tags": "not-a-tag-list",
		},
	}
	blob, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.ExecContext(ctx, `
		INSERT INTO series_episodes (library_id, episodes_json, episode_count, fetched_at)
		VALUES (?, ?, ?, ?)`, "legacy-show", string(blob), len(legacy), time.Now().Unix()); err != nil {
		t.Fatal(err)
	}

	got, err := st.GetSeriesEpisodes(ctx, "legacy-show")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Episodes) != 2 {
		t.Fatalf("legacy episodes = %+v, want two playable entries", got.Episodes)
	}
	for _, episode := range got.Episodes {
		if episode.CommunityRating != 0 || len([]rune(episode.Overview)) > 2048 || len(episode.Tags) > 16 {
			t.Fatalf("unsanitized legacy evidence reached cache read: %+v", episode)
		}
	}
	if got.Episodes[1].Overview != "" || len(got.Episodes[1].Tags) != 0 {
		t.Fatalf("malformed legacy text became evidence: %+v", got.Episodes[1])
	}
	lineup := schedule.ComputeDesiredAt(
		schedule.Channel{ID: "ch", Strategy: schedule.Sequential},
		[]schedule.LineupEntry{{
			Key: "series:tmdb:456", Title: "Series",
			EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"}},
		}},
		cachedEpisodeAvailability(got.Episodes), schedule.PodFill, schedule.ChannelPolicy{}, time.Time{},
	)
	if lineup.ProgramCount() != 2 {
		t.Fatalf("malformed legacy evidence selected %d programs, want complete fallback of 2", lineup.ProgramCount())
	}
}

func TestLegacySeriesEpisodeDuplicateEditorialKeysBecomeUnavailable(t *testing.T) {
	tests := []struct {
		name string
		row  string
	}{
		{
			name: "exact duplicates",
			row: `{"LibraryItemID":"duplicate-editorial","Title":"Ordinary","DurationMs":1,"Season":1,"Episode":1,"EpisodeEnd":0,
				"CommunityRating":9.5,"CommunityRating":8.5,
				"Overview":"Christmas story","Overview":"Halloween story",
				"Tags":["christmas"],"Tags":["halloween"]}`,
		},
		{
			name: "case-fold duplicates",
			row: `{"LibraryItemID":"duplicate-editorial","Title":"Ordinary","DurationMs":1,"Season":1,"Episode":1,"EpisodeEnd":0,
				"CommunityRating":9.5,"communityrating":8.5,
				"Overview":"Christmas story","overview":"Halloween story",
				"Tags":["christmas"],"tags":["halloween"]}`,
		},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st := newSQLiteStore(t).(*sqlStore)
			libraryID := fmt.Sprintf("duplicate-editorial-%d", i)
			if _, err := st.db.ExecContext(ctx, `
				INSERT INTO series_episodes (library_id, episodes_json, episode_count, fetched_at)
				VALUES (?, ?, ?, ?)`, libraryID, "["+tt.row+"]", 1, time.Now().Unix()); err != nil {
				t.Fatal(err)
			}

			got, err := st.GetSeriesEpisodes(ctx, libraryID)
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Episodes) != 1 {
				t.Fatalf("legacy episodes = %+v, want one playable entry", got.Episodes)
			}
			episode := got.Episodes[0]
			if episode.CommunityRating != 0 || episode.Overview != "" || len(episode.Tags) != 0 {
				t.Fatalf("duplicate editorial evidence remained available: %+v", episode)
			}
		})
	}
}

func TestLegacySeriesEpisodeMixedTagArrayBecomesUnavailable(t *testing.T) {
	ctx := context.Background()
	st := newSQLiteStore(t).(*sqlStore)
	row := `[{"LibraryItemID":"mixed-tags","Title":"Ordinary","DurationMs":1,` +
		`"Season":1,"Episode":1,"EpisodeEnd":0,"Tags":["christmas",42]}]`
	if _, err := st.db.ExecContext(ctx, `
		INSERT INTO series_episodes (library_id, episodes_json, episode_count, fetched_at)
		VALUES (?, ?, ?, ?)`, "mixed-tags", row, 1, time.Now().Unix()); err != nil {
		t.Fatal(err)
	}

	got, err := st.GetSeriesEpisodes(ctx, "mixed-tags")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Episodes) != 1 {
		t.Fatalf("episodes = %+v, want one playable episode", got.Episodes)
	}
	if len(got.Episodes[0].Tags) != 0 {
		t.Fatalf("partially decoded malformed tags remained available: %v", got.Episodes[0].Tags)
	}
}

func TestLegacySeriesEpisodeRejectsStructurallyUnplayableRows(t *testing.T) {
	ctx := context.Background()
	st := newSQLiteStore(t).(*sqlStore)
	tests := []struct {
		name string
		row  string
		blob string
	}{
		{name: "empty object", row: `{}`},
		{name: "null episode", row: `null`},
		{name: "null cache blob", blob: `null`},
		{name: "missing identity", row: `{"DurationMs":1,"Season":1,"Episode":1,"EpisodeEnd":0}`},
		{name: "null identity", row: `{"LibraryItemID":null,"DurationMs":1,"Season":1,"Episode":1,"EpisodeEnd":0}`},
		{name: "malformed identity", row: `{"LibraryItemID":42,"DurationMs":1,"Season":1,"Episode":1,"EpisodeEnd":0}`},
		{name: "missing runtime", row: `{"LibraryItemID":"ep","Season":1,"Episode":1,"EpisodeEnd":0}`},
		{name: "null runtime", row: `{"LibraryItemID":"ep","DurationMs":null,"Season":1,"Episode":1,"EpisodeEnd":0}`},
		{name: "zero runtime", row: `{"LibraryItemID":"ep","DurationMs":0,"Season":1,"Episode":1,"EpisodeEnd":0}`},
		{name: "negative runtime", row: `{"LibraryItemID":"ep","DurationMs":-1,"Season":1,"Episode":1,"EpisodeEnd":0}`},
		{name: "malformed runtime", row: `{"LibraryItemID":"ep","DurationMs":"long","Season":1,"Episode":1,"EpisodeEnd":0}`},
		{name: "missing season", row: `{"LibraryItemID":"ep","DurationMs":1,"Episode":1,"EpisodeEnd":0}`},
		{name: "null season", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":null,"Episode":1,"EpisodeEnd":0}`},
		{name: "missing episode", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"EpisodeEnd":0}`},
		{name: "null episode", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"Episode":null,"EpisodeEnd":0}`},
		{name: "zero episode", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"Episode":0,"EpisodeEnd":0}`},
		{name: "negative episode", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"Episode":-1,"EpisodeEnd":0}`},
		{name: "malformed episode", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"Episode":"one","EpisodeEnd":0}`},
		{name: "negative season", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":-1,"Episode":1,"EpisodeEnd":0}`},
		{name: "malformed season", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":"one","Episode":1,"EpisodeEnd":0}`},
		{name: "missing episode end", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"Episode":1}`},
		{name: "null episode end", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"Episode":1,"EpisodeEnd":null}`},
		{name: "negative episode end", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"Episode":1,"EpisodeEnd":-1}`},
		{name: "episode end before episode", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"Episode":2,"EpisodeEnd":1}`},
		{name: "malformed episode end", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"Episode":1,"EpisodeEnd":"two"}`},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			libraryID := fmt.Sprintf("unplayable-%d", i)
			blob := tt.blob
			if blob == "" {
				blob = "[" + tt.row + "]"
			}
			if _, err := st.db.ExecContext(ctx, `
				INSERT INTO series_episodes (library_id, episodes_json, episode_count, fetched_at)
				VALUES (?, ?, ?, ?)`, libraryID, blob, 1, time.Now().Unix()); err != nil {
				t.Fatal(err)
			}

			if got, err := st.GetSeriesEpisodes(ctx, libraryID); err == nil {
				t.Fatalf("structurally unplayable cache row was accepted: %+v", got.Episodes)
			}
		})
	}
}

func TestLegacySeriesEpisodeRejectsDuplicateStructuralMembers(t *testing.T) {
	tests := []struct {
		name string
		row  string
	}{
		{name: "exact identity", row: `{"LibraryItemID":"ep-a","LibraryItemID":"ep-b","DurationMs":1,"Season":1,"Episode":1,"EpisodeEnd":1}`},
		{name: "case-fold identity", row: `{"LibraryItemID":"ep-a","libraryitemid":"ep-b","DurationMs":1,"Season":1,"Episode":1,"EpisodeEnd":1}`},
		{name: "exact runtime", row: `{"LibraryItemID":"ep","DurationMs":1,"DurationMs":2,"Season":1,"Episode":1,"EpisodeEnd":1}`},
		{name: "case-fold runtime", row: `{"LibraryItemID":"ep","DurationMs":1,"durationms":2,"Season":1,"Episode":1,"EpisodeEnd":1}`},
		{name: "exact season", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"Season":2,"Episode":1,"EpisodeEnd":1}`},
		{name: "case-fold season", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"season":2,"Episode":1,"EpisodeEnd":1}`},
		{name: "exact episode", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"Episode":1,"Episode":2,"EpisodeEnd":2}`},
		{name: "case-fold episode", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"Episode":1,"episode":2,"EpisodeEnd":2}`},
		{name: "exact episode end", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"Episode":1,"EpisodeEnd":1,"EpisodeEnd":2}`},
		{name: "case-fold episode end", row: `{"LibraryItemID":"ep","DurationMs":1,"Season":1,"Episode":1,"EpisodeEnd":1,"episodeend":2}`},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st := newSQLiteStore(t).(*sqlStore)
			libraryID := fmt.Sprintf("duplicate-structural-%d", i)
			if _, err := st.db.ExecContext(ctx, `
				INSERT INTO series_episodes (library_id, episodes_json, episode_count, fetched_at)
				VALUES (?, ?, ?, ?)`, libraryID, "["+tt.row+"]", 1, time.Now().Unix()); err != nil {
				t.Fatal(err)
			}

			if got, err := st.GetSeriesEpisodes(ctx, libraryID); err == nil {
				t.Fatalf("ambiguous structural cache row was accepted: %+v", got.Episodes)
			}
		})
	}
}

func TestSeriesEpisodeCacheRejectsInvalidDocuments(t *testing.T) {
	for i, tt := range []struct {
		name string
		blob string
	}{
		{name: "empty document", blob: ""},
		{name: "object document", blob: `{}`},
		{name: "scalar document", blob: `"episode"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			st := newSQLiteStore(t).(*sqlStore)
			libraryID := fmt.Sprintf("invalid-document-%d", i)
			if _, err := st.db.ExecContext(ctx, `
				INSERT INTO series_episodes (library_id, episodes_json, episode_count, fetched_at)
				VALUES (?, ?, ?, ?)`, libraryID, tt.blob, 0, time.Now().Unix()); err != nil {
				t.Fatal(err)
			}

			if got, err := st.GetSeriesEpisodes(ctx, libraryID); err == nil {
				t.Fatalf("invalid cache document %q was accepted as an empty series: %+v", tt.blob, got.Episodes)
			}
		})
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
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "taxonomy-upgrade.db")
	dsn := "sqlite://" + path
	s, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	clip := sampleClip("upgrade-clip", "upgrade.mp4", filler.Commercial, 1994, filler.General, "")
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
	// asserted leaf is trustworthy, while its rollup, category shadow, and closure are stale.
	raw, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `DELETE FROM clip_tags WHERE leaf = FALSE;
		UPDATE clips SET category = 'cars' WHERE hash = 'upgrade-clip';
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
	assertSet(t, "healed tags", got.Tags, []string{"cereal", "food"})
}

func TestUnknownSchemeFailsFast(t *testing.T) {
	_, err := Open(context.Background(), "mysql://nope", false)
	if err == nil {
		t.Fatal("expected error for unknown DATABASE_URL scheme")
	}
}
