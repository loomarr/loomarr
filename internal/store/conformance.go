package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
)

// NewStoreFunc builds a fresh, migrated, empty Store for one test. The SQLite
// and Postgres test files each supply one; RunConformance runs the SAME
// assertions against both (CLAUDE.md: one suite, two backends — never forked).
type NewStoreFunc func(t *testing.T) Store

// RunConformance is the single store conformance suite. Every backend must pass
// it identically. Kept in a non-_test.go file so both backends' test packages
// can call it.
func RunConformance(t *testing.T, newStore NewStoreFunc) {
	t.Run("TitleRoundTrip", func(t *testing.T) { testTitleRoundTrip(t, newStore) })
	t.Run("UpdateTitleProgress", func(t *testing.T) { testUpdateTitleProgress(t, newStore) })
	t.Run("UpsertIsIdempotent", func(t *testing.T) { testUpsertIdempotent(t, newStore) })
	t.Run("GetMissingIsNotFound", func(t *testing.T) { testGetMissing(t, newStore) })
	t.Run("ListByState", func(t *testing.T) { testListByState(t, newStore) })
	t.Run("ClaimDueTitles", func(t *testing.T) { testClaimDue(t, newStore) })
	t.Run("ClaimDueConcurrent", func(t *testing.T) { testClaimConcurrent(t, newStore) })
	t.Run("SettingsKV", func(t *testing.T) { testSettings(t, newStore) })
	t.Run("ChannelRoundTrip", func(t *testing.T) { testChannelRoundTrip(t, newStore) })
	t.Run("ChannelListAndDelete", func(t *testing.T) { testChannelListDelete(t, newStore) })
	t.Run("ChannelIconRoundTrip", func(t *testing.T) { testChannelIconRoundTrip(t, newStore) })
	t.Run("ClaimDueChannels", func(t *testing.T) { testClaimDueChannels(t, newStore) })
	t.Run("ClaimDueChannelsConcurrent", func(t *testing.T) { testClaimChannelsConcurrent(t, newStore) })
	t.Run("JobRoundTrip", func(t *testing.T) { testJobRoundTrip(t, newStore) })
	t.Run("ClaimDueJobs", func(t *testing.T) { testClaimDueJobs(t, newStore) })
	t.Run("ClaimDueJobsConcurrent", func(t *testing.T) { testClaimJobsConcurrent(t, newStore) })
	t.Run("ScheduledJobRoundTrip", func(t *testing.T) { testScheduledJobRoundTrip(t, newStore) })
	t.Run("ClaimDueScheduledJobs", func(t *testing.T) { testClaimDueScheduledJobs(t, newStore) })
	t.Run("ScheduledJobPaused", func(t *testing.T) { testScheduledJobPaused(t, newStore) })
	t.Run("JobCacheByIntentHash", func(t *testing.T) { testJobCacheByHash(t, newStore) })
	t.Run("ProposalRoundTripAndQueues", func(t *testing.T) { testProposalQueues(t, newStore) })
	t.Run("ClipRoundTripAndFilters", func(t *testing.T) { testClipFilters(t, newStore) })
	t.Run("ClipTagsAndPrune", func(t *testing.T) { testClipTagsAndPrune(t, newStore) })
	t.Run("SessionLifecycle", func(t *testing.T) { testSessionLifecycle(t, newStore) })
	t.Run("ClipNameSearch", func(t *testing.T) { testClipNameSearch(t, newStore) })
	t.Run("ClipPlayCounters", func(t *testing.T) { testClipPlayCounters(t, newStore) })
	t.Run("ObservabilityCounts", func(t *testing.T) { testCounts(t, newStore) })
	t.Run("SeriesEpisodesCache", func(t *testing.T) { testSeriesEpisodes(t, newStore) })
	t.Run("AiringHistory", func(t *testing.T) { testAiringHistory(t, newStore) })
	t.Run("ActivityFeed", func(t *testing.T) { testActivityFeed(t, newStore) })
	t.Run("RetentionPurge", func(t *testing.T) { testRetentionPurge(t, newStore) })
	t.Run("FillerSourceRegistry", func(t *testing.T) { testFillerSources(t, newStore) })
	t.Run("SeededDefaultSources", func(t *testing.T) { testSeededDefaultSources(t, newStore) })
	t.Run("FillerPulls", func(t *testing.T) { testFillerPulls(t, newStore) })
	t.Run("ClipLicense", func(t *testing.T) { testClipLicense(t, newStore) })
	t.Run("ClipHeldLifecycle", func(t *testing.T) { testClipHeld(t, newStore) })
	t.Run("SplitProposals", func(t *testing.T) { testSplitProposals(t, newStore) })
}

