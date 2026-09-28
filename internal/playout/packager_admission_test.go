package playout

import (
	"errors"
	"slices"
	"testing"
	"time"
)

// runningChannels lists the channels with a live packager, for assertions.
func (m *PackagerHLS) runningChannels() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for key := range m.channels {
		out = append(out, key.channel+"/"+string(key.format))
	}
	slices.Sort(out)
	return out
}

// #1780: on the real Shield, 38 of 120 channel surfs got 503 while neighbour warms and sessions in
// their grace, none of them watched, held the whole ledger. A viewer's tune evicts idle work before
// it is refused; speculative work never evicts anything, and watched work is never evicted.
func TestPackagerHLS_AViewerTuneEvictsAnIdleWarmBeforeItIsRefused(t *testing.T) {
	facts := nvencFacts()
	facts.OperatorCap = 2
	budget := NewResourceBudget(func() BudgetFacts { return facts })
	m := newTestPackagerHLS(t, slowItemSource{}, time.Hour)
	m.WithBudget(budget)

	watched, err := m.acquirePlaylist("watched", PlanBaseline, false)
	if err != nil {
		t.Fatal(err)
	}
	defer watched.release()
	warm, err := m.acquirePlaylist("neighbour", PlanBaseline, true)
	if err != nil {
		t.Fatal(err)
	}
	warm.release() // the warm is done; its packager lingers in its grace, holding a slot

	// Warming only ever uses spare room: a second warm on the full host is refused, displacing nothing.
	if _, err := m.acquirePlaylist("other-neighbour", PlanBaseline, true); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("a warm on a full host: err = %v, want ErrAtCapacity", err)
	}
	viewer, err := m.acquirePlaylist("surfed-to", PlanBaseline, false)
	if err != nil {
		t.Fatalf("a viewer was refused while an idle warm held the ledger: %v", err)
	}
	defer viewer.release()
	if got, want := m.runningChannels(), []string{"surfed-to/" + string(FormatBaseline), "watched/" + string(FormatBaseline)}; !slices.Equal(got, want) {
		t.Fatalf("running after the viewer's tune = %v, want %v (the idle warm evicted)", got, want)
	}
	if use := budget.Snapshot().InUse; use.Sessions != 2 {
		t.Fatalf("ledger after eviction = %+v, want exactly the two watched sessions", use)
	}
	// Only watched work is left, so the next viewer is refused rather than evicting a viewer.
	if _, err := m.acquirePlaylist("third", PlanBaseline, false); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("a viewer with only watched work running: err = %v, want ErrAtCapacity", err)
	}
}

// The order a full host gives up idle work for a viewer (#1780): speculative warms first, then
// sessions in their grace (oldest first), then a premium variant nobody is fetching whose channel's
// baseline still runs. A session polled within watchedWithin is watched, even between polls, and a
// baseline is never given up for a premium.
func TestPickIdleVictim_Order(t *testing.T) {
	now := time.Unix(10_000, 0)
	base := func(ch string) packagedKey { return packagedKey{channel: ch, format: FormatBaseline} }
	premium := func(ch string) packagedKey { return packagedKey{channel: ch, format: Format4KHDR} }
	channels := map[packagedKey]*packagedChannel{
		base("playing"):         {viewers: 1},
		base("between-polls"):   {idleSince: now.Add(-2 * time.Second)},
		base("grace-old"):       {idleSince: now.Add(-25 * time.Second)},
		base("grace-new"):       {idleSince: now.Add(-watchedWithin - time.Second)},
		base("warm"):            {speculative: true, idleSince: now.Add(-time.Second)},
		base("warm-joined"):     {speculative: true, viewers: 1},
		base("4k"):              {viewers: 1},
		premium("4k"):           {idleSince: now.Add(-time.Second)},
		premium("premium-only"): {idleSince: now.Add(-time.Second)},
	}
	var order []string
	for {
		key, victim, ok := pickIdleVictim(channels, now)
		if !ok {
			break
		}
		if channels[key] != victim {
			t.Fatalf("victim for %v is not the channel's packager", key)
		}
		delete(channels, key)
		order = append(order, key.channel+"/"+string(key.format))
	}
	want := []string{
		"warm/" + string(FormatBaseline),
		"grace-old/" + string(FormatBaseline),
		"grace-new/" + string(FormatBaseline),
		"4k/" + string(Format4KHDR),
	}
	if !slices.Equal(order, want) {
		t.Fatalf("eviction order = %v, want %v", order, want)
	}
}

// A premium variant is not kept running for a channel with no premium viewer (#1780): its grace is
// at most watchedWithin, while a baseline keeps the full grace that absorbs surfing.
func TestPackagerHLS_PremiumGraceIsShort(t *testing.T) {
	m := &PackagerHLS{grace: DefaultGrace}
	if got := m.graceFor(packagedKey{format: Format4KHDR}); got != watchedWithin {
		t.Errorf("premium grace = %s, want %s", got, watchedWithin)
	}
	if got := m.graceFor(packagedKey{format: FormatBaseline}); got != DefaultGrace {
		t.Errorf("baseline grace = %s, want %s", got, DefaultGrace)
	}
}
