package binder_test

import (
	"context"
	"encoding/json"
	"errors"
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
	seedApprovedProposalRetiring(t, st, id, jobID, approvedBy, picks, mustExclude, nil)
}

// seedApprovedProposalRetiring is seedApprovedProposal plus the turnstile's rotate-out decisions
// (§8.2a) — the `Retired` keys `recurate` records on the proposal for the binder to apply.
func seedApprovedProposalRetiring(
	t *testing.T,
	st store.Store,
	id, jobID, approvedBy string,
	picks []suggest.ProposalItem,
	mustExclude []string,
	retired []provision.Key,
) {
	t.Helper()
	body := suggest.Proposal{
		Lineup:  picks,
		Intent:  suggest.Intent{MustExclude: mustExclude},
		Retired: retired,
	}
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
func TestPlan_AutoCurate_IsAdditive_NeverDropsAvailable(t *testing.T) {
	st := newStore(t)
	// Existing lineup: 3 available films.
	putAvailable(t, st, 1, "RoboCop")
	putAvailable(t, st, 2, "Terminator")
	putAvailable(t, st, 3, "Raiders")
	seedChannel(t, st, "c1", "job1",
		[]schedule.LineupEntry{entry(1, "RoboCop"), entry(2, "Terminator"), entry(3, "Raiders")},
		schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{AutoCurate: &schedule.AutoCurate{}}})
	// The refresh re-picks only Raiders and adds a NEW film (Predator 2) — RoboCop + Terminator
	// are simply omitted (not excluded, still available).
	putAvailable(t, st, 4, "Predator 2")
	seedApprovedProposal(t, st, "p1", "job1", suggest.AutoCuratedBy,
		[]suggest.ProposalItem{inLib(3, "Raiders"), inLib(4, "Predator 2")}, nil)

	b := binder.New(st, nil, nil, testkit.Logger())
	ch, err := b.PlanApprovedChannel(context.Background(), mustProposal(t, st, "p1"))
	if err != nil {
		t.Fatal(err)
	}

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

// THE OTHER HALF OF THE TURNSTILE (§8.2a). `recurate` decides which title to rotate out and
// records it on the proposal; the BINDER applies it. This test is what makes that split safe.
//
// ⚠ The retired title is still perfectly AVAILABLE, and that is the whole point. The additive
// union's other two drop signals both ask "is this title still wanted?" — an available title
// answers yes to both — so without the retirement signal the union would keep it and the
// turnstile's swap would silently never happen. Before V41 `recurate` avoided that by writing
// the trimmed channel itself moments before the binder ran, which made it a second lineup
// writer ordered against this one by a comment.
func TestPlan_AutoCurate_AppliesRetirementsFromTheProposal(t *testing.T) {
	st := newStore(t)
	putAvailable(t, st, 1, "Keeper")
	putAvailable(t, st, 2, "Retired") // still in the library — only the turnstile wants it gone
	seedChannel(t, st, "c1", "job1",
		[]schedule.LineupEntry{entry(1, "Keeper"), entry(2, "Retired")},
		schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{AutoCurate: &schedule.AutoCurate{}}})
	putAvailable(t, st, 3, "Incoming")
	seedApprovedProposalRetiring(t, st, "p1", "job1", suggest.AutoCuratedBy,
		[]suggest.ProposalItem{inLib(3, "Incoming")}, nil, []provision.Key{movieKey(2)})

	b := binder.New(st, nil, nil, testkit.Logger())
	ch, err := b.PlanApprovedChannel(context.Background(), mustProposal(t, st, "p1"))
	if err != nil {
		t.Fatal(err)
	}

	keys := lineupKeys(ch)
	if keys[movieKey(2)] {
		t.Error("a title the turnstile retired is still in the lineup — the swap never happened")
	}
	if !keys[movieKey(1)] {
		t.Error("an un-retired available title was dropped — retirement must not widen the prune")
	}
	if !keys[movieKey(3)] {
		t.Error("the incoming title did not take the freed slot")
	}
}

