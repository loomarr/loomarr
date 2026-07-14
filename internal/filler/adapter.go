package filler

import (
	"context"
	"log/slog"
)

// PodAdapter implements the scheduler's PodFiller port (§10): it loads the clip
// catalog, assembles a matched clip pool for a channel (era/audience-matched,
// seed-deterministic), and returns the clips' Tunarr program uuids for the channel
// filler-list the reconcile engine attaches. It bridges the pure Assemble() to the
// store-backed catalog — keeping the filler domain and the scheduler free of each
// other's concerns. Since Tunarr plays the filler-list into ANY flex gap, this is
// a per-channel pool (not a per-gap sequence): the scheduler no longer sizes pods
// to individual gaps.
type PodAdapter struct {
	catalog CatalogReader
	policy  Policy
	log     *slog.Logger
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

// poolGapMs is the notional window Assemble fills to size the channel's clip pool.
// A channel's filler-list is a POOL Tunarr draws from, not a single sized break,
// so we assemble a generous pool (~one long break's worth); Tunarr picks per gap.
const poolGapMs = 600_000 // 10 min of clips

// BuildFillerList implements channels.PodFiller: assemble a matched clip pool for a
// channel and return the clips' Tunarr program uuids for its filler-list. ok=false
// on any error or an empty/all-fallback pool, so the reconcile skips the attach and
// the channel's flex falls back to the bumper card — never dead air (§10, §9). The
// pool is seed-deterministic (same catalog + seed → same uuids), so a re-reconcile
// produces the same list (idempotency at the EnsureFillerList layer, §9).
func (a *PodAdapter) BuildFillerList(ctx context.Context, channelID string, era int, seed int64) ([]string, bool) {
	clips, err := a.catalog.AllClips(ctx)
	if err != nil {
		if a.log != nil {
			a.log.Warn("filler list: load catalog failed (channel stays flex)", "channel", channelID, "err", err)
		}
		return nil, false
	}
	if len(clips) == 0 {
		return nil, false // no filler yet → flex
	}
	w := Window{
		ChannelID: channelID, Seed: seed, Era: era,
		Audience: General, // v1: channel-level audience wiring is future; General matches broadly
		GapMs:    poolGapMs, PodMax: 32,
	}
	pod := Assemble(clips, w, a.policy, map[string]bool{})
	ids := make([]string, 0, len(pod.Entries))
	for _, e := range pod.Entries {
		if e.TunarrProgramID == "" {
			continue // the embedded bumper-card fallback isn't a real Tunarr program
		}
		ids = append(ids, e.TunarrProgramID)
	}
	if len(ids) == 0 {
		return nil, false // only the fallback card → nothing to attach; flex
	}
	return ids, true
}

var _ interface {
	BuildFillerList(ctx context.Context, channelID string, era int, seed int64) ([]string, bool)
} = (*PodAdapter)(nil)
