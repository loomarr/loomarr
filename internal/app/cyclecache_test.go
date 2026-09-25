package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// countingCycle is stubCycle that records how many arrangements were actually computed.
//
// The COUNT is the whole point: the cache's contract is "the same channel, unchanged, is
// arranged once", and a test that only asserted on the returned slots would pass just as
// happily with no cache at all. Counting is what makes these tests capable of failing.
type countingCycle struct {
	mu     sync.Mutex
	calls  int
	slots  []schedule.Slot
	window time.Duration
}

func (c *countingCycle) CyclePreview(context.Context, string, time.Time) (
	time.Time, []schedule.Slot, schedule.ActiveRuleAttribution, time.Duration, error,
) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return time.Time{}, c.slots, schedule.ActiveRuleAttribution{}, c.window, nil
}

func (c *countingCycle) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

// stubChannels serves one mutable channel, so a test can change a lineup or policy mid-run and
// assert the fingerprint noticed.
type stubChannels struct {
	mu sync.Mutex
	ch store.Channel
}

func (s *stubChannels) GetChannel(context.Context, string) (store.Channel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ch.PlayoutAnchor.IsZero() {
		s.ch.PlayoutAnchor = time.Unix(1_700_000_000, 0).UTC()
	}
	return s.ch, nil
}

func (s *stubChannels) set(mutate func(*store.Channel)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	mutate(&s.ch)
}

func cachedResolver(t *testing.T, ch store.Channel) (*playoutResolver, *countingCycle, *stubChannels) {
	t.Helper()
	eng := &countingCycle{slots: []schedule.Slot{
		{Kind: schedule.SlotProgram, LibraryItemID: "x", DurationMs: 600_000},
	}}
	chans := &stubChannels{ch: ch}
	return &playoutResolver{
		engine:   eng,
		now:      time.Now,
		channels: chans,
		cycles:   newCycleCache(time.Now),
	}, eng, chans
}

func testChannel() store.Channel {
	return store.Channel{
		Lineup: []schedule.LineupEntry{{Key: provision.Key("tmdb:tv:1"), Title: "A"}},
	}
}

// THE GAP THIS CLOSES: the guide re-arranged every channel on every poll — 53% of the request's
// CPU spent recomputing an answer that had not changed. If this ever goes back to computing per
// request, the endpoint silently returns to ~100ms/channel.
func TestCycleCache_UnchangedChannelIsArrangedOnce(t *testing.T) {
	t.Parallel()
	r, eng, _ := cachedResolver(t, testChannel())
	at := time.Now()

	for range 5 {
		if _, _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
			t.Fatalf("cycleAt: %v", err)
		}
	}

	if got := eng.count(); got != 1 {
		t.Fatalf("arrangements computed = %d, want 1 (the cache is not holding)", got)
	}
}

// A changed LINEUP must never be served from the cache. This is the correctness half: the
// fingerprint exists so a stale arrangement is unreachable rather than merely unlikely.
func TestCycleCache_ChangedLineupIsRearranged(t *testing.T) {
	t.Parallel()
	r, eng, chans := cachedResolver(t, testChannel())
	at := time.Now()

	if _, _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}
	chans.set(func(c *store.Channel) {
		c.Lineup = append(c.Lineup, schedule.LineupEntry{Key: provision.Key("tmdb:tv:2"), Title: "B"})
	})
	if _, _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}

	if got := eng.count(); got != 2 {
		t.Fatalf("arrangements computed = %d, want 2 (a changed lineup was served stale)", got)
	}
}

// A changed POLICY must also miss. Policy is hashed WHOLE rather than field-by-field precisely
// so this holds for fields nobody enumerated — CollectionID, Genres, a rule added next quarter.
// Mutating a deeply-nested one is the point: a shallow hash would pass the lineup test above and
// still fail here.
func TestCycleCache_ChangedPolicyIsRearranged(t *testing.T) {
	t.Parallel()
	r, eng, chans := cachedResolver(t, testChannel())
	at := time.Now()

	if _, _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}
	chans.set(func(c *store.Channel) {
		c.Policy.Rules = append(c.Policy.Rules, schedule.SchedulingRule{ID: "r1", Label: "Marathon"})
	})
	if _, _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}

	if got := eng.count(); got != 2 {
		t.Fatalf("arrangements computed = %d, want 2 (a changed policy was served stale)", got)
	}
}