// ⚠ Retirements are SCOPED TO AUTO-CURATE. A human approval replaces the lineup wholesale (a
// person decided, including what to remove), so a `Retired` list riding a manually-approved
// proposal must not quietly delete anything the person kept. Asserted because the field is on
// the shared proposal body and nothing in the type system stops a non-auto-curate path setting it.
func TestPlan_ManualApproval_IgnoresRetirements(t *testing.T) {
	st := newStore(t)
	putAvailable(t, st, 1, "Kept By A Human")
	seedChannel(t, st, "c1", "job1",
		[]schedule.LineupEntry{entry(1, "Kept By A Human")},
		schedule.ChannelPolicy{})
	// A human's approval re-picks the title, while the body still carries a retirement for it.
	seedApprovedProposalRetiring(t, st, "p1", "job1", "admin",
		[]suggest.ProposalItem{inLib(1, "Kept By A Human")}, nil, []provision.Key{movieKey(1)})

	b := binder.New(st, nil, nil, testkit.Logger())
	ch, err := b.PlanApprovedChannel(context.Background(), mustProposal(t, st, "p1"))
	if err != nil {
		t.Fatal(err)
	}

	if !lineupKeys(ch)[movieKey(1)] {
		t.Error("a manual approval honoured a retirement — replace semantics must ignore the field")
	}
}

