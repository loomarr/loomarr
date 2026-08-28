package channels

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// The persisted episode cache (§5, §9 series expansion).
//
// These count LIBRARY CALLS rather than assert on returned values, deliberately: the pre-cache
// code returned exactly the same episodes, just slowly. A value-only test passes identically
// with the cache removed and would guard nothing — the same trap that made an earlier TTL test
// vacuous until it was sabotage-checked.

func seedSeries(t *testing.T, st store.Store, key provision.Key, libraryID string) {
	t.Helper()
	rec := provision.Record{
		Key: key, State: provision.Available, LibraryID: libraryID,
		Title:     provision.Title{Name: "Bench Series"},
		UpdatedAt: time.Now(),
	}
	if err := st.UpsertTitle(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
}

func twoEpisodes() []schedule.ResolvedProgram {
	return []schedule.ResolvedProgram{
		{LibraryItemID: "ep-1", Title: "Pilot", DurationMs: 1_320_000, Season: 1, Episode: 1},
		{LibraryItemID: "ep-2", Title: "Second", DurationMs: 1_320_000, Season: 1, Episode: 2},
	}
}

// The first resolve enumerates from the library and WRITES THE ANSWER BACK, so a later
// availability (a restart, a different request) reads the store instead of the media server.
func TestResolveEpisodesPersistsWhatItEnumerated(t *testing.T) {
	st := availTestStore(t)
	key := provision.Key("series:tvdb:71663")
	seedSeries(t, st, key, "show-1")

	calls := 0
	eps := func(_ context.Context, _ string) ([]schedule.ResolvedProgram, error) {
		calls++
		return twoEpisodes(), nil
	}

	av := NewStoreAvailability(context.Background(), st, nil, eps)
	if got := av.ResolveEpisodes(key); len(got.Programs) != 2 {
		t.Fatalf("first resolve = %d eps, want 2", len(got.Programs))
	}
	if calls != 1 {
		t.Fatalf("first resolve made %d library calls, want 1", calls)
	}

	// The cache row must exist, or nothing was persisted and every restart pays full price.
	se, err := st.GetSeriesEpisodes(context.Background(), "show-1")
	if err != nil {
		t.Fatalf("episodes were not persisted: %v", err)
	}
	if len(se.Episodes) != 2 {
		t.Fatalf("persisted %d episodes, want 2", len(se.Episodes))
	}
	if se.FetchedAt.IsZero() {
		t.Fatal("persisted row has no FetchedAt — the refresh job selects on it, so it would never age out")
	}
}

// THE test. A fresh availability (as a later request or a restart gets) must serve from the
// store and never touch the media server. This is the ~232ms that was on the guide's request
// path for a 4-show channel.
func TestResolveEpisodesServesFromTheStoreWithoutTheLibrary(t *testing.T) {
	st := availTestStore(t)
	key := provision.Key("series:tvdb:71663")
	seedSeries(t, st, key, "show-1")

	// Pre-populate exactly as a previous request (or the refresh job) would have.
	if err := st.UpsertSeriesEpisodes(context.Background(), store.SeriesEpisodes{
		LibraryID: "show-1", Episodes: twoEpisodes(), FetchedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	calls := 0
	eps := func(_ context.Context, _ string) ([]schedule.ResolvedProgram, error) {
		calls++
		return twoEpisodes(), nil
	}

	av := NewStoreAvailability(context.Background(), st, nil, eps)
	got := av.ResolveEpisodes(key)
	if len(got.Programs) != 2 {
		t.Fatalf("cached resolve = %d eps, want 2", len(got.Programs))
	}
	if calls != 0 {
		t.Fatalf("a warm cache made %d library calls, want 0 — the enumeration is back on the request path", calls)
	}
}

// A cached EMPTY list is a HIT, not a miss. Otherwise a show with no episodes present
// re-enumerates on every single request — the N+1 the cache exists to remove, surviving in the
// one case nobody would think to check.
func TestResolveEpisodesTreatsACachedEmptyListAsAnAnswer(t *testing.T) {
	st := availTestStore(t)
	key := provision.Key("series:tvdb:71663")
	seedSeries(t, st, key, "show-empty")

	if err := st.UpsertSeriesEpisodes(context.Background(), store.SeriesEpisodes{
		LibraryID: "show-empty", Episodes: nil, FetchedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	calls := 0
	eps := func(_ context.Context, _ string) ([]schedule.ResolvedProgram, error) {
		calls++
		return nil, nil
	}

	av := NewStoreAvailability(context.Background(), st, nil, eps)
	if got := av.ResolveEpisodes(key); len(got.Programs) > 0 {
		t.Fatal("a show with no episodes must not resolve as available")
	}
	if calls != 0 {
		t.Fatalf("a cached EMPTY list made %d library calls, want 0", calls)
	}
}

// The cold path must degrade to TODAY's behaviour, never to an empty channel: an install whose
// cache has not been built yet still gets its episodes, just at the old cost.
func TestResolveEpisodesFallsBackToTheLibraryOnAColdCache(t *testing.T) {
	st := availTestStore(t)
	key := provision.Key("series:tvdb:71663")
	seedSeries(t, st, key, "show-cold")

	eps := func(_ context.Context, _ string) ([]schedule.ResolvedProgram, error) {
		return twoEpisodes(), nil
	}

	av := NewStoreAvailability(context.Background(), st, nil, eps)
	got := av.ResolveEpisodes(key)
	if len(got.Programs) != 2 {
		t.Fatalf("cold cache = %d eps, want 2 — a cold cache must not empty a channel", len(got.Programs))
	}
}

// This is the public availability → ComputeDesiredAt seam. An aged cache still
// preserves safe playable identity when the live library cannot enumerate, but
// never presents its stale editorial evidence as a highlights/holiday subset.
func TestAgedEpisodeCacheFailureUsesCompleteSafeDeckWithoutEditorialSelection(t *testing.T) {
	st := availTestStore(t)
	key := provision.Key("series:tvdb:71663")
	seedSeries(t, st, key, "show-1")
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	episodes := make([]schedule.ResolvedProgram, 0, 8)
	for i := 1; i <= 8; i++ {
		episodes = append(episodes, schedule.ResolvedProgram{
			LibraryItemID:   "ep-" + string(rune('0'+i)),
			Title:           "Episode",
			DurationMs:      22 * time.Minute.Milliseconds(),
			Season:          1,
			Episode:         i,
			OfficialRating:  "TV-G",
			CommunityRating: float64(i),
			Overview:        "Christmas special",
		})
	}
	if err := st.UpsertSeriesEpisodes(context.Background(), store.SeriesEpisodes{
		LibraryID: "show-1", Episodes: episodes, FetchedAt: now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	av := WithEpisodeMaxAge(NewStoreAvailability(context.Background(), st, nil,
		func(context.Context, string) ([]schedule.ResolvedProgram, error) {
			return nil, errors.New("library unavailable")
		}), func() time.Duration { return time.Hour })
	sa := av.(*storeAvailability)
	sa.now = func() time.Time { return now }

	for _, selection := range []schedule.EpisodeSelection{
		{Mode: schedule.EpisodeHighlights},
		{Mode: schedule.EpisodeHoliday, Holidays: []string{"christmas"}},
	} {
		desired := schedule.ComputeDesiredAt(schedule.Channel{ID: "channel", Strategy: schedule.Sequential},
			[]schedule.LineupEntry{{Key: key, Title: "Series", OfficialRating: "TV-G", EpisodeSelection: selection}},
			av, schedule.PodFill, schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
				Audience: schedule.AudiencePolicy{Ceiling: "TV-PG"},
			}}, now)
		var ids []string
		for _, slot := range desired.Slots {
			if slot.IsProgram() {
				ids = append(ids, slot.LibraryItemID)
			}
		}
		want := []string{"ep-1", "ep-2", "ep-3", "ep-4", "ep-5", "ep-6", "ep-7", "ep-8"}
		if !slices.Equal(ids, want) {
			t.Fatalf("%s on failed aged refresh = %v, want full safe deck %v", selection.Mode, ids, want)
		}
	}
}

func TestAgedEmptyEpisodeCacheFailureIsUnavailable(t *testing.T) {
	st := availTestStore(t)
	key := provision.Key("series:tvdb:71663")
	seedSeries(t, st, key, "show-empty")
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	if err := st.UpsertSeriesEpisodes(context.Background(), store.SeriesEpisodes{
		LibraryID: "show-empty", Episodes: nil, FetchedAt: now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	av := WithEpisodeMaxAge(NewStoreAvailability(context.Background(), st, nil,
		func(context.Context, string) ([]schedule.ResolvedProgram, error) { return nil, errors.New("down") }),
		func() time.Duration { return time.Hour })
	sa := av.(*storeAvailability)
	sa.now = func() time.Time { return now }
	if got := av.ResolveEpisodes(key); len(got.Programs) != 0 {
		t.Fatalf("aged empty failed refresh = %+v, want unavailable", got)
	}
}

func TestAgedEpisodeCacheRefreshesAndRestoresEditorialSelection(t *testing.T) {
	st := availTestStore(t)
	key := provision.Key("series:tvdb:71663")
	seedSeries(t, st, key, "show-1")
	now := time.Date(2026, time.August, 28, 12, 0, 0, 0, time.UTC)
	if err := st.UpsertSeriesEpisodes(context.Background(), store.SeriesEpisodes{
		LibraryID: "show-1", Episodes: twoEpisodes(), FetchedAt: now.Add(-2 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	fresh := make([]schedule.ResolvedProgram, 0, 8)
	for i := 1; i <= 8; i++ {
		fresh = append(fresh, schedule.ResolvedProgram{
			LibraryItemID:   "fresh-" + string(rune('0'+i)),
			DurationMs:      (22 * time.Minute).Milliseconds(),
			Season:          1,
			Episode:         i,
			CommunityRating: float64(i),
		})
	}
	av := WithEpisodeMaxAge(NewStoreAvailability(context.Background(), st, nil,
		func(context.Context, string) ([]schedule.ResolvedProgram, error) { return fresh, nil }),
		func() time.Duration { return time.Hour })
	sa := av.(*storeAvailability)
	sa.now = func() time.Time { return now }

	resolution := av.ResolveEpisodes(key)
	if resolution.EditorialUnavailable || len(resolution.Programs) != len(fresh) {
		t.Fatalf("refreshed resolution = %+v, want fresh editorial evidence and %d programs", resolution, len(fresh))
	}
	stored, err := st.GetSeriesEpisodes(context.Background(), "show-1")
	if err != nil || !slices.EqualFunc(stored.Episodes, fresh, func(a, b schedule.ResolvedProgram) bool {
		return a.LibraryItemID == b.LibraryItemID
	}) {
		t.Fatalf("fresh cache was not persisted: episodes=%+v err=%v", stored.Episodes, err)
	}
	desired := schedule.ComputeDesiredAt(schedule.Channel{ID: "channel", Strategy: schedule.Sequential},
		[]schedule.LineupEntry{{Key: key, Title: "Series", EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights}}},
		av, schedule.PodFill, schedule.ChannelPolicy{}, now)
	if got := desired.ProgramCount(); got != 4 {
		t.Fatalf("fresh highlights program count = %d, want 4", got)
	}
}
