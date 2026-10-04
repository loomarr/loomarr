package channels_test

import (
	"context"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/tunarr/tunarrtest"
)

// sundayNoon is a Sunday, so a Sunday-evening rule lands inside a 24h horizon from it.
var sundayNoon = time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)

func film(key, title, rating string, genres ...string) schedule.LineupEntry {
	return schedule.LineupEntry{
		Key: provision.Key(key), Title: title, DurationMs: time.Hour.Milliseconds(),
		OfficialRating: schedule.Rating(rating), Genres: genres,
	}
}

// diffEngine is an internal-playout engine with a 24h rolling window, the production default
// shape: the forecast walks the same carry-over turns the guide and the encoder do.
func diffEngine(t testing.TB, entries ...schedule.LineupEntry) (*channels.Engine, store.Store, *tunarrtest.Tunarr) {
	t.Helper()
	st := testkit.MigratedSQLiteStore(t)
	tun := tunarrtest.NewTunarr()
	avail := mapAvail{}
	for _, e := range entries {
		avail[e.Key] = "lib-" + string(e.Key)
	}
	e := channels.New(st, tun, avail, nil, channels.Config{
		ResolveDefaultWindow:         func() time.Duration { return 24 * time.Hour },
		ResolvePlayoutBackendContext: func(context.Context) (string, error) { return schedule.PlayoutBackendInternal, nil },
	}, func() time.Time { return sundayNoon }, testkit.Logger())

	ch := store.Channel{Lineup: entries}
	ch.ID, ch.Name, ch.Number = "c1", "Movies", 5
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusLive
	ch.PlayoutAnchor = sundayNoon.Add(-36 * time.Hour)
	if _, err := st.SaveChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}
	return e, st, tun
}

