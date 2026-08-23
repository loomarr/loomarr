package recurate_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

func lineupEntry(tmdbID int, title string) schedule.LineupEntry {
	return schedule.LineupEntry{
		Key:   provision.Key("movie:tmdb:" + itoa(tmdbID)),
		Title: title,
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// seedFullChannel writes an auto-curate channel AT its cap, with `airing` marked as currently
// scheduled (ch.Desired) and the rest retirable.
func seedFullChannel(t *testing.T, st store.Store, id, jobID string, lineup []schedule.LineupEntry, airing []provision.Key) {
	t.Helper()
	ch := store.Channel{Lineup: lineup}
	ch.ID = id
	ch.IntentRef = jobID
	ch.Name = "Full " + id
	ch.Number = 77
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusLive
	ch.Policy = schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{AutoCurate: &schedule.AutoCurate{}}}
	for _, k := range airing {
		ch.Desired = append(ch.Desired, schedule.Slot{Kind: schedule.SlotProgram, Key: k, DurationMs: 1000})
	}
	if _, err := st.SaveChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
}

// retiredOf reads the durable audit of what the turnstile decided to rotate out. Final channel
// assertions below remain the load-bearing behavior check; this verifies the proposal explains it.
func retiredOf(t *testing.T, st store.Store, proposalID string) map[provision.Key]bool {
	t.Helper()
	p, err := st.GetProposal(context.Background(), proposalID)
	if err != nil {
		t.Fatal(err)
	}
	var body suggest.Proposal
	if uerr := json.Unmarshal([]byte(p.ProposalJSON), &body); uerr != nil {
		t.Fatalf("proposal %s malformed: %v", proposalID, uerr)
	}
	out := map[provision.Key]bool{}
	for _, k := range body.Retired {
		out[k] = true
	}
	return out
}

func lineupOf(t *testing.T, st store.Store, id string) map[provision.Key]bool {
	t.Helper()
	ch, err := st.GetChannel(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	out := map[provision.Key]bool{}
	for _, e := range ch.Lineup {
		out[e.Key] = true
	}
	return out
}

// THE GAP THIS CLOSES: additive binding plus a title cap has an end state nobody chose — the
// channel grows to the cap and then FREEZES. room hits 0, every future candidate is dropped,
// and re-curation keeps running and spending tokens while nothing can ever change. Observed
// live: 25 → 27 → 30 → 34 against a cap of 40, nothing ever leaving.
func TestCurator_AtTheCapABetterTitleRetiresTheWeakest(t *testing.T) {
	st := newStore(t)
	// Cap of 2, both slots full. Only "Bench Title" is retirable — the other is on the air.
	seedFullChannel(t, st, "ch1", "job1",
		[]schedule.LineupEntry{lineupEntry(100, "Airing Title"), lineupEntry(200, "Bench Title")},
		[]provision.Key{provision.Key("movie:tmdb:100")})

	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{
		acqItem(300, "Much Better Fit", 0.95),
	})
	cur := newCurator(t, st, fixedThresholds{minScorePct: 60, maxTitles: 2})

	d, err := cur.Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Enqueued != 1 {
		t.Fatalf("enqueued = %d, want 1 (the turnstile should admit a better title)", d.Enqueued)
	}
	retired := retiredOf(t, st, "p1")
	if !retired[provision.Key("movie:tmdb:200")] {
		t.Error("the weakest retirable title should have been retired")
	}
	if retired[provision.Key("movie:tmdb:100")] {
		t.Error("the AIRING title must never be retired")
	}
	lineup := lineupOf(t, st, "ch1")
	if lineup[provision.Key("movie:tmdb:200")] {
		t.Error("retired title still exists in the committed channel lineup")
	}
	if !lineup[provision.Key("movie:tmdb:100")] {
		t.Error("airing title disappeared from the committed channel lineup")
	}
	if !lineup[provision.Key("movie:tmdb:300")] {
		t.Error("admitted replacement is missing from the committed channel lineup")
	}
}

func TestCurator_KeepFeedbackProtectsTitleFromAutomaticRetirement(t *testing.T) {
	st := newStore(t)
	seedFullChannel(t, st, "ch-keep", "job-keep",
		[]schedule.LineupEntry{lineupEntry(100, "Airing Title"), lineupEntry(200, "Family Favorite")},
		[]provision.Key{"movie:tmdb:100"})
	if err := st.AppendDiscoveryFeedback(context.Background(), store.DiscoveryFeedback{
		ID: "keep-1", ActorID: "admin", Scope: store.FeedbackChannel, ScopeID: "ch-keep",
		Target: "movie:tmdb:200", Action: store.FeedbackKeep,
	}); err != nil {
		t.Fatal(err)
	}
	p := seedProposal(t, st, "p-keep", "job-keep", nil, []suggest.ProposalItem{
		acqItem(300, "New Candidate", 0.99),
	})
	d, err := newCurator(t, st, fixedThresholds{minScorePct: 60, maxTitles: 2}).Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Enqueued != 0 || retiredOf(t, st, "p-keep")["movie:tmdb:200"] {
		t.Fatalf("keep feedback allowed automatic retirement: decision=%+v", d)
	}
	if !lineupOf(t, st, "ch-keep")["movie:tmdb:200"] {
		t.Fatal("kept title disappeared from the channel")
	}
}

// ⚠ THE SAFETY PROPERTY. A title in ch.Desired is airing in the current window — someone may be
// planning to watch it today. When the ONLY thing at the cap is scheduled, the newcomer is
// dropped over-cap instead: a stale channel beats yanking a programme out from under a viewer.
func TestCurator_NeverRetiresSomethingCurrentlyAiring(t *testing.T) {
	st := newStore(t)
	// Cap of 1, and that one title is on the air.
	seedFullChannel(t, st, "ch1", "job1",
		[]schedule.LineupEntry{lineupEntry(100, "Airing Title")},
		[]provision.Key{provision.Key("movie:tmdb:100")})

	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{
		acqItem(300, "Much Better Fit", 0.99),
	})
	cur := newCurator(t, st, fixedThresholds{minScorePct: 60, maxTitles: 1})

	d, err := cur.Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Enqueued != 0 {
		t.Fatalf("enqueued = %d, want 0 (nothing retirable ⇒ drop the newcomer)", d.Enqueued)
	}
	if !lineupOf(t, st, "ch1")[provision.Key("movie:tmdb:100")] {
		t.Fatal("an airing title was retired — a viewer's programme was pulled out from under them")
	}
}

