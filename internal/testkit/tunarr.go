package testkit

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/schedule"
)

// Tunarr is the shared Programmer test double (AGENTS.md: one mock per service,
// never private). It is an in-memory Tunarr that faithfully models the two
// contract facts the reconcile engine depends on: the server assigns channel ids
// (Phase-0 finding 1), and lineup pushes replace programming. It records call
// counts so tests can assert idempotency (a second reconcile makes no new pushes)
// and minimal-diff.
type Tunarr struct {
	mu       sync.Mutex
	seq      int                    // server id counter (assigns ids)
	channels map[string]*tunarrChan // by server-assigned id
	// Call counters for assertions.
	Creates      int
	Updates      int
	Pushes       int // SetLineup calls that actually happened
	Deletes      int
	FillerWrites int // EnsureFillerList calls that changed the attached list
	// Local filler-source observations let adapter tests prove optional Tunarr annotation
	// stays dormant while unconfigured and hot-enables without a network service.
	FillerSourceEnsures int
	FillerClipReads     int
	LocalFillerClips    []programmer.LocalClip
	// Injectable failures (nil = success).
	SetLineupErr error
	// Optional synchronization hooks for deterministic concurrency tests. They run
	// BEFORE the fake takes its mutex, so a hook may block while the test commits a
	// competing store write without deadlocking Tunarr introspection. Production
	// interfaces do not expose these; they are observation points on the one shared
	// Programmer adapter rather than private per-package doubles.
	BeforeEnsureChannel func(programmer.ChannelSpec)
	BeforeSetLineup     func(tunarrID string, slots []schedule.Slot)
	BeforeDeleteChannel func(tunarrID string)
	// NowMs is the fake's clock for stamping a new channel's loop anchor (epoch ms). 0 ⇒ a
	// fixed non-zero default so a create always has a plausible, non-1970 anchor a test can
	// assert against; a test can set it to script the "preserve on update" check.
	NowMs int64
	// fillerLists records the program ids last attached per channel, so the double
	// models EnsureFillerList's internal idempotency (a second identical call is a
	// no-op → FillerWrites unchanged), mirroring the real adapter (§10).
	fillerLists map[string][]string
	// localFillerSource records whether EnsureLocalFillerSource has already created the
	// shared source, preserving the real adapter's idempotent result shape.
	localFillerSource bool
	// Media-source state for tunarr-connect (§6): the Emby source Loomarr wires so
	// Tunarr can index the library. sourceID is empty until EnsureEmbySource.
	sourceID         string
	msLibs           []*msLibrary
	MediaSourceToken string // captures the access token used (assert: the admin key, no user login)
	Scans            int    // library scans triggered
}

// msLibrary models one of Tunarr's enumerated media-server libraries (movies/shows/…).
type msLibrary struct {
	mediaType string
	enabled   bool
}

type tunarrChan struct {
	spec      programmer.ChannelSpec
	lineup    []schedule.Slot
	set       bool  // whether programming has been set (else GetLineup mimics 400→empty)
	startTime int64 // the loop anchor (epoch ms), stamped on create, preserved on update
}

// NewTunarr builds an empty in-memory Tunarr.
func NewTunarr() *Tunarr {
	return &Tunarr{channels: map[string]*tunarrChan{}, fillerLists: map[string][]string{}}
}

// defaultNowMs is a fixed, plausible epoch-ms anchor (2023-11-14T22:13:20Z) used when a
// test doesn't set NowMs — chosen far from 0/1970 so a "start time isn't epoch 0" assertion
// is meaningful.
const defaultNowMs int64 = 1_700_000_000_000

func (m *Tunarr) nowMs() int64 {
	if m.NowMs != 0 {
		return m.NowMs
	}
	return defaultNowMs
}