// testSeriesEpisodes covers the cached episode lists (§5, §9 series expansion) on BOTH backends
// — the upsert-replaces semantics, the empty-vs-missing distinction, and the staleness query
// the refresh job runs.
func testSeriesEpisodes(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	st := newStore(t)

	// A show never enumerated is NOT FOUND — distinct from one cached as empty, below.
	if _, err := st.GetSeriesEpisodes(ctx, "show-unknown"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown show = %v, want ErrNotFound", err)
	}

	fetched := time.Now().Truncate(time.Second)
	eps := []schedule.ResolvedProgram{
		{LibraryItemID: "ep-1", Title: "Pilot", DurationMs: 1_320_000, Season: 1, Episode: 1},
		{LibraryItemID: "ep-2", Title: "Second", DurationMs: 1_320_000, Season: 1, Episode: 2, EpisodeEnd: 3},
	}
	if err := st.UpsertSeriesEpisodes(ctx, SeriesEpisodes{LibraryID: "show-a", Episodes: eps, FetchedAt: fetched}); err != nil {
		t.Fatal(err)
	}

	got, err := st.GetSeriesEpisodes(ctx, "show-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Episodes) != 2 || got.Episodes[0].LibraryItemID != "ep-1" {
		t.Fatalf("round-trip lost episodes: %+v", got.Episodes)
	}
	// EpisodeEnd is the multi-part span (§5) — a field a naive round-trip silently drops.
	if got.Episodes[1].EpisodeEnd != 3 {
		t.Fatalf("EpisodeEnd = %d, want 3 (multi-part span must survive the blob)", got.Episodes[1].EpisodeEnd)
	}
	if !got.FetchedAt.Equal(fetched) {
		t.Fatalf("FetchedAt = %v, want %v", got.FetchedAt, fetched)
	}

	// Upsert REPLACES rather than merging: the library's answer is the truth for that show, so
	// an episode it no longer reports must disappear rather than linger in every lineup.
	if err := st.UpsertSeriesEpisodes(ctx, SeriesEpisodes{
		LibraryID: "show-a",
		Episodes:  []schedule.ResolvedProgram{{LibraryItemID: "ep-1", Title: "Pilot", DurationMs: 1_320_000}},
		FetchedAt: fetched,
	}); err != nil {
		t.Fatal(err)
	}
	if got, err = st.GetSeriesEpisodes(ctx, "show-a"); err != nil || len(got.Episodes) != 1 {
		t.Fatalf("upsert did not replace: %d episodes, err=%v", len(got.Episodes), err)
	}

	// An EMPTY list is a legitimate cached answer ("no episodes present yet") and must read
	// back as a hit, not as a miss — otherwise a genuinely-empty show re-enumerates on every
	// request, which is the N+1 this cache exists to remove.
	if err := st.UpsertSeriesEpisodes(ctx, SeriesEpisodes{LibraryID: "show-empty", FetchedAt: fetched}); err != nil {
		t.Fatal(err)
	}
	empty, err := st.GetSeriesEpisodes(ctx, "show-empty")
	if err != nil {
		t.Fatalf("a cached EMPTY list must be a hit, got %v", err)
	}
	if len(empty.Episodes) != 0 {
		t.Fatalf("empty cache entry returned %d episodes", len(empty.Episodes))
	}

	// Staleness: the refresh job asks for rows older than a cutoff, oldest first.
	old := fetched.Add(-48 * time.Hour)
	if err := st.UpsertSeriesEpisodes(ctx, SeriesEpisodes{LibraryID: "show-old", FetchedAt: old}); err != nil {
		t.Fatal(err)
	}
	stale, err := st.ListStaleSeriesEpisodes(ctx, fetched.Add(-time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0].LibraryID != "show-old" {
		t.Fatalf("stale = %+v, want exactly show-old (the fresh rows must be excluded)", stale)
	}
}

func sampleRecord(key provision.Key, state provision.State, deadline time.Time) provision.Record {
	return provision.Record{
		Key:         key,
		Title:       provision.Title{MediaType: provision.Movie, TMDBID: 1111867, Name: "In Flames", Year: 2023},
		State:       state,
		Deadline:    deadline,
		RequestedAt: time.Unix(1_700_000_000, 0).UTC(),
		UpdatedAt:   time.Unix(1_700_000_000, 0).UTC(),
	}
}

func testTitleRoundTrip(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	want := sampleRecord("movie:tmdb:1111867", provision.Requested, time.Unix(1_800_000_000, 0).UTC())
	if err := s.UpsertTitle(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTitle(ctx, want.Key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != want.Key || got.State != want.State || got.Title.TMDBID != want.Title.TMDBID {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, want)
	}
	if !got.Deadline.Equal(want.Deadline) || !got.RequestedAt.Equal(want.RequestedAt) {
		t.Errorf("epoch time round-trip lost precision: got dl=%v ra=%v", got.Deadline, got.RequestedAt)
	}
}

// testUpdateTitleProgress proves the targeted progress write persists the download fields AND
// leaves the state-machine columns untouched (§18.1 — the poller must not clobber state).
func testUpdateTitleProgress(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	rec := sampleRecord("movie:tmdb:603", provision.Downloading, time.Unix(1_800_000_000, 0).UTC())
	if err := s.UpsertTitle(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateTitleProgress(ctx, rec.Key, 0.42, "00:14:32", "downloading"); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetTitle(ctx, rec.Key)
	if err != nil {
		t.Fatal(err)
	}
	if got.Progress != 0.42 || got.ETAText != "00:14:32" || got.DownloadStatus != "downloading" {
		t.Errorf("progress not persisted: got %+v", got)
	}
	// State-machine fields survive the targeted write.
	if got.State != provision.Downloading || !got.Deadline.Equal(rec.Deadline) {
		t.Errorf("progress write clobbered state/deadline: got state=%s dl=%v", got.State, got.Deadline)
	}
	// Updating with zeros clears it (e.g. an import completed) without touching state.
	if err := s.UpdateTitleProgress(ctx, rec.Key, 0, "", ""); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetTitle(ctx, rec.Key)
	if got.Progress != 0 || got.ETAText != "" || got.State != provision.Downloading {
		t.Errorf("progress reset failed or clobbered state: got %+v", got)
	}
}

func testUpsertIdempotent(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	rec := sampleRecord("movie:tmdb:1", provision.Wanted, time.Time{})
	if err := s.UpsertTitle(ctx, rec); err != nil {
		t.Fatal(err)
	}
	rec.State = provision.Available
	rec.LibraryID = "999"
	if err := s.UpsertTitle(ctx, rec); err != nil {
		t.Fatal(err) // second upsert on same key must update, not error
	}
	got, _ := s.GetTitle(ctx, rec.Key)
	if got.State != provision.Available || got.LibraryID != "999" {
		t.Errorf("upsert didn't overwrite: %+v", got)
	}
	all, _ := s.ListTitlesByState(ctx, provision.Available)
	if len(all) != 1 {
		t.Errorf("upsert created a duplicate row: %d available", len(all))
	}
}

func testGetMissing(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	if _, err := s.GetTitle(context.Background(), "movie:tmdb:404"); err != ErrNotFound {
		t.Errorf("GetTitle(missing) = %v, want ErrNotFound", err)
	}
}

func testListByState(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:10", provision.Requested, time.Time{}))
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:11", provision.Requested, time.Time{}))
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:12", provision.Available, time.Time{}))
	req, err := s.ListTitlesByState(ctx, provision.Requested)
	if err != nil {
		t.Fatal(err)
	}
	if len(req) != 2 {
		t.Errorf("ListTitlesByState(requested) = %d, want 2", len(req))
	}
}

func testClaimDue(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	// Due: requested, deadline in the past.
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:20", provision.Requested, now.Add(-time.Hour)))
	// Not due: deadline in the future.
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:21", provision.Requested, now.Add(time.Hour)))
	// Not eligible: terminal state, even though deadline is past.
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:22", provision.Available, now.Add(-time.Hour)))

	claimed, err := s.ClaimDueTitles(ctx, now, time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].Key != "movie:tmdb:20" {
		t.Errorf("ClaimDueTitles returned %d rows, want just movie:tmdb:20: %+v", len(claimed), claimed)
	}
	// The claimed row is now leased (deadline pushed to now+lease); a second
	// claim at the same `now` must NOT re-return it.
	again, err := s.ClaimDueTitles(ctx, now, time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Errorf("re-claim returned %d leased rows, want 0 (lease not honored)", len(again))
	}
	// The claim leased the row: its stored deadline is now pushed to now+lease.
	got, _ := s.GetTitle(ctx, "movie:tmdb:20")
	if !got.Deadline.Equal(now.Add(time.Minute)) {
		t.Errorf("claimed row deadline = %v, want leased %v", got.Deadline, now.Add(time.Minute))
	}
}

// testClaimConcurrent is the reason ClaimDue is a distinct method (§5): under
// concurrency, each due row is claimed at most once across callers. On SQLite
// (single-writer) this is trivially true; on Postgres it exercises SKIP LOCKED.
func testClaimConcurrent(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	const n = 20
	for i := 0; i < n; i++ {
		key := provision.Key("movie:tmdb:" + string(rune('a'+i%26)) + string(rune('0'+i/26)))
		_ = s.UpsertTitle(ctx, sampleRecord(key, provision.Requested, now.Add(-time.Hour)))
	}

	var mu sync.Mutex
	seen := map[provision.Key]int{}
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				batch, err := s.ClaimDueTitles(ctx, now, time.Minute, 3)
				if err != nil || len(batch) == 0 {
					return
				}
				mu.Lock()
				for _, r := range batch {
					seen[r.Key]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != n {
		t.Errorf("claimed %d distinct rows, want %d", len(seen), n)
	}
	for k, c := range seen {
		if c != 1 {
			t.Errorf("row %s claimed %d times, want exactly 1", k, c)
		}
	}
}

func sampleChannel(id string, number int, deadline time.Time) Channel {
	ch := Channel{}
	ch.ID = id
	ch.IntentRef = "intent-" + id
	ch.Name = "Channel " + id
	ch.Number = number
	ch.Group = "Loomarr"
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusLive
	ch.Shuffle.Seed = 7
	ch.UpdatedAt = 1_700_000_000
	ch.Lineup = []schedule.LineupEntry{
		{Key: "movie:tmdb:1", Title: "A", DurationMs: 3600000},
		{Key: "movie:tmdb:2", Title: "B"},
	}
	ch.Desired = []schedule.Slot{
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:1", LibraryItemID: "lib-1", Title: "A", DurationMs: 3600000},
		{Kind: schedule.SlotFiller, Key: "movie:tmdb:2", Title: "B"},
	}
	// A non-empty policy exercises the policy_json round-trip: an audience ceiling,
	// a separation window (Duration → "168h" JSON), an era range, and an applied
	// relaxation (the enforcement-written field). Every column path must preserve it.
	ch.Policy = schedule.ChannelPolicy{
		ProposalPolicy: schedule.ProposalPolicy{
			Audience:   schedule.AudiencePolicy{Ceiling: "TV-Y7"},
			Separation: schedule.SeparationPolicy{EpisodeNoRepeat: schedule.Duration(168 * time.Hour)},
			Scope:      schedule.ScopePolicy{Era: &schedule.Range{From: 1990, To: 1999}},
			Ordering:   schedule.OrderSyndication,
		},
		Applied: []schedule.AppliedRelaxation{{Kind: "episodeNoRepeat", From: "168h", To: "84h"}},
	}
	ch.ReconcileDeadline = deadline
	return ch
}

func testChannelRoundTrip(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	want := sampleChannel("ch-a", 5, time.Unix(1_800_000_000, 0).UTC())
	if err := s.UpsertChannel(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetChannel(ctx, "ch-a")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != want.ID || got.Number != want.Number || got.Strategy != want.Strategy || got.Status != want.Status {
		t.Errorf("channel scalar round-trip mismatch: got %+v", got.Channel)
	}
	if len(got.Lineup) != 2 || got.Lineup[0].Key != "movie:tmdb:1" || got.Lineup[0].DurationMs != 3600000 {
		t.Errorf("lineup JSON round-trip lost data: %+v", got.Lineup)
	}
	if len(got.Desired) != 2 || got.Desired[0].Kind != schedule.SlotProgram || got.Desired[1].Kind != schedule.SlotFiller {
		t.Errorf("desired JSON round-trip lost data: %+v", got.Desired)
	}
	// Policy round-trip: ceiling, the Duration ("168h") window, the era range, and
	// the applied-relaxation entry must all survive policy_json.
	if got.Policy.Audience.Ceiling != "TV-Y7" {
		t.Errorf("policy ceiling round-trip: got %q", got.Policy.Audience.Ceiling)
	}
	if got.Policy.Separation.EpisodeNoRepeat.Std() != 168*time.Hour {
		t.Errorf("policy duration round-trip: got %v", got.Policy.Separation.EpisodeNoRepeat.Std())
	}
	if got.Policy.Scope.Era == nil || got.Policy.Scope.Era.From != 1990 || got.Policy.Scope.Era.To != 1999 {
		t.Errorf("policy era round-trip: got %+v", got.Policy.Scope.Era)
	}
	if len(got.Policy.Applied) != 1 || got.Policy.Applied[0].Kind != "episodeNoRepeat" {
		t.Errorf("policy applied-relaxation round-trip: got %+v", got.Policy.Applied)
	}
	if !got.ReconcileDeadline.Equal(want.ReconcileDeadline) {
		t.Errorf("reconcile deadline round-trip: got %v want %v", got.ReconcileDeadline, want.ReconcileDeadline)
	}
	// Upsert is idempotent: a second write with an edited field updates in place.
	want.Status = schedule.StatusDrifted
	if err := s.UpsertChannel(ctx, want); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.GetChannel(ctx, "ch-a")
	if got2.Status != schedule.StatusDrifted {
		t.Errorf("upsert didn't update status: %s", got2.Status)
	}
	all, _ := s.ListChannels(ctx)
	if len(all) != 1 {
		t.Errorf("upsert created a duplicate channel: %d rows", len(all))
	}
	// GetChannelByNumber resolves the same row.
	byNum, err := s.GetChannelByNumber(ctx, 5)
	if err != nil || byNum.ID != "ch-a" {
		t.Errorf("GetChannelByNumber(5) = %q,%v want ch-a", byNum.ID, err)
	}
	// Missing lookups are ErrNotFound.
	if _, err := s.GetChannel(ctx, "nope"); err != ErrNotFound {
		t.Errorf("GetChannel(missing) = %v, want ErrNotFound", err)
	}
	if _, err := s.GetChannelByNumber(ctx, 999); err != ErrNotFound {
		t.Errorf("GetChannelByNumber(missing) = %v, want ErrNotFound", err)
	}
}

func testChannelListDelete(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	_ = s.UpsertChannel(ctx, sampleChannel("ch-2", 2, time.Time{}))
	_ = s.UpsertChannel(ctx, sampleChannel("ch-1", 1, time.Time{}))
	_ = s.UpsertChannel(ctx, sampleChannel("ch-3", 3, time.Time{}))

	all, err := s.ListChannels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("ListChannels = %d, want 3", len(all))
	}
	// Ordered by number.
	if all[0].Number != 1 || all[1].Number != 2 || all[2].Number != 3 {
		t.Errorf("ListChannels not ordered by number: %d,%d,%d", all[0].Number, all[1].Number, all[2].Number)
	}
	if err := s.DeleteChannel(ctx, "ch-2"); err != nil {
		t.Fatal(err)
	}
	after, _ := s.ListChannels(ctx)
	if len(after) != 2 {
		t.Errorf("after delete: %d channels, want 2", len(after))
	}
	if _, err := s.GetChannel(ctx, "ch-2"); err != ErrNotFound {
		t.Errorf("deleted channel still present: %v", err)
	}
}

// testChannelIconRoundTrip covers the upload-icon blob store: put → get, replace-on-reput,
// missing → ok=false, and delete-channel cleans up the icon (no orphaned blob).
func testChannelIconRoundTrip(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	_ = s.UpsertChannel(ctx, sampleChannel("ch-ico", 7, time.Time{}))

	// Missing → ok=false.
	if _, _, _, ok, err := s.GetChannelIcon(ctx, "ch-ico"); err != nil || ok {
		t.Fatalf("no icon yet: ok=%v err=%v, want ok=false/nil", ok, err)
	}

	// Put → get round-trips bytes + content type.
	png := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 1, 2, 3}
	at := time.Unix(1_700_000_000, 0)
	if err := s.PutChannelIcon(ctx, "ch-ico", "image/png", png, at); err != nil {
		t.Fatal(err)
	}
	ct, data, gotAt, ok, err := s.GetChannelIcon(ctx, "ch-ico")
	if err != nil || !ok {
		t.Fatalf("get after put: ok=%v err=%v", ok, err)
	}
	if ct != "image/png" || !bytes.Equal(data, png) || gotAt.Unix() != at.Unix() {
		t.Errorf("round-trip mismatch: ct=%q len=%d at=%d", ct, len(data), gotAt.Unix())
	}

	// Replace on re-put (one row per channel PK).
	jpg := []byte{0xFF, 0xD8, 0xFF, 9, 9}
	if err := s.PutChannelIcon(ctx, "ch-ico", "image/jpeg", jpg, at.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	ct2, data2, _, _, _ := s.GetChannelIcon(ctx, "ch-ico")
	if ct2 != "image/jpeg" || !bytes.Equal(data2, jpg) {
		t.Errorf("replace didn't take: ct=%q len=%d", ct2, len(data2))
	}

	// Deleting the channel drops the icon (no orphan).
	if err := s.DeleteChannel(ctx, "ch-ico"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, ok, _ := s.GetChannelIcon(ctx, "ch-ico"); ok {
		t.Error("icon survived channel delete (orphaned blob)")
	}
}

func testClaimDueChannels(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	// Due: deadline in the past, live.
	_ = s.UpsertChannel(ctx, sampleChannel("ch-due", 1, now.Add(-time.Hour)))
	// Not due: future deadline.
	_ = s.UpsertChannel(ctx, sampleChannel("ch-future", 2, now.Add(time.Hour)))
	// Not eligible: detached, even with a past deadline.
	detached := sampleChannel("ch-detached", 3, now.Add(-time.Hour))
	detached.Status = schedule.StatusDetached
	_ = s.UpsertChannel(ctx, detached)

	claimed, err := s.ClaimDueChannels(ctx, now, time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].ID != "ch-due" {
		t.Fatalf("ClaimDueChannels = %d rows, want just ch-due: %+v", len(claimed), claimed)
	}
	// Leased: a second claim at the same now returns nothing.
	again, _ := s.ClaimDueChannels(ctx, now, time.Minute, 10)
	if len(again) != 0 {
		t.Errorf("re-claim returned %d leased channels, want 0", len(again))
	}
	got, _ := s.GetChannel(ctx, "ch-due")
	if !got.ReconcileDeadline.Equal(now.Add(time.Minute)) {
		t.Errorf("claimed channel deadline = %v, want leased %v", got.ReconcileDeadline, now.Add(time.Minute))
	}
}

// testClaimChannelsConcurrent is the §18 guarantee: two replicas never reconcile
// the same channel. On Postgres it exercises FOR UPDATE SKIP LOCKED.
func testClaimChannelsConcurrent(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	const n = 20
	for i := 0; i < n; i++ {
		_ = s.UpsertChannel(ctx, sampleChannel("chan-"+string(rune('a'+i)), i+1, now.Add(-time.Hour)))
	}

	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				batch, err := s.ClaimDueChannels(ctx, now, time.Minute, 3)
				if err != nil || len(batch) == 0 {
					return
				}
				mu.Lock()
				for _, c := range batch {
					seen[c.ID]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(seen) != n {
		t.Errorf("claimed %d distinct channels, want %d", len(seen), n)
	}
	for id, c := range seen {
		if c != 1 {
			t.Errorf("channel %s claimed %d times, want exactly 1", id, c)
		}
	}
}

func sampleJob(id, hash string, deadline, createdAt time.Time) Job {
	return Job{
		ID: id, Kind: "suggest", Status: "queued",
		IntentJSON: `{"description":"90s action"}`, IntentHash: hash,
		CreatedBy: "user-1", Deadline: deadline, CreatedAt: createdAt, UpdatedAt: createdAt,
	}
}

func testJobRoundTrip(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	want := sampleJob("job-1", "hash-abc", now, now)
	if err := s.CreateJob(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetJob(ctx, "job-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "queued" || got.IntentHash != "hash-abc" || got.CreatedBy != "user-1" {
		t.Errorf("job round-trip mismatch: %+v", got)
	}
	// Update transitions status.
	got.Status = "done"
	got.UpdatedAt = now
	if err := s.UpdateJob(ctx, got); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetJob(ctx, "job-1")
	if after.Status != "done" {
		t.Errorf("update didn't persist status: %s", after.Status)
	}
	if _, err := s.GetJob(ctx, "nope"); err != ErrNotFound {
		t.Errorf("GetJob(missing) = %v, want ErrNotFound", err)
	}
}

func testClaimDueJobs(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	_ = s.CreateJob(ctx, sampleJob("due", "h1", now.Add(-time.Hour), now))
	future := sampleJob("future", "h2", now.Add(time.Hour), now)
	_ = s.CreateJob(ctx, future)
	running := sampleJob("running", "h3", now.Add(-time.Hour), now)
	running.Status = "running"
	_ = s.CreateJob(ctx, running)

	claimed, err := s.ClaimDueJobs(ctx, now, time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].ID != "due" {
		t.Fatalf("ClaimDueJobs = %d, want just 'due': %+v", len(claimed), claimed)
	}
	// Leased: second claim returns nothing.
	again, _ := s.ClaimDueJobs(ctx, now, time.Minute, 10)
	if len(again) != 0 {
		t.Errorf("re-claim returned %d leased jobs, want 0", len(again))
	}
}

// testScheduledJobRoundTrip: upsert creates then updates a job's state row; list + get read
// it back; a missing row is ErrNotFound.
func testScheduledJobRoundTrip(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()

	if _, err := s.GetScheduledJob(ctx, "nope"); err != ErrNotFound {
		t.Errorf("missing scheduled job = %v, want ErrNotFound", err)
	}
	if err := s.UpsertScheduledJob(ctx, ScheduledJob{Name: "reconcile", NextRun: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	// Update in place (same name) — last_result + next_run change.
	next := now.Add(5 * time.Minute)
	if err := s.UpsertScheduledJob(ctx, ScheduledJob{
		Name: "reconcile", LastRun: now, LastResult: "ok", NextRun: next, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetScheduledJob(ctx, "reconcile")
	if err != nil {
		t.Fatal(err)
	}
	if got.LastResult != "ok" || !got.NextRun.Equal(next) || !got.LastRun.Equal(now) {
		t.Errorf("round-tripped scheduled job = %+v, want ok/next=%v/last=%v", got, next, now)
	}
	all, _ := s.ListScheduledJobs(ctx)
	if len(all) != 1 || all[0].Name != "reconcile" {
		t.Errorf("list = %+v, want one 'reconcile'", all)
	}
}

// testClaimDueScheduledJobs: only due rows (next_run <= now) are claimed, and claiming leases
// next_run forward so a second claim returns nothing until rescheduled.
func testClaimDueScheduledJobs(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	_ = s.UpsertScheduledJob(ctx, ScheduledJob{Name: "due", NextRun: now.Add(-time.Minute), UpdatedAt: now})
	_ = s.UpsertScheduledJob(ctx, ScheduledJob{Name: "future", NextRun: now.Add(time.Hour), UpdatedAt: now})

	claimed, err := s.ClaimDueScheduledJobs(ctx, now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(claimed) != 1 || claimed[0].Name != "due" {
		t.Fatalf("ClaimDueScheduledJobs = %d, want just 'due': %+v", len(claimed), claimed)
	}
	// Leased forward → an immediate re-claim returns nothing.
	again, _ := s.ClaimDueScheduledJobs(ctx, now, time.Minute)
	if len(again) != 0 {
		t.Errorf("re-claim returned %d leased jobs, want 0", len(again))
	}
}

func testClaimJobsConcurrent(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	const n = 20
	for i := 0; i < n; i++ {
		_ = s.CreateJob(ctx, sampleJob("job-"+string(rune('a'+i)), "h", now.Add(-time.Hour), now))
	}
	var mu sync.Mutex
	seen := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < 4; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				batch, err := s.ClaimDueJobs(ctx, now, time.Minute, 3)
				if err != nil || len(batch) == 0 {
					return
				}
				mu.Lock()
				for _, j := range batch {
					seen[j.ID]++
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(seen) != n {
		t.Errorf("claimed %d distinct jobs, want %d", len(seen), n)
	}
	for id, c := range seen {
		if c != 1 {
			t.Errorf("job %s claimed %d times, want 1", id, c)
		}
	}
}

func testJobCacheByHash(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	_ = s.CreateJob(ctx, sampleJob("cached", "hash-X", now, now))

	// A search within TTL finds it.
	got, err := s.FindJobByIntentHash(ctx, "hash-X", now.Add(-24*time.Hour))
	if err != nil || got.ID != "cached" {
		t.Fatalf("FindJobByIntentHash = %q,%v want cached", got.ID, err)
	}
	// A search with `since` after the job's creation misses (TTL expired).
	if _, err := s.FindJobByIntentHash(ctx, "hash-X", now.Add(time.Hour)); err != ErrNotFound {
		t.Errorf("expired cache lookup = %v, want ErrNotFound", err)
	}
	// A different hash misses.
	if _, err := s.FindJobByIntentHash(ctx, "hash-other", now.Add(-24*time.Hour)); err != ErrNotFound {
		t.Errorf("miss lookup = %v, want ErrNotFound", err)
	}
}

func testProposalQueues(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	mk := func(id, status, creator string) Proposal {
		return Proposal{ID: id, JobID: "job-1", Status: status, CreatedBy: creator,
			ProposalJSON: `{"lineup":[]}`, CreatedAt: now, UpdatedAt: now}
	}
	_ = s.CreateProposal(ctx, mk("p1", "submitted", "alice"))
	_ = s.CreateProposal(ctx, mk("p2", "submitted", "bob"))
	_ = s.CreateProposal(ctx, mk("p3", "approved", "alice"))

	// The approval queue = submitted proposals.
	sub, err := s.ListProposalsByStatus(ctx, "submitted")
	if err != nil {
		t.Fatal(err)
	}
	if len(sub) != 2 {
		t.Errorf("submitted queue = %d, want 2", len(sub))
	}
	// My proposals = by creator.
	aliceProps, _ := s.ListProposalsByCreator(ctx, "alice")
	if len(aliceProps) != 2 {
		t.Errorf("alice's proposals = %d, want 2", len(aliceProps))
	}
	// Approve p1: status + approved_by persist (survives restart — it's in the store).
	p1, _ := s.GetProposal(ctx, "p1")
	p1.Status = "approved"
	p1.ApprovedBy = "admin"
	p1.UpdatedAt = now
	if err := s.UpdateProposal(ctx, p1); err != nil {
		t.Fatal(err)
	}
	after, _ := s.GetProposal(ctx, "p1")
	if after.Status != "approved" || after.ApprovedBy != "admin" {
		t.Errorf("approve didn't persist: %+v", after)
	}
	if _, err := s.GetProposal(ctx, "missing"); err != ErrNotFound {
		t.Errorf("GetProposal(missing) = %v, want ErrNotFound", err)
	}
}

func sampleClip(id, name string, kind filler.Kind, era int, aud filler.Audience, cat string) Clip {
	c := Clip{}
	// ⚠ Identity is the HASH since V38c, not the path (§10).
	//
	// These tests use the READABLE id as the hash — "c1", not 64 hex characters — so assertions
	// stay legible (`GetClip(ctx, "c1")`) and a failure names a clip a human recognises. The
	// store does not care what a hash looks like; only `filler.ClipPath` validates the shape, and
	// that is covered where it belongs, in `filler/clippath_test.go`. Using real hashes here
	// would make every assertion a wall of hex and would test nothing extra.
	c.Hash = id
	c.Path = id
	c.TunarrProgramID = "tun-" + id
	c.Name = name
	c.Kind = kind
	c.Era = era
	c.Audience = aud
	c.Category = cat
	c.DurationMs = 30000
	c.Source = "archive"
	c.UpdatedAt = time.Unix(1_700_000_000, 0).UTC()
	return c
}

func testClipFilters(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	_ = s.UpsertClip(ctx, sampleClip("c1", "Frosted Flakes", filler.Commercial, 1992, filler.Kids, "cereal"))
	_ = s.UpsertClip(ctx, sampleClip("c2", "TMNT figures", filler.Commercial, 1994, filler.Kids, "toys"))
	_ = s.UpsertClip(ctx, sampleClip("b1", "Bumper", filler.Bumper, 1992, filler.General, ""))
	_ = s.UpsertClip(ctx, sampleClip("u1", "untagged.mp4", filler.Commercial, 0, "", "")) // untagged

	// Round-trip.
	got, err := s.GetClip(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Frosted Flakes" || got.Kind != filler.Commercial || got.Era != 1992 || got.Audience != filler.Kids || got.Category != "cereal" {
		t.Errorf("clip round-trip mismatch: %+v", got.Clip)
	}
	if got.DurationMs != 30000 {
		t.Errorf("duration lost: %d", got.DurationMs)
	}
	if _, err := s.GetClip(ctx, "nope"); err != ErrNotFound {
		t.Errorf("GetClip(missing) = %v, want ErrNotFound", err)
	}

	// Filter by kind.
	comms, _ := s.ListClips(ctx, ClipFilter{Kind: filler.Commercial})
	if len(comms) != 3 {
		t.Errorf("kind=commercial = %d, want 3", len(comms))
	}
	// Filter by audience + era.
	kids92, _ := s.ListClips(ctx, ClipFilter{Audience: filler.Kids, Era: 1992})
	if len(kids92) != 1 || kids92[0].Path != "c1" {
		t.Errorf("kids+1992 = %+v, want just c1", ids2(kids92))
	}
	// Untagged only.
	untagged, _ := s.ListClips(ctx, ClipFilter{UntaggedOnly: true})
	if len(untagged) != 1 || untagged[0].Path != "u1" {
		t.Errorf("untagged = %+v, want just u1", ids2(untagged))
	}
	// Empty filter = all.
	all, _ := s.ListClips(ctx, ClipFilter{})
	if len(all) != 4 {
		t.Errorf("no filter = %d, want 4", len(all))
	}
}

func testClipTagsAndPrune(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	_ = s.UpsertClip(ctx, sampleClip("u1", "untagged.mp4", filler.Commercial, 0, "", ""))
	_ = s.UpsertClip(ctx, sampleClip("keep", "keep.mp4", filler.Bumper, 1992, filler.General, ""))

	// Tag the untagged clip (the AI-tagging job path).
	if err := s.UpdateClipTags(ctx, "u1", 1994, "kids", "cereal", 0, true, now); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetClip(ctx, "u1")
	if got.Era != 1994 || got.Audience != filler.Kids || got.Category != "cereal" || !got.AITagged {
		t.Errorf("tag update didn't persist: %+v", got.Clip)
	}
	if !got.Tagged() {
		t.Error("clip should be Tagged() after update")
	}
	// Tagging a missing clip → ErrNotFound.
	if err := s.UpdateClipTags(ctx, "gone", 1990, "kids", "toys", 0, false, now); err != ErrNotFound {
		t.Errorf("UpdateClipTags(missing) = %v, want ErrNotFound", err)
	}

	// Era suggestions (§10 V34) — the conditional suggested_era write:
	//  **record** an ungrounded suggestion on an era-less clip,
	//  **keep** it across a tag edit that carries neither era nor suggestion,
	//  **clear** it in the same write that sets era (the operator confirming).
	if err := s.UpdateClipTags(ctx, "keep", 0, "family", "", 1985, false, now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, "keep")
	if got.SuggestedEra != 1985 || got.Era != 0 {
		t.Errorf("suggestion not recorded: era=%d suggestedEra=%d, want 0/1985", got.Era, got.SuggestedEra)
	}
	if err := s.UpdateClipTags(ctx, "keep", 0, "general", "", 0, false, now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, "keep")
	if got.SuggestedEra != 1985 {
		t.Errorf("era-less tag edit wiped the suggestion: suggestedEra=%d, want 1985", got.SuggestedEra)
	}
	if err := s.UpdateClipTags(ctx, "keep", 1985, "", "", 0, false, now); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, "keep")
	if got.Era != 1985 || got.SuggestedEra != 0 {
		t.Errorf("confirming era did not clear the suggestion: era=%d suggestedEra=%d, want 1985/0", got.Era, got.SuggestedEra)
	}
	// A suggestion survives a sync upsert (sync.go merges it like the other tags).
	keep, _ := s.GetClip(ctx, "keep")
	keep.SuggestedEra = 1990
	if err := s.UpsertClip(ctx, keep); err != nil {
		t.Fatal(err)
	}
	got, _ = s.GetClip(ctx, "keep")
	if got.SuggestedEra != 1990 {
		t.Errorf("suggested_era did not round-trip through UpsertClip: %d, want 1990", got.SuggestedEra)
	}

	// Prune: keep only "keep" — u1 is removed (it left the media server's library).
	n, err := s.DeleteClipsNotIn(ctx, []string{"keep"})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("prune removed %d, want 1", n)
	}
	if _, err := s.GetClip(ctx, "u1"); err != ErrNotFound {
		t.Error("pruned clip still present")
	}
	if _, err := s.GetClip(ctx, "keep"); err != nil {
		t.Error("kept clip was wrongly pruned")
	}
	// Prune with empty keep set deletes all.
	n, _ = s.DeleteClipsNotIn(ctx, nil)
	if n != 1 {
		t.Errorf("prune-all removed %d, want 1", n)
	}
}

func ids2(clips []Clip) []string {
	out := make([]string, len(clips))
	for i, c := range clips {
		out[i] = c.Path
	}
	return out
}

func testSettings(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	if _, err := s.GetSetting(ctx, "instance_id"); err != ErrNotFound {
		t.Errorf("GetSetting(missing) = %v, want ErrNotFound", err)
	}
	if err := s.SetSetting(ctx, "instance_id", "abc123"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting(ctx, "instance_id", "def456"); err != nil {
		t.Fatal(err) // upsert
	}
	v, err := s.GetSetting(ctx, "instance_id")
	if err != nil || v != "def456" {
		t.Errorf("GetSetting = %q,%v want def456,nil", v, err)
	}

	// --- audited overrides (config-design §3): ListSettings + UpsertSetting +
	// DeleteSetting round-trip with audit metadata, one suite both backends. ---
	when := time.Unix(1_700_000_000, 0).UTC()
	if err := s.UpsertSetting(ctx, SettingRow{Key: "library.url", Value: "http://emby:8096", UpdatedAt: when, UpdatedBy: "matt"}); err != nil {
		t.Fatal(err)
	}
	// A system write (SetSetting) leaves updated_by NULL — surfaced as "".
	if err := s.SetSetting(ctx, "llm.model", "qwen3:8b"); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ListSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]SettingRow{}
	for _, r := range rows {
		got[r.Key] = r
	}
	if r := got["library.url"]; r.Value != "http://emby:8096" || r.UpdatedBy != "matt" || !r.UpdatedAt.Equal(when) {
		t.Errorf("audited override: %+v want value/matt/%v", r, when)
	}
	if r := got["llm.model"]; r.Value != "qwen3:8b" || r.UpdatedBy != "" {
		t.Errorf("system write should have empty updated_by: %+v", r)
	}
	// Upsert overwrites value + audit.
	later := time.Unix(1_700_000_100, 0).UTC()
	if err := s.UpsertSetting(ctx, SettingRow{Key: "library.url", Value: "http://emby:9096", UpdatedAt: later, UpdatedBy: "ana"}); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ListSettings(ctx)
	for _, r := range rows {
		if r.Key == "library.url" && (r.Value != "http://emby:9096" || r.UpdatedBy != "ana") {
			t.Errorf("upsert did not overwrite audit: %+v", r)
		}
	}
	// --- the env-override claim (config-design §3.1), one suite both backends ---
	//
	// ⚠ The bug this guards: env_override must NOT be in UpsertSetting's DO UPDATE list.
	// If it were, an ordinary save would silently re-lock a key the operator had just
	// unlocked — at the exact moment they are certain to be editing it.
	if err := s.SetSettingEnvOverride(ctx, "library.url", true, "http://seed:8096", "matt"); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ListSettings(ctx)
	got = map[string]SettingRow{}
	for _, r := range rows {
		got[r.Key] = r
	}
	if r := got["library.url"]; !r.EnvOverride {
		t.Errorf("claim not persisted: %+v", r)
	}
	// A plain value save must leave the claim standing.
	if err := s.UpsertSetting(ctx, SettingRow{Key: "library.url", Value: "http://edited:8096", UpdatedAt: later, UpdatedBy: "ana"}); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ListSettings(ctx)
	for _, r := range rows {
		if r.Key == "library.url" {
			if !r.EnvOverride {
				t.Error("an ordinary save cleared the env-override claim — the key silently re-locked")
			}
			if r.Value != "http://edited:8096" {
				t.Errorf("value did not save alongside the claim: %+v", r)
			}
		}
	}
	// Seeding only applies when the row is absent; an existing value is never clobbered.
	if err := s.SetSettingEnvOverride(ctx, "library.url", true, "http://should-not-apply:8096", "matt"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.GetSetting(ctx, "library.url"); v != "http://edited:8096" {
		t.Errorf("re-claiming overwrote the stored value with the seed: %q", v)
	}
	// Handing the key back keeps the value, so the round trip loses nothing.
	if err := s.SetSettingEnvOverride(ctx, "library.url", false, "", "matt"); err != nil {
		t.Fatal(err)
	}
	rows, _ = s.ListSettings(ctx)
	for _, r := range rows {
		if r.Key == "library.url" {
			if r.EnvOverride {
				t.Error("claim not cleared on re-lock")
			}
			if r.Value != "http://edited:8096" {
				t.Errorf("re-lock discarded the stored value: %+v", r)
			}
		}
	}
	// Claiming a key with NO existing row creates one carrying the seed — the ordinary
	// unlock case, since an env-pinned key usually has nothing stored.
	if err := s.SetSettingEnvOverride(ctx, "seerr.url", true, "http://seerr:5055", "matt"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.GetSetting(ctx, "seerr.url"); err != nil || v != "http://seerr:5055" {
		t.Errorf("unlock did not seed a new row: %q %v", v, err)
	}

	// Delete reverts the key (config-design §9).
	if err := s.DeleteSetting(ctx, "library.url"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSetting(ctx, "library.url"); err != ErrNotFound {
		t.Errorf("after delete, GetSetting = %v want ErrNotFound", err)
	}
}

// testSessionLifecycle covers the §11 session rules that authentication depends on:
// sessions are revocable rows, expiry is immediate for reads even though purging is
// eventual, and disabling a user kills every session at once. None of this had store
// conformance coverage before — it is the one area where a dialect difference would be
// a security bug rather than a correctness bug, so it belongs in the shared suite.
func testSessionLifecycle(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	if err := s.UpsertUser(ctx, User{ID: "u1", Name: "Ada", Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUser(ctx, User{ID: "u2", Name: "Grace", Role: RoleMember}); err != nil {
		t.Fatal(err)
	}

	live := Session{TokenHash: "h-live", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	older := Session{TokenHash: "h-older", UserID: "u1", CreatedAt: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}
	dead := Session{TokenHash: "h-dead", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(-time.Minute)}
	other := Session{TokenHash: "h-other", UserID: "u2", CreatedAt: now, ExpiresAt: now.Add(time.Hour)}
	for _, sess := range []Session{live, older, dead, other} {
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.ListSessionsForUser(ctx, "u1", now)
	if err != nil {
		t.Fatal(err)
	}
	// Expired excluded, another user's session excluded, newest first.
	if len(got) != 2 {
		t.Fatalf("ListSessionsForUser = %d sessions, want 2 (expired and other-user excluded)", len(got))
	}
	if got[0].TokenHash != "h-live" || got[1].TokenHash != "h-older" {
		t.Errorf("order = [%s %s], want newest first [h-live h-older]", got[0].TokenHash, got[1].TokenHash)
	}
	if !got[0].CreatedAt.Equal(now) || !got[0].ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Errorf("timestamps did not round-trip: created=%v expires=%v", got[0].CreatedAt, got[0].ExpiresAt)
	}

	// Revoking one leaves the rest — the admin "sign out this device" path.
	if err := s.RevokeSession(ctx, "h-live"); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.ListSessionsForUser(ctx, "u1", now); len(got) != 1 {
		t.Errorf("after RevokeSession: %d sessions, want 1", len(got))
	}

	// Disabling a user kills their sessions immediately (§11) — and only theirs.
	if err := s.RevokeSessionsForUser(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.ListSessionsForUser(ctx, "u1", now); len(got) != 0 {
		t.Errorf("after RevokeSessionsForUser: %d sessions, want 0", len(got))
	}
	if got, _ = s.ListSessionsForUser(ctx, "u2", now); len(got) != 1 {
		t.Errorf("RevokeSessionsForUser hit another user's sessions: u2 has %d, want 1", len(got))
	}
}

// testClipNameSearch covers the §7.2 `name LIKE` clip search. It is in the shared suite
// because the two dialects disagree by default: SQLite's LIKE folds ASCII case while
// Postgres's does not, so a naive implementation would make search case-sensitive on
// exactly one backend — the dialect fork the store rules forbid.
func testClipNameSearch(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	_ = s.UpsertClip(ctx, sampleClip("c1", "Frosted Flakes", filler.Commercial, 1992, filler.Kids, "cereal"))
	_ = s.UpsertClip(ctx, sampleClip("c2", "TMNT figures", filler.Commercial, 1994, filler.Kids, "toys"))
	_ = s.UpsertClip(ctx, sampleClip("c3", "100% Juice", filler.Commercial, 1993, filler.Kids, "drinks"))

	names := func(f ClipFilter) []string {
		t.Helper()
		got, err := s.ListClips(ctx, f)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]string, 0, len(got))
		for _, c := range got {
			out = append(out, c.Name)
		}
		return out
	}

	// Substring, and case-insensitive on BOTH backends.
	if got := names(ClipFilter{Query: "flakes"}); len(got) != 1 || got[0] != "Frosted Flakes" {
		t.Errorf("Query=flakes → %v, want [Frosted Flakes] (case-insensitive on both dialects)", got)
	}
	if got := names(ClipFilter{Query: "FROSTED"}); len(got) != 1 {
		t.Errorf("Query=FROSTED → %v, want 1 match", got)
	}

	// A literal % must not act as a wildcard. Without escaping this returns everything,
	// which reads as "search is broken" and scans the whole table.
	if got := names(ClipFilter{Query: "%"}); len(got) != 1 || got[0] != "100% Juice" {
		t.Errorf("Query=%% → %v, want only [100%% Juice] — %% must be literal, not a wildcard", got)
	}
	// Likewise _, which would otherwise match any single character.
	if got := names(ClipFilter{Query: "_"}); len(got) != 0 {
		t.Errorf("Query=_ → %v, want none — _ must be literal, not a single-char wildcard", got)
	}

	// Search composes with the other filters rather than replacing them.
	if got := names(ClipFilter{Query: "e", Category: "toys"}); len(got) != 1 || got[0] != "TMNT figures" {
		t.Errorf("Query+Category → %v, want [TMNT figures]", got)
	}
}

// testCounts covers the §17 observability gauges: grouped counts must reflect
// the rows present, and (for sessions) honor the same expiry predicate the read
// path uses. Same assertions on both backends (one suite, two dialects).
func testCounts(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_700_000_000, 0).UTC()

	// Titles across three states: two requested, one available.
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:20", provision.Requested, time.Time{}))
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:21", provision.Requested, time.Time{}))
	_ = s.UpsertTitle(ctx, sampleRecord("movie:tmdb:22", provision.Available, time.Time{}))

	titles, err := s.CountTitlesByState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if titles[provision.Requested] != 2 || titles[provision.Available] != 1 {
		t.Errorf("CountTitlesByState = %v, want requested:2 available:1", titles)
	}
	// A state with no rows is absent, not zero — the collector zero-fills it.
	if _, ok := titles[provision.Downloading]; ok {
		t.Errorf("CountTitlesByState included an empty state: %v", titles)
	}

	// Jobs: two queued (sampleJob defaults to queued), one flipped to running.
	_ = s.CreateJob(ctx, sampleJob("job-1", "h1", now, now))
	_ = s.CreateJob(ctx, sampleJob("job-2", "h2", now, now))
	running := sampleJob("job-3", "h3", now, now)
	running.Status = "running"
	_ = s.CreateJob(ctx, running)

	jobs, err := s.CountJobsByStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if jobs["queued"] != 2 || jobs["running"] != 1 {
		t.Errorf("CountJobsByStatus = %v, want queued:2 running:1", jobs)
	}

	// Sessions: two live, one expired — only the live ones count as of now.
	if err := s.UpsertUser(ctx, User{ID: "u1", Name: "Ada", Role: RoleAdmin}); err != nil {
		t.Fatal(err)
	}
	for _, sess := range []Session{
		{TokenHash: "c-live1", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{TokenHash: "c-live2", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		{TokenHash: "c-dead", UserID: "u1", CreatedAt: now, ExpiresAt: now.Add(-time.Minute)},
	} {
		if err := s.CreateSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
	}
	active, err := s.CountActiveSessions(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if active != 2 {
		t.Errorf("CountActiveSessions = %d, want 2 (expired excluded)", active)
	}
}

// V28: play counters are written ONLY by RecordClipPlay, and a re-sync must not reset them.
//
// The reset is the bug worth pinning: UpsertClip lists most columns in its ON CONFLICT DO
// UPDATE, so adding play_count there would zero every counter on each sync pass — silently,
// and visible only as "usage never goes up".
func testClipPlayCounters(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()

	c := sampleClip("1994/toys.mp4", "TMNT toys", filler.Commercial, 1994, filler.Kids, "toys")
	c.Thumbnail = "1994/toys.jpg"
	// ⚠ The animated preview is a SEPARATE column, not derived from the still (V39, 00035). Both
	// are asserted here because a column added to the INSERT and forgotten in the SELECT (or
	// vice versa) is a silent data loss the type system cannot see — the two lists are
	// hand-maintained and positional.
	c.Preview = "1994/toys.webp"
	if err := s.UpsertClip(ctx, c); err != nil {
		t.Fatalf("seed clip: %v", err)
	}

	got, err := s.GetClip(ctx, c.Path)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Thumbnail != "1994/toys.jpg" {
		t.Errorf("thumbnail = %q, want it round-tripped", got.Thumbnail)
	}
	if got.Preview != "1994/toys.webp" {
		t.Errorf("preview = %q, want it round-tripped — a preview that vanishes on read means "+
			"every card silently falls back to its still", got.Preview)
	}
	if got.PlayCount != 0 || !got.LastPlayedAt.IsZero() {
		t.Errorf("a fresh clip must start unplayed, got count=%d at=%v", got.PlayCount, got.LastPlayedAt)
	}

	aired := time.Unix(1_800_000_000, 0).UTC()
	for i := 0; i < 3; i++ {
		if err := s.RecordClipPlay(ctx, c.Path, aired); err != nil {
			t.Fatalf("record play %d: %v", i, err)
		}
	}
	got, _ = s.GetClip(ctx, c.Path)
	if got.PlayCount != 3 {
		t.Errorf("play count = %d, want 3", got.PlayCount)
	}
	if !got.LastPlayedAt.Equal(aired) {
		t.Errorf("last played = %v, want %v", got.LastPlayedAt, aired)
	}

	// A re-sync (what the periodic scan does) must leave the counters alone. Everything else
	// about the row is legitimately refreshed.
	c.Name = "TMNT toys (renamed)"
	if err := s.UpsertClip(ctx, c); err != nil {
		t.Fatalf("re-sync: %v", err)
	}
	got, _ = s.GetClip(ctx, c.Path)
	if got.Name != "TMNT toys (renamed)" {
		t.Errorf("a re-sync must refresh the name, got %q", got.Name)
	}
	if got.PlayCount != 3 {
		t.Errorf("a re-sync RESET the play count to %d — play_count must not be in the DO UPDATE list", got.PlayCount)
	}
	if !got.LastPlayedAt.Equal(aired) {
		t.Errorf("a re-sync reset last_played_at to %v", got.LastPlayedAt)
	}

	// Playout may resolve a clip the catalog has since pruned; that is telemetry missing a
	// row, not a playback error.
	if err := s.RecordClipPlay(ctx, "gone/missing.mp4", aired); err != nil {
		t.Errorf("recording a play for a pruned clip must not error, got %v", err)
	}
}

// testAiringHistory pins the recency signal's storage contract (§5, programming-design §3.1).
//
// The scheduler's separation rules are within-cycle; this table is the ONLY memory of what aired
// across cycles, so its two properties — upsert-to-latest and per-channel scoping — are what make
// recency-aware placement possible at all.
func testAiringHistory(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)

	base := time.Now().Truncate(time.Second)
	kAkira := provision.Key("movie:tmdb:149")
	kAliens := provision.Key("movie:tmdb:679")

	if err := st.RecordAiring(ctx, "ch-1", kAkira, "lib-149", base.Add(-72*time.Hour)); err != nil {
		t.Fatalf("record: %v", err)
	}
	if err := st.RecordAiring(ctx, "ch-1", kAliens, "lib-679", base.Add(-24*time.Hour)); err != nil {
		t.Fatalf("record: %v", err)
	}

	got, err := st.LastAiredByChannel(ctx, "ch-1")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("history has %d entries, want 2", len(got))
	}

	// UPSERT TO LATEST: re-airing moves the timestamp forward rather than adding a row. The
	// reader asks "when did this LAST air", so an append-only log would accumulate rows to
	// answer a question about its own maximum.
	if err := st.RecordAiring(ctx, "ch-1", kAkira, "lib-149", base); err != nil {
		t.Fatalf("re-record: %v", err)
	}
	got, err = st.LastAiredByChannel(ctx, "ch-1")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("re-airing added a row (%d entries); it must upsert", len(got))
	}
	if !got[kAkira].Equal(base) {
		t.Errorf("last-aired = %v, want the LATEST airing %v", got[kAkira], base)
	}

	// PER-CHANNEL SCOPING: the same film on two channels is two independent rotations, and
	// collapsing them would let one channel's schedule suppress another's.
	if err := st.RecordAiring(ctx, "ch-2", kAkira, "lib-149", base.Add(-time.Hour)); err != nil {
		t.Fatalf("record ch-2: %v", err)
	}
	other, err := st.LastAiredByChannel(ctx, "ch-2")
	if err != nil {
		t.Fatalf("read ch-2: %v", err)
	}
	if len(other) != 1 {
		t.Fatalf("ch-2 history has %d entries, want 1 (channels are independent)", len(other))
	}
	if !other[kAkira].Equal(base.Add(-time.Hour)) {
		t.Errorf("ch-2 timestamp leaked from ch-1: %v", other[kAkira])
	}

	// A channel that has never aired anything reads as empty, not as an error — that is what
	// lets placement degrade to today's behaviour on a fresh install.
	none, err := st.LastAiredByChannel(ctx, "ch-never")
	if err != nil {
		t.Fatalf("unknown channel must not error: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("unknown channel returned %d entries, want 0", len(none))
	}
}

// testActivityFeed covers the Dashboard feed's three operations on both backends (§5, V32):
// append, newest-first read, and age-based purge.
func testActivityFeed(t *testing.T, newStore func(t *testing.T) Store) {
	t.Helper()
	st := newStore(t)
	ctx := context.Background()

	base := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	for i, tc := range []struct {
		text  string
		level string
		age   time.Duration
	}{
		{"oldest — CH 12 reconciled", ActivityInfo, 90 * time.Minute},
		{"middle — Seerr timed out, retrying", ActivityWarn, 60 * time.Minute},
		{"newest — Darkwing Duck landed", ActivityInfo, 30 * time.Minute},
	} {
		if err := st.RecordActivity(ctx, Activity{
			At: base.Add(-tc.age).Unix(), Kind: ActivityKindTitle, Level: tc.level,
			Text: tc.text, SubjectID: fmt.Sprintf("subj-%d", i),
		}); err != nil {
			t.Fatalf("record activity %d: %v", i, err)
		}
	}

	// Newest first — the ONLY ordering the feed has.
	got, err := st.ListActivity(ctx, 10)
	if err != nil {
		t.Fatalf("list activity: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("listed %d rows, want 3", len(got))
	}
	if !strings.HasPrefix(got[0].Text, "newest") || !strings.HasPrefix(got[2].Text, "oldest") {
		t.Errorf("wrong order: %q ... %q", got[0].Text, got[2].Text)
	}
	if got[1].Level != ActivityWarn {
		t.Errorf("level = %q, want warn — it drives the UI dot", got[1].Level)
	}
	if got[0].ID == "" {
		t.Error("row has no id; the store must mint one")
	}

	// The limit is honoured, or a dashboard panel would render the whole table.
	if two, err := st.ListActivity(ctx, 2); err != nil || len(two) != 2 {
		t.Errorf("ListActivity(2) = %d rows, %v; want 2", len(two), err)
	}

	// An unrecognised level is normalised on the way in — the UI has no colour for it, so
	// storing it would render an invisible dot.
	if err := st.RecordActivity(ctx, Activity{Text: "odd", Level: "banana"}); err != nil {
		t.Fatalf("record odd level: %v", err)
	}
	after, _ := st.ListActivity(ctx, 1)
	if len(after) != 1 || after[0].Level != ActivityInfo {
		t.Errorf("level %q was stored as-is; want normalisation to info", after[0].Level)
	}

	// An empty text is dropped rather than stored: a blank row occupies a slot in a list
	// the operator is scanning.
	before, _ := st.ListActivity(ctx, 100)
	if err := st.RecordActivity(ctx, Activity{Text: ""}); err != nil {
		t.Fatalf("record empty: %v", err)
	}
	if all, _ := st.ListActivity(ctx, 100); len(all) != len(before) {
		t.Errorf("an empty-text row was stored (%d -> %d)", len(before), len(all))
	}

	// Purge is by AGE — the feed is the one append-only table here, so it is the one that
	// would otherwise grow without bound.
	n, err := st.PurgeActivity(ctx, base.Add(-45*time.Minute))
	if err != nil {
		t.Fatalf("purge activity: %v", err)
	}
	if n != 2 {
		t.Errorf("purged %d rows, want 2 (the two older than the horizon)", n)
	}
	left, _ := st.ListActivity(ctx, 100)
	for _, a := range left {
		if strings.HasPrefix(a.Text, "oldest") || strings.HasPrefix(a.Text, "middle") {
			t.Errorf("row %q survived a purge that should have removed it", a.Text)
		}
	}
}

// testRetentionPurge covers §5's retention on both backends — and, more importantly, what it
// must NOT remove.
func testRetentionPurge(t *testing.T, newStore func(t *testing.T) Store) {
	t.Helper()
	st := newStore(t)
	ctx := context.Background()

	old := time.Now().Add(-100 * 24 * time.Hour)
	recent := time.Now().Add(-time.Hour)

	// Jobs: one of each status, all OLD, so status is the only thing distinguishing them.
	for _, j := range []Job{
		{ID: "j-done", Kind: "suggest", Status: "done", UpdatedAt: old},
		{ID: "j-failed", Kind: "suggest", Status: "failed", UpdatedAt: old},
		{ID: "j-queued", Kind: "suggest", Status: "queued", UpdatedAt: old},
		{ID: "j-running", Kind: "suggest", Status: "running", UpdatedAt: old},
		{ID: "j-fresh", Kind: "suggest", Status: "done", UpdatedAt: recent},
	} {
		if err := st.CreateJob(ctx, j); err != nil {
			t.Fatalf("seed job %s: %v", j.ID, err)
		}
	}

	n, err := st.PurgeFinishedJobs(ctx, time.Now().Add(-30*24*time.Hour))
	if err != nil {
		t.Fatalf("purge jobs: %v", err)
	}
	if n != 2 {
		t.Errorf("purged %d jobs, want 2 (done + failed, both old)", n)
	}
	// ⚠ The invariants: in-flight work survives regardless of age, and a recent finished job
	// is inside the window.
	for _, id := range []string{"j-queued", "j-running", "j-fresh"} {
		if _, err := st.GetJob(ctx, id); err != nil {
			t.Errorf("job %s was purged; only finished jobs past the window may go (%v)", id, err)
		}
	}
	for _, id := range []string{"j-done", "j-failed"} {
		if _, err := st.GetJob(ctx, id); err == nil {
			t.Errorf("job %s survived a purge that should have removed it", id)
		}
	}

	// Proposals: one of each status, all OLD.
	for _, p := range []Proposal{
		{ID: "p-denied", Status: "denied", UpdatedAt: old},
		{ID: "p-approved", Status: "approved", UpdatedAt: old},
		{ID: "p-submitted", Status: "submitted", UpdatedAt: old},
		{ID: "p-fresh-denied", Status: "denied", UpdatedAt: recent},
	} {
		if err := st.CreateProposal(ctx, p); err != nil {
			t.Fatalf("seed proposal %s: %v", p.ID, err)
		}
	}

	n, err = st.PurgeDeniedProposals(ctx, time.Now().Add(-90*24*time.Hour))
	if err != nil {
		t.Fatalf("purge proposals: %v", err)
	}
	if n != 1 {
		t.Errorf("purged %d proposals, want 1 (the old denied one)", n)
	}
	// ⚠ THE AUDIT TRAIL. An approved proposal is the record that someone authorized spending
	// real resources (§7); a submitted one is a member still waiting for an answer. Neither
	// may be aged out.
	for _, id := range []string{"p-approved", "p-submitted", "p-fresh-denied"} {
		if _, err := st.GetProposal(ctx, id); err != nil {
			t.Errorf("proposal %s was purged; only OLD DENIED proposals may go (%v)", id, err)
		}
	}
	if _, err := st.GetProposal(ctx, "p-denied"); err == nil {
		t.Error("the old denied proposal survived a purge that should have removed it")
	}
}

// testScheduledJobPaused: the pause flag persists, survives an ordinary state write, and keeps
// the job out of the due-claim (§18.1). One suite, both dialects — the claim SQL differs
// (guarded UPDATE vs FOR UPDATE SKIP LOCKED) and both must skip paused rows.
func testScheduledJobPaused(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	s := newStore(t)
	now := time.Unix(1_700_000_000, 0).UTC()

	// Due NOW, not paused: the control case — without it, "did not run" proves nothing.
	if err := s.UpsertScheduledJob(ctx, ScheduledJob{Name: "reconcile", NextRun: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetScheduledJobPaused(ctx, "reconcile", true); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetScheduledJob(ctx, "reconcile")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Paused {
		t.Fatal("pause did not persist")
	}

	// ⚠ An ordinary state write must NOT clear it. This runs after every execution, so if
	// `paused` rode in UpsertScheduledJob's DO UPDATE list, the next run would silently resume
	// a job the operator paused.
	if err := s.UpsertScheduledJob(ctx, ScheduledJob{
		Name: "reconcile", LastResult: "ok", LastRun: now, NextRun: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ = s.GetScheduledJob(ctx, "reconcile"); !got.Paused {
		t.Error("a routine state write cleared paused — it must be absent from ON CONFLICT DO UPDATE")
	}

	// ⚠ The behaviour: a paused row is never claimed, even though it is due.
	due, err := s.ClaimDueScheduledJobs(ctx, now.Add(time.Minute), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	for _, j := range due {
		if j.Name == "reconcile" {
			t.Error("a paused job was claimed; it would then run on its schedule")
		}
	}

	// Resuming makes it claimable again, or pause is a one-way door.
	if err := s.SetScheduledJobPaused(ctx, "reconcile", false); err != nil {
		t.Fatal(err)
	}
	due, err = s.ClaimDueScheduledJobs(ctx, now.Add(2*time.Minute), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, j := range due {
		if j.Name == "reconcile" {
			found = true
		}
	}
	if !found {
		t.Error("a resumed job was still not claimed")
	}

	// Pausing a job that has never run creates the row, so a task can be paused before its
	// first execution rather than only after it has already gone off once.
	if err := s.SetScheduledJobPaused(ctx, "never-ran", true); err != nil {
		t.Fatal(err)
	}
	if got, err = s.GetScheduledJob(ctx, "never-ran"); err != nil || !got.Paused {
		t.Errorf("pausing an unseen job = (%+v, %v), want a created paused row", got, err)
	}
}

// testFillerSources covers the persisted REMOTE source registry (§10, V33) on BOTH backends.
//
// The interesting assertions are the two that protect against silent data loss: a re-register
// must not reset "last fetched", and deleting a source must not take its clips with it.
// clipAt builds a catalog clip whose identity is its readable path.
//
// ⚠ Real 64-hex hashes are deliberately NOT used here. The store does not care what a hash looks
// like — only `filler.ClipPath` validates the shape, and that is tested where it belongs
// (`filler/clippath_test.go`). Using real hashes would turn every assertion in this file into a
// wall of hex and would test nothing the hash tests do not already cover.
func clipAt(path, name string, kind filler.Kind, durationMs int64) filler.Clip {
	return filler.Clip{Hash: path, Path: path, Name: name, Kind: kind, DurationMs: durationMs}
}

// findSource returns one source by id, and whether it is still listed.
//
// ⚠ By id, not by position. V37's migration seeds two singleton rows (`folder`, `library`) so a
// fresh store can still express "not configured", which means `ListFillerSources(ctx)[0]` is no
// longer the row a test just added — it is whichever seeded row sorts first. Every positional
// read in this suite was rewritten through here after exactly that broke.
func findSource(t *testing.T, s Store, id string) (FillerSource, bool) {
	t.Helper()
	all, err := s.ListFillerSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range all {
		if f.ID == id {
			return f, true
		}
	}
	return FillerSource{}, false
}

// src1 is the source this suite registers first, re-read. Fatals if it has gone missing, so a
// caller can assert on its fields without a nil check at every use.
func src1(t *testing.T, s Store) FillerSource {
	t.Helper()
	f, ok := findSource(t, s, "src-1")
	if !ok {
		t.Fatal("src-1 is not listed")
	}
	return f
}

// testSeededDefaultSources covers what migration 00034 puts in a FRESH store, on BOTH backends
// (§10 V38c.8).
//
// ⚠ **This is the ONLY test that may depend on the seeded set.** Every other suite clears it —
// `newFillerServer` in internal/api does so explicitly — because eleven tests phrased as absolute
// counts ("want 1", "unconfigured") went red the moment the migration landed, none of them wrong
// about the behaviour they described. Concentrating the dependency here means the next change to
// the seed breaks exactly one test, and it breaks the one whose job is to notice.
//
// ⚠ It runs on both dialects because the seed is HAND-DUPLICATED per backend — two nearly
// identical SQL files, differing in `unixepoch()` vs its Postgres spelling. That is precisely the
// shape that drifts: a row added to one file and forgotten in the other produces a Postgres
// install that silently ships fewer sources than a SQLite one, and no single-dialect test can see
// it.
func testSeededDefaultSources(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)

	all, err := s.ListFillerSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]FillerSource{}
	for _, f := range all {
		byID[f.ID] = f
	}

	// The three VERIFIED archive collections (checked against the live API 2026-08-03 — five
	// plausible-looking identifiers returned zero items, which is why this list is short).
	for _, want := range []struct{ id, label string }{
		{"archive:classic_tv_commercials", "Classic TV Commercials"},
		{"archive:vhscommercials", "Commercials From The Vault"},
		{"archive:tv_ads", "TV Ads"},
	} {
		got, ok := byID[want.id]
		if !ok {
			t.Errorf("%s is missing — a fresh install must have something it can fetch", want.id)
			continue
		}
		if !got.Enabled {
			t.Errorf("%s is disabled; the seeded defaults are on so filler works on day one", want.id)
		}
		if !got.Fetchable() {
			t.Errorf("%s is not fetchable — a scanned-only default would leave the install stuck", want.id)
		}
		// ⚠ A human-readable name, not the identifier. `vhscommercials` is not something an
		// operator recognises in the Sources list.
		if got.Label != want.label {
			t.Errorf("%s label = %q, want %q", want.id, got.Label, want.label)
		}
		// ⚠ EMPTY licence, deliberately. ~92% of archive items declare none and §10 defines empty
		// as UNKNOWN, never "public domain" — a reassuring default here would have Loomarr
		// asserting a legal fact nobody checked.
		if got.License != "" {
			t.Errorf("%s license = %q, want empty (unknown) — absence carries no permission",
				want.id, got.License)
		}
	}

	// ⚠ The YouTube row is present but has NO target, and that is the design rather than an
	// oversight. §10 says Loomarr never recommends YouTube content itself; the operator brings
	// their own playlist. An empty uri also keeps the row out of every pull plan on its own,
	// because Fetchable() requires one.
	yt, ok := byID["youtube"]
	if !ok {
		t.Fatal("the youtube row is missing — the mock draws it as a present, empty prompt")
	}
	if yt.URI != "" {
		t.Errorf("youtube uri = %q, want empty — seeding a target IS the recommendation §10 forbids",
			yt.URI)
	}
	if yt.Fetchable() {
		t.Error("the empty youtube row is fetchable; it must not reach the ingest job until someone fills it in")
	}
}

func testFillerSources(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)

	// ⚠ A FRESH install ships with fetchable sources (§10 V38c.8, migration 00034). Asserted here,
	// on BOTH backends, because a seed that lands on sqlite and not on postgres is exactly the
	// dialect drift this one-suite-two-backends rule exists to catch — and it would show up as
	// "filler mysteriously does nothing" on one deployment only.
	for _, want := range []struct{ id, label, uri string }{
		{"archive:classic_tv_commercials", "Classic TV Commercials", "classic_tv_commercials"},
		{"archive:vhscommercials", "Commercials From The Vault", "vhscommercials"},
		{"archive:tv_ads", "TV Ads", "tv_ads"},
	} {
		got, ok := findSource(t, s, want.id)
		if !ok {
			t.Fatalf("a fresh store is missing the seeded source %q — a new install cannot fetch", want.id)
		}
		// ⚠ The LABEL is human-readable. `vhscommercials` is not a name an operator recognises,
		// and the row renders the label above the target.
		if got.Label != want.label {
			t.Errorf("%s label = %q, want %q", want.id, got.Label, want.label)
		}
		if got.URI != want.uri {
			t.Errorf("%s uri = %q, want %q", want.id, got.URI, want.uri)
		}
		if !got.Enabled {
			t.Errorf("%s seeded switched OFF — it would sit in the UI doing nothing", want.id)
		}
		// ⚠ Fetchable, which is the whole point: `folder` and `library` are SCANNED, so before
		// this seed a fresh install had no source it could download from at all.
		if !got.Fetchable() {
			t.Errorf("%s is not fetchable — the seed exists so a new install CAN fetch", want.id)
		}
		// ⚠ EMPTY licence, and that is correct rather than missing data. All three declare none,
		// and §10 defines empty as UNKNOWN — never "public domain". A reassuring default here
		// would have Loomarr asserting a legal fact nobody checked.
		if got.License != "" {
			t.Errorf("%s licence = %q, want empty (unknown, NOT public domain)", want.id, got.License)
		}
	}

	// ⚠ YouTube seeds PRESENT BUT EMPTY. §10: Loomarr never recommends YouTube content itself, so
	// the operator brings the playlist — a seeded target would be that recommendation. The empty
	// uri also fails `Fetchable()`, which keeps the row out of every pull plan until it is filled
	// in; without that, approval would hand `Ingest` an empty string.
	if yt, ok := findSource(t, s, "youtube"); !ok {
		t.Error("a fresh store is missing the YouTube row — the mock draws it, unconfigured")
	} else {
		if yt.URI != "" {
			t.Errorf("youtube seeded with uri %q — Loomarr must not recommend a playlist", yt.URI)
		}
		if yt.Fetchable() {
			t.Error("an unconfigured youtube row is fetchable — a pull would ingest an empty string")
		}
	}

	created := time.Now().UTC().Truncate(time.Second)
	src := FillerSource{
		// Enabled explicitly: a Go bool zero-values to false, so a literal that omits it
		// describes a source that is switched OFF. Real add paths go through
		// NewFillerSource for exactly that reason.
		Enabled: true,
		// ⚠ NOT `classic_tv_commercials` — that is a SEEDED row now (00034), and 00032's unique
		// index on (kind, uri) correctly refuses a second row pointing at the same collection.
		// The fixture needs its own target; the index is doing its job.
		ID: "src-1", Kind: "archive", URI: "conformance_fixture_collection",
		Label:   "Classic TV commercials",
		License: "https://creativecommons.org/licenses/by-nc-sa/4.0/",
		// ⚠ Only ~8% of archive items declare a licence, so the empty case below is the
		// common one — both are covered.
		CreatedAt: created,
	}
	if err := s.UpsertFillerSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	unlicensed := FillerSource{ID: "src-2", Kind: "archive", URI: "vintage_ads", CreatedAt: created.Add(time.Second)}
	if err := s.UpsertFillerSource(ctx, unlicensed); err != nil {
		t.Fatal(err)
	}

	all, err := s.ListFillerSources(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// ⚠ V37: the list is no longer empty on a fresh store. Migration 00029 materialises the two
	// CONFIG-BACKED singletons (`folder`, `library`) so the flat list can still say "you could
	// set up a drop-folder but have not" — §10's own answer to "why is my catalog empty?", which
	// a table of things-that-exist otherwise cannot express. So this suite asserts on the rows it
	// added, BY ID, rather than by position in the whole list.
	byID := map[string]FillerSource{}
	for _, f := range all {
		byID[f.ID] = f
	}
	for _, want := range []string{"folder", "library"} {
		if _, ok := byID[want]; !ok {
			t.Fatalf("singleton row %q missing — a fresh store must be able to say 'not configured'", want)
		}
	}
	if byID["folder"].URI != "" {
		t.Errorf("seeded folder URI = %q, want empty (= not configured, never a guessed path)", byID["folder"].URI)
	}

	// Ordering is still oldest-first and still explicit — an unordered list reshuffles between
	// reads on Postgres and the Sources tab's rows would move under the pointer. Checked over the
	// two rows this test added rather than the whole list, whose head is now seeded.
	//
	// ⚠ Filtered by ID, not by KIND. `Kind == "archive"` was unambiguous while every archive row
	// came from this test; migration 00034 seeds three of them, so the kind filter started
	// collecting the seed too and the count assertion below failed for the right reason.
	wanted := map[string]bool{"src-1": true, "src-2": true}
	var added []FillerSource
	for _, f := range all {
		if wanted[f.ID] {
			added = append(added, f)
		}
	}
	if len(added) != 2 {
		t.Fatalf("listed %d of this test's own sources, want 2", len(added))
	}
	if added[0].ID != "src-1" || added[1].ID != "src-2" {
		t.Errorf("order = %s,%s; want src-1,src-2 (oldest first)", added[0].ID, added[1].ID)
	}
	if added[0].License != src.License {
		t.Errorf("licence = %q, want %q", added[0].License, src.License)
	}
	if added[1].License != "" {
		t.Errorf("unlicensed source has licence %q, want empty (= unknown)", added[1].License)
	}
	if !added[0].LastFetchedAt.IsZero() {
		t.Errorf("a never-fetched source has LastFetchedAt %v, want zero", added[0].LastFetchedAt)
	}

	// ⚠ THE invariant the flat model has to carry itself (§10), MOVED in V38c from the kind to
	// the TARGET. 00029 allowed exactly one folder row; 00032 allows many, because commercials
	// living in two places is ordinary and V37 gave it no expression.
	//
	// What must still be impossible is ONE folder appearing as TWO rows — a stale row disagreeing
	// with another about the same directory, which is the precedence question 00023 refused to
	// have. So a DISTINCT path is accepted and a DUPLICATE path is refused, by the database
	// rather than by a Go guard the next caller forgets.
	second := FillerSource{ID: "folder-2", Kind: "folder", URI: "/other", Enabled: true, CreatedAt: created}
	if err := s.UpsertFillerSource(ctx, second); err != nil {
		t.Errorf("a second DISTINCT folder was refused (%v) — V38c allows many watched folders", err)
	}
	dup := FillerSource{ID: "folder-3", Kind: "folder", URI: "/other", Enabled: true, CreatedAt: created}
	if err := s.UpsertFillerSource(ctx, dup); err == nil {
		t.Error("a DUPLICATE folder path was accepted — one directory must not appear as two rows")
	}

	// ⚠ THE three-state encoding (§10 V38c). `nil` = inherit the global, `0` = never fetch this
	// source, `N` = every N seconds. They cannot collapse: `filler.fetch.every = 0` already means
	// "off", so a non-nullable column would make "unset" and "never" the same value and read as
	// "every existing source is switched off" on upgrade — 00026's mistake exactly.
	never, every900 := 0, 900
	if err := s.SetFillerSourceFetchPolicy(ctx, "src-2", &never, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFillerSourceFetchPolicy(ctx, "folder-2", &every900, &every900); err != nil {
		t.Fatal(err)
	}
	policies, err := s.ListFillerSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byPolicyID := map[string]FillerSource{}
	for _, f := range policies {
		byPolicyID[f.ID] = f
	}
	// src-1 was never given a policy: nil, and it must resolve to the global.
	if got := byPolicyID["src-1"]; got.FetchEverySeconds != nil {
		t.Errorf("src-1 override = %v, want nil (inherit)", *got.FetchEverySeconds)
	} else if d, ok := got.FetchEvery(time.Hour); !ok || d != time.Hour {
		t.Errorf("an un-overridden source resolved to (%v, %v), want the global hour", d, ok)
	}
	// src-2 was set to 0 = NEVER. ⚠ It must NOT read as "inherit" — that is the collapse.
	if got := byPolicyID["src-2"]; got.FetchEverySeconds == nil {
		t.Error("a 0 override round-tripped as NULL — 'never' collapsed into 'inherit'")
	} else if _, ok := got.FetchEvery(time.Hour); ok {
		t.Error("a 0 override resolved to a poll interval — 0 must mean NEVER")
	}
	if got := byPolicyID["folder-2"]; got.FetchEverySeconds == nil || *got.FetchEverySeconds != 900 {
		t.Errorf("folder-2 override did not round-trip: %v", got.FetchEverySeconds)
	} else if d, _ := got.FetchEvery(time.Hour); d != 900*time.Second {
		t.Errorf("resolved to %v, want 15m from the override rather than the global", d)
	}
	// Clearing goes back to inherit — a real operator action ("stop treating this specially").
	if err := s.SetFillerSourceFetchPolicy(ctx, "src-2", nil, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := findSource(t, s, "src-2"); got.FetchEverySeconds != nil {
		t.Error("clearing an override did not return the source to inherit")
	}

	// ⚠ Two BLANK-uri rows must both survive. A seeded-but-unconfigured row carries no target —
	// that is how "you could set up a drop-folder but have not" is expressed (§10) — and a plain
	// unique index rather than a partial one would allow only ONE blank row across the table,
	// so a fresh install could not have both an unconfigured folder and an unconfigured library.
	for _, blank := range []FillerSource{
		{ID: "blank-a", Kind: "folder", URI: "", Enabled: true, CreatedAt: created},
		{ID: "blank-b", Kind: "library", URI: "", Enabled: true, CreatedAt: created},
	} {
		if err := s.UpsertFillerSource(ctx, blank); err != nil {
			t.Errorf("an unconfigured %s row was refused (%v) — 'not configured' must stay expressible",
				blank.Kind, err)
		}
	}

	// Fetch stamps.
	fetched := created.Add(time.Hour)
	if err := s.MarkFillerSourceFetched(ctx, "src-1", fetched); err != nil {
		t.Fatal(err)
	}
	if !src1(t, s).LastFetchedAt.Equal(fetched) {
		t.Errorf("LastFetchedAt = %v, want %v", src1(t, s).LastFetchedAt, fetched)
	}

	// ⚠ THE assertion this table's ON CONFLICT clause exists for. Re-registering a source
	// (an operator fixing its label) knows nothing about fetches; if last_fetched_at joined
	// the DO UPDATE list, a working source would silently look like it had never run.
	relabelled := src
	relabelled.Label = "Renamed"
	if err := s.UpsertFillerSource(ctx, relabelled); err != nil {
		t.Fatal(err)
	}
	if src1(t, s).Label != "Renamed" {
		t.Errorf("label = %q, want Renamed", src1(t, s).Label)
	}
	if !src1(t, s).LastFetchedAt.Equal(fetched) {
		t.Errorf("re-registering reset LastFetchedAt to %v — it must survive an upsert", src1(t, s).LastFetchedAt)
	}

	// The Sources tab's on/off switch (V35). Two properties, each a claim the switch's own
	// copy makes to the operator.
	if !src1(t, s).Enabled {
		t.Error("source is not enabled — a registered source must be on until switched off")
	}
	if err := s.SetFillerSourceEnabled(ctx, "src-1", false); err != nil {
		t.Fatal(err)
	}
	if src1(t, s).Enabled {
		t.Error("source still enabled after being switched off")
	}

	// 1. ⚠ Disabling is NOT deleting. The row keeps its licence and its fetch history, which
	//    is what makes switching it back on restore what was there instead of starting over.
	if src1(t, s).License != src.License {
		t.Errorf("licence lost on disable: %q", src1(t, s).License)
	}
	if !src1(t, s).LastFetchedAt.Equal(fetched) {
		t.Error("fetch history lost on disable — the row was rewritten rather than updated")
	}

	// 2. ⚠ A re-register must not flip the switch back. `UpsertFillerSource` deliberately omits
	//    `enabled` from its DO UPDATE list, for the same reason last_fetched_at is omitted: a
	//    caller fixing a label knows nothing about the switch, and a Go bool zero-values to
	//    FALSE, so writing it would silently disable a source behind the operator's back. The
	//    first draft of V35 had exactly that bug.
	reRegistered := src
	reRegistered.Label = "Renamed again"
	reRegistered.Enabled = true // what a caller who does not know about the switch would send
	if err := s.UpsertFillerSource(ctx, reRegistered); err != nil {
		t.Fatal(err)
	}
	if src1(t, s).Enabled {
		t.Error("re-registering re-enabled a disabled source — the switch is not the upsert's business")
	}
	if src1(t, s).Label != "Renamed again" {
		t.Errorf("label = %q, want the re-registered one", src1(t, s).Label)
	}

	// Put it back on, so the delete assertions below run against the normal state.
	if err := s.SetFillerSourceEnabled(ctx, "src-1", true); err != nil {
		t.Fatal(err)
	}

	// ⚠ Deleting a source must NOT delete its clips: they are real files already tagged and
	// possibly pinned into a channel, and forgetting where something came from is not a
	// reason to throw it away.
	if err := s.UpsertClip(ctx, Clip{Clip: filler.Clip{
		Hash: "from-src-1.mp4",
		Path: "from-src-1.mp4", Name: "From src 1", Kind: filler.Commercial, DurationMs: 30000,
		Source: "archive", License: src.License,
	}, UpdatedAt: created}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFillerSource(ctx, "src-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetClip(ctx, "from-src-1.mp4"); err != nil {
		t.Errorf("deleting a source removed its clip: %v", err)
	}
	if _, ok := findSource(t, s, "src-1"); ok {
		t.Error("after delete, src-1 is still listed")
	}

	// An unknown id is ErrNotFound on both write paths, so a caller cannot believe it
	// recorded something.
	if err := s.DeleteFillerSource(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete unknown = %v, want ErrNotFound", err)
	}
	if err := s.MarkFillerSourceFetched(ctx, "nope", fetched); !errors.Is(err, ErrNotFound) {
		t.Errorf("mark unknown fetched = %v, want ErrNotFound", err)
	}
	if err := s.SetFillerSourceEnabled(ctx, "nope", false); !errors.Is(err, ErrNotFound) {
		t.Errorf("set enabled on unknown = %v, want ErrNotFound", err)
	}
}

// testSplitProposals covers the persisted split proposal (§10, V34) on BOTH backends: the
// segments JSON round-trip, ONE proposal per clip (re-detection replaces, and the new id
// wins), delete, and DeleteClip (the confirm path's drop of the compilation row).
func testSplitProposals(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	p := filler.SplitProposal{
		ID: "sp_1", ClipPath: "comps/1987.mp4", CreatedAt: now,
		Segments: []filler.SplitSegment{
			{Index: 0, StartMs: 0, EndMs: 30000, Name: "comps/1987 part 1", Era: 1987, Audience: filler.Kids, Category: "toys"},
			{Index: 1, StartMs: 30000, EndMs: 61000, Name: "unknown", SuggestedEra: 1985, DupOf: "old/ad.mp4"},
			{Index: 2, StartMs: 61000, EndMs: 149000, Name: "comps/1987 part 3", Unsplittable: true, Transcript: "[00:00] …"},
		},
	}
	if err := s.UpsertSplitProposal(ctx, p); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSplitProposal(ctx, "sp_1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ClipPath != p.ClipPath || len(got.Segments) != 3 || !got.CreatedAt.Equal(now) {
		t.Fatalf("proposal round-trip = %+v", got)
	}
	// Every segment field survives the JSON round-trip — including the V34-specific
	// suggestion, dedup flag, and unsplittable marker the review renders.
	s1 := got.Segments[1]
	if s1.SuggestedEra != 1985 || s1.DupOf != "old/ad.mp4" || s1.Era != 0 {
		t.Errorf("segment suggestion/dedup fields lost: %+v", s1)
	}
	if !got.Segments[2].Unsplittable || got.Segments[2].Transcript == "" {
		t.Errorf("unsplittable marker/transcript lost: %+v", got.Segments[2])
	}

	// ⚠ Re-detection REPLACES the pending proposal for the same clip — two competing
	// cut-lists for one file is a review bug, not a choice. The NEW id answers the old
	// one's GET with ErrNotFound.
	p2 := filler.SplitProposal{ID: "sp_2", ClipPath: p.ClipPath, CreatedAt: now.Add(time.Hour),
		Segments: []filler.SplitSegment{{Index: 0, StartMs: 0, EndMs: 149000, Name: "whole"}}}
	if err := s.UpsertSplitProposal(ctx, p2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSplitProposal(ctx, "sp_1"); !errors.Is(err, ErrNotFound) {
		t.Errorf("stale proposal after re-detection = %v, want ErrNotFound", err)
	}
	got2, err := s.GetSplitProposal(ctx, "sp_2")
	if err != nil || len(got2.Segments) != 1 {
		t.Fatalf("replacement proposal = (%+v, %v)", got2, err)
	}

	// DeleteClip (confirm drops the compilation row) + proposal cleanup.
	if err := s.UpsertClip(ctx, Clip{Clip: clipAt("comps/1987.mp4", "1987", filler.Commercial, 149000), UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteClip(ctx, "comps/1987.mp4"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetClip(ctx, "comps/1987.mp4"); !errors.Is(err, ErrNotFound) {
		t.Errorf("compilation row survived DeleteClip: %v", err)
	}
	if err := s.DeleteClip(ctx, "comps/1987.mp4"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteClip twice = %v, want ErrNotFound", err)
	}
	if err := s.DeleteSplitProposal(ctx, "sp_2"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSplitProposal(ctx, "sp_2"); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteSplitProposal twice = %v, want ErrNotFound", err)
	}
}

// testClipLicense pins that a clip's declared licence round-trips on BOTH backends, and that an
// absent one stays absent. ⚠ Empty means UNKNOWN, never "public domain" — ~92% of archive.org
// items declare none, so the empty case is the common one and must not acquire a default.
func testClipLicense(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	now := time.Now().UTC().Truncate(time.Second)

	licensed := Clip{Clip: filler.Clip{
		Hash: "licensed.mp4",
		Path: "licensed.mp4", Name: "Licensed", Kind: filler.Commercial, DurationMs: 30000,
		License: "https://creativecommons.org/publicdomain/zero/1.0/",
	}, UpdatedAt: now}
	unknown := Clip{Clip: filler.Clip{
		Hash: "unknown.mp4",
		Path: "unknown.mp4", Name: "Unknown", Kind: filler.Commercial, DurationMs: 30000,
	}, UpdatedAt: now}
	for _, c := range []Clip{licensed, unknown} {
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.GetClip(ctx, "licensed.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if got.License != licensed.License {
		t.Errorf("licence = %q, want %q", got.License, licensed.License)
	}
	if got, err = s.GetClip(ctx, "unknown.mp4"); err != nil {
		t.Fatal(err)
	}
	if got.License != "" {
		t.Errorf("a clip with no declared licence has %q, want empty", got.License)
	}
}

// testFillerPulls covers the filler approval gate (§10 V35) on BOTH backends.
//
// The assertions that matter are the ones protecting the AUDIT: a decided pull is kept, and a
// dropped plan row is retained with its flag rather than removed. "We approved this" is only
// meaningful next to what was proposed.
func testFillerPulls(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	created := time.Now().UTC().Truncate(time.Second)

	p := filler.Pull{
		ID: "pull_1", Title: "Top up the 1990s", Reason: "Saturday Mornings falls back to bumpers.",
		ProposedBy: "admin-1", Status: filler.PullPending, CreatedAt: created,
		Plan: []filler.PullPlanRow{
			{SourceID: "classic", Tag: "1990s", Name: "Classic TV commercials", Why: "Era match", EstimateClips: 40},
			{SourceID: "psa", Tag: "psa", Name: "Public service", Why: "Filler variety", EstimateClips: 12},
		},
	}
	if err := s.UpsertPull(ctx, p); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetPull(ctx, "pull_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Plan) != 2 || got.Plan[0].SourceID != "classic" || got.Plan[0].EstimateClips != 40 {
		t.Errorf("plan did not round-trip: %+v", got.Plan)
	}
	if !got.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, created)
	}
	// Pending means undecided, and that must be legible as a ZERO time rather than an epoch
	// date nobody meant.
	if !got.DecidedAt.IsZero() {
		t.Errorf("a pending pull has DecidedAt %v, want zero", got.DecidedAt)
	}
	if got.EstimatedClips() != 52 {
		t.Errorf("EstimatedClips = %d, want 52", got.EstimatedClips())
	}

	// Approve with one row dropped.
	decided := created.Add(time.Hour)
	got.Plan[1].Dropped = true
	got.Status = filler.PullApproved
	got.Note = "no local dealers"
	got.DecidedAt = decided
	got.DecidedBy = "admin-2"
	if err := s.UpsertPull(ctx, got); err != nil {
		t.Fatal(err)
	}

	after, err := s.GetPull(ctx, "pull_1")
	if err != nil {
		t.Fatal(err)
	}
	// ⚠ The dropped row is STILL THERE, flagged. Removing it would leave a record of what was
	// fetched with no record of what was declined, which is the half a reviewer needs.
	if len(after.Plan) != 2 {
		t.Fatalf("plan has %d rows after approval, want 2 — a dropped row must be retained", len(after.Plan))
	}
	if !after.Plan[1].Dropped {
		t.Error("the dropped flag did not persist")
	}
	if n := len(after.Committed()); n != 1 {
		t.Errorf("Committed() = %d rows, want 1", n)
	}
	if after.EstimatedClips() != 40 {
		t.Errorf("EstimatedClips after drop = %d, want 40", after.EstimatedClips())
	}
	if !after.DecidedAt.Equal(decided) || after.DecidedBy != "admin-2" || after.Note != "no local dealers" {
		t.Errorf("decision not recorded: %+v", after)
	}

	// Status filtering, and the fact that a decided pull is KEPT rather than deleted.
	if pending, err := s.ListPulls(ctx, filler.PullPending); err != nil || len(pending) != 0 {
		t.Errorf("pending = %d (%v), want 0", len(pending), err)
	}
	approved, err := s.ListPulls(ctx, filler.PullApproved)
	if err != nil || len(approved) != 1 {
		t.Fatalf("approved = %d (%v), want 1 — the history must survive the decision", len(approved), err)
	}
	if all, err := s.ListPulls(ctx, ""); err != nil || len(all) != 1 {
		t.Errorf("all = %d (%v), want 1", len(all), err)
	}

	if _, err := s.GetPull(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetPull unknown = %v, want ErrNotFound", err)
	}
}

// testClipHeld covers the V38 clip lifecycle on BOTH backends: a held clip is recorded but is not
// in the playable catalog, and only SetClipsHeld moves it.
//
// ⚠ The first assertion is the property the whole lifecycle rests on. Pod assembly, coverage, the
// filler-list builder and the catalog listing all read through ListClips with a zero filter, so if
// held clips were not excluded THERE, every untagged unreviewed download would air.
func testClipHeld(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	ctx := context.Background()
	s := newStore(t)
	at := time.Now().UTC().Truncate(time.Second)

	filed := Clip{Clip: filler.Clip{
		Hash: "filed.mp4",
		Path: "filed.mp4", Name: "Filed", Kind: filler.Commercial, DurationMs: 30000,
		Era: 1990, Audience: filler.Kids, Category: "toys",
	}, UpdatedAt: at}
	held := Clip{Clip: filler.Clip{
		Hash: "held.mp4",
		Path: "held.mp4", Name: "Held", Kind: filler.Commercial, DurationMs: 30000,
		Era: 1990, Audience: filler.Kids, Category: "toys", Held: true, Confidence: 40,
	}, UpdatedAt: at}
	for _, c := range []Clip{filed, held} {
		if err := s.UpsertClip(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	// ⚠ A ZERO filter is what pod assembly passes. A held clip must not be in this answer.
	got, err := s.ListClips(ctx, ClipFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range got {
		if c.Path == "held.mp4" {
			t.Fatal("a HELD clip came back from a zero-filter ListClips — pod assembly reads " +
				"exactly this, so an unreviewed clip would air")
		}
	}
	if len(got) != 1 {
		t.Fatalf("catalog has %d clips, want 1 (the filed one)", len(got))
	}

	// The review queue is the inverse, and it is how Incoming finds its work.
	queue, err := s.ListClips(ctx, ClipFilter{HeldOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 1 || queue[0].Path != "held.mp4" {
		t.Fatalf("HeldOnly returned %d clips, want just held.mp4", len(queue))
	}
	if queue[0].Confidence != 40 {
		t.Errorf("confidence = %d, want 40 — the score must round-trip", queue[0].Confidence)
	}

	// ⚠ THE trap this lifecycle has to survive: `clips` is a synced CACHE, so the folder scan
	// re-upserts every file it finds with held=false. If `held` rode along in UpsertClip's DO
	// UPDATE list, one scan pass would file every held clip — emptying the review queue into live
	// channels with no operator action and nothing in the logs.
	rescan := held
	rescan.Held = false
	rescan.Confidence = 0 // a scan knows nothing about tagging
	if err := s.UpsertClip(ctx, rescan); err != nil {
		t.Fatal(err)
	}
	after, err := s.ListClips(ctx, ClipFilter{HeldOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 {
		t.Fatal("a re-scan FILED a held clip — UpsertClip must omit `held` from its DO UPDATE " +
			"list, exactly as it omits the removal tombstone")
	}
	if after[0].Confidence != 40 {
		t.Errorf("a re-scan blanked the confidence score (%d) — it must be omitted too, or a "+
			"trusted clip starts asking again for no reason", after[0].Confidence)
	}

	// Filing is the only way out, and it records that nobody looked.
	if _, err := s.SetClipsHeld(ctx, []string{"held.mp4"}, false, true, at); err != nil {
		t.Fatal(err)
	}
	catalog, err := s.ListClips(ctx, ClipFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) != 2 {
		t.Fatalf("after filing, catalog has %d clips, want 2", len(catalog))
	}
	var flag bool
	for _, c := range catalog {
		if c.Path == "held.mp4" {
			flag = c.AutoFiled
		}
	}
	if !flag {
		t.Error("auto_filed did not survive — it is the only thing that can answer " +
			"'which of these did I never see?'")
	}
}
