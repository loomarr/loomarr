package recurate_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/recurate"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
	"github.com/mantonx/loomarr/internal/testkit"
)

func newStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+filepath.Join(t.TempDir(), "r.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// fixedThresholds is a Thresholds with constant knobs for deterministic tests.
type fixedThresholds struct{ minScorePct, maxTitles int }

func (f fixedThresholds) MinScorePct(context.Context) int { return f.minScorePct }
func (f fixedThresholds) MaxTitles(context.Context) int   { return f.maxTitles }

// seedAutoCurateChannel writes a live, intent-backed, auto-curate channel bound to jobID.
func seedAutoCurateChannel(t *testing.T, st store.Store, id, jobID string, lineup []schedule.LineupEntry, ac *schedule.AutoCurate) {
	t.Helper()
	ch := store.Channel{Lineup: lineup}
	ch.ID = id
	ch.IntentRef = jobID
	ch.Name = "Ch " + id
	ch.Number = 5
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusLive
	ch.Policy = schedule.ChannelPolicy{AutoCurate: ac}
	if err := st.UpsertChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
}

// movie builds a proposal item (an acquisition candidate) with a confidence.
func acqItem(tmdbID int, name string, confidence float64) suggest.ProposalItem {
	return suggest.ProposalItem{MediaType: provision.Movie, TMDBID: tmdbID, Name: name, Confidence: confidence}
}

// inLibItem builds an in-library lineup pick (already available).
func inLibItem(tmdbID int, name, libID string) suggest.ProposalItem {
	return suggest.ProposalItem{MediaType: provision.Movie, TMDBID: tmdbID, Name: name, InLibrary: true, LibraryItemID: libID}
}

// seedProposal writes a submitted proposal for jobID with the given picks.
func seedProposal(t *testing.T, st store.Store, id, jobID string, lineup, acquisitions []suggest.ProposalItem) store.Proposal {
	t.Helper()
	body := suggest.Proposal{Lineup: lineup, Acquisitions: acquisitions}
	blob, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	p := store.Proposal{ID: id, JobID: jobID, Status: "submitted", ProposalJSON: string(blob), CreatedAt: now, UpdatedAt: now}
	if err := st.CreateProposal(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	return p
}

func movieKey(tmdbID int) provision.Key {
	k, _ := provision.Title{MediaType: provision.Movie, TMDBID: tmdbID}.Key()
	return k
}

func titleState(t *testing.T, st store.Store, tmdbID int) provision.State {
	t.Helper()
	rec, err := st.GetTitle(context.Background(), movieKey(tmdbID))
	if err != nil {
		return "" // not found
	}
	return rec.State
}

// A net-new acquisition ABOVE the quality bar is requested (wanted) via the approval gate;
// one BELOW the bar is dropped — never requested. This is the intent-weight gate (§8.2).
func TestCurator_QualityBar(t *testing.T) {
	st := newStore(t)
	seedAutoCurateChannel(t, st, "ch1", "job1", nil, &schedule.AutoCurate{}) // global thresholds
	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{
		acqItem(100, "Great Fit", 0.90), // above a 60% bar → requested
		acqItem(200, "Weak Fit", 0.30),  // below → dropped
	})
	cur := recurate.NewCurator(st, fixedThresholds{minScorePct: 60, maxTitles: 0}, time.Now, testkit.Logger())

	d, err := cur.Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Approved || d.Enqueued != 1 {
		t.Fatalf("decision = %+v, want approved with 1 enqueued (only the above-bar title)", d)
	}
	if titleState(t, st, 100) != provision.Wanted {
		t.Errorf("above-bar title = %q, want wanted", titleState(t, st, 100))
	}
	if s := titleState(t, st, 200); s != "" {
		t.Errorf("below-bar title = %q, want NOT requested (dropped)", s)
	}
}

// An in-library pick is added regardless of any acquisition bar (it's already available, no
// acquisition), and it does NOT create a wanted row.
func TestCurator_InLibraryAddedNoAcquisition(t *testing.T) {
	st := newStore(t)
	seedAutoCurateChannel(t, st, "ch1", "job1", nil, &schedule.AutoCurate{})
	p := seedProposal(t, st, "p1", "job1",
		[]suggest.ProposalItem{inLibItem(300, "Already Here", "lib-300")}, // in-library lineup pick
		nil)
	cur := recurate.NewCurator(st, fixedThresholds{minScorePct: 99, maxTitles: 0}, time.Now, testkit.Logger())

	d, err := cur.Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Approved || d.Enqueued != 0 {
		t.Fatalf("decision = %+v, want approved with 0 acquisitions (in-library only)", d)
	}
	// The in-library pick becomes an `available` record so the scheduler can place it — NOT wanted.
	if s := titleState(t, st, 300); s != provision.Available {
		t.Errorf("in-library pick = %q, want available (no acquisition)", s)
	}
}

// THE APPROVAL-GATE NEGATIVE (§19/prime-directive-#3): a channel NOT opted into auto-curate is
// never auto-approved — its proposal stays submitted and NO title is requested by re-curation.
func TestCurator_NotOptedInNeverRequests(t *testing.T) {
	st := newStore(t)
	// Channel exists + is intent-backed, but AutoCurate is nil (not opted in).
	seedAutoCurateChannel(t, st, "ch1", "job1", nil, nil)
	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{acqItem(100, "Film", 0.99)})
	cur := recurate.NewCurator(st, fixedThresholds{minScorePct: 60, maxTitles: 0}, time.Now, testkit.Logger())

	d, err := cur.Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Approved {
		t.Fatal("a non-opted-in channel must never auto-curate")
	}
	if s := titleState(t, st, 100); s != "" {
		t.Errorf("title = %q, want NONE (nothing requested for a non-opted-in channel)", s)
	}
	// The proposal is untouched — still submitted for an admin.
	got, _ := st.GetProposal(context.Background(), "p1")
	if got.Status != "submitted" {
		t.Errorf("proposal status = %q, want submitted (waits for an admin)", got.Status)
	}
}

