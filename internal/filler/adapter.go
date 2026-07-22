package filler

import (
	"context"
	"log/slog"

	"github.com/mantonx/loomarr/internal/metrics"
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

// Selection is a channel's per-channel filler choice in filler-native terms (§10) — the
// projection of schedule.FillerSelection that assembly needs. Callers (channels/app,
// which know both packages) translate the policy type to this at the boundary, keeping
// the filler domain free of a schedule dependency. All fields optional; the zero
// Selection is the whole catalog (the prior global behaviour).
type Selection struct {
	Era        int      // 0 = any (a representative year for the era ladder)
	Audience   Audience // "" = any
	Categories []string // empty = any
	Kinds      []string // empty = the default kinds
	Pinned     []string // clip ids always included
	Excluded   []string // clip ids never used
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
func (a *PodAdapter) BuildFillerList(ctx context.Context, channelID string, seed int64, sel Selection) ([]string, bool) {
	pod, err := a.Preview(ctx, channelID, seed, sel)
	if err != nil {
		if a.log != nil {
			a.log.Warn("filler list: load catalog failed (channel stays flex)", "channel", channelID, "err", err)
		}
		return nil, false
	}
	// §17 fallback-ladder depth: record the rung this reconcile's pod reached.
	// Recorded here (the attach path) not in Preview, so UI previews don't inflate
	// it; skipped when the catalog is empty (a zero-value MatchLevel).
	if pod.MatchLevel != "" {
		metrics.FillerPodAssembled(string(pod.MatchLevel))
	}
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

// Preview assembles the pool a channel WOULD get, without touching Tunarr (§12: the
// Filler view previews a channel's pods). It is the SAME call BuildFillerList makes —
// deliberately one code path, because a preview that diverges from what reconcile
// actually attaches is worse than no preview: it would confidently show pods the
// operator never receives.
//
// Assemble is pure and seeded-deterministic, so preview and the next reconcile of the
// same channel produce identical output. Returns an empty pod (not an error) when the
// catalog is empty — "no clips yet" is a normal state the UI renders, not a failure.
func (a *PodAdapter) Preview(ctx context.Context, channelID string, seed int64, sel Selection) (Pod, error) {
	clips, err := a.catalog.AllClips(ctx)
	if err != nil {
		return Pod{}, err
	}
	if len(clips) == 0 {
		return Pod{}, nil
	}
	podMax := a.policy.PodMax
	if podMax <= 0 {
		podMax = 4 // matches fillCommercials' own fallback (the setting default is 4)
	}
	// The window IS the per-channel selection (§10): era/audience narrow the ladder,
	// categories/kinds narrow the catalog, pinned/excluded are the overrides. An empty
	// Selection leaves every field at its zero "any" value → the whole catalog, which is
	// the prior behaviour (and the additive default for a channel with no filler choice).
	w := Window{
		ChannelID: channelID, Seed: seed,
		Era: sel.Era, Audience: sel.Audience,
		GapMs: poolGapMs, PodMax: podMax,
		Categories: sel.Categories, Kinds: sel.Kinds,
		Pinned: sel.Pinned, Excluded: sel.Excluded,
	}
	return Assemble(clips, w, a.policy, map[string]bool{}), nil
}

var _ interface {
	BuildFillerList(ctx context.Context, channelID string, seed int64, sel Selection) ([]string, bool)
} = (*PodAdapter)(nil)
