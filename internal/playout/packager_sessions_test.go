package playout

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// hostPerChannelSource airs slowItemSource's item on every channel, encoded on the channel's host.
type hostPerChannelSource struct {
	slowItemSource
	hosts map[string]HostProfile
}

func (s hostPerChannelSource) Output(_ context.Context, channelID string, _ FormatClass, rung int) (HostProfile, OutputProfile) {
	_, out := s.slowItemSource.Output(context.Background(), channelID, FormatBaseline, rung)
	return s.hosts[channelID], out
}

// countingObserver records the active-packager deltas the metrics see.
type countingObserver struct {
	mu     sync.Mutex
	active int
}

func (o *countingObserver) PlayoutSessionStarted(string) {}
func (o *countingObserver) PlayoutSessionActive(d int) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.active += d
}
func (o *countingObserver) PlayoutProcessFailure(string) {}
func (o *countingObserver) Active() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.active
}

// newTestPackagerHLS runs channel packagers whose item encoders never produce output: the channels
// are on air (running, counted, stoppable) without an ffmpeg.
func newTestPackagerHLS(t *testing.T, source PackagerSource, grace time.Duration) *PackagerHLS {
	t.Helper()
	stuck := filepath.Join(t.TempDir(), "ffmpeg")
	if err := os.WriteFile(stuck, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	m, err := NewPackagerHLS(source, stuck, t.TempDir(), grace, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Stop)
	return m
}

func tune(t *testing.T, m *PackagerHLS, channelID string) func() {
	t.Helper()
	_, release, err := m.acquire(channelID, FormatBaseline)
	if err != nil {
		t.Fatal(err)
	}
	return release
}

// The dashboard's rows (§12, V16): one per running packager, sorted by channel so they do not
// reshuffle between polls, each counting its viewers and naming the encoder it resolved.
func TestPackagerStatsSnapshotsEachRunningPackager(t *testing.T) {
	m := newTestPackagerHLS(t, hostPerChannelSource{hosts: map[string]HostProfile{
		"a": HostFor(EncoderSoftware, false, GPUFilters{}),
		"b": HostFor(EncoderNVENC, true, GPUFilters{}),
	}}, time.Minute)
	defer tune(t, m, "b")()
	defer tune(t, m, "a")()
	defer tune(t, m, "a")()

	now := time.Now().Add(time.Second)
	stats := m.Stats(now)
	if len(stats) != 2 {
		t.Fatalf("Stats = %+v, want a row per running packager", stats)
	}
	a, b := stats[0], stats[1]
	if a.ChannelID != "a" || b.ChannelID != "b" {
		t.Fatalf("rows not sorted by channel: %q, %q", a.ChannelID, b.ChannelID)
	}
	if a.Viewers != 2 || b.Viewers != 1 {
		t.Errorf("viewers = %d, %d, want 2 (two tunes share one packager) and 1", a.Viewers, b.Viewers)
	}
	if a.Encoder != string(EncoderSoftware) || a.Hardware || b.Encoder != string(EncoderNVENC) || !b.Hardware {
		t.Errorf("encoders = %+v / %+v, want software then hardware nvenc", a, b)
	}
	for _, s := range stats {
		if s.Target != string(FormatBaseline) || s.TranscodeCost != 1 || s.UptimeMS < 1000 || s.ColdStartMS != 0 {
			t.Errorf("%s: %+v, want the baseline target, cost 1, uptime >= 1s and no cold start before a segment", s.ChannelID, s)
		}
	}
	if m.ActiveCount() != 2 {
		t.Errorf("ActiveCount = %d, want 2", m.ActiveCount())
	}
}

// OnChange is the dashboard's SSE frame: it fires when a packager starts and when one stops, and
// not when a viewer joins a packager that is already running.
func TestPackagerOnChangeFiresOnStartAndStopOnly(t *testing.T) {
	m := newTestPackagerHLS(t, slowItemSource{}, 50*time.Millisecond)
	changes := make(chan struct{}, 8)
	m.OnChange(func() { changes <- struct{}{} })
	expect := func(what string, want int) {
		t.Helper()
		got := 0
		for deadline := time.After(300 * time.Millisecond); ; {
			select {
			case <-changes:
				got++
				continue
			case <-deadline:
			}
			break
		}
		if got != want {
			t.Errorf("%s: %d change frames, want %d", what, got, want)
		}
	}

	first := tune(t, m, "ch")
	expect("first tune starts the packager", 1)
	second := tune(t, m, "ch")
	expect("a second viewer joins it", 0)
	first()
	second()
	expect("the last viewer leaves and the grace expires", 1)
	if m.ActiveCount() != 0 {
		t.Fatalf("ActiveCount = %d after the grace, want 0", m.ActiveCount())
	}
}

// A schedule edit invalidates the channel (reconcile's scheduleInvalidator → Origin.StopChannel):
// its packager stops at once, even with a viewer on it, so the next tune starts a fresh packager
// that reads the new schedule at the right offset. Other channels keep airing.
func TestPackagerStopChannelInvalidatesOnlyThatChannel(t *testing.T) {
	m := newTestPackagerHLS(t, slowItemSource{}, time.Minute)
	obs := &countingObserver{}
	m.WithObserver(obs)
	changes := make(chan struct{}, 8)
	m.OnChange(func() { changes <- struct{}{} })

	defer tune(t, m, "edited")()
	defer tune(t, m, "other")()
	m.mu.Lock()
	stale := m.channels[packagedKey{channel: "edited", format: FormatBaseline}]
	m.mu.Unlock()
	for range 2 {
		<-changes
	}

	m.StopChannel("edited")
	select {
	case <-stale.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the invalidated channel's packager kept running")
	}
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Error("no change frame for the stopped packager")
	}
	if got := m.Stats(time.Now()); len(got) != 1 || got[0].ChannelID != "other" {
		t.Fatalf("Stats after invalidation = %+v, want only the other channel", got)
	}
	if obs.Active() != 1 {
		t.Errorf("active packagers metric = %d, want 1", obs.Active())
	}

	defer tune(t, m, "edited")()
	m.mu.Lock()
	fresh := m.channels[packagedKey{channel: "edited", format: FormatBaseline}]
	m.mu.Unlock()
	if fresh == nil || fresh == stale {
		t.Fatal("the next tune after invalidation did not start a fresh packager")
	}
	if fresh.viewers != 1 {
		t.Errorf("fresh packager viewers = %d, want 1 (the stale packager's viewer does not carry over)", fresh.viewers)
	}
}
