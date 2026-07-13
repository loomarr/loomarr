package testkit

import (
	"context"
	"fmt"
	"sync"

	"github.com/mantonx/loomarr/internal/programmer"
	"github.com/mantonx/loomarr/internal/schedule"
)

// Tunarr is the shared Programmer test double (CLAUDE.md: one mock per service,
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
	Creates int
	Updates int
	Pushes  int // SetLineup calls that actually happened
	Deletes int
	// Injectable failures (nil = success).
	SetLineupErr error
}

type tunarrChan struct {
	spec   programmer.ChannelSpec
	lineup []schedule.Slot
	set    bool // whether programming has been set (else GetLineup mimics 400→empty)
}

// NewTunarr builds an empty in-memory Tunarr.
func NewTunarr() *Tunarr {
	return &Tunarr{channels: map[string]*tunarrChan{}}
}

func (m *Tunarr) EnsureChannel(_ context.Context, spec programmer.ChannelSpec) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if spec.TunarrID == "" {
		// Create: assign a fresh server id, IGNORING any client-supplied id
		// (Phase-0 finding 1). Return the assigned id.
		m.seq++
		id := fmt.Sprintf("srv-%d", m.seq)
		spec.TunarrID = id
		m.channels[id] = &tunarrChan{spec: spec}
		m.Creates++
		return id, nil
	}
	// Update: overwrite the stored spec (create it if the test pre-set an id).
	ch, ok := m.channels[spec.TunarrID]
	if !ok {
		ch = &tunarrChan{}
		m.channels[spec.TunarrID] = ch
	}
	ch.spec = spec
	m.Updates++
	return spec.TunarrID, nil
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
	}, true, nil
}

func (m *Tunarr) SetLineup(_ context.Context, tunarrID string, slots []schedule.Slot) error {
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
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.channels, tunarrID)
	m.Deletes++
	return nil
}

// readback mirrors programmer.slotToItem+itemToSlot: what a real Tunarr would
// return for a pushed slot. A program or filler-with-item is content (keeps id);
// everything else is flex (loses id/key). This is deliberately lossy — it's what
// forces the reconcile diff to compare on the *pushable* shape, not the domain
// slot, so idempotency holds against a real Tunarr too.
func readback(s schedule.Slot) schedule.Slot {
	switch s.Kind {
	case schedule.SlotProgram:
		return schedule.Slot{Kind: schedule.SlotProgram, LibraryItemID: s.LibraryItemID, DurationMs: s.DurationMs}
	case schedule.SlotFiller:
		if s.LibraryItemID != "" {
			return schedule.Slot{Kind: schedule.SlotProgram, LibraryItemID: s.LibraryItemID, DurationMs: s.DurationMs}
		}
		return schedule.Slot{Kind: schedule.SlotFlex, DurationMs: s.DurationMs}
	default:
		return schedule.Slot{Kind: schedule.SlotFlex, DurationMs: s.DurationMs}
	}
}

var _ programmer.Programmer = (*Tunarr)(nil)