func savedChannel(t testing.TB, st store.Store) store.Channel {
	t.Helper()
	ch, err := st.GetChannel(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

// A draft identical to the saved channel must report nothing: both sides go through the same
// engine call at the same instants, so an unchanged slot compares equal. A diff that reported
// noise here would make every real change unreadable.
func TestScheduleDiff_IdenticalDraftChangesNothing(t *testing.T) {
	e, st, _ := diffEngine(t,
		film("movie:tmdb:1", "Alpha", "PG"), film("movie:tmdb:2", "Bravo", "PG"),
		film("movie:tmdb:3", "Charlie", "PG"), film("movie:tmdb:4", "Delta", "PG"))
	saved := savedChannel(t, st)
	policy := saved.Policy

	for name, draft := range map[string]struct {
		lineup []schedule.LineupEntry
		policy *schedule.ChannelPolicy
	}{
		"no draft":      {},
		"same as saved": {lineup: slices.Clone(saved.Lineup), policy: &policy},
		"policy only":   {policy: &policy},
		"lineup only":   {lineup: slices.Clone(saved.Lineup)},
	} {
		t.Run(name, func(t *testing.T) {
			diff, err := e.ScheduleDiffDraft(context.Background(), "c1", sundayNoon, 7*24*time.Hour, draft.lineup, draft.policy)
			if err != nil {
				t.Fatal(err)
			}
			if len(diff.Changes) != 0 {
				t.Fatalf("identical draft reported %d changes, first %+v", len(diff.Changes), diff.Changes[0])
			}
			if diff.Compared == 0 {
				t.Fatal("compared 0 slots — an empty forecast would pass this test vacuously")
			}
		})
	}
}

// A rule draft changes exactly the slots inside the rule's window: before 18:00 and after
// 20:00 the draft airs what the saved channel airs, so nothing there may be reported.
func TestScheduleDiff_RuleDraftChangesOnlyItsWindow(t *testing.T) {
	e, st, _ := diffEngine(t,
		film("movie:tmdb:1", "Alpha", "PG", "Comedy"), film("movie:tmdb:2", "Bravo", "PG", "Comedy"),
		film("movie:tmdb:3", "Charlie", "PG", "Comedy"), film("movie:tmdb:4", "Delta", "PG", "Comedy"),
		film("movie:tmdb:5", "Fright", "PG", "Horror"), film("movie:tmdb:6", "Ghoul", "PG", "Horror"))
	draft := savedChannel(t, st).Policy
	draft.Rules = []schedule.SchedulingRule{{
		ID: "sun", Label: "Sunday horror", Priority: 10,
		When: schedule.WhenPredicate{Days: []time.Weekday{time.Sunday}, HourFrom: 18, HourTo: 20},
		What: &schedule.ScopePolicy{Genres: schedule.GenreFilter{Include: []string{"Horror"}}},
	}}

	diff, err := e.ScheduleDiffDraft(context.Background(), "c1", sundayNoon, 24*time.Hour, nil, &draft)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Changes) == 0 {
		t.Fatal("a Sunday 18–20 horror rule changed nothing — rule boundaries are not being forecast")
	}
	open := time.Date(2026, 7, 26, 18, 0, 0, 0, time.UTC)
	shut := time.Date(2026, 7, 26, 20, 0, 0, 0, time.UTC)
	for _, c := range diff.Changes {
		if c.Start.Before(open) || !c.Start.Before(shut) {
			t.Errorf("change at %s is outside the rule's 18:00–20:00 window: %+v", c.Start.Format(time.Kitchen), c)
			continue
		}
		if c.After == nil || !c.After.Rule.Matched || c.After.Rule.ID != "sun" {
			t.Errorf("change at %s: after side not attributed to the draft rule: %+v", c.Start.Format(time.Kitchen), c.After)
		} else if c.After.Title != "Fright" && c.After.Title != "Ghoul" {
			t.Errorf("change at %s: rule window airs %q, want a horror title", c.Start.Format(time.Kitchen), c.After.Title)
		}
		if c.Before == nil || c.Before.Rule.Matched {
			t.Errorf("change at %s: before side should be the base policy: %+v", c.Start.Format(time.Kitchen), c.Before)
		}
	}
	if first := diff.Changes[0].Start; !first.Equal(open) {
		t.Errorf("first change at %s, want the rule opening 18:00", first.Format(time.Kitchen))
	}
	// Hour-long films aligned to the hour: the window holds exactly two slots.
	if len(diff.Changes) != 2 {
		t.Errorf("changes = %d, want exactly the 18:00 and 19:00 slots", len(diff.Changes))
	}
}

// A policy change that shrinks the pool: the refused title disappears from the after side, and
// the rows that change name it on the before side.
func TestScheduleDiff_PolicyShrinkingThePoolReportsBeforeAndAfter(t *testing.T) {
	e, st, _ := diffEngine(t,
		film("movie:tmdb:1", "Alpha", "PG"), film("movie:tmdb:2", "Gore", "R"),
		film("movie:tmdb:3", "Charlie", "PG"), film("movie:tmdb:4", "Delta", "PG"))
	draft := savedChannel(t, st).Policy
	draft.Audience.Ceiling = "PG"

	diff, err := e.ScheduleDiffDraft(context.Background(), "c1", sundayNoon, 24*time.Hour, nil, &draft)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Changes) == 0 {
		t.Fatal("refusing a title changed nothing")
	}
	sawGoreBefore := false
	for _, c := range diff.Changes {
		if c.After != nil && c.After.Title == "Gore" {
			t.Errorf("change at %s still airs the refused title after the edit", c.Start)
		}
		if c.Before != nil && c.Before.Title == "Gore" {
			sawGoreBefore = true
		}
		if c.Before != nil && c.After != nil && c.Before.Key == c.After.Key && c.Before.Season == c.After.Season && c.Before.Episode == c.After.Episode {
			t.Errorf("change at %s reports the same airing on both sides: %+v", c.Start, c)
		}
		if !c.End.After(c.Start) {
			t.Errorf("change at %s has an empty span (end %s)", c.Start, c.End)
		}
	}
	if !sawGoreBefore {
		t.Error("no change names the refused title on the before side")
	}
	for i := 1; i < len(diff.Changes); i++ {
		if !diff.Changes[i].Start.After(diff.Changes[i-1].Start) {
			t.Fatalf("changes not in time order at %d", i)
		}
	}
}