// A genuinely-gone title (unavailable in the library) IS dropped by auto-curate — that's the
// conservative prune the §8.2 semantics allow.
func TestPlan_AutoCurate_DropsUnavailable(t *testing.T) {
	st := newStore(t)
	putAvailable(t, st, 1, "Keeper")
	putUnavailable(t, st, 2, "Gone") // left the library
	seedChannel(t, st, "c1", "job1",
		[]schedule.LineupEntry{entry(1, "Keeper"), entry(2, "Gone")},
		schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{AutoCurate: &schedule.AutoCurate{}}})
	seedApprovedProposal(t, st, "p1", "job1", suggest.AutoCuratedBy,
		[]suggest.ProposalItem{inLib(1, "Keeper")}, nil) // neither re-picked "Gone"

	b := binder.New(st, nil, nil, testkit.Logger())
	ch, err := b.PlanApprovedChannel(context.Background(), mustProposal(t, st, "p1"))
	if err != nil {
		t.Fatal(err)
	}
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
func TestPlan_ManualApprove_ReplacesLineup(t *testing.T) {
	st := newStore(t)
	putAvailable(t, st, 1, "Old")
	seedChannel(t, st, "c1", "job1",
		[]schedule.LineupEntry{entry(1, "Old")},
		schedule.ChannelPolicy{})
	// A human approved a proposal that DROPS Old and picks New.
	putAvailable(t, st, 2, "New")
	seedApprovedProposal(t, st, "p1", "job1", "admin-user", []suggest.ProposalItem{inLib(2, "New")}, nil)

	b := binder.New(st, nil, nil, testkit.Logger())
	ch, err := b.PlanApprovedChannel(context.Background(), mustProposal(t, st, "p1"))
	if err != nil {
		t.Fatal(err)
	}
	keys := lineupKeys(ch)
	if keys[movieKey(1)] || !keys[movieKey(2)] {
		t.Errorf("manual approve must REPLACE (drop Old, keep New): %v", ch.Lineup)
	}
	stored, err := st.GetChannel(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if storedKeys := lineupKeys(stored); !storedKeys[movieKey(1)] || storedKeys[movieKey(2)] {
		t.Errorf("planning mutated the stored channel before commit: %v", stored.Lineup)
	}
}

// THE SELF-DISABLING BUG: auto-curate binding must PRESERVE the channel's operator-owned policy
// fields — the AutoCurate opt-in itself, hand-edited Rules, and Window — none of which the
// refreshed proposal carries. Without this, a channel auto-curates once then turns itself off.
func TestPlan_AutoCurate_PreservesOperatorOwnedPolicy(t *testing.T) {
	st := newStore(t)
	putAvailable(t, st, 1, "Film")
	ac := &schedule.AutoCurate{MinScorePct: 75}
	rules := []schedule.SchedulingRule{{ID: "r1", Label: "Weekend", When: schedule.WhenPredicate{Weekend: true}}}
	seedChannel(t, st, "c1", "job1",
		[]schedule.LineupEntry{entry(1, "Film")},
		schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{Rules: rules}, OperatorPolicy: schedule.OperatorPolicy{AutoCurate: ac, Window: schedule.WindowFull}})
	// The refreshed proposal's policy carries NONE of these operator-owned fields.
	seedApprovedProposal(t, st, "p1", "job1", suggest.AutoCuratedBy, []suggest.ProposalItem{inLib(1, "Film")}, nil)

	b := binder.New(st, nil, nil, testkit.Logger())
	ch, err := b.PlanApprovedChannel(context.Background(), mustProposal(t, st, "p1"))
	if err != nil {
		t.Fatal(err)
	}
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

func TestPlan_UsesTheExactCandidateWithoutWriting(t *testing.T) {
	st := newStore(t)
	ctx := context.Background()
	body := suggest.Proposal{
		ChannelName: "The Exact Candidate",
		Policy: schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
			Scope: schedule.ScopePolicy{Era: &schedule.Range{From: 1990, To: 1999}},
		}},
		Lineup: []suggest.ProposalItem{inLib(1, "Exact")},
	}
	blob, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	candidate := store.Proposal{
		ID: "p-exact", JobID: "job-exact", Status: "approved", ApprovedBy: "admin",
		ProposalJSON: string(blob), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	// A different approved row for the same job proves planning does not re-resolve
	// "newest approved" behind the caller's back.
	seedApprovedProposal(t, st, "p-other", candidate.JobID, "other-admin",
		[]suggest.ProposalItem{inLib(2, "Other")}, nil)

	before := time.Now().UTC()
	ch, err := binder.New(st, nil, nil, testkit.Logger()).PlanApprovedChannel(ctx, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if ch.ID == "" || ch.IntentRef != candidate.JobID || ch.Name != body.ChannelName || ch.Number != 1 {
		t.Errorf("planned channel identity = id:%q intent:%q name:%q number:%d", ch.ID, ch.IntentRef, ch.Name, ch.Number)
	}
	if ch.Status != schedule.StatusBuilding || ch.ReconcileDeadline.Before(before) {
		t.Errorf("planned convergence state = status:%q deadline:%v", ch.Status, ch.ReconcileDeadline)
	}
	if got := lineupKeys(ch); !got[movieKey(1)] || got[movieKey(2)] {
		t.Errorf("planner did not use exact candidate: %+v", ch.Lineup)
	}
	if ch.Policy.Filler == nil || ch.Policy.Filler.Era == nil || ch.Policy.Filler.Era.From != 1990 {
		t.Errorf("filler era was not seeded from exact candidate policy: %+v", ch.Policy.Filler)
	}
	if _, err := st.GetChannel(ctx, ch.ID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("planning wrote channel %q: %v", ch.ID, err)
	}
}

func TestPlan_RejectsAnythingOtherThanAnApprovedCandidate(t *testing.T) {
	st := newStore(t)
	b := binder.New(st, nil, nil, testkit.Logger())

	if _, err := b.PlanApprovedChannel(context.Background(), store.Proposal{
		ID: "p-submitted", JobID: "job", Status: "submitted", ProposalJSON: `{}`,
	}); err == nil {
		t.Fatal("planned a submitted proposal")
	}
	if _, err := b.PlanApprovedChannel(context.Background(), store.Proposal{
		ID: "p-bad", JobID: "job", Status: "approved", ProposalJSON: `{`,
	}); err == nil {
		t.Fatal("planned malformed proposal JSON")
	}
}

type recordingCodec struct {
	channelID string
	err       error
}

func (r *recordingCodec) ComputeChannelCodec(_ context.Context, channelID string) (string, error) {
	r.channelID = channelID
	return store.BroadcastCodecH264, r.err
}

type recordingReconciler struct {
	channelID string
	err       error
}

func (r *recordingReconciler) Reconcile(_ context.Context, channelID string) error {
	r.channelID = channelID
	return r.err
}

type recordedActivity struct {
	kind, subjectID, text string
}

func (r *recordedActivity) Error(_ context.Context, kind, subjectID, text string) {
	r.kind, r.subjectID, r.text = kind, subjectID, text
}

func TestAfterApprovalCommitted_IsBestEffort(t *testing.T) {
	st := newStore(t)
	codec := &recordingCodec{err: errors.New("probe unavailable")}
	reconciler := &recordingReconciler{err: errors.New("tunarr unavailable")}
	activity := &recordedActivity{}
	b := binder.New(st, reconciler, codec, nil).WithActivity(activity)
	ch := store.Channel{Channel: schedule.Channel{ID: "ch-committed", Name: "Committed", Number: 7}}

	// There is deliberately no returned error: once local approval has committed,
	// derived/external convergence cannot undo it.
	b.AfterApprovalCommitted(context.Background(), ch)

	if codec.channelID != ch.ID || reconciler.channelID != ch.ID {
		t.Errorf("post-commit calls = codec:%q reconcile:%q, want %q", codec.channelID, reconciler.channelID, ch.ID)
	}
	if activity.kind != "channel.reconcile" || activity.subjectID != ch.ID || activity.text == "" {
		t.Errorf("reconcile failure activity = %+v", activity)
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
