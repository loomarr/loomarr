package store

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
)

// Channels and the scheduling caches (§5, §9): the channel row and its lease, plus the
// two derived tables the scheduler reads — cached series episodes and airing history.

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
