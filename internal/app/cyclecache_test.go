package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// countingCycle is stubCycle that records how many arrangements were actually computed.
//
// The COUNT is the whole point: the cache's contract is "the same channel, unchanged, is
// arranged once", and a test that only asserted on the returned slots would pass just as
// happily with no cache at all. Counting is what makes these tests capable of failing.
type countingCycle struct {
	mu    sync.Mutex
	calls int
	slots []schedule.Slot
}

func (c *countingCycle) CyclePreview(context.Context, string, time.Time) (
	time.Time, []schedule.Slot, schedule.ActiveRuleAttribution, time.Duration, error,
) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
	return time.Time{}, c.slots, schedule.ActiveRuleAttribution{}, 0, nil
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
	r, eng, _ := cachedResolver(t, testChannel())
	at := time.Now()

	for range 5 {
		if _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
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
	r, eng, chans := cachedResolver(t, testChannel())
	at := time.Now()

	if _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}
	chans.set(func(c *store.Channel) {
		c.Lineup = append(c.Lineup, schedule.LineupEntry{Key: provision.Key("tmdb:tv:2"), Title: "B"})
	})
	if _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
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
	r, eng, chans := cachedResolver(t, testChannel())
	at := time.Now()

	if _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}
	chans.set(func(c *store.Channel) {
		c.Policy.Rules = append(c.Policy.Rules, schedule.SchedulingRule{ID: "r1", Label: "Marathon"})
	})
	if _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
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
	r, eng, chans := cachedResolver(t, testChannel())
	at := time.Now()

	if _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}
	chans.set(func(c *store.Channel) { c.Lineup[0].CollectionID = 42 })
	if _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}

	if got := eng.count(); got != 2 {
		t.Fatalf("arrangements computed = %d, want 2 (healed CollectionID was served stale)", got)
	}
}

// Crossing a bucket re-arranges: `at` feeds ComputeDesiredAt, ActiveRuleAt and ResolveWindow, so
// a curation rule that switches the channel at 21:00 genuinely changes the answer. The bucket is
// what bounds how long the pre-boundary arrangement can survive.
func TestCycleCache_CrossingABucketRearranges(t *testing.T) {
	r, eng, _ := cachedResolver(t, testChannel())
	at := time.Now().Truncate(cycleBucket)

	if _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}
	if _, err := r.cycleAt(context.Background(), "ch1", at.Add(cycleBucket)); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}

	if got := eng.count(); got != 2 {
		t.Fatalf("arrangements computed = %d, want 2 (the bucket is not in the key)", got)
	}
}

// Two requests a few seconds apart SHARE an arrangement. Without quantisation the guide's window
// start moves every poll and the cache would never hit — a cache that is correct and useless.
func TestCycleCache_NearbyInstantsShareAnArrangement(t *testing.T) {
	r, eng, _ := cachedResolver(t, testChannel())
	at := time.Now().Truncate(cycleBucket)

	if _, err := r.cycleAt(context.Background(), "ch1", at); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}
	if _, err := r.cycleAt(context.Background(), "ch1", at.Add(time.Second)); err != nil {
		t.Fatalf("cycleAt: %v", err)
	}

	if got := eng.count(); got != 1 {
		t.Fatalf("arrangements computed = %d, want 1 (quantisation is not working)", got)
	}
}

// An entry must not outlive its TTL: availability, hot-applied settings and the filler pool are
// invisible to the fingerprint by design, and the TTL is the ONLY thing bounding their staleness.
func TestCycleCache_EntryExpiresAfterTTL(t *testing.T) {
	now := time.Now()
	clock := func() time.Time { return now }
	c := newCycleCache(clock)

	key, ok := fingerprintChannel("ch1", nil, schedule.ChannelPolicy{}, 0)
	if !ok {
		t.Fatal("fingerprint failed on an empty channel")
	}
	c.put(key, []schedule.Slot{{Kind: schedule.SlotProgram}}, now)

	if _, _, hit := c.get(key); !hit {
		t.Fatal("fresh entry missed")
	}
	now = now.Add(cycleCacheTTL + time.Second)
	if _, _, hit := c.get(key); hit {
		t.Fatal("entry survived its TTL — availability/settings staleness is now unbounded")
	}
}

// With no store wired the resolver must still answer, computing live. Tests and any install that
// skips the wiring get correct (merely slower) listings rather than an empty grid.
func TestCycleCache_NoStoreFallsBackToLiveComputation(t *testing.T) {
	eng := &countingCycle{slots: []schedule.Slot{{Kind: schedule.SlotProgram}}}
	r := &playoutResolver{engine: eng, now: time.Now} // no channels, no cycles

	slots, err := r.cycleAt(context.Background(), "ch1", time.Now())
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