// Below the cap nothing is retired: the turnstile is a CAP behaviour, not a general churn.
func TestCurator_BelowTheCapNothingIsRetired(t *testing.T) {
	st := newStore(t)
	seedFullChannel(t, st, "ch1", "job1",
		[]schedule.LineupEntry{lineupEntry(100, "Keeper"), lineupEntry(200, "Also Keeper")},
		nil) // nothing airing ⇒ both retirable, but there's room

	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{
		acqItem(300, "New Title", 0.95),
	})
	cur := newCurator(t, st, fixedThresholds{minScorePct: 60, maxTitles: 10})

	if _, err := cur.Consider(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got := lineupOf(t, st, "ch1")
	if !got[provision.Key("movie:tmdb:100")] || !got[provision.Key("movie:tmdb:200")] {
		t.Fatal("nothing should be retired while the channel is under its cap")
	}
}

// ⚠ FAIL CLOSED on an unknown schedule. An empty ch.Desired means "we do not know what is
// airing" — a channel that has never reconciled — NOT "nothing is airing". Reading it as
// all-retirable would let a single run churn an entire lineup, the exact opposite of the
// guard's purpose. Found by breaking TestCurator_TitleCap, whose channel has no Desired.
func TestCurator_UnknownScheduleRetiresNothing(t *testing.T) {
	st := newStore(t)
	seedFullChannel(t, st, "ch1", "job1",
		[]schedule.LineupEntry{lineupEntry(100, "A"), lineupEntry(200, "B")},
		nil) // never reconciled ⇒ Desired empty ⇒ schedule UNKNOWN

	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{
		acqItem(300, "Better", 0.99), acqItem(400, "Also Better", 0.98),
	})
	cur := newCurator(t, st, fixedThresholds{minScorePct: 60, maxTitles: 2})

	d, err := cur.Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Enqueued != 0 {
		t.Fatalf("enqueued = %d, want 0 (an unknown schedule must retire nothing)", d.Enqueued)
	}
	got := lineupOf(t, st, "ch1")
	if !got[provision.Key("movie:tmdb:100")] || !got[provision.Key("movie:tmdb:200")] {
		t.Fatal("a channel with an unknown schedule had its lineup churned")
	}
}

// A tie never retires. Retiring a title for one of IDENTICAL confidence is a coin flip that
// churns the lineup every week — precisely the failure additive binding (§8.2) exists to
// prevent. "No better" means nothing moves.
func TestCurator_EqualConfidenceDoesNotRetire(t *testing.T) {
	st := newStore(t)
	seedFullChannel(t, st, "ch1", "job1",
		[]schedule.LineupEntry{lineupEntry(100, "Airing"), lineupEntry(200, "Bench")},
		[]provision.Key{provision.Key("movie:tmdb:100")})

	// A lineup entry carries no stored confidence, so the bench scores 0 — an incoming 0
	// therefore ties it and must NOT displace it.
	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{
		acqItem(300, "Ties The Bench", 0),
	})
	cur := newCurator(t, st, fixedThresholds{minScorePct: 0, maxTitles: 2})

	if _, err := cur.Consider(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if retiredOf(t, st, "p1")[provision.Key("movie:tmdb:200")] {
		t.Fatal("a tie retired the bench title — that is coin-flip churn")
	}
}

