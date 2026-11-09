package scan

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/tmdb"
)

// clock is a settable clock for the Tracker's rate baseline.
type clock struct{ at time.Time }

func (c *clock) now() time.Time                    { return c.at }
func (c *clock) advance(d time.Duration) time.Time { c.at = c.at.Add(d); return c.at }

func newClockTracker() (*Tracker, *clock) {
	c := &clock{at: time.Unix(1_900_000_000, 0)}
	return NewTracker(c.now, nil), c
}

// A step counts what it was given to do, what grew and what is done, and names what it is on.
func TestTrackerCounts(t *testing.T) {
	tr, c := newClockTracker()
	if got := tr.Snapshot(c.now()); got != (ProgressSnapshot{}) {
		t.Fatalf("idle snapshot = %+v, want zero", got)
	}
	tr.Begin(StepReading, 10)
	tr.Grow(StepReading, 5)
	tr.Grow(StepReading, -2)
	tr.Advance(StepReading, 4)
	tr.Current(StepReading, "Night Ferry S02E03.mkv")
	got := tr.Snapshot(c.now())
	if got.Step != StepReading || got.Number != 2 || got.Done != 4 || got.Total != 13 || got.Current != "Night Ferry S02E03.mkv" {
		t.Fatalf("snapshot = %+v", got)
	}
	// Due raises the total only so far that the given number remain.
	tr.Due(StepReading, 20)
	if got := tr.Snapshot(c.now()); got.Total != 24 {
		t.Fatalf("total after due = %d, want 24", got.Total)
	}
	tr.Due(StepReading, 1)
	if got := tr.Snapshot(c.now()); got.Total != 24 {
		t.Fatalf("total after a smaller due = %d, want it kept at 24", got.Total)
	}
	// A step that has not begun ignores reports.
	tr.Advance(StepLookingUp, 3)
	tr.End(StepReading)
	if got := tr.Snapshot(c.now()); got != (ProgressSnapshot{}) {
		t.Fatalf("after end = %+v, want zero", got)
	}
	tr.Begin(StepLookingUp, 2)
	if got := tr.Snapshot(c.now()); got.Done != 0 || got.Number != 3 {
		t.Fatalf("a report before the step began counted: %+v", got)
	}
}

// The snapshot shows the earliest step still running.
func TestTrackerShowsTheEarliestActiveStep(t *testing.T) {
	tr, c := newClockTracker()
	tr.Begin(StepLookingUp, 5)
	tr.Begin(StepReading, 7)
	if got := tr.Snapshot(c.now()); got.Step != StepReading || got.Total != 7 {
		t.Fatalf("snapshot = %+v, want reading", got)
	}
	tr.Begin(StepWalking, 3)
	if got := tr.Snapshot(c.now()); got.Step != StepWalking || got.Number != 1 {
		t.Fatalf("snapshot = %+v, want walking", got)
	}
	tr.End(StepWalking)
	tr.End(StepReading)
	if got := tr.Snapshot(c.now()); got.Step != StepLookingUp || got.Total != 5 {
		t.Fatalf("snapshot = %+v, want looking up", got)
	}
}

// The time left is remaining × elapsed ÷ done, only once 30 s have passed and 2% is done.
func TestTrackerETA(t *testing.T) {
	tr, c := newClockTracker()
	tr.Begin(StepWalking, 100)
	tr.Advance(StepWalking, 10)
	if got := tr.Snapshot(c.advance(29 * time.Second)); got.HasETA {
		t.Fatalf("an ETA before 30 s: %+v", got)
	}
	got := tr.Snapshot(c.advance(11 * time.Second)) // 40 s for 10: 4 s each, 90 left
	if !got.HasETA || got.ETA != 360*time.Second {
		t.Fatalf("eta = %v %v, want 6m0s", got.ETA, got.HasETA)
	}

	tr.Begin(StepWalking, 1000)
	tr.Advance(StepWalking, 19)
	if got := tr.Snapshot(c.advance(time.Minute)); got.HasETA {
		t.Fatalf("an ETA under 2%% done: %+v", got)
	}
	tr.Advance(StepWalking, 1)
	if got := tr.Snapshot(c.now()); !got.HasETA || got.ETA != 49*time.Minute {
		t.Fatalf("eta at 2%% = %v %v, want 49m0s", got.ETA, got.HasETA)
	}
}