// EnsureLocalFillerSource models the idempotent local-source registration used by filler
// annotation. It is part of the shared Tunarr service double so unit tests never need a
// private HTTP server for this programmer slice.
func (m *Tunarr) EnsureLocalFillerSource(
	_ context.Context, _ string,
) (programmer.EnsureLocalSourceResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FillerSourceEnsures++
	added := !m.localFillerSource
	m.localFillerSource = true
	return programmer.EnsureLocalSourceResult{
		SourceID: "local-source", LibraryIDs: []string{"local-library"},
		SourceAdded: added, Scanned: true,
	}, nil
}

// ListLocalFillerClipsAll returns the configured in-memory local clips and records the read.
func (m *Tunarr) ListLocalFillerClipsAll(context.Context) ([]programmer.LocalClip, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FillerClipReads++
	return append([]programmer.LocalClip(nil), m.LocalFillerClips...), nil
}

func (m *Tunarr) EnsureChannel(_ context.Context, spec programmer.ChannelSpec) (string, error) {
	if m.BeforeEnsureChannel != nil {
		m.BeforeEnsureChannel(spec)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if spec.TunarrID == "" {
		// ⚠ **A DUPLICATE NUMBER IS REFUSED, because the real Tunarr refuses it** (§9 V54). This
		// fake used to accept any number on create, so every test passed while production failed:
		// Tunarr answers `POST /api/channels` with `500` and an EMPTY BODY when the number is
		// taken, which stranded a channel permanently. A double that cannot say no cannot catch
		// the bug — the same lesson as the context-ignoring doubles in filler and the scheduler.
		for _, existing := range m.channels {
			if existing.spec.Number == spec.Number {
				return "", fmt.Errorf("create channel %d: status 500: {}", spec.Number)
			}
		}
		// Create: assign a fresh server id, IGNORING any client-supplied id
		// (Phase-0 finding 1). Stamp the loop anchor NOW (mirrors the real adapter's
		// StartTime = now().UnixMilli()), so a later update can be checked for preserving it.
		m.seq++
		id := fmt.Sprintf("srv-%d", m.seq)
		spec.TunarrID = id
		m.channels[id] = &tunarrChan{spec: spec, startTime: m.nowMs()}
		m.Creates++
		return id, nil
	}
	// Update: overwrite the stored spec (create it if the test pre-set an id). Preserve the
	// existing loop anchor — the whole point of the §9 start-time fix.
	ch, ok := m.channels[spec.TunarrID]
	if !ok {
		ch = &tunarrChan{startTime: m.nowMs()}
		m.channels[spec.TunarrID] = ch
	}
	ch.spec = spec
	m.Updates++
	return spec.TunarrID, nil
}

// ListChannels reports every channel this Tunarr holds — including ones Loomarr didn't create, so
// a test can seed a foreign occupant on a number and prove Loomarr moves around it (§9 V54).
func (m *Tunarr) ListChannels(_ context.Context) ([]programmer.ActualChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]programmer.ActualChannel, 0, len(m.channels))
	for id, ch := range m.channels {
		out = append(out, programmer.ActualChannel{
			TunarrID: id, Number: ch.spec.Number, Name: ch.spec.Name,
			Group: ch.spec.Group, Logo: ch.spec.Logo, StartTime: ch.startTime,
		})
	}
	// Sorted so a test's assertions don't depend on Go's map iteration order.
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out, nil
}

// SeedForeignChannel plants a channel Loomarr did NOT create, on a given number — the state a
// reset database or an earlier install leaves behind, and the one that made a create fail forever.
func (m *Tunarr) SeedForeignChannel(number int, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	id := fmt.Sprintf("foreign-%d", m.seq)
	m.channels[id] = &tunarrChan{
		spec:      programmer.ChannelSpec{TunarrID: id, Number: number, Name: name},
		startTime: m.nowMs(),
	}
}

