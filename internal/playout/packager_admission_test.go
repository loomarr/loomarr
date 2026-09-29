package playout

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
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

	watched, err := m.acquirePlaylist("watched", PlanBaseline, false, "")
	if err != nil {
		t.Fatal(err)
	}
	defer watched.release()
	warm, err := m.acquirePlaylist("neighbour", PlanBaseline, true, "")
	if err != nil {
		t.Fatal(err)
	}
	warm.release() // the warm is done; its packager lingers in its grace, holding a slot

	// Warming only ever uses spare room: a second warm on the full host is refused, displacing nothing.
	if _, err := m.acquirePlaylist("other-neighbour", PlanBaseline, true, ""); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("a warm on a full host: err = %v, want ErrAtCapacity", err)
	}
	viewer, err := m.acquirePlaylist("surfed-to", PlanBaseline, false, "")
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
	if _, err := m.acquirePlaylist("third", PlanBaseline, false, ""); !errors.Is(err, ErrAtCapacity) {
		t.Fatalf("a viewer with only watched work running: err = %v, want ErrAtCapacity", err)
	}
}

// The order a full host gives up idle work for a viewer (#1780): premium warms first (#1037), each
// longest idle first, whether or not a viewer once watched it, then baseline warms, then
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
		premium("warm-4k"):      {speculative: true, warmHeld: true, idleSince: now.Add(-time.Second)},
		premium("adjacent-4k"):  {warmHeld: true, idleSince: now.Add(-2 * time.Second)},
	}
	var order []string
	for {
		key, victim, ok := pickIdleVictim(channels, now, victimPremium)
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
		"adjacent-4k/" + string(Format4KHDR),
		"warm-4k/" + string(Format4KHDR),
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
// at most watchedWithin, while a baseline keeps the full grace that absorbs surfing. A premium a
// neighbour warm last held is adjacent to its viewer and keeps the baseline's grace (#1037).
func TestPackagerHLS_PremiumGraceIsShortUnlessAdjacent(t *testing.T) {
	m := &PackagerHLS{grace: DefaultGrace}
	if got := m.graceFor(packagedKey{format: Format4KHDR}, &packagedChannel{}); got != watchedWithin {
		t.Errorf("premium grace = %s, want %s", got, watchedWithin)
	}
	if got := m.graceFor(packagedKey{format: Format4KHDR}, &packagedChannel{warmHeld: true}); got != DefaultGrace {
		t.Errorf("adjacent premium grace = %s, want %s", got, DefaultGrace)
	}
	if got := m.graceFor(packagedKey{format: FormatBaseline}, &packagedChannel{}); got != DefaultGrace {
		t.Errorf("baseline grace = %s, want %s", got, DefaultGrace)
	}
}

// lockedBuffer is a log sink the packagers' goroutines may write while the test reads it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// warmFacts is a measured NVENC host where every stream is cheap, so only the operator cap (0 =
// none) bounds how many packagers run.
func warmFacts(operatorCap int) BudgetFacts {
	f := premiumFacts(true)
	f.OperatorCap = operatorCap
	f.Costs[HDRKey(ClassHDR4K, 1080, DefaultToneCurve)] = ClassCost{Speed: 12, CPUCores: 0.1}
	f.Costs[CostKey{Class: ClassPremium4K, Height: premiumHeight}] = ClassCost{Speed: 12, CPUCores: 0.1}
	return f
}

// takePremium plays channelID's premium variant as viewer: the premium playlist request a client
// that takes the premium makes after reading the master. The sleeping encoder never lists a segment,
// so the request gives up after a moment, leaving the premium running in its grace.
func takePremium(t *testing.T, m *PackagerHLS, viewer, channelID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(WithViewer(t.Context(), viewer), 200*time.Millisecond)
	defer cancel()
	if _, _, err := m.MediaPlaylist(ctx, channelID, PlanBaseline, string(Format4KHDR)+".m3u8"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("premium play on %s: err = %v, want the wait for its first segment", channelID, err)
	}
}

// warmNeighbour is the neighbour warmer's master request for channelID on viewer's behalf. It reads
// the master through, which waits for the premium lookup and so for any premium warm it started.
func warmNeighbour(t *testing.T, m *PackagerHLS, viewer, channelID string) (string, error) {
	t.Helper()
	lease, err := m.acquirePlaylist(channelID, PlanBaseline, true, viewer)
	if err != nil {
		return "", err
	}
	defer lease.release()
	master, err := lease.snapshot(t.Context())
	return string(master), err
}

// #1037: on the real Shield every tune of a 4K channel started its premium cold, because a neighbour
// warm warmed only the baseline and the premium dies watchedWithin after its last fetch. A warm made
// for a viewer whose client takes the premium warms the neighbour's premium too, and it keeps the
// baseline's grace while a warm holds it; a viewer that has not taken a premium warms the baseline
// alone.
func TestPackagerHLS_APremiumViewersWarmAlsoWarmsTheNeighboursPremium(t *testing.T) {
	m, err := NewPackagerHLS(&premiumSource{premium: Format4KHDR}, sleepingFFmpeg(t), t.TempDir(), DefaultGrace, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.WithBudget(NewResourceBudget(func() BudgetFacts { return warmFacts(0) }))
	t.Cleanup(m.Stop)

	takePremium(t, m, "tv", "watched")
	master, err := warmNeighbour(t, m, "tv", "neighbour")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(master, string(Format4KHDR)+".m3u8") {
		t.Fatalf("the warm master does not name the premium:\n%s", master)
	}
	if _, err := warmNeighbour(t, m, "phone", "other"); err != nil {
		t.Fatal(err)
	}
	if _, err := warmNeighbour(t, m, "", "untagged"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"neighbour/" + string(FormatBaseline), "neighbour/" + string(Format4KHDR),
		"other/" + string(FormatBaseline),
		"untagged/" + string(FormatBaseline),
		"watched/" + string(Format4KHDR),
	}
	if got := m.runningChannels(); !slices.Equal(got, want) {
		t.Fatalf("running = %v, want %v (only the premium viewer's warm warms a premium)", got, want)
	}
	key := packagedKey{channel: "neighbour", format: Format4KHDR}
	m.mu.Lock()
	c := m.channels[key]
	grace := m.graceFor(key, c)
	m.mu.Unlock()
	if grace != DefaultGrace {
		t.Fatalf("a warmed premium's grace = %s, want the baseline's %s", grace, DefaultGrace)
	}
}

// A premium warm uses only spare room (#1037): it evicts nothing and never takes a baseline warm's
// place, so a baseline warm that finds the host full gives up a premium warm (and nothing else).
// Each decision is logged at INFO under one message, with the ledger's use against its limits.
func TestPackagerHLS_APremiumWarmOnlyUsesSpareRoom(t *testing.T) {
	var logs lockedBuffer
	m, err := NewPackagerHLS(&premiumSource{premium: Format4KHDR}, sleepingFFmpeg(t), t.TempDir(), DefaultGrace,
		slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err != nil {
		t.Fatal(err)
	}
	m.WithBudget(NewResourceBudget(func() BudgetFacts { return warmFacts(3) }))
	t.Cleanup(m.Stop)

	takePremium(t, m, "tv", "watched")
	if _, err := warmNeighbour(t, m, "tv", "left"); err != nil {
		t.Fatal(err)
	}
	if got, want := m.runningChannels(), []string{
		"left/" + string(FormatBaseline), "left/" + string(Format4KHDR), "watched/" + string(Format4KHDR),
	}; !slices.Equal(got, want) {
		t.Fatalf("running after the first warm = %v, want %v", got, want)
	}
	// The host is full. The next warm's baseline gives up the premium warm, never the watched
	// premium or the other baseline; its own premium finds no room and evicts nothing.
	if _, err := warmNeighbour(t, m, "tv", "right"); err != nil {
		t.Fatalf("a baseline warm was refused while a premium warm held its room: %v", err)
	}
	if got, want := m.runningChannels(), []string{
		"left/" + string(FormatBaseline), "right/" + string(FormatBaseline), "watched/" + string(Format4KHDR),
	}; !slices.Equal(got, want) {
		t.Fatalf("running after the second warm = %v, want %v", got, want)
	}

	out := logs.String()
	for _, want := range []string{
		`msg="packager hls: admission" decision=admit channel=left format=4k-hevc-hdr reason=premium-warm`,
		`msg="packager hls: admission" decision=evict channel=left format=4k-hevc-hdr reason=premium-warm for_channel=right for_format=1080p-h264-sdr`,
		`msg="packager hls: admission" decision=admit channel=right format=1080p-h264-sdr reason=warm`,
		`msg="packager hls: admission" decision=refuse channel=right format=4k-hevc-hdr reason=premium-warm`,
		`budget.encoders=3/3`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("logs lack %q:\n%s", want, out)
		}
	}
}