// A pause for watching hides the ETA and what the step is on; going again restarts the rate
// baseline, so the wait never counts against the rate.
func TestTrackerPauseResetsTheRate(t *testing.T) {
	tr, c := newClockTracker()
	tr.Begin(StepReading, 100)
	tr.Current(StepReading, "a.mkv")
	tr.Advance(StepReading, 50)
	c.advance(time.Minute)
	tr.Pause(StepReading, true)
	got := tr.Snapshot(c.advance(time.Hour))
	if !got.Paused || got.HasETA || got.Current != "" {
		t.Fatalf("paused snapshot = %+v, want paused with no ETA or current", got)
	}
	tr.Pause(StepReading, false)
	if got := tr.Snapshot(c.advance(time.Minute)); got.Paused || got.HasETA {
		t.Fatalf("an ETA with nothing done since the pause: %+v", got)
	}
	tr.Advance(StepReading, 10)
	if got := tr.Snapshot(c.now()); !got.HasETA || got.ETA != 4*time.Minute {
		t.Fatalf("eta after the pause = %v %v, want 4m0s (40 left at 6 s each)", got.ETA, got.HasETA)
	}
}

// A pass nobody asked for is quiet until there is something to read or look up; an asked pass
// never is, and the ask lasts one pass.
func TestTrackerQuiet(t *testing.T) {
	tr, c := newClockTracker()
	tr.Begin(StepWalking, 4)
	if !tr.Snapshot(c.now()).Quiet {
		t.Fatal("an unasked pass is not quiet")
	}
	tr.Begin(StepReading, 2)
	if tr.Snapshot(c.now()).Quiet {
		t.Fatal("a pass with files to read is quiet")
	}
	tr.End(StepReading)
	tr.Begin(StepLookingUp, 2)
	if tr.Snapshot(c.now()).Quiet {
		t.Fatal("a pass with titles to look up is quiet")
	}
	tr.End(StepLookingUp)
	tr.End(StepWalking)

	tr.Ask()
	tr.Begin(StepWalking, 4)
	if tr.Snapshot(c.now()).Quiet {
		t.Fatal("an asked pass is quiet")
	}
	tr.End(StepWalking)
	tr.Begin(StepWalking, 4)
	if !tr.Snapshot(c.now()).Quiet {
		t.Fatal("the ask outlived its pass")
	}
}

// recordSnapshots makes the harness's Tracker keep a snapshot of every change.
func recordSnapshots(h *harness) func() []ProgressSnapshot {
	var mu sync.Mutex
	var seen []ProgressSnapshot
	h.im.Tracker.OnChange = func(bool) {
		s := h.im.Tracker.Snapshot(time.Now())
		mu.Lock()
		seen = append(seen, s)
		mu.Unlock()
	}
	return func() []ProgressSnapshot {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(seen)
	}
}

// The walk is step 1: its total is the root's top-level entries, each is done once walked, and it
// names each folder it lists from the root's own folder name down. Then the header reads are
// step 2, naming each file read.
func TestImportReportsTheWalkAndTheReads(t *testing.T) {
	h := newHarness(t)
	h.file("Alpha (2001)/Alpha.mkv", 10)
	h.file("Bravo (2002)/Bravo.mkv", 10)
	h.file("Show/Season 2/Show S02E01.mkv", 10)
	seen := recordSnapshots(h)
	h.run()

	name := filepath.Base(h.root)
	var walking, reading []ProgressSnapshot
	for _, s := range seen() {
		switch s.Step {
		case StepWalking:
			walking = append(walking, s)
		case StepReading:
			reading = append(reading, s)
		}
	}
	if len(walking) == 0 || len(reading) == 0 {
		t.Fatalf("walking %d reading %d snapshots, want both", len(walking), len(reading))
	}
	last := walking[len(walking)-1]
	if last.Total != 3 || last.Done != 3 || last.Number != 1 {
		t.Fatalf("last walk snapshot = %+v, want 3 of 3", last)
	}
	currents := map[string]bool{}
	for _, s := range walking {
		currents[s.Current] = true
		if s.Done > s.Total {
			t.Errorf("walk done %d past its total %d", s.Done, s.Total)
		}
	}
	for _, want := range []string{name, name + " / Alpha (2001)", name + " / Show / Season 2"} {
		if !currents[want] {
			t.Errorf("the walk never named %q; named %v", want, currents)
		}
	}
	if last := reading[len(reading)-1]; last.Total != 3 || last.Done != 3 || last.Number != 2 {
		t.Fatalf("last read snapshot = %+v, want 3 of 3", last)
	}
	// Reads run side by side, so which file each snapshot names is a race; each is one of them.
	named := 0
	for _, s := range reading {
		switch s.Current {
		case "":
		case "Alpha.mkv", "Bravo.mkv", "Show S02E01.mkv":
			named++
		default:
			t.Errorf("a read named %q, not a file's name", s.Current)
		}
	}
	if named == 0 {
		t.Error("the reads never named a file")
	}
	if got := h.im.Tracker.Snapshot(time.Now()); got.Step != "" {
		t.Fatalf("after the pass = %+v, want idle", got)
	}
	if !h.im.Settled() {
		t.Fatal("a completed pass is not settled")
	}
}