func (m *Tunarr) GetChannel(_ context.Context, tunarrID string) (programmer.ActualChannel, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.channels[tunarrID]
	if !ok {
		return programmer.ActualChannel{}, false, nil
	}
	pc := 0
	for _, s := range ch.lineup {
		if s.Kind == schedule.SlotProgram {
			pc++
		}
	}
	return programmer.ActualChannel{
		TunarrID:     tunarrID,
		Number:       ch.spec.Number,
		Name:         ch.spec.Name,
		Group:        ch.spec.Group,
		Logo:         ch.spec.Logo,
		ProgramCount: pc,
		StartTime:    ch.startTime,
	}, true, nil
}

func (m *Tunarr) SetLineup(_ context.Context, tunarrID string, slots []schedule.Slot) error {
	if m.BeforeSetLineup != nil {
		m.BeforeSetLineup(tunarrID, slots)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.SetLineupErr != nil {
		return m.SetLineupErr
	}
	ch, ok := m.channels[tunarrID]
	if !ok {
		return fmt.Errorf("testkit tunarr: set lineup on unknown channel %s", tunarrID)
	}
	// Store the pushed shape as Tunarr would read it back (content items keep
	// their library id; everything else becomes flex) so GetLineup round-trips
	// what a real Tunarr returns — this is what makes the idempotency diff honest.
	rt := make([]schedule.Slot, len(slots))
	for i, s := range slots {
		rt[i] = readback(s)
	}
	ch.lineup = rt
	ch.set = true
	m.Pushes++
	return nil
}

func (m *Tunarr) GetLineup(_ context.Context, tunarrID string) ([]schedule.Slot, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch, ok := m.channels[tunarrID]
	if !ok || !ch.set {
		return []schedule.Slot{}, nil // mimics Tunarr's 400-on-unprogrammed → empty
	}
	out := make([]schedule.Slot, len(ch.lineup))
	copy(out, ch.lineup)
	return out, nil
}

func (m *Tunarr) DeleteChannel(_ context.Context, tunarrID string) error {
	if m.BeforeDeleteChannel != nil {
		m.BeforeDeleteChannel(tunarrID)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.channels, tunarrID)
	m.Deletes++
	return nil
}

// EnsureFillerList models the real adapter's build+attach with internal
// idempotency (§10): it records the attached program-id set per channel and only
// counts a FillerWrite when that set CHANGES — matching the real adapter, which
// compares the desired set against the list's ACTUAL contents (not just count), so
// a re-tagged equal-sized pool still triggers a write. A "second reconcile makes no
// writes" assertion (§9) holds for an unchanged pool. An empty pool detaches.
func (m *Tunarr) EnsureFillerList(_ context.Context, tunarrID string, programIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := append([]string(nil), programIDs...)
	if sameIDs(m.fillerLists[tunarrID], ids) {
		return nil // unchanged → no write (idempotent)
	}
	if len(ids) == 0 {
		delete(m.fillerLists, tunarrID)
	} else {
		m.fillerLists[tunarrID] = ids
	}
	m.FillerWrites++
	return nil
}

// FillerListFor returns the program ids currently attached to a channel (test
// introspection).
func (m *Tunarr) FillerListFor(tunarrID string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.fillerLists[tunarrID]))
	copy(out, m.fillerLists[tunarrID])
	return out
}

func sameIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// readback mirrors programmer.slotToItem+itemToSlot: what a real Tunarr would
// return for a pushed slot. Only a program is content (keeps id); everything else
// — including filler, which post-§10-redesign lives in a Tunarr filler-list, never
// inline — is flex (loses id/key). Deliberately lossy: it forces the reconcile diff
// to compare on the *pushable* shape, not the domain slot, so idempotency holds
// against a real Tunarr too.
func readback(s schedule.Slot) schedule.Slot {
	if s.Kind == schedule.SlotProgram {
		return schedule.Slot{Kind: schedule.SlotProgram, LibraryItemID: s.LibraryItemID, DurationMs: s.DurationMs}
	}
	return schedule.Slot{Kind: schedule.SlotFlex, DurationMs: s.DurationMs}
}

var _ programmer.Programmer = (*Tunarr)(nil)
