package channels_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

// --- test harness ---

func newStore(t *testing.T) store.Store {
	t.Helper()
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "test.db")
	s, err := store.Open(context.Background(), dsn, true)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// mapAvail is a mutable Availability tests drive to simulate content landing.
type mapAvail map[provision.Key]string

func (m mapAvail) Resolve(k provision.Key) (string, int64, bool) {
	id, ok := m[k]
	return id, 0, ok
}
func (m mapAvail) ResolveEpisodes(provision.Key) ([]schedule.ResolvedProgram, bool) {
	return nil, false
}

// seedChannel writes a channel with the given approved lineup, unreconciled.
func seedChannel(t *testing.T, st store.Store, id string, number int, entries ...schedule.LineupEntry) {
	t.Helper()
	ch := store.Channel{Lineup: entries}
	ch.ID = id
	ch.Name = "Ch " + id
	ch.Number = number
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusBuilding
	if err := st.UpsertChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
}

func entry(key, title string) schedule.LineupEntry {
	return schedule.LineupEntry{Key: provision.Key(key), Title: title, DurationMs: 3600000}
}

func newEngine(st store.Store, tun *testkit.Tunarr, avail channels.Availability, guide channels.GuidePoker) *channels.Engine {
	return channels.New(st, tun, avail, guide, channels.Config{ReconcileTTL: 10 * time.Minute},
		func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }, testkit.Logger())
}

// --- §19 gate tests ---

