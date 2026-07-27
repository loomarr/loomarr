package store

import (
	"bytes"
	"context"
	"errors"
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
	// Path is the identity since §9.1; the Tunarr uuid rides alongside for filler-lists and
	// is deliberately exercised as a NON-identity field (an install with no Tunarr has none).
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
	if err := s.UpdateClipTags(ctx, "u1", 1994, "kids", "cereal", true, now); err != nil {
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
	if err := s.UpdateClipTags(ctx, "gone", 1990, "kids", "toys", false, now); err != ErrNotFound {
		t.Errorf("UpdateClipTags(missing) = %v, want ErrNotFound", err)
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