// Metadata the scheduler reads but a naive hash would skip. CollectionID is HEALED by a later
// reconcile (0 = unresolved → a real id), which changes franchise ordering — so a fingerprint
// that only covered Key/season would pin the channel to its pre-heal arrangement forever.
func TestCycleCache_ChangedEntryMetadataIsRearranged(t *testing.T) {
	t.Parallel()
	r, eng, chans := cachedResolver(t, testChannel())
	at := time.Now()

	if _, _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}
	chans.set(func(c *store.Channel) { c.Lineup[0].CollectionID = 42 })
	if _, _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}

	if got := eng.count(); got != 2 {
		t.Fatalf("arrangements computed = %d, want 2 (healed CollectionID was served stale)", got)
	}
}

// Two requests a few seconds apart SHARE an arrangement. Without quantisation the guide's window
// start moves every poll and the cache would never hit — a cache that is correct and useless.
func TestCycleCache_NearbyInstantsShareAnArrangement(t *testing.T) {
	t.Parallel()
	r, eng, _ := cachedResolver(t, testChannel())
	at := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)

	if _, _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}
	if _, _, err := r.cycleAt(context.Background(), "ch1", at.Add(time.Second)); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}

	if got := eng.count(); got != 1 {
		t.Fatalf("arrangements computed = %d, want 1 (quantisation is not working)", got)
	}
}

// An entry must not outlive its TTL: availability, hot-applied settings and the filler pool are
// invisible to the fingerprint by design, and the TTL is the ONLY thing bounding their staleness.
func TestCycleCache_EntryExpiresAfterTTL(t *testing.T) {
	t.Parallel()
	now := time.Now()
	clock := func() time.Time { return now }
	c := newCycleCache(clock)

	key, ok := fingerprintChannel("ch1", nil, schedule.ChannelPolicy{}, 0)
	if !ok {
		t.Fatal("fingerprint failed on an empty channel")
	}
	c.put(key, now, []schedule.Slot{{Kind: schedule.SlotProgram}}, 24*time.Hour)

	if _, _, hit := c.get(key, now); !hit {
		t.Fatal("fresh entry missed")
	}
	stored := now
	now = now.Add(cycleCacheTTL + time.Second)
	if _, _, hit := c.get(key, stored); hit {
		t.Fatal("entry survived its TTL — availability/settings staleness is now unbounded")
	}
}

// With no store wired the resolver must still answer, computing live. Tests and any install that
// skips the wiring get correct (merely slower) listings rather than an empty grid.
func TestCycleCache_NoStoreFallsBackToLiveComputation(t *testing.T) {
	t.Parallel()
	eng := &countingCycle{slots: []schedule.Slot{{Kind: schedule.SlotProgram}}}
	r := &playoutResolver{engine: eng, now: time.Now} // no channels, no cycles

	slots, _, err := r.cycleAt(context.Background(), "ch1", time.Now())
	if err != nil {
		t.Fatalf("cycleAt: %v", err)
	}
	if len(slots) != 1 {
		t.Fatalf("slots = %d, want 1 (the uncached path must still resolve)", len(slots))
	}
	if got := eng.count(); got != 1 {
		t.Fatalf("arrangements computed = %d, want 1", got)
	}
}