// The horizon defaults to the channel's schedule window and is capped, so one request cannot
// forecast a month of arrangements.
func TestScheduleDiff_HorizonDefaultsToTheWindowAndIsCapped(t *testing.T) {
	e, _, _ := diffEngine(t, film("movie:tmdb:1", "Alpha", "PG"), film("movie:tmdb:2", "Bravo", "PG"))

	diff, err := e.ScheduleDiffDraft(context.Background(), "c1", sundayNoon, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.From.Equal(sundayNoon) || diff.To.Sub(diff.From) != 24*time.Hour {
		t.Errorf("default horizon = [%s, %s), want the 24h schedule window", diff.From, diff.To)
	}

	diff, err = e.ScheduleDiffDraft(context.Background(), "c1", sundayNoon, 30*24*time.Hour, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := diff.To.Sub(diff.From); got != channels.MaxScheduleDiffHorizon {
		t.Errorf("30-day horizon served %s, want the %s cap", got, channels.MaxScheduleDiffHorizon)
	}
}

// Determinism and read-only: two runs of the same draft are identical, and nothing is saved or
// pushed. A zero `from` resolves to the engine clock, never to a second wall-clock read.
func TestScheduleDiff_IsDeterministicAndReadOnly(t *testing.T) {
	e, st, tun := diffEngine(t,
		film("movie:tmdb:1", "Alpha", "PG"), film("movie:tmdb:2", "Gore", "R"), film("movie:tmdb:3", "Charlie", "PG"))
	before := savedChannel(t, st)
	draft := before.Policy
	draft.Audience.Ceiling = "PG"

	first, err := e.ScheduleDiffDraft(context.Background(), "c1", time.Time{}, 48*time.Hour, nil, &draft)
	if err != nil {
		t.Fatal(err)
	}
	second, err := e.ScheduleDiffDraft(context.Background(), "c1", time.Time{}, 48*time.Hour, nil, &draft)
	if err != nil {
		t.Fatal(err)
	}
	if !first.From.Equal(sundayNoon) {
		t.Errorf("zero from resolved to %s, want the engine clock", first.From)
	}
	if !reflect.DeepEqual(first, second) {
		t.Error("two runs of the same draft disagree")
	}
	after := savedChannel(t, st)
	if after.Revision != before.Revision || !reflect.DeepEqual(after.Policy, before.Policy) {
		t.Error("the diff persisted something")
	}
	if tun.Creates != 0 || tun.Pushes != 0 {
		t.Errorf("the diff touched Tunarr: creates=%d pushes=%d", tun.Creates, tun.Pushes)
	}
}

// Inside the carry-over tail — the window has turned but the programme crossing the boundary is
// still on — the forecast must air what the encoder airs: the ACCEPTED cycle from its anchor
// (playout.WindowTurn), and the next window only once that programme ends.
func TestScheduleDiff_ForecastHonoursTheCarryOverTail(t *testing.T) {
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	entries := []schedule.LineupEntry{
		film("movie:tmdb:1", "Alpha", "PG"), film("movie:tmdb:2", "Bravo", "PG"),
		film("movie:tmdb:3", "Charlie", "PG"), film("movie:tmdb:4", "Delta", "PG"), film("movie:tmdb:5", "Echo", "PG"),
	}
	avail := mapAvail{}
	for _, e := range entries {
		avail[e.Key] = "lib-" + string(e.Key)
	}
	clock := sundayNoon
	e := channels.New(st, nil, avail, nil, channels.Config{
		ResolveDefaultWindow:         func() time.Duration { return 24 * time.Hour },
		ResolvePlayoutBackendContext: func(context.Context) (string, error) { return schedule.PlayoutBackendInternal, nil },
	}, func() time.Time { return clock }, testkit.Logger())
	ch := store.Channel{Lineup: entries}
	ch.ID, ch.Name, ch.Number, ch.Strategy, ch.Status = "c1", "Movies", 5, schedule.Sequential, schedule.StatusBuilding
	if _, err := st.SaveChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}

	window, zone := e.RollingWindow(ch.Policy, sundayNoon)
	boundary := schedule.NextWindowStart(sundayNoon, window, zone)
	clock = boundary.Add(-30 * time.Minute) // go live 30 minutes before the window turns
	if err := e.Reconcile(ctx, "c1"); err != nil {
		t.Fatal(err)
	}
	accepted := savedChannel(t, st)
	if !accepted.PlayoutAnchor.Equal(clock) {
		t.Fatalf("anchor = %s, want the reconcile instant %s", accepted.PlayoutAnchor, clock)
	}

	from := boundary.Add(20 * time.Minute)
	clock = from
	onAir := playout.AiringAt(accepted.Desired, accepted.PlayoutAnchor, from)
	got, err := e.ForecastSaved(ctx, "c1", from, from.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 2 {
		t.Fatalf("forecast = %d blocks, want the tail and what follows", len(got))
	}
	if got[0].Title != onAir.Title || !got[0].Start.Equal(accepted.PlayoutAnchor) {
		t.Errorf("at %s the forecast airs %q from %s; the encoder airs %q from the anchor %s",
			from.Format(time.Kitchen), got[0].Title, got[0].Start.Format(time.Kitchen), onAir.Title, accepted.PlayoutAnchor.Format(time.Kitchen))
	}
	if want := accepted.PlayoutAnchor.Add(time.Hour); !got[1].Start.Equal(want) {
		t.Errorf("next window starts at %s, want where the crossing programme ends (%s)", got[1].Start.Format(time.Kitchen), want.Format(time.Kitchen))
	}

	// And an identical draft is still silent from inside the tail.
	diff, err := e.ScheduleDiffDraft(ctx, "c1", from, 48*time.Hour, slices.Clone(accepted.Lineup), &accepted.Policy)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Changes) != 0 {
		t.Errorf("identical draft from inside the carry-over tail reported %d changes", len(diff.Changes))
	}
}

// Rules match on the instant's own hour, so the caller's UTC offset must not decide which rule
// is active: the same instant written with another offset gives the same change list.
func TestScheduleDiff_CallerOffsetDoesNotMoveRuleHours(t *testing.T) {
	e, st, _ := diffEngine(t,
		film("movie:tmdb:1", "Alpha", "PG", "Comedy"), film("movie:tmdb:2", "Bravo", "PG", "Comedy"),
		film("movie:tmdb:5", "Fright", "PG", "Horror"))
	draft := savedChannel(t, st).Policy
	draft.Rules = []schedule.SchedulingRule{{
		ID: "sun", Priority: 10, When: schedule.WhenPredicate{Days: []time.Weekday{time.Sunday}, HourFrom: 18, HourTo: 20},
		What: &schedule.ScopePolicy{Genres: schedule.GenreFilter{Include: []string{"Horror"}}},
	}}

	utc, err := e.ScheduleDiffDraft(context.Background(), "c1", sundayNoon, 24*time.Hour, nil, &draft)
	if err != nil {
		t.Fatal(err)
	}
	kolkata := sundayNoon.In(time.FixedZone("IST", 5*3600+1800))
	ist, err := e.ScheduleDiffDraft(context.Background(), "c1", kolkata, 24*time.Hour, nil, &draft)
	if err != nil {
		t.Fatal(err)
	}
	if len(utc.Changes) == 0 || len(ist.Changes) != len(utc.Changes) {
		t.Fatalf("changes: UTC %d, +05:30 %d — want the same, non-empty", len(utc.Changes), len(ist.Changes))
	}
	for i := range utc.Changes {
		if !utc.Changes[i].Start.Equal(ist.Changes[i].Start) {
			t.Errorf("change %d at %s (UTC) vs %s (+05:30)", i, utc.Changes[i].Start, ist.Changes[i].Start)
		}
	}
}

func TestScheduleDiff_UnknownChannelIsNotFound(t *testing.T) {
	e, _, _ := diffEngine(t, film("movie:tmdb:1", "Alpha", "PG"))
	if _, err := e.ScheduleDiffDraft(context.Background(), "nope", sundayNoon, time.Hour, nil, nil); err == nil {
		t.Fatal("unknown channel returned no error")
	}
}

// BenchmarkScheduleDiff_SevenDays measures the realistic worst request: a week ahead, a
// 200-title channel with a shuffled base and a nightly rule, internal playout (carry-over).
func BenchmarkScheduleDiff_SevenDays(b *testing.B) {
	entries := make([]schedule.LineupEntry, 0, 200)
	for i := range 200 {
		genre := "Comedy"
		if i%5 == 0 {
			genre = "Horror"
		}
		entries = append(entries, film("movie:tmdb:"+strconv.Itoa(i+1), "Film "+strconv.Itoa(i+1), "PG", genre))
	}
	e, st, _ := diffEngine(b, entries...)
	draft := savedChannel(b, st).Policy
	draft.Ordering = schedule.OrderShuffle
	draft.Rules = []schedule.SchedulingRule{{
		ID: "night", Priority: 10, When: schedule.WhenPredicate{HourFrom: 22, HourTo: 2},
		What: &schedule.ScopePolicy{Genres: schedule.GenreFilter{Include: []string{"Horror"}}},
	}}
	b.ResetTimer()
	for range b.N {
		if _, err := e.ScheduleDiffDraft(context.Background(), "c1", sundayNoon, 7*24*time.Hour, nil, &draft); err != nil {
			b.Fatal(err)
		}
	}
}
