package playout

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// ErrAtCapacity reports that a new channel packager would exceed the admission budget.
//
// A distinct error because the API must render it as 503 with an actionable message, not as
// a generic failure: the operator's fixes (wait for a slot, relax a lowering-only safety cap,
// or choose a lower quality tier) are only discoverable if we say which wall was hit.
var ErrAtCapacity = errors.New("playout: at channel capacity")

// DVRHorizon is the one server-side time-shift contract shared by the packager's playlist window,
// prepared playback and the Watch timeline (§9.1 V60): one bounded window per (Channel, format).
const DVRHorizon = 15 * time.Minute

// SessionObserver receives bounded lifecycle facts without Channel or plan identity.
type SessionObserver interface {
	PlayoutSessionStarted(result string)
	PlayoutSessionActive(delta int)
	PlayoutProcessFailure(stage string)
	// PlayoutFallback counts a degradation: "hardware_to_software" when an item's retry moves a
	// failed GPU stage onto the CPU.
	PlayoutFallback(reason string)
}

// SessionStat is a snapshot of one running channel packager, for the dashboard (§12, V16).
//
// A VALUE, not a pointer into the live packager: the caller reads it without holding a lock
// and cannot accidentally mutate an encoder's state while it runs.
type SessionStat struct {
	ChannelID string `json:"channelId"`
	// Target names the codec audience this encoder serves — "browser" or "mediaserver" (§9.1 V47).
	Target   string  `json:"target"`
	Viewers  int     `json:"viewers"`
	Encoder  string  `json:"encoder" doc:"Resolved encoder: hardware vendor, or 'software'"`
	Hardware bool    `json:"hardware" doc:"Whether the resolved encoder is hardware-accelerated"`
	Speed    float64 `json:"speed" doc:"Realtime multiple from ffmpeg; sustained <1.0 means the channel will stutter"`
	// BufferedMS is how far ahead of realtime the encoder has produced output. Negative means it
	// is BEHIND, the same condition a sub-1.0 speed reports, seen as accumulated deficit.
	BufferedMS int64 `json:"bufferedMs"`
	UptimeMS   int64 `json:"uptimeMs"`
	// ColdStartMS is how long this channel took from start to its first segment — the black-screen
	// window a viewer waited through (§9.1 V47 doctor). 0 before the first segment lands.
	ColdStartMS int64 `json:"coldStartMs"`
	// TranscodeCost is this packager's video-transcode admission cost: one measured encode slot.
	TranscodeCost int `json:"transcodeCost"`
}

// WithObserver reports packagers starting, running and failing to the metrics.
func (m *PackagerHLS) WithObserver(observer SessionObserver) *PackagerHLS {
	m.observer = observer
	return m
}

// OnChange registers a hook fired whenever a packager starts or stops (the dashboard's SSE frame).
// It runs on its own goroutine, never under the packager lock.
func (m *PackagerHLS) OnChange(fn func()) { m.onChange = fn }

func (m *PackagerHLS) observe(f func(SessionObserver)) {
	if m.observer != nil {
		f(m.observer)
	}
}

func (m *PackagerHLS) changed() {
	if m.onChange != nil {
		go m.onChange()
	}
}

// ActiveCount is the number of running channel packagers: the encodes the load-aware quality
// ladder divides the host between.
func (m *PackagerHLS) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.channels)
}

// Capacity is the ledger's concurrent-transcode capacity, the denominator in the dashboard's
// "2 / 4" line; 0 when unbounded (no ledger).
func (m *PackagerHLS) Capacity() int {
	if m.budget == nil {
		return 0
	}
	return m.budget.Capacity()
}

// Budget snapshots the admission ledger for the status endpoint (#1520): capacity terms, per-class
// costs and what the running packagers hold.
func (m *PackagerHLS) Budget() BudgetSnapshot {
	if m.budget == nil {
		return BudgetSnapshot{}
	}
	return m.budget.Snapshot()
}

// Stats snapshots every running packager for the dashboard (§12, V16), sorted by channel so the
// rows do not reshuffle between polls.
func (m *PackagerHLS) Stats(now time.Time) []SessionStat {
	type row struct {
		key packagedKey
		c   *packagedChannel
		n   int
	}
	m.mu.Lock()
	rows := make([]row, 0, len(m.channels))
	for k, c := range m.channels {
		rows = append(rows, row{k, c, c.viewers})
	}
	m.mu.Unlock()
	slices.SortFunc(rows, func(a, b row) int {
		if c := strings.Compare(a.key.channel, b.key.channel); c != 0 {
			return c
		}
		return strings.Compare(string(a.key.format), string(b.key.format))
	})
	out := make([]SessionStat, 0, len(rows))
	for _, r := range rows {
		s := SessionStat{
			ChannelID: r.key.channel, Target: string(r.key.format), Viewers: r.n,
			Encoder: string(r.c.host.Encoder), Hardware: r.c.host.Encoder != EncoderSoftware,
			BufferedMS: r.c.p.Lead(now).Milliseconds(), UptimeMS: now.Sub(r.c.started).Milliseconds(),
			TranscodeCost: 1,
		}
		if ready := r.c.p.ReadyAt(); !ready.IsZero() {
			s.ColdStartMS = ready.Sub(r.c.started).Milliseconds()
		}
		out = append(out, s)
	}
	return out
}

// newestStillSegment is the channel's newest listed baseline segment behind its init, for the
// channel-switch still of a channel that is on air.
func (m *PackagerHLS) newestStillSegment(_ context.Context, channelID string, _ EncodePlan) (stillSegment, bool, error) {
	m.mu.Lock()
	c := m.channels[packagedKey{channel: channelID, format: FormatBaseline}]
	m.mu.Unlock()
	if c == nil {
		return stillSegment{}, false, nil
	}
	init := c.p.Init()
	name, at, ok := c.p.Newest(time.Now())
	if init == nil || !ok {
		return stillSegment{}, false, nil
	}
	return stillSegment{key: c.dir + "/" + name, at: at, parts: []func() (io.ReadCloser, error){
		func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(init)), nil },
		func() (io.ReadCloser, error) { return os.Open(filepath.Join(c.dir, name)) },
	}}, true, nil
}
