package store

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
)

// Channels and the scheduling caches (§5, §9): the channel row and its lease, plus the
// two derived tables the scheduler reads — cached series episodes and airing history.

func sampleChannel(id string, number int, deadline time.Time) Channel {
	ch := Channel{}
	ch.ID = id
	ch.IntentRef = "intent-" + id
	ch.IntentJSON = `{"description":"intent of ` + id + `"}`
	ch.Name = "Channel " + id
	ch.Number = number
	ch.Group = "Loomarr"
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusLive
	ch.Shuffle.Seed = 7
	ch.UpdatedAt = 1_700_000_000
	ch.Lineup = []schedule.LineupEntry{
		{Key: "movie:tmdb:90000001", Title: "A", DurationMs: 3600000},
		{Key: "movie:tmdb:90000002", Title: "B"},
	}
	ch.Desired = []schedule.Slot{
		{Kind: schedule.SlotProgram, Key: "movie:tmdb:90000001", LibraryItemID: "lib-1", Title: "A", DurationMs: 3600000},
		{Kind: schedule.SlotFiller, Key: "movie:tmdb:90000002", Title: "B"},
	}
	// A non-empty policy exercises the policy_json round-trip: an audience ceiling,
	// a separation window (Duration → "168h" JSON), an era range, and an applied
	// relaxation (the enforcement-written field). Every column path must preserve it.
	ch.Policy = schedule.ChannelPolicy{
		ProposalPolicy: schedule.ProposalPolicy{
			Audience:   schedule.AudiencePolicy{Ceiling: "TV-Y7"},
			Separation: schedule.SeparationPolicy{EpisodeNoRepeat: schedule.Duration(168 * time.Hour)},
			Scope:      schedule.ScopePolicy{Dates: schedule.EraDates(schedule.Range{From: 1990, To: 1999})},
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
	if len(got.Lineup) != 2 || got.Lineup[0].Key != "movie:tmdb:90000001" || got.Lineup[0].DurationMs != 3600000 {
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
	if era, ok := got.Policy.Scope.Dates.Era(); !ok || era != (schedule.Range{From: 1990, To: 1999}) {
		t.Errorf("policy dates round-trip: got %+v", got.Policy.Scope.Dates)
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
	if _, err := s.GetChannel(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetChannel(missing) = %v, want ErrNotFound", err)
	}
	if _, err := s.GetChannelByNumber(ctx, 999); !errors.Is(err, ErrNotFound) {
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
	if _, err := s.GetChannel(ctx, "ch-2"); !errors.Is(err, ErrNotFound) {
		t.Errorf("deleted channel still present: %v", err)
	}
}

// testChannelOrigin (#1663, Home's "New channel"): a channel remembers when it was created, which
// no later save moves, and who asked for it, read from the channel's own `requested_by`.
func testChannelOrigin(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	for _, u := range []User{{ID: "u-member", Name: "Member One", Role: RoleMember}, {ID: "u-gone", Name: "Gone", Role: RoleMember}} {
		if err := s.UpsertUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	day0 := time.Unix(1_800_000_000, 0).UTC()

	requested := sampleChannel("ch-req", 1, time.Time{})
	requested.IntentRef, requested.RequestedBy = "job-member", "u-member"
	before := time.Now().Add(-time.Second)
	mustSaveChannel(t, s, requested) // no CreatedAt: the store stamps it
	handMade := sampleChannel("ch-hand", 2, time.Time{})
	handMade.IntentRef, handMade.RequestedBy, handMade.CreatedAt = "", "", day0
	mustSaveChannel(t, s, handMade)
	ghost := sampleChannel("ch-ghost", 3, time.Time{})
	ghost.IntentRef, ghost.RequestedBy = "job-ghost", "u-gone"
	mustSaveChannel(t, s, ghost)
	// The person is deleted: the channel stays, with no requester (requested_by is SET NULL).
	sqlSt, ok := s.(*sqlStore)
	if !ok {
		t.Skip("needs the SQL store")
	}
	if _, err := sqlSt.db.ExecContext(ctx, sqlSt.ph(`DELETE FROM users WHERE id = ?`), "u-gone"); err != nil {
		t.Fatal(err)
	}
	if g, err := s.GetChannel(ctx, "ch-ghost"); err != nil || g.RequestedBy != "" {
		t.Errorf("channel whose requester was deleted = %q, %v; want kept with no requester", g.RequestedBy, err)
	}
	// A request outlives its author, and a channel read before they went may be saved after: either
	// save records no requester rather than failing.
	stray := sampleChannel("ch-stray", 4, time.Time{})
	stray.IntentRef, stray.RequestedBy = "job-stray", "u-gone"
	mustSaveChannel(t, s, stray)
	ghost.Revision = 1
	ghost.Name = "Ghost, renamed"
	mustSaveChannel(t, s, ghost)
	for _, id := range []string{"ch-stray", "ch-ghost"} {
		if g, err := s.GetChannel(ctx, id); err != nil || g.RequestedBy != "" {
			t.Errorf("%s saved naming a deleted requester = %q, %v; want no requester", id, g.RequestedBy, err)
		}
	}
	// Underneath, a requester must be a person: the column is a foreign key.
	if _, err := sqlSt.db.ExecContext(ctx, sqlSt.ph(`UPDATE channels SET requested_by = ? WHERE id = ?`),
		"u-never", "ch-stray"); err == nil {
		t.Error("wrote a requester who is no person; the foreign key is missing")
	}

	got, err := s.GetChannel(ctx, "ch-req")
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatedAt.Before(before) || got.CreatedAt.After(time.Now().Add(time.Second)) {
		t.Errorf("stamped CreatedAt = %v, want about now", got.CreatedAt)
	}
	hand, _ := s.GetChannel(ctx, "ch-hand")
	hand.Name, hand.CreatedAt = "Renamed", day0.Add(48*time.Hour)
	mustSaveChannel(t, s, hand)
	if again, _ := s.GetChannel(ctx, "ch-hand"); !again.CreatedAt.Equal(day0) {
		t.Errorf("CreatedAt after an update = %v, want %v (creation never moves)", again.CreatedAt, day0)
	}

	requesters, err := s.ChannelRequesters(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// A hand-made channel and one whose requester is gone have no requester.
	if want := map[string]string{"ch-req": "Member One"}; fmt.Sprint(requesters) != fmt.Sprint(want) {
		t.Errorf("ChannelRequesters = %v, want %v", requesters, want)
	}
}

// Deleting a channel must drop its IMAGE REFS (channel_images cascades, #2793).
//
// ⚠ **This is a regression test for a real leak.** A deleted channel's ref once survived it, and a
// ref is precisely what tells the GC an image is still in use (§22), so the icon was never orphaned
// and never collected — bytes on disk owned by a channel that no longer exists, forever.
// A lineup is rows in channel_lineup (#2793): it reads back in order, every field intact; a
// shorter lineup leaves no stale rows behind; and the rows go with their channel.
func testChannelLineupRows(t *testing.T, newStore NewStoreFunc) {
	s := newStore(t)
	ctx := context.Background()
	ch := sampleChannel("ch-lineup", 41, time.Time{})
	ch.Lineup = []schedule.LineupEntry{
		{Key: "series:tmdb:90000013", Title: "C", SeasonMin: 1, SeasonMax: 4, Genres: []string{"Comedy"}, Year: 1991,
			EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights}, BoxSetIDs: []string{"bs-1"}, BoxSetsResolved: true},
		{Key: "movie:tmdb:90000011", Title: "A", DurationMs: 3600000, CollectionID: -1, Fit: 0.75},
		{Key: "movie:tmdb:90000012", Title: "B", AddedAt: time.Unix(1_700_000_000, 0).UTC()},
	}
	saved := mustSaveChannel(t, s, ch)
	got, err := s.GetChannel(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Lineup, ch.Lineup) {
		t.Fatalf("lineup read back as\n%+v\nwant\n%+v", got.Lineup, ch.Lineup)
	}

	got.Lineup = got.Lineup[1:2]
	saved = mustSaveChannel(t, s, got)
	if again, err := s.GetChannel(ctx, saved.ID); err != nil || len(again.Lineup) != 1 || again.Lineup[0].Key != "movie:tmdb:90000011" {
		t.Fatalf("after shortening, lineup = %+v (err %v); want just movie:tmdb:90000011", again.Lineup, err)
	}

	saved.Lineup = nil
	saved = mustSaveChannel(t, s, saved)
	if again, err := s.GetChannel(ctx, saved.ID); err != nil || again.Lineup == nil || len(again.Lineup) != 0 {
		t.Fatalf("an emptied lineup read back as %#v (err %v); want an empty, non-nil list", again.Lineup, err)
	}

	saved.Lineup = ch.Lineup
	saved = mustSaveChannel(t, s, saved)

	// Every lineup title has a titles row (title_key is a foreign key); one nothing has asked for
	// is pending, which reads as no record and counts in no state.
	if _, err := s.GetTitle(ctx, ch.Lineup[0].Key); !errors.Is(err, ErrNotFound) {
		t.Errorf("a pending lineup title read as %v, want ErrNotFound", err)
	}
	if counts, err := s.CountTitlesByState(ctx); err != nil || len(counts) != 0 {
		t.Errorf("titles by state = %v (err %v); pending titles must not count", counts, err)
	}
	// A real write of the title replaces its pending row.
	asked := provision.Record{Key: ch.Lineup[1].Key, State: provision.Wanted,
		Title: provision.Title{MediaType: provision.Movie, TMDBID: 90000011, Name: "A"}}
	if err := s.UpsertTitle(ctx, asked); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetTitle(ctx, asked.Key); err != nil || got.State != provision.Wanted {
		t.Errorf("after a request the title reads (%+v, %v), want wanted", got, err)
	}

	if err := s.DeleteChannel(ctx, saved.ID, saved.Revision); err != nil {
		t.Fatal(err)
	}
	sqlSt, ok := s.(*sqlStore)
	if !ok {
		return
	}
	var left int
	if err := sqlSt.db.QueryRowContext(ctx, `SELECT count(*) FROM channel_lineup`).Scan(&left); err != nil || left != 0 {
		t.Errorf("%d lineup rows outlived their channel (err %v)", left, err)
	}
	// Once no lineup names them, the purge takes the pending rows, and only those.
	if _, err := s.PurgeUnusedTitles(ctx, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	var pending, kept int
	if err := sqlSt.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN state = 'pending' THEN 1 ELSE 0 END), 0), COUNT(*) FROM titles`).Scan(&pending, &kept); err != nil {
		t.Fatal(err)
	}
	if pending != 0 || kept != 1 {
		t.Errorf("after the purge %d pending of %d titles remain; want 0 of 1 (the requested one)", pending, kept)
	}
}

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

// testScheduledAiringHistory pins the scheduled ledger (#1677): what a channel's schedule put on air
// whether or not anyone watched, kept apart from the watched airings (decision 1 of #1678: "aired"
// means both, as separate signals). It reads like the watched history: latest airing per unit, a
// strict recorded-before cutoff, per-channel scoping, and the same retention.
func testScheduledAiringHistory(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	for i, id := range []string{"sched-1", "sched-2"} {
		mustSaveChannel(t, st, sampleChannel(id, i+1, time.Time{}))
	}
	opened := time.Now().Truncate(time.Second)
	film := provision.Key("movie:tmdb:90000149")
	show := provision.Key("series:tvdb:90071663")
	record := func(ch string, recordedAt time.Time, airings ...ScheduledAiring) {
		t.Helper()
		for _, a := range airings {
			if a.LibraryItemID != "" {
				saveLibraryItems(t, st, a.LibraryItemID)
			}
		}
		if err := st.RecordScheduledAirings(ctx, ch, airings, recordedAt); err != nil {
			t.Fatalf("record %s: %v", ch, err)
		}
	}
	read := func(ch string, before time.Time) map[string]time.Time {
		t.Helper()
		got, err := st.LastScheduledByChannel(ctx, ch, before)
		if err != nil {
			t.Fatalf("read %s: %v", ch, err)
		}
		return got
	}

	// One reconcile records everything that has started; the next records it again with the newer
	// programmes. Only the first write of an airing lands, so recorded_at is when it was first seen.
	record("sched-1", opened.Add(-20*time.Hour),
		ScheduledAiring{Key: film, LibraryItemID: "lib-1", AiredAt: opened.Add(-26 * time.Hour)},
		ScheduledAiring{Key: show, LibraryItemID: "ep-1", AiredAt: opened.Add(-22 * time.Hour)})
	record("sched-1", opened.Add(time.Hour),
		ScheduledAiring{Key: show, LibraryItemID: "ep-1", AiredAt: opened.Add(-22 * time.Hour)},
		ScheduledAiring{Key: show, LibraryItemID: "ep-2", AiredAt: opened.Add(-30 * time.Minute)})
	got := read("sched-1", opened)
	if len(got) != 2 || !got["lib-1"].Equal(opened.Add(-26*time.Hour)) || !got["ep-1"].Equal(opened.Add(-22*time.Hour)) {
		t.Fatalf("history as of the window start = %v, want lib-1 and ep-1 only", got)
	}
	if _, late := got["ep-2"]; late {
		t.Error("an airing first recorded after the window opened changed that window's history")
	}
	if got := read("sched-1", opened.Add(2*time.Hour)); !got["ep-2"].Equal(opened.Add(-30 * time.Minute)) {
		t.Errorf("next read = %v, want ep-2 recorded", got)
	}

	// The latest airing of a unit wins, and retention keeps the newest one past the horizon.
	record("sched-2", opened.Add(-30*24*time.Hour), ScheduledAiring{Key: film, LibraryItemID: "lib-1", AiredAt: opened.Add(-30 * 24 * time.Hour)})
	record("sched-2", opened.Add(-20*24*time.Hour), ScheduledAiring{Key: film, LibraryItemID: "lib-1", AiredAt: opened.Add(-20 * 24 * time.Hour)})
	record("sched-2", opened, ScheduledAiring{Key: film, LibraryItemID: "lib-1", AiredAt: opened})
	if got := read("sched-2", opened); !got["lib-1"].Equal(opened.Add(-20 * 24 * time.Hour)) {
		t.Errorf("after pruning, last scheduled before the window = %v, want 20 days ago", got["lib-1"])
	}
	if got := read("sched-2", opened.Add(-25*24*time.Hour)); len(got) != 0 {
		t.Errorf("pruned airings still answer a read far outside retention: %v", got)
	}
	if got := read("sched-2", opened.Add(time.Hour)); len(got) != 1 || !got["lib-1"].Equal(opened) {
		t.Errorf("sched-2 history = %v, want only its own airing", got)
	}

	// The two signals stay apart: a scheduled airing is not a watched one.
	if watched, err := st.LastAiredByChannel(ctx, "sched-1", opened.Add(2*time.Hour)); err != nil || len(watched) != 0 {
		t.Errorf("watched history = %v, %v; want the scheduled ledger kept out of it", watched, err)
	}
	if none, err := st.LastScheduledByChannel(ctx, "ch-never", opened); err != nil || len(none) != 0 {
		t.Errorf("unknown channel = %v, %v; want empty", none, err)
	}
	// An airing with no unit identifies nothing and is skipped, as RecordAiring skips it.
	record("sched-1", opened.Add(3*time.Hour), ScheduledAiring{Key: film, AiredAt: opened.Add(2 * time.Hour)})
}

// testAiringHistory pins the recency signal's storage contract (§5, programming-design §3.1).
//
// The scheduler's separation rules are within-cycle; this table is the ONLY memory of what aired
// across cycles. Its properties — latest airing per UNIT, a strict recorded-before cutoff, and
// per-channel scoping — are what make recency-aware placement possible without letting it
// re-arrange a window on air (#1674).
func testAiringHistory(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	// An airing belongs to a channel (a foreign key since #2323), so each channel exists first.
	for i, id := range []string{"ch-1", "ch-2", "ch-bykey", "ch-bykey-other"} {
		mustSaveChannel(t, st, sampleChannel(id, i+1, time.Time{}))
	}

	// `opened` plays the start of the window being arranged.
	opened := time.Now().Truncate(time.Second)
	record := func(ch string, key provision.Key, item string, airedAt, recordedAt time.Time) {
		t.Helper()
		if item != "" {
			saveLibraryItems(t, st, item)
		}
		if err := st.RecordAiring(ctx, ch, key, item, airedAt, recordedAt); err != nil {
			t.Fatalf("record %s/%s: %v", ch, item, err)
		}
	}
	read := func(ch string, before time.Time) map[string]time.Time {
		t.Helper()
		got, err := st.LastAiredByChannel(ctx, ch, before)
		if err != nil {
			t.Fatalf("read %s: %v", ch, err)
		}
		return got
	}
	film := provision.Key("movie:tmdb:90000149")
	show := provision.Key("series:tvdb:90071663")

	record("ch-1", film, "lib-90000149", opened.Add(-72*time.Hour), opened.Add(-72*time.Hour))
	// Two episodes of ONE series are two units: history is per episode, not per title.
	record("ch-1", show, "ep-1", opened.Add(-30*time.Hour), opened.Add(-30*time.Hour))
	record("ch-1", show, "ep-2", opened.Add(-29*time.Hour), opened.Add(-29*time.Hour))

	got := read("ch-1", opened)
	if len(got) != 3 || !got["ep-1"].Equal(opened.Add(-30*time.Hour)) || !got["ep-2"].Equal(opened.Add(-29*time.Hour)) {
		t.Fatalf("history = %v, want one entry per unit", got)
	}

	// THE CUTOFF: the film re-airs inside the window. As of the window's start it still last aired
	// three days ago — an upserted "last airing" would have lost that — while the next window sees
	// the new airing. Playout re-resolves one airing many times; only the first write counts.
	record("ch-1", film, "lib-90000149", opened.Add(2*time.Hour), opened.Add(2*time.Hour))
	record("ch-1", film, "lib-90000149", opened.Add(2*time.Hour), opened.Add(3*time.Hour))
	if got := read("ch-1", opened); !got["lib-90000149"].Equal(opened.Add(-72 * time.Hour)) {
		t.Errorf("as of the window start, last-aired = %v, want the airing before it %v", got["lib-90000149"], opened.Add(-72*time.Hour))
	}
	if got := read("ch-1", opened.Add(24*time.Hour)); !got["lib-90000149"].Equal(opened.Add(2 * time.Hour)) {
		t.Errorf("next window last-aired = %v, want the latest airing %v", got["lib-90000149"], opened.Add(2*time.Hour))
	}

	// A LATE WRITE — a programme that started before the boundary, first observed after it (a
	// viewer tuning in mid-programme) — does not count for the window already open.
	record("ch-1", show, "ep-3", opened.Add(-10*time.Minute), opened.Add(5*time.Minute))
	if _, ok := read("ch-1", opened)["ep-3"]; ok {
		t.Error("an airing recorded after the window opened changed that window's history")
	}

	// RETENTION: a unit keeps its recent airings plus the newest older one, so the log stays
	// bounded by lineup size and a read as of any recent instant still has its answer.
	record("ch-2", film, "lib-90000149", opened.Add(-40*24*time.Hour), opened.Add(-40*24*time.Hour))
	record("ch-2", film, "lib-90000149", opened.Add(-30*24*time.Hour), opened.Add(-30*24*time.Hour))
	record("ch-2", film, "lib-90000149", opened.Add(-20*24*time.Hour), opened.Add(-20*24*time.Hour))
	record("ch-2", film, "lib-90000149", opened, opened)
	if got := read("ch-2", opened); !got["lib-90000149"].Equal(opened.Add(-20 * 24 * time.Hour)) {
		t.Errorf("after pruning, last-aired before the window = %v, want %v", got["lib-90000149"], opened.Add(-20*24*time.Hour))
	}
	if got := read("ch-2", opened.Add(-25*24*time.Hour)); len(got) != 0 {
		t.Errorf("pruned airings still answer a read far outside retention: %v", got)
	}

	// PER-CHANNEL SCOPING: the same film on two channels is two independent rotations, and
	// collapsing them would let one channel's schedule suppress another's.
	if got := read("ch-2", opened.Add(time.Hour)); len(got) != 1 || !got["lib-90000149"].Equal(opened) {
		t.Errorf("ch-2 history = %v, want only its own airing", got)
	}

	// A channel that has never aired anything reads as empty, not as an error — that is what
	// lets placement degrade to today's behaviour on a fresh install.
	none, err := st.LastAiredByChannel(ctx, "ch-never", opened)
	if err != nil {
		t.Fatalf("unknown channel must not error: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("unknown channel returned %d entries, want 0", len(none))
	}

	// BY KEY: the latest airing of any unit of a key, scoped to the channel.
	base := time.Now().Truncate(time.Second)
	epA, epB := provision.Key("series:tvdb:90071700"), provision.Key("movie:tmdb:90000700")
	record("ch-bykey", epA, "ep-1", base.Add(-48*time.Hour), base.Add(-48*time.Hour))
	record("ch-bykey", epA, "ep-2", base.Add(-24*time.Hour), base.Add(-24*time.Hour))
	record("ch-bykey", epB, "lib-90000700", base.Add(-72*time.Hour), base.Add(-72*time.Hour))
	record("ch-bykey-other", epB, "lib-90000700", base, base)
	keyed, kerr := st.LastAiredByKey(ctx, "ch-bykey")
	if kerr != nil {
		t.Fatalf("LastAiredByKey: %v", kerr)
	}
	if len(keyed) != 2 || !keyed[epA].Equal(base.Add(-24*time.Hour)) || !keyed[epB].Equal(base.Add(-72*time.Hour)) {
		t.Errorf("LastAiredByKey = %v, want the newest unit per key and no other channel's rows", keyed)
	}
	if empty, eerr := st.LastAiredByKey(ctx, "ch-never"); eerr != nil || len(empty) != 0 {
		t.Errorf("unknown channel = %v, %v, want an empty map", empty, eerr)
	}
}