// A channel that CANNOT vary with time must share one entry across every instant. This is the
// whole win: the guide's window start advances with the wall clock, so a bucketed key missed
// every sixty seconds and re-paid a full arrangement for a byte-identical answer — which is what
// put the endpoint's p99 at 90ms against a p50 of 21ms.
func TestCycleCache_TimeInvariantChannelIgnoresTheBucket(t *testing.T) {
	t.Parallel()
	ch := testChannel()
	ch.Policy.Seasonal.Mode = schedule.SeasonalOff // explicit off + no rules ⇒ invariant
	r, eng, _ := cachedResolver(t, ch)
	at := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)

	// Instants a minute, an hour and a day apart must all hit.
	for _, d := range []time.Duration{0, time.Minute, time.Hour, 24 * time.Hour} {
		if _, _, err := r.cycleAt(context.Background(), "ch1", at.Add(d)); err != nil {
			t.Fatalf("cycleAt(+%v): %v", d, err)
		}
	}

	if got := eng.count(); got != 1 {
		t.Fatalf("arrangements computed = %d, want 1 (the bucket is still fragmenting an invariant channel)", got)
	}
}

// ⚠ THE TRAP the holiday case in the table test below guards: the resolved seasonal default is
// Auto, NOT Off, so a channel with an entirely EMPTY seasonal policy still benches and unbenches
// items as holiday windows open. The key must split there, or a Christmas lineup is served in
// January.

// #1397: `at` is the Guide window's `from`, so stepping ±1h or opening the Guide a minute later
// used to change the minute-bucketed key and re-arrange every channel. Nothing the arrangement
// reads had changed: the same rule, the same holiday state, the same rolling window.
func TestCycleCache_AdjacentWindowsReuseTheArrangement(t *testing.T) {
	t.Parallel()
	r, eng, _ := cachedResolver(t, testChannel()) // empty seasonal policy: the prod shape
	at := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)

	for _, d := range []time.Duration{0, time.Hour, -time.Hour, 3 * time.Hour, 90 * time.Second} {
		if _, _, err := r.cycleAt(context.Background(), "ch1", at.Add(d)); err != nil {
			t.Fatalf("cycleAt(%v): %v", d, err)
		}
	}
	if got := eng.count(); got != 1 {
		t.Fatalf("arrangements computed = %d, want 1 for windows inside one rolling window", got)
	}
}

// What the arrangement DOES read from the clock must still split the key, or the fix above would
// serve a stale lineup. Each case moves `at` across exactly one real boundary.
func TestCycleCache_ClockInputsThatChangeTheArrangementSplitTheKey(t *testing.T) {
	t.Parallel()
	july := time.Date(2026, time.July, 20, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		mutate func(*store.Channel)
		window time.Duration
		a, b   time.Time
		want   int
	}{
		{"holiday window opens (empty seasonal policy)", func(*store.Channel) {}, 0,
			july, time.Date(2026, time.December, 20, 12, 0, 0, 0, time.UTC), 2},
		{"rule boundary crossed", func(c *store.Channel) {
			c.Policy.Rules = []schedule.SchedulingRule{{ID: "late", When: schedule.WhenPredicate{HourFrom: 21, HourTo: 23}}}
		}, 0, july, july.Add(10 * time.Hour), 2},
		{"rule boundary NOT crossed", func(c *store.Channel) {
			c.Policy.Rules = []schedule.SchedulingRule{{ID: "late", When: schedule.WhenPredicate{HourFrom: 21, HourTo: 23}}}
		}, 0, july, july.Add(2 * time.Hour), 1},
		{"rolling window rotates", func(*store.Channel) {}, 24 * time.Hour,
			july, july.Add(25 * time.Hour), 2},
		{"same rolling window", func(*store.Channel) {}, 24 * time.Hour,
			july, july.Add(2 * time.Hour), 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ch := testChannel()
			c.mutate(&ch)
			r, eng, _ := cachedResolver(t, ch)
			eng.window = c.window
			for _, at := range []time.Time{c.a, c.b} {
				if _, _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
					t.Fatalf("cycleAt: %v", err)
				}
			}
			if got := eng.count(); got != c.want {
				t.Fatalf("arrangements computed = %d, want %d", got, c.want)
			}
		})
	}
}
