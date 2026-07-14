package filler

import (
	"context"
	"log/slog"

	"github.com/mantonx/loomarr/internal/schedule"
)

// PodAdapter implements the scheduler's PodFiller port (§10): it loads the clip
// catalog and assembles a matched pod for a channel's filler gap, returning the
// clips as schedule.Slots the reconcile engine places. It bridges the pure
// Assemble() to the store-backed catalog + the scheduler's slot type — keeping
// both the filler domain and the scheduler free of each other's concerns.
type PodAdapter struct {
	catalog CatalogReader
	policy  Policy
	log     *slog.Logger
	// perChannelUsed tracks no-repeat-in-window across gaps of one reconcile pass.
	// v1 uses a fresh set per FillGap call (per-gap no-repeat); window-wide
	// no-repeat across a channel's whole lineup is a future refinement (§10/§20).
}

// CatalogReader loads clips for pod assembly (implemented by the store).
type CatalogReader interface {
	// AllClips returns the full filler catalog for matching.
	AllClips(ctx context.Context) ([]Clip, error)
}

// NewPodAdapter builds the scheduler-facing pod assembler.
func NewPodAdapter(catalog CatalogReader, policy Policy, log *slog.Logger) *PodAdapter {
	return &PodAdapter{catalog: catalog, policy: policy, log: log}
}

// FillGap implements channels.PodFiller: assemble a matched pod for one gap and
// return its clips as filler slots. On any error (or an empty catalog) it returns
// nil so the reconcile leaves the gap as flex — pods degrade to flex, never fail
// the reconcile (§10 never dead air, and §9 reconcile resilience).
func (a *PodAdapter) FillGap(ctx context.Context, channelID string, era int, gapMs, seed int64) []schedule.Slot {
	clips, err := a.catalog.AllClips(ctx)
	if err != nil {
		if a.log != nil {
			a.log.Warn("pod fill: load catalog failed (gap stays flex)", "channel", channelID, "err", err)
		}
		return nil
	}
	if len(clips) == 0 {
		return nil // no filler yet → flex
	}
	w := Window{
		ChannelID: channelID, Seed: seed, Era: era,
		Audience: General, // v1: channel-level audience wiring is future; General matches broadly
		GapMs:    gapMs, PodMax: 4,
	}
	pod := Assemble(clips, w, a.policy, map[string]bool{})
	slots := make([]schedule.Slot, 0, len(pod.Entries))
	for _, e := range pod.Entries {
		slots = append(slots, schedule.Slot{
			Kind:          schedule.SlotFiller,
			LibraryItemID: e.LibraryItemID, // "" for the embedded bumper card → renders as flex
			Title:         e.Name,
			DurationMs:    e.DurationMs,
		})
	}
	return slots
}

var _ interface {
	FillGap(ctx context.Context, channelID string, era int, gapMs, seed int64) []schedule.Slot
} = (*PodAdapter)(nil)