// Fetching titles new to the catalog cache is step 3: its total is every key due as new, each
// fetched is done, and it names the title by the Library's name for it.
func TestEnricherReportsTheLookups(t *testing.T) {
	ctx := context.Background()
	h := newHarness(t)
	h.im.Catalog = h.st
	h.file("Golf (2013) {tmdb-90000201}/Golf (2013) {tmdb-90000201}.mkv", 10)
	h.file("Hotel (2014) {tmdb-90000202}/Hotel (2014) {tmdb-90000202}.mkv", 10)
	h.run()

	seen := recordSnapshots(h)
	e := &Enricher{Store: h.st, Provider: &fakeCatalog{}, Tracker: h.im.Tracker,
		Sleep: func(context.Context, time.Duration) error { return nil }}
	if err := e.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	var looking []ProgressSnapshot
	for _, s := range seen() {
		if s.Step == StepLookingUp {
			looking = append(looking, s)
		}
	}
	if len(looking) == 0 {
		t.Fatal("the lookups were never shown")
	}
	if first := looking[0]; first.Total != 2 || first.Done != 0 || first.Number != 3 {
		t.Fatalf("first lookup snapshot = %+v, want 0 of 2", first)
	}
	if last := looking[len(looking)-1]; last.Done != 2 {
		t.Fatalf("last lookup snapshot = %+v, want 2 done", last)
	}
	names := map[string]bool{}
	for _, s := range looking {
		names[s.Current] = true
	}
	if !names["Golf (2013)"] {
		t.Errorf("the lookup never named Golf (2013); named %v", names)
	}
	// Nothing is left due: the step ended.
	if h.im.Tracker.Active(StepLookingUp) {
		t.Fatal("the lookups still run with nothing due")
	}
}

// A catalog without a key, or one that can't be reached, moves nothing, so step 3 never shows:
// it would otherwise sit at 0 of N for as long as the catalog is down.
func TestEnricherShowsNoLookupsItCannotMake(t *testing.T) {
	for name, err := range map[string]error{
		"no key":      tmdb.ErrAPIKeyRequired,
		"unreachable": errors.New("catalog down"),
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.im.Catalog = h.st
			h.file("Golf (2013) {tmdb-90000201}/Golf (2013) {tmdb-90000201}.mkv", 10)
			h.file("Hotel (2014) {tmdb-90000202}/Hotel (2014) {tmdb-90000202}.mkv", 10)
			h.run()

			seen := recordSnapshots(h)
			e := &Enricher{Store: h.st, Provider: &fakeCatalog{err: err}, Tracker: h.im.Tracker,
				Sleep: func(context.Context, time.Duration) error { return nil }}
			if err := e.Tick(context.Background()); err != nil {
				t.Fatal(err)
			}
			for _, s := range seen() {
				if s.Step == StepLookingUp {
					t.Fatalf("the lookups were shown: %+v", s)
				}
			}
		})
	}
}

// OnChange hears every change, urgently when a step begins or ends or its pause changes.
func TestTrackerTellsOfChanges(t *testing.T) {
	var urgent, plain int
	tr := NewTracker(nil, func(u bool) {
		if u {
			urgent++
		} else {
			plain++
		}
	})
	tr.Begin(StepReading, 3)
	tr.Advance(StepReading, 1)
	tr.Current(StepReading, "a.mkv")
	tr.Pause(StepReading, true)
	tr.Pause(StepReading, true) // no change
	tr.Pause(StepReading, false)
	tr.End(StepReading)
	if urgent != 4 || plain != 2 {
		t.Fatalf("urgent %d plain %d, want 4 and 2", urgent, plain)
	}
	var none *Tracker
	none.Begin(StepWalking, 1)
	none.Pause(StepWalking, true)
	if none.Active(StepWalking) || none.Snapshot(time.Now()) != (ProgressSnapshot{}) {
		t.Fatal("a nil Tracker recorded something")
	}
}
