package recurate_test

import (
	"context"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/recurate"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
	"github.com/mantonx/loomarr/internal/testkit"
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
	if err := st.UpsertChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
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
	cur := recurate.NewCurator(st, fixedThresholds{minScorePct: 60, maxTitles: 2}, time.Now, testkit.Logger())

	d, err := cur.Consider(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if d.Enqueued != 1 {
		t.Fatalf("enqueued = %d, want 1 (the turnstile should admit a better title)", d.Enqueued)
	}
	got := lineupOf(t, st, "ch1")
	if got[provision.Key("movie:tmdb:200")] {
		t.Error("the weakest retirable title should have been retired")
	}
	if !got[provision.Key("movie:tmdb:100")] {
		t.Error("the AIRING title must never be retired")
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
	cur := recurate.NewCurator(st, fixedThresholds{minScorePct: 60, maxTitles: 1}, time.Now, testkit.Logger())

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
	cur := recurate.NewCurator(st, fixedThresholds{minScorePct: 60, maxTitles: 10}, time.Now, testkit.Logger())

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
	cur := recurate.NewCurator(st, fixedThresholds{minScorePct: 60, maxTitles: 2}, time.Now, testkit.Logger())

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
	cur := recurate.NewCurator(st, fixedThresholds{minScorePct: 0, maxTitles: 2}, time.Now, testkit.Logger())

	if _, err := cur.Consider(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	if !lineupOf(t, st, "ch1")[provision.Key("movie:tmdb:200")] {
		t.Fatal("a tie retired the bench title — that is coin-flip churn")
	}
}