// The growth cap bounds how many net-new titles re-curation requests: current lineup + kept
// acquisitions never exceeds maxTitles. The BEST (highest-confidence) survivors fill the room.
func TestCurator_TitleCap(t *testing.T) {
	st := newStore(t)
	// Channel already has 2 titles; cap is 3 → room for exactly 1 net-new.
	existing := []schedule.LineupEntry{
		{Key: movieKey(1), Title: "A"}, {Key: movieKey(2), Title: "B"},
	}
	seedAutoCurateChannel(t, st, "ch1", "job1", existing, &schedule.AutoCurate{})
	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{
		acqItem(100, "Best", 0.95),   // highest → kept
		acqItem(200, "Middle", 0.80), // over cap → dropped
		acqItem(300, "Good", 0.85),   // over cap → dropped
	})
	cur := recurate.NewCurator(st, fixedThresholds{minScorePct: 50, maxTitles: 3}, time.Now, testkit.Logger())

	d, err := cur.Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Approved || d.Enqueued != 1 {
		t.Fatalf("decision = %+v, want 1 enqueued (cap leaves room for 1)", d)
	}
	if titleState(t, st, 100) != provision.Wanted {
		t.Error("the highest-confidence survivor should be the one kept under the cap")
	}
	if titleState(t, st, 200) != "" || titleState(t, st, 300) != "" {
		t.Error("over-cap titles must not be requested")
	}
}

// A per-channel MinScorePct override is stricter/looser than the global default.
func TestCurator_PerChannelOverride(t *testing.T) {
	st := newStore(t)
	// Global bar 60, but this channel overrides to 90 → an 0.80 title is now below the bar.
	seedAutoCurateChannel(t, st, "ch1", "job1", nil, &schedule.AutoCurate{MinScorePct: 90})
	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{acqItem(100, "Film", 0.80)})
	cur := recurate.NewCurator(st, fixedThresholds{minScorePct: 60, maxTitles: 0}, time.Now, testkit.Logger())

	d, err := cur.Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Enqueued != 0 {
		t.Errorf("enqueued %d, want 0 (0.80 is below the per-channel 90%% bar)", d.Enqueued)
	}
	_ = d
	if titleState(t, st, 100) != "" {
		t.Error("below the per-channel bar → not requested")
	}
}