// THE GAP THIS CLOSES: retiring only at 100% of cap made the lineup a RATCHET — every run
// appended, nothing ever left, and the channel froze once full. A channel curated for weeks
// still led with its original picks, which is what "why do I still see the same old movies"
// describes. Above the rotation target a channel TRADES: a better candidate displaces the
// stalest retirable title even while free slots remain.
func TestCurator_AboveRotationTargetTradesEvenWithRoom(t *testing.T) {
	st := newStore(t)
	// Cap 4 ⇒ target 3. Three titles: at the target, with a free slot still available.
	seedFullChannel(t, st, "ch1", "job1",
		[]schedule.LineupEntry{
			lineupEntry(100, "Airing"), lineupEntry(200, "Stale"), lineupEntry(300, "Also Stale"),
		},
		[]provision.Key{provision.Key("movie:tmdb:100")})

	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{
		acqItem(400, "Fresh Pick", 0.95),
	})
	cur := newCurator(t, st, fixedThresholds{minScorePct: 60, maxTitles: 4})

	d, err := cur.Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Enqueued != 1 {
		t.Fatalf("enqueued = %d, want 1", d.Enqueued)
	}
	// The observable is a RETIREMENT, not lineup size: a net-new acquisition becomes a
	// `wanted` title and does not join ch.Lineup until it lands, so counting entries cannot
	// distinguish "traded" from "took a free slot". One of the two stale titles must be gone.
	retired := retiredOf(t, st, "p1")
	staleGone := retired[provision.Key("movie:tmdb:200")] || retired[provision.Key("movie:tmdb:300")]
	if !staleGone {
		t.Fatal("above the rotation target a better candidate must retire a stale title, " +
			"even though a free slot exists (the ratchet is back)")
	}
	if retired[provision.Key("movie:tmdb:100")] {
		t.Error("the airing title must never be retired, target or no target")
	}
}

// BELOW the target a young channel fills up rather than churning — rotation pressure applies to
// a mature lineup, not to one that is still being assembled.
func TestCurator_BelowRotationTargetJustGrows(t *testing.T) {
	st := newStore(t)
	// Cap 10 ⇒ target 7. Two titles is well below it.
	seedFullChannel(t, st, "ch1", "job1",
		[]schedule.LineupEntry{lineupEntry(100, "A"), lineupEntry(200, "B")},
		[]provision.Key{provision.Key("movie:tmdb:100")})

	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{
		acqItem(300, "New", 0.95),
	})
	cur := newCurator(t, st, fixedThresholds{minScorePct: 60, maxTitles: 10})

	if _, err := cur.Consider(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	got := lineupOf(t, st, "ch1")
	if !got[provision.Key("movie:tmdb:100")] || !got[provision.Key("movie:tmdb:200")] {
		t.Fatal("a young channel must fill up, not churn")
	}
}

// Above the target, a candidate that beats NOTHING retirable still takes a free slot — rotation
// is a preference, not a gate. Only at the hard cap does "nothing to displace" mean "dropped".
func TestCurator_AboveTargetWithNothingWeakerStillAdds(t *testing.T) {
	st := newStore(t)
	// Cap 4 ⇒ target 3. Everything present is AIRING, so nothing is retirable.
	seedFullChannel(t, st, "ch1", "job1",
		[]schedule.LineupEntry{lineupEntry(100, "A"), lineupEntry(200, "B"), lineupEntry(300, "C")},
		[]provision.Key{
			provision.Key("movie:tmdb:100"), provision.Key("movie:tmdb:200"), provision.Key("movie:tmdb:300"),
		})

	p := seedProposal(t, st, "p1", "job1", nil, []suggest.ProposalItem{
		acqItem(400, "New", 0.95),
	})
	cur := newCurator(t, st, fixedThresholds{minScorePct: 60, maxTitles: 4})

	d, err := cur.Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Enqueued != 1 {
		t.Fatalf("enqueued = %d, want 1 (a free slot exists; rotation must not block the add)", d.Enqueued)
	}
	lineup := lineupOf(t, st, "ch1")
	if len(lineup) != 4 {
		t.Fatalf("lineup = %d titles, want the three protected titles plus the newcomer", len(lineup))
	}
	for _, key := range []provision.Key{"movie:tmdb:100", "movie:tmdb:200", "movie:tmdb:300"} {
		if !lineup[key] {
			t.Errorf("airing title %s was retired", key)
		}
	}
}
