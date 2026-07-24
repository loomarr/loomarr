package binder_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/binder"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
	"github.com/mantonx/loomarr/internal/testkit"
)

func newStore(t *testing.T) store.Store {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+filepath.Join(t.TempDir(), "b.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func movieKey(tmdbID int) provision.Key {
	k, _ := provision.Title{MediaType: provision.Movie, TMDBID: tmdbID}.Key()
	return k
}

// putAvailable writes an available title record so the merge sees it as still-in-library.
func putAvailable(t *testing.T, st store.Store, tmdbID int, name string) {
	t.Helper()
	rec := provision.Record{
		Key:       movieKey(tmdbID),
		Title:     provision.Title{MediaType: provision.Movie, TMDBID: tmdbID, Name: name},
		State:     provision.Available,
		LibraryID: "lib-" + name,
	}
	if err := st.UpsertTitle(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
}

func putUnavailable(t *testing.T, st store.Store, tmdbID int, name string) {
	t.Helper()
	rec := provision.Record{
		Key:   movieKey(tmdbID),
		Title: provision.Title{MediaType: provision.Movie, TMDBID: tmdbID, Name: name},
		State: provision.Unavailable,
	}
	if err := st.UpsertTitle(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
}

func entry(tmdbID int, name string) schedule.LineupEntry {
	return schedule.LineupEntry{Key: movieKey(tmdbID), Title: name}
}

// seedChannel writes an auto-curate channel bound to jobID with an existing lineup + policy.
func seedChannel(t *testing.T, st store.Store, id, jobID string, lineup []schedule.LineupEntry, pol schedule.ChannelPolicy) {
	t.Helper()
	ch := store.Channel{Lineup: lineup}
	ch.ID = id
	ch.IntentRef = jobID
	ch.Name = "Ch " + id
	ch.Number = 5
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusLive
	ch.Policy = pol
	if err := st.UpsertChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
}

// seedApprovedProposal writes an APPROVED proposal for jobID (approvedBy sets the path).
func seedApprovedProposal(t *testing.T, st store.Store, id, jobID, approvedBy string, picks []suggest.ProposalItem, mustExclude []string) {
	t.Helper()
	body := suggest.Proposal{Lineup: picks, Intent: suggest.Intent{MustExclude: mustExclude}}
	blob, _ := json.Marshal(body)
	now := time.Now()
	p := store.Proposal{ID: id, JobID: jobID, Status: "approved", ApprovedBy: approvedBy, ProposalJSON: string(blob), CreatedAt: now, UpdatedAt: now}
	if err := st.CreateProposal(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

func inLib(tmdbID int, name string) suggest.ProposalItem {
	return suggest.ProposalItem{MediaType: provision.Movie, TMDBID: tmdbID, Name: name, InLibrary: true, LibraryItemID: "lib-" + name}
}

func lineupKeys(ch store.Channel) map[provision.Key]bool {
	m := map[provision.Key]bool{}
	for _, e := range ch.Lineup {
		m[e.Key] = true
	}
	return m
}

// AUTO-CURATE IS ADDITIVE (§8.2): a refresh that re-picks a DIFFERENT subset must not drop a
// still-available title the LLM merely didn't re-pick this run — it ADDS the new picks and
// keeps the rest. This is the "1980s Action Heroes lost RoboCop/Terminator/Raiders" fix.
func TestBind_AutoCurate_IsAdditive_NeverDropsAvailable(t *testing.T) {
	st := newStore(t)
	// Existing lineup: 3 available films.
	putAvailable(t, st, 1, "RoboCop")
	putAvailable(t, st, 2, "Terminator")
	putAvailable(t, st, 3, "Raiders")
	seedChannel(t, st, "c1", "job1",
		[]schedule.LineupEntry{entry(1, "RoboCop"), entry(2, "Terminator"), entry(3, "Raiders")},
		schedule.ChannelPolicy{AutoCurate: &schedule.AutoCurate{}})
	// The refresh re-picks only Raiders and adds a NEW film (Predator 2) — RoboCop + Terminator
	// are simply omitted (not excluded, still available).
	putAvailable(t, st, 4, "Predator 2")
	seedApprovedProposal(t, st, "p1", "job1", suggest.AutoCuratedBy,
		[]suggest.ProposalItem{inLib(3, "Raiders"), inLib(4, "Predator 2")}, nil)

	b := binder.New(st, nil, testkit.Logger())
	if _, err := b.BindApprovedChannel(context.Background(), mustProposal(t, st, "p1")); err != nil {
		t.Fatal(err)
	}

	ch, _ := st.GetChannel(context.Background(), "c1")
	keys := lineupKeys(ch)
	// The omitted-but-available films are KEPT.
	for _, tmdbID := range []int{1, 2} {
		if !keys[movieKey(tmdbID)] {
			t.Errorf("available film tmdb:%d was dropped by auto-curate (churn — must be additive)", tmdbID)
		}
	}
	// The new film was ADDED; Raiders (re-picked) still present.
	if !keys[movieKey(4)] || !keys[movieKey(3)] {
		t.Errorf("auto-curate did not add the new pick / keep the re-picked one: %v", ch.Lineup)
	}
	if len(ch.Lineup) != 4 {
		t.Errorf("lineup = %d titles, want 4 (3 kept + 1 added)", len(ch.Lineup))
	}
}

// A genuinely-gone title (unavailable in the library) IS dropped by auto-curate — that's the
// conservative prune the §8.2 semantics allow.
func TestBind_AutoCurate_DropsUnavailable(t *testing.T) {
	st := newStore(t)
	putAvailable(t, st, 1, "Keeper")
	putUnavailable(t, st, 2, "Gone") // left the library
	seedChannel(t, st, "c1", "job1",
		[]schedule.LineupEntry{entry(1, "Keeper"), entry(2, "Gone")},
		schedule.ChannelPolicy{AutoCurate: &schedule.AutoCurate{}})
	seedApprovedProposal(t, st, "p1", "job1", suggest.AutoCuratedBy,
		[]suggest.ProposalItem{inLib(1, "Keeper")}, nil) // neither re-picked "Gone"

	b := binder.New(st, nil, testkit.Logger())
	if _, err := b.BindApprovedChannel(context.Background(), mustProposal(t, st, "p1")); err != nil {
		t.Fatal(err)
	}
	ch, _ := st.GetChannel(context.Background(), "c1")
	keys := lineupKeys(ch)
	if !keys[movieKey(1)] {
		t.Error("available Keeper must stay")
	}
	if keys[movieKey(2)] {
		t.Error("unavailable Gone should be dropped (conservative prune of a clearly-gone title)")
	}
}

// A MANUAL approval (not auto-curate) REPLACES the lineup — a person decided, including to
// remove titles. This guards that the additive behavior is auto-curate-ONLY.
func TestBind_ManualApprove_ReplacesLineup(t *testing.T) {
	st := newStore(t)
	putAvailable(t, st, 1, "Old")
	seedChannel(t, st, "c1", "job1",
		[]schedule.LineupEntry{entry(1, "Old")},
		schedule.ChannelPolicy{})
	// A human approved a proposal that DROPS Old and picks New.
	putAvailable(t, st, 2, "New")
	seedApprovedProposal(t, st, "p1", "job1", "admin-user", []suggest.ProposalItem{inLib(2, "New")}, nil)

	b := binder.New(st, nil, testkit.Logger())
	if _, err := b.BindApprovedChannel(context.Background(), mustProposal(t, st, "p1")); err != nil {
		t.Fatal(err)
	}
	ch, _ := st.GetChannel(context.Background(), "c1")
	keys := lineupKeys(ch)
	if keys[movieKey(1)] || !keys[movieKey(2)] {
		t.Errorf("manual approve must REPLACE (drop Old, keep New): %v", ch.Lineup)
	}
}

// THE SELF-DISABLING BUG: auto-curate binding must PRESERVE the channel's operator-owned policy
// fields — the AutoCurate opt-in itself, hand-edited Rules, and Window — none of which the
// refreshed proposal carries. Without this, a channel auto-curates once then turns itself off.
func TestBind_AutoCurate_PreservesOperatorOwnedPolicy(t *testing.T) {
	st := newStore(t)
	putAvailable(t, st, 1, "Film")
	ac := &schedule.AutoCurate{MinScorePct: 75}
	rules := []schedule.SchedulingRule{{ID: "r1", Label: "Weekend", When: schedule.WhenPredicate{Weekend: true}}}
	seedChannel(t, st, "c1", "job1",
		[]schedule.LineupEntry{entry(1, "Film")},
		schedule.ChannelPolicy{AutoCurate: ac, Rules: rules, Window: schedule.WindowFull})
	// The refreshed proposal's policy carries NONE of these operator-owned fields.
	seedApprovedProposal(t, st, "p1", "job1", suggest.AutoCuratedBy, []suggest.ProposalItem{inLib(1, "Film")}, nil)

	b := binder.New(st, nil, testkit.Logger())
	if _, err := b.BindApprovedChannel(context.Background(), mustProposal(t, st, "p1")); err != nil {
		t.Fatal(err)
	}
	ch, _ := st.GetChannel(context.Background(), "c1")
	if ch.Policy.AutoCurate == nil {
		t.Fatal("auto-curate opt-in was WIPED by its own bind (self-disabling bug)")
	}
	if ch.Policy.AutoCurate.MinScorePct != 75 {
		t.Errorf("per-channel override lost: minScorePct = %d, want 75", ch.Policy.AutoCurate.MinScorePct)
	}
	if len(ch.Policy.Rules) != 1 || ch.Policy.Rules[0].ID != "r1" {
		t.Errorf("hand-edited rules wiped on rebind: %+v", ch.Policy.Rules)
	}
	if ch.Policy.Window != schedule.WindowFull {
		t.Errorf("window override wiped on rebind: %v", ch.Policy.Window)
	}
}

func mustProposal(t *testing.T, st store.Store, id string) store.Proposal {
	t.Helper()
	p, err := st.GetProposal(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