// A fresh channel with all content available reconciles to a live Tunarr channel;
// a second reconcile is a no-op (idempotent, minimal-diff).
func TestReconcile_CreatesThenIdempotent(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	avail := mapAvail{"movie:tmdb:1": "lib-1", "movie:tmdb:2": "lib-2"}
	e := newEngine(st, tun, avail, nil)
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"))

	// First reconcile: creates the Tunarr channel + pushes the lineup.
	if err := e.Reconcile(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	if tun.Creates != 1 {
		t.Fatalf("want 1 create, got %d", tun.Creates)
	}
	if tun.Pushes != 1 {
		t.Fatalf("want 1 lineup push, got %d", tun.Pushes)
	}
	// The server-assigned id was persisted (Phase-0 finding 1).
	ch, _ := st.GetChannel(context.Background(), "c1")
	if ch.TunarrID == "" {
		t.Fatal("server-assigned TunarrID not persisted")
	}
	if ch.Status != schedule.StatusLive {
		t.Fatalf("status = %s, want live", ch.Status)
	}

	// Second reconcile with no input change: no new create, NO new push.
	if err := e.Reconcile(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	if tun.Creates != 1 {
		t.Errorf("idempotent reconcile created again: %d creates", tun.Creates)
	}
	if tun.Pushes != 1 {
		t.Errorf("idempotent reconcile re-pushed lineup: %d pushes (want 1)", tun.Pushes)
	}
}

// A lineup-push failure right after channel creation must NOT lose the new Tunarr
// id: the create is checkpointed to the store before the push. So the next reconcile
// UPDATES the existing channel instead of re-creating it — the fix for the live-smoke
// orphan loop, where a push timeout discarded the id and every retry re-created the
// channel at the same number and collided (a 500). Regression guard for §9 atomicity.
func TestReconcile_ChannelIDCheckpointedBeforeLineupPush(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	tun.SetLineupErr = context.DeadlineExceeded // simulate the big-library resolve timeout
	avail := mapAvail{"movie:tmdb:1": "lib-1"}
	e := newEngine(st, tun, avail, nil)
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:1", "A"))

	// First reconcile: the channel IS created in Tunarr, but the lineup push fails →
	// the reconcile returns an error.
	if err := e.Reconcile(context.Background(), "c1"); err == nil {
		t.Fatal("expected the reconcile to surface the lineup-push failure")
	}
	if tun.Creates != 1 {
		t.Fatalf("channel should have been created once, got %d creates", tun.Creates)
	}
	// The critical assertion: despite the push failure, the new id was checkpointed.
	ch, _ := st.GetChannel(context.Background(), "c1")
	if ch.TunarrID == "" {
		t.Fatal("new Tunarr id was NOT checkpointed — a retry will re-create and collide")
	}
	checkpointed := ch.TunarrID

	// Clear the injected failure and reconcile again: it must UPDATE the same channel,
	// never create a second one (no orphan, no number collision).
	tun.SetLineupErr = nil
	if err := e.Reconcile(context.Background(), "c1"); err != nil {
		t.Fatalf("retry reconcile should succeed once the push works, got %v", err)
	}
	if tun.Creates != 1 {
		t.Errorf("retry RE-CREATED the channel (%d creates) — the orphan-loop bug", tun.Creates)
	}
	if tun.Pushes != 1 {
		t.Errorf("retry should have pushed the lineup once, got %d", tun.Pushes)
	}
	ch, _ = st.GetChannel(context.Background(), "c1")
	if ch.TunarrID != checkpointed {
		t.Errorf("id changed across retry: %q → %q (should be stable)", checkpointed, ch.TunarrID)
	}
	if ch.Status != schedule.StatusLive {
		t.Errorf("after a successful retry, status = %s, want live", ch.Status)
	}
}

// fakeRatings is a RatingResolver over a fixed key→rating map (present = ok).
type fakeRatings map[provision.Key]string

func (f fakeRatings) Rating(_ context.Context, k provision.Key) (string, bool, error) {
	r, ok := f[k]
	return r, ok, nil
}

// §389 amendment: an entry that reached the scheduler UNRATED (an acquisition that
// wasn't in the library at proposal time, or a pre-fix cached proposal) is healed
// from the library at reconcile — otherwise a fail-closed audience ceiling drops it
// and the channel plays nothing (§9 dead air, the shape of FINDING 6). The heal is
// persisted, so it happens once.
func TestReconcile_HealsUnratedEntryFromLibrary(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	avail := mapAvail{"movie:tmdb:1": "lib-1"}
	// The library rates it TV-Y7 — within the channel's kids ceiling.
	e := newEngine(st, tun, avail, nil).
		WithRatings(fakeRatings{"movie:tmdb:1": "TV-Y7"})

	// A channel whose approved entry has NO rating, under a TV-Y7 audience ceiling.
	ch := store.Channel{Lineup: []schedule.LineupEntry{
		{Key: "movie:tmdb:1", Title: "Kids Pick", DurationMs: 3600000}, // OfficialRating unset
	}}
	ch.ID, ch.Number, ch.Strategy, ch.Status = "cheal", 7, schedule.Sequential, schedule.StatusBuilding
	ch.Policy = schedule.ChannelPolicy{Audience: schedule.AudiencePolicy{Ceiling: "TV-Y7"}}
	if err := st.UpsertChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}

	if err := e.Reconcile(context.Background(), "cheal"); err != nil {
		t.Fatal(err)
	}

	got, _ := st.GetChannel(context.Background(), "cheal")
	// Healed AND persisted, so a future reconcile skips the lookup.
	if got.Lineup[0].OfficialRating != "TV-Y7" {
		t.Errorf("entry rating = %q, want TV-Y7 (healed from the library + persisted)", got.Lineup[0].OfficialRating)
	}
	// And the payoff: the entry survived the audience gate and became a program,
	// rather than being dropped to dead air.
	if n := programCount(got); n < 1 {
		t.Errorf("channel has %d programs — the unrated entry was dropped instead of healed", n)
	}
}

// Backfill: a pending slot is pod-filled, then the title lands and an availability
// event places the real program IN PLACE, re-pushing the lineup.
func TestReconcile_BackfillOnAvailabilityEvent(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	avail := mapAvail{"movie:tmdb:1": "lib-1"} // #2 not yet available
	e := newEngine(st, tun, avail, nil)
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"))

	if err := e.Reconcile(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	// Only 1 program so far; slot 2 is pod-fill (flex on the wire).
	ch, _ := st.GetChannel(context.Background(), "c1")
	if got := programCount(ch); got != 1 {
		t.Fatalf("want 1 program before backfill, got %d", got)
	}
	pushesBefore := tun.Pushes

	// #2 lands. Emit the availability event.
	avail["movie:tmdb:2"] = "lib-2"
	e.OnAvailability(context.Background(), provision.DomainEvent{
		Key: "movie:tmdb:2", State: provision.Available,
	})

	ch, _ = st.GetChannel(context.Background(), "c1")
	if got := programCount(ch); got != 2 {
		t.Fatalf("want 2 programs after backfill, got %d", got)
	}
	if tun.Pushes <= pushesBefore {
		t.Fatalf("backfill did not re-push the lineup (pushes %d→%d)", pushesBefore, tun.Pushes)
	}
	// Placed in place: slot index 1 is B's program.
	if ch.Desired[1].Kind != schedule.SlotProgram || ch.Desired[1].LibraryItemID != "lib-2" {
		t.Errorf("B not placed in its own slot: %+v", ch.Desired[1])
	}
}

// Event-loss recovery (§9/§19): DROP the availability event entirely; the
// periodic sweep must still backfill from the store.
func TestSweep_RecoversFromLostEvent(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	avail := mapAvail{"movie:tmdb:1": "lib-1"}
	e := newEngine(st, tun, avail, nil)
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"))

	if err := e.Reconcile(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}

	// #2 lands, but we DO NOT emit the event (simulate a crash between event and
	// re-push, or a cross-replica in-memory event that never arrives).
	avail["movie:tmdb:2"] = "lib-2"

	// The reconcile set a future ReconcileDeadline; wind the clock past it so the
	// sweep claims the channel.
	now := time.Unix(1_800_000_000, 0).Add(20 * time.Minute)
	r := channels.NewRunner(e, st, time.Minute, 5*time.Minute, 50,
		func() time.Time { return now }, testkit.Logger())

	n := r.Sweep(context.Background())
	if n != 1 {
		t.Fatalf("sweep reconciled %d channels, want 1", n)
	}
	ch, _ := st.GetChannel(context.Background(), "c1")
	if got := programCount(ch); got != 2 {
		t.Fatalf("sweep did not backfill the lost event: %d programs, want 2", got)
	}
}

// Drift substitution (§9/§19): a scheduled program vanishes from the library; the
// sweep flags the channel drifted and demotes the slot.
func TestSweep_FlagsDriftWhenProgramVanishes(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	avail := mapAvail{"movie:tmdb:1": "lib-1", "movie:tmdb:2": "lib-2"}
	e := newEngine(st, tun, avail, nil)
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:1", "A"), entry("movie:tmdb:2", "B"))
	if err := e.Reconcile(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}

	// #2 is deleted/re-id'd in the library.
	delete(avail, "movie:tmdb:2")

	now := time.Unix(1_800_000_000, 0).Add(20 * time.Minute)
	r := channels.NewRunner(e, st, time.Minute, 5*time.Minute, 50,
		func() time.Time { return now }, testkit.Logger())
	r.Sweep(context.Background())

	ch, _ := st.GetChannel(context.Background(), "c1")
	if ch.Status != schedule.StatusDrifted {
		t.Fatalf("status = %s, want drifted", ch.Status)
	}
	if ch.Desired[1].IsProgram() {
		t.Errorf("vanished program not demoted: %+v", ch.Desired[1])
	}
}

// Media-server freshness (§9), operation-specific: CREATING a channel triggers a
// tuner RE-SCAN (so the media server discovers the new channel — a guide refresh
// alone won't surface it, the bug found in the first live smoke); an idempotent
// no-op reconcile pokes nothing.
func TestReconcile_RescansTunerOnCreate(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	avail := mapAvail{"movie:tmdb:1": "lib-1"}
	guide := &fakeGuide{}
	e := newEngine(st, tun, avail, guide)
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:1", "A"))

	_ = e.Reconcile(context.Background(), "c1") // create
	if guide.rescans != 1 {
		t.Fatalf("create should re-scan the tuner once (discover new channel), got %d", guide.rescans)
	}
	if guide.pokes != 0 {
		t.Errorf("create should re-scan the tuner, not (only) poke the guide: pokes=%d", guide.pokes)
	}

	_ = e.Reconcile(context.Background(), "c1") // no-op
	if guide.rescans != 1 || guide.pokes != 0 {
		t.Errorf("no-op reconcile touched the media server: rescans=%d pokes=%d (want 1/0)", guide.rescans, guide.pokes)
	}
}

// A detached channel is never reconciled (§9 ownership).
func TestReconcile_SkipsDetached(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	e := newEngine(st, tun, mapAvail{}, nil)
	ch := store.Channel{}
	ch.ID = "c1"
	ch.Name = "X"
	ch.Number = 5
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusDetached
	_ = st.UpsertChannel(context.Background(), ch)

	if err := e.Reconcile(context.Background(), "c1"); err != nil {
		t.Fatal(err)
	}
	if tun.Creates != 0 {
		t.Errorf("detached channel was reconciled: %d creates", tun.Creates)
	}
}

// A guide-poke failure degrades freshness but never fails the reconcile (§9).
func TestReconcile_GuidePokeFailureIsNonFatal(t *testing.T) {
	st := newStore(t)
	tun := testkit.NewTunarr()
	guide := &fakeGuide{err: errPoke}
	e := newEngine(st, tun, mapAvail{"movie:tmdb:1": "lib-1"}, guide)
	seedChannel(t, st, "c1", 5, entry("movie:tmdb:1", "A"))

	if err := e.Reconcile(context.Background(), "c1"); err != nil {
		t.Fatalf("guide poke failure must not fail the reconcile: %v", err)
	}
	ch, _ := st.GetChannel(context.Background(), "c1")
	if ch.Status != schedule.StatusLive {
		t.Errorf("channel should still be live despite poke failure, got %s", ch.Status)
	}
}

// programCount counts real program slots in a persisted channel's desired lineup.
func programCount(ch store.Channel) int {
	return schedule.DesiredLineup{Slots: ch.Desired}.ProgramCount()
}

// --- fakes ---

type fakeGuide struct {
	pokes   int // guide (EPG) refreshes — for existing-channel lineup changes
	rescans int // tuner re-scans — for new/removed channels (§9)
	err     error
}

func (f *fakeGuide) PokeGuideRefresh(context.Context) error {
	f.pokes++
	return f.err
}
func (f *fakeGuide) RescanTuner(context.Context) error {
	f.rescans++
	return f.err
}

var errPoke = errPokeType("poke boom")

type errPokeType string

func (e errPokeType) Error() string { return string(e) }
