package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
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
	// A non-default (hevc) broadcast codec (§9.1 V50) — using the non-default value
	// proves the column preserves what's written, not that it fell back to the DDL default.
	ch.BroadcastCodec = BroadcastCodecHEVC
	ch.PlayoutAnchor = time.Unix(1_700_000_123, 0).UTC()
	ch.ReconcileDeadline = deadline
	return ch
}

func mustSaveChannel(t *testing.T, s Store, ch Channel) Channel {
	t.Helper()
	saved, err := s.SaveChannel(context.Background(), ch)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func testChannelRoundTrip(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	want := sampleChannel("ch-a", 5, time.Unix(1_800_000_000, 0).UTC())
	want = mustSaveChannel(t, s, want)
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
	// BroadcastCodec (§9.1 V50) round-trips the non-default hevc value verbatim — both the
	// direct GetChannel path and (below) the claim RETURNING path scan the same column.
	if got.BroadcastCodec != BroadcastCodecHEVC {
		t.Errorf("broadcast_codec round-trip: got %q want %q", got.BroadcastCodec, BroadcastCodecHEVC)
	}
	if !got.PlayoutAnchor.Equal(want.PlayoutAnchor) {
		t.Errorf("playout anchor round-trip: got %v want %v", got.PlayoutAnchor, want.PlayoutAnchor)
	}
	if !got.ReconcileDeadline.Equal(want.ReconcileDeadline) {
		t.Errorf("reconcile deadline round-trip: got %v want %v", got.ReconcileDeadline, want.ReconcileDeadline)
	}
	// A CAS save with the returned revision updates the row in place.
	want.Status = schedule.StatusDrifted
	want = mustSaveChannel(t, s, want)
	got2, _ := s.GetChannel(ctx, "ch-a")
	if got2.Status != schedule.StatusDrifted {
		t.Errorf("save didn't update status: %s", got2.Status)
	}
	all, _ := s.ListChannels(ctx)
	if len(all) != 1 {
		t.Errorf("save created a duplicate channel: %d rows", len(all))
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

func testChannelRevisionCAS(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()

	created, err := s.SaveChannel(ctx, sampleChannel("ch-cas", 51, time.Time{}))
	if err != nil {
		t.Fatal(err)
	}
	if created.Revision != 1 {
		t.Fatalf("created revision = %d, want 1", created.Revision)
	}
	stale := created
	created.Name = "winner"
	saved, err := s.SaveChannel(ctx, created)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Revision != 2 {
		t.Fatalf("updated revision = %d, want 2", saved.Revision)
	}
	stale.Name = "loser"
	if _, err := s.SaveChannel(ctx, stale); !errors.Is(err, ErrChannelStale) {
		t.Fatalf("stale save = %v, want ErrChannelStale", err)
	}
	got, err := s.GetChannel(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "winner" || got.Revision != 2 {
		t.Fatalf("stale save changed row: name=%q revision=%d", got.Name, got.Revision)
	}

	if err := s.DeleteChannel(ctx, got.ID, stale.Revision); !errors.Is(err, ErrChannelStale) {
		t.Fatalf("stale delete = %v, want ErrChannelStale", err)
	}
	if err := s.DeleteChannel(ctx, got.ID, got.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveChannel(ctx, got); !errors.Is(err, ErrNotFound) {
		t.Fatalf("save after delete = %v, want ErrNotFound", err)
	}
}

func testChannelTargetedRevisionWrites(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	created := mustSaveChannel(t, s, sampleChannel("ch-targeted", 52, time.Time{}))

	codecRevision, err := s.SetChannelBroadcastCodec(ctx, created.ID, created.Revision, BroadcastCodecH264)
	if err != nil {
		t.Fatal(err)
	}
	if codecRevision != 2 {
		t.Fatalf("codec revision = %d, want 2", codecRevision)
	}
	if _, err := s.SetChannelBroadcastCodec(ctx, created.ID, created.Revision, BroadcastCodecHEVC); !errors.Is(err, ErrChannelStale) {
		t.Fatalf("stale codec write = %v, want ErrChannelStale", err)
	}

	attachedRevision, err := s.AttachTunarrChannel(ctx, created.ID, "", "tunarr-52", created.Number, 53)
	if err != nil {
		t.Fatal(err)
	}
	if attachedRevision != 3 {
		t.Fatalf("attach revision = %d, want 3", attachedRevision)
	}
	if _, err := s.AttachTunarrChannel(ctx, created.ID, "", "tunarr-other", created.Number, 54); !errors.Is(err, ErrChannelStale) {
		t.Fatalf("stale Tunarr attach = %v, want ErrChannelStale", err)
	}
	got, err := s.GetChannel(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.TunarrID != "tunarr-52" || got.Number != 53 || got.BroadcastCodec != BroadcastCodecH264 || got.Revision != 3 {
		t.Fatalf("targeted writes = tunarr %q number %d codec %q revision %d", got.TunarrID, got.Number, got.BroadcastCodec, got.Revision)
	}
	if _, err := s.SaveChannel(ctx, created); !errors.Is(err, ErrChannelStale) {
		t.Fatalf("pre-targeted snapshot save = %v, want ErrChannelStale", err)
	}
}

func testChannelListDelete(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	mustSaveChannel(t, s, sampleChannel("ch-2", 2, time.Time{}))
	mustSaveChannel(t, s, sampleChannel("ch-1", 1, time.Time{}))
	mustSaveChannel(t, s, sampleChannel("ch-3", 3, time.Time{}))

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
	toDelete, _ := s.GetChannel(ctx, "ch-2")
	if err := s.DeleteChannel(ctx, "ch-2", toDelete.Revision); err != nil {
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

// Deleting a channel must drop its IMAGE REFS — the cascade that replaced the `channel_icons` retired-ok
// cleanup when the table went (V52 phase 8).
//
// ⚠ **This is a regression test for a real leak, not a rename of the old icon round-trip.** Phase 5
// moved icon bytes into the image service but left DeleteChannel deleting the old blob table, so a
// deleted channel's `image_refs` row survived it. A ref is precisely what tells the GC an image is
// still in use (§22), so the icon was never orphaned and never collected — bytes on disk owned by a
// channel that no longer exists, forever. `DeleteImageRefs` had no caller at all until phase 8.
func testChannelDeleteDropsImageRefs(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	iconChannel := mustSaveChannel(t, s, sampleChannel("ch-ico", 7, time.Time{}))

	img := imageAt("channel-icon", time.Unix(1_700_000_000, 0))
	if err := s.PutImage(ctx, img); err != nil {
		t.Fatal(err)
	}
	if err := s.PutImageRef(ctx, ImageRef{
		ImageHash: img.Hash, OwnerKind: "channel", OwnerID: "ch-ico", Role: "icon",
	}); err != nil {
		t.Fatal(err)
	}

	owned, err := s.ImagesForOwner(ctx, "channel", "ch-ico")
	if err != nil || len(owned) != 1 {
		t.Fatalf("before delete: %d images for the channel (err=%v), want 1", len(owned), err)
	}

	if err := s.DeleteChannel(ctx, "ch-ico", iconChannel.Revision); err != nil {
		t.Fatal(err)
	}

	owned, err = s.ImagesForOwner(ctx, "channel", "ch-ico")
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 0 {
		t.Errorf("the channel's image ref survived the delete — its icon can never be collected as an orphan")
	}

	// ⚠ The IMAGE ITSELF must survive, and this half is not incidental. The ref is a usage record;
	// the GC's orphan sweep is what deletes bytes. Deleting the image here would destroy artwork
	// that a second channel may still point at — two channels sharing one icon is ordinary, since
	// identity is the content hash.
	if _, err := s.GetImage(ctx, img.Hash); err != nil {
		t.Errorf("the image row was deleted along with the channel: %v — refs are a usage record, not ownership", err)
	}
}

func testClaimDueChannels(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	now := time.Unix(1_800_000_000, 0).UTC()
	// Due: deadline in the past, live.
	dueBefore := mustSaveChannel(t, s, sampleChannel("ch-due", 1, now.Add(-time.Hour)))
	// Not due: future deadline.
	mustSaveChannel(t, s, sampleChannel("ch-future", 2, now.Add(time.Hour)))
	// Not eligible: detached, even with a past deadline.
	detached := sampleChannel("ch-detached", 3, now.Add(-time.Hour))
	detached.Status = schedule.StatusDetached
	mustSaveChannel(t, s, detached)
	// Not eligible: paused is deliberately off air and must not be claimed either.
	paused := sampleChannel("ch-paused", 5, now.Add(-time.Hour))
	paused.Status = schedule.StatusPaused
	mustSaveChannel(t, s, paused)
	// ⚠ **DUE: a ZERO deadline means due NOW** (§9 V54), and this case was never covered.
	//
	// The claim carried `AND reconcile_deadline > 0`, and the deadline's only writer is the LAST
	// step of a SUCCESSFUL reconcile — so a channel whose first reconcile failed kept 0 and was
	// invisible to the sweep FOREVER: stranded in `building`, never pushed to Tunarr, while the
	// binder's comment promised "the sweep retries". Found in the wild on a real install.
	//
	// It is asserted here, in the conformance suite, because the guard lived in two hand-written
	// dialect statements that no test compared — exactly the drift class one suite / two backends
	// exists to catch.
	zero := sampleChannel("ch-never-reconciled", 4, time.Time{})
	zero.Status = schedule.StatusBuilding
	mustSaveChannel(t, s, zero)

	claimed, err := s.ClaimDueChannels(ctx, now, time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, c := range claimed {
		ids[c.ID] = true
	}
	if len(claimed) != 2 || !ids["ch-due"] || !ids["ch-never-reconciled"] {
		t.Fatalf("ClaimDueChannels = %d rows (%v), want ch-due AND ch-never-reconciled — a channel "+
			"whose first reconcile never ran must not be stranded", len(claimed), ids)
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
	if got.Revision != dueBefore.Revision+1 {
		t.Errorf("claimed channel revision = %d, want %d", got.Revision, dueBefore.Revision+1)
	}
	dueBefore.Name = "stale writer"
	if _, err := s.SaveChannel(ctx, dueBefore); !errors.Is(err, ErrChannelStale) {
		t.Errorf("pre-claim snapshot save = %v, want ErrChannelStale", err)
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
		mustSaveChannel(t, s, sampleChannel("chan-"+string(rune('a'+i)), i+1, now.Add(-time.Hour)))
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

	for i, tt := range []struct {
		name    string
		episode schedule.ResolvedProgram
	}{
		{name: "blank identity", episode: schedule.ResolvedProgram{DurationMs: 1, Season: 1, Episode: 1}},
		{name: "zero runtime", episode: schedule.ResolvedProgram{LibraryItemID: "ep", Season: 1, Episode: 1}},
		{name: "negative runtime", episode: schedule.ResolvedProgram{LibraryItemID: "ep", DurationMs: -1, Season: 1, Episode: 1}},
		{name: "negative season", episode: schedule.ResolvedProgram{LibraryItemID: "ep", DurationMs: 1, Season: -1, Episode: 1}},
		{name: "missing or zero episode", episode: schedule.ResolvedProgram{LibraryItemID: "ep", DurationMs: 1, Season: 1}},
		{name: "negative episode", episode: schedule.ResolvedProgram{LibraryItemID: "ep", DurationMs: 1, Season: 1, Episode: -1}},
		{name: "negative episode end", episode: schedule.ResolvedProgram{LibraryItemID: "ep", DurationMs: 1, Season: 1, Episode: 1, EpisodeEnd: -1}},
		{name: "episode end before episode", episode: schedule.ResolvedProgram{LibraryItemID: "ep", DurationMs: 1, Season: 1, Episode: 2, EpisodeEnd: 1}},
	} {
		t.Run("RejectsUnplayableWrite/"+tt.name, func(t *testing.T) {
			libraryID := fmt.Sprintf("invalid-show-%d", i)
			err := st.UpsertSeriesEpisodes(ctx, SeriesEpisodes{LibraryID: libraryID, Episodes: []schedule.ResolvedProgram{tt.episode}})
			if err == nil {
				t.Fatalf("structurally unplayable episode was persisted: %+v", tt.episode)
			}
			if _, err := st.GetSeriesEpisodes(ctx, libraryID); !errors.Is(err, ErrNotFound) {
				t.Fatalf("rejected write left a cache row: %v", err)
			}
		})
	}

	fetched := time.Now().Truncate(time.Second)
	eps := []schedule.ResolvedProgram{
		{LibraryItemID: "ep-1", Title: "Pilot", DurationMs: 1_320_000, Season: 1, Episode: 1, Year: 1993,
			CommunityRating: 9.1, Overview: "A Christmas homecoming", Tags: []string{"holiday", "family"}},
		{LibraryItemID: "ep-2", Title: "Second", DurationMs: 1_320_000, Season: 1, Episode: 2, EpisodeEnd: 3},
		{LibraryItemID: "ep-3", Title: "Third", DurationMs: 1_320_000, Season: 1, Episode: 4,
			CommunityRating: 11, Overview: strings.Repeat("x", 2050), Tags: append([]string{"holiday"}, make([]string, 20)...)},
	}
	if err := st.UpsertSeriesEpisodes(ctx, SeriesEpisodes{LibraryID: "show-a", Episodes: eps, FetchedAt: fetched}); err != nil {
		t.Fatal(err)
	}

	got, err := st.GetSeriesEpisodes(ctx, "show-a")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Episodes) != 3 || got.Episodes[0].LibraryItemID != "ep-1" {
		t.Fatalf("round-trip lost episodes: %+v", got.Episodes)
	}
	if got.Episodes[0].Year != 1993 {
		t.Fatalf("Year = %d, want 1993 (episode era must survive the cache blob)", got.Episodes[0].Year)
	}
	if got.Episodes[0].CommunityRating != 9.1 || got.Episodes[0].Overview != "A Christmas homecoming" ||
		len(got.Episodes[0].Tags) != 2 || got.Episodes[0].Tags[0] != "holiday" || got.Episodes[0].Tags[1] != "family" {
		t.Fatalf("editorial evidence did not survive the cache blob: %+v", got.Episodes[0])
	}
	// EpisodeEnd is the multi-part span (§5) — a field a naive round-trip silently drops.
	if got.Episodes[1].EpisodeEnd != 3 {
		t.Fatalf("EpisodeEnd = %d, want 3 (multi-part span must survive the blob)", got.Episodes[1].EpisodeEnd)
	}
	if got.Episodes[2].CommunityRating != 0 || len([]rune(got.Episodes[2].Overview)) != 2048 ||
		len(got.Episodes[2].Tags) != 1 {
		t.Fatalf("durable write/read did not sanitize episode evidence: %+v", got.Episodes[2])
	}
	if !got.FetchedAt.Equal(fetched) {
		t.Fatalf("FetchedAt = %v, want %v", got.FetchedAt, fetched)
	}

	// Upsert REPLACES rather than merging: the library's answer is the truth for that show, so
	// an episode it no longer reports must disappear rather than linger in every lineup.
	if err := st.UpsertSeriesEpisodes(ctx, SeriesEpisodes{
		LibraryID: "show-a",
		Episodes: []schedule.ResolvedProgram{{
			LibraryItemID: "ep-1", Title: "Pilot", DurationMs: 1_320_000, Season: 1, Episode: 1,
		}},
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
