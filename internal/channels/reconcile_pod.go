package channels

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/language"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout"
)

// SelectionForChannel translates a channel's persisted filler policy (§10
// FillerSelection) into the filler-package Selection the assembler consumes — the
// boundary translation that keeps the filler domain free of the schedule type. The era
// defaults from the channel's PROGRAM scope era when the filler selection sets no era of
// its own (a "90s action" channel gets 90s ads for free); a Range is a from–to window,
// so we take its lower bound as the representative era for the ladder. A nil filler
// policy yields the zero Selection = the whole catalog (the additive default).
//
// Exported so the §12 pod PREVIEW derives the selection identically. If preview computed
// its own, the two would drift and the UI would confidently show pods the reconciler
// never builds — the whole failure mode preview exists to prevent.
func SelectionForChannel(ch channelstore.Channel) pod.Selection {
	return SelectionForPolicy(ch.Policy, ch.Lineup)
}

// SelectionForPolicy is SelectionForChannel for a policy that is not (yet) the saved one — a draft.
// It carries everything the saved path does, break length and commercial spacing included, so a
// draft preview cannot assemble with the install defaults while the channel airs its own.
func SelectionForPolicy(pol schedule.ChannelPolicy, lineup []schedule.LineupEntry) pod.Selection {
	sel := SelectionFrom(pol.Filler, pol.Scope, schedule.DeriveFiller(pol, lineup))
	if pol.BreakDuration != nil {
		sel.BreakDurationMs = pol.BreakDuration.Std().Milliseconds()
	}
	if pol.CommercialSpacing != nil {
		spacing := pol.CommercialSpacing.Std()
		sel.Spacing = &spacing
	}
	return sel
}

// BreaksPerHourFor resolves a channel's commercial-break density (§10, V51f).
//
// ⚠ **One writer.** Reconcile and preview both call this rather than each carrying a copy of
// "no pool ⇒ 0, else the global value": that shape grows a quiet disagreement the moment a
// per-channel override exists. Preview claiming a density reconcile does not apply is the
// drift preview exists to prevent.
//
// ⚠ **The dead-air rule wins over the operator's override, deliberately.** No pool means no
// breaks whatever the policy says: break gaps with nothing to fill them leave empty flex that
// Tunarr renders as large channel-named blocks. The override lowers density or switches breaks
// off; it cannot conjure clips.
func BreaksPerHourFor(pol schedule.ChannelPolicy, hasFillerPool bool, global int) int {
	if !hasFillerPool {
		return 0
	}
	if pol.BreaksPerHour == nil {
		return global
	}
	if n := *pol.BreaksPerHour; n > 0 {
		return n
	}
	// A present zero (or a nonsense negative) is "no breaks on this channel".
	return 0
}

// MidRollFor reports whether a channel breaks INSIDE long programmes (§10 mid-roll): only on
// internal playout (Tunarr inserts filler at programme boundaries only), and on unless the
// channel's policy switches it off. Density and the filler pool are BreaksPerHourFor's decision.
func MidRollFor(pol schedule.ChannelPolicy, playsInternally bool) bool {
	return playsInternally && (pol.MidRoll == nil || *pol.MidRoll)
}

// MidRollFreezeHorizon is how far ahead of `now` a programme's accepted split is frozen: whatever is
// on air, or starts within it, keeps the parts the encoder and the guide were given, and a new
// fade or a flipped switch applies to later airings only. Longer than the default reconcile
// interval (10 min), so a programme cannot go from "later" to "on air" between two passes
// without being pinned by one of them.
const MidRollFreezeHorizon = 30 * time.Minute

// pinnedCutsAt is the split every programme on air or about to be must keep (schedule.PinnedCuts),
// read from the accepted Desired cycle with the encoder's own arithmetic. Nil before a channel has
// an anchored cycle, or on Tunarr, which never splits.
func pinnedCutsAt(ch channelstore.Channel, playsInternally bool, at time.Time) map[string][]int64 {
	if !playsInternally || ch.PlayoutAnchor.IsZero() || len(ch.Desired) == 0 {
		return nil
	}
	return playout.CommittedSplits(ch.Desired, ch.PlayoutAnchor, at, at.Add(MidRollFreezeHorizon))
}

// naturalBreaksFor is the scene-fade source for one scheduling pass, or nil when the channel does
// not break mid-programme.
func (e *Engine) naturalBreaksFor(ctx context.Context, pol schedule.ChannelPolicy, playsInternally bool) schedule.NaturalBreakSource {
	if e.naturalBreaks == nil || !MidRollFor(pol, playsInternally) {
		return nil
	}
	return e.naturalBreaks(ctx)
}

// BreakDurationFor resolves the per-channel break length against the live global setting, through
// pod.ResolveBreakDurationMs — the same rule the pod is assembled to. An invalid (zero or
// sub-30s) value falls to the next level, ending at the documented 2m default; disabling belongs
// exclusively to BreaksPerHour.
func BreakDurationFor(pol schedule.ChannelPolicy, global time.Duration) time.Duration {
	var channelMs int64
	if pol.BreakDuration != nil {
		channelMs = pol.BreakDuration.Std().Milliseconds()
	}
	return time.Duration(pod.ResolveBreakDurationMs(channelMs, global.Milliseconds())) * time.Millisecond
}

// SelectionFrom is the ONE place a filler selection becomes a domain Selection, scope era and all.
//
// ⚠ **It is exported so every derivation (saved channel, draft, pod preview) shares one rule.**
// Parallel copies drift, and two copies can cancel each other's mistakes so nothing looks wrong
// until "explicitly any era" is chosen: a fallback keyed on `Era == 0` cannot tell an unset era
// from a chosen one, so it would overwrite the operator's answer with the channel's.
//
// `derived` is what the channel's programming implies (schedule.DeriveFiller). It fills ONLY what
// the operator left unset — an explicit audience or era, including an explicit "any" era, is never
// overridden — and is applied live so channels created before it existed benefit too.
func SelectionFrom(f *schedule.FillerSelection, scope schedule.ScopePolicy, derived schedule.DerivedFiller) pod.Selection {
	sel := pod.Selection{}
	inheritEra := true
	if f != nil {
		sel.Audience = clips.Audience(f.Audience)
		sel.Categories = append([]string(nil), f.Categories...)
		sel.Roles = append([]string(nil), f.Roles...)
		sel.Pinned = append([]string(nil), f.Pinned...)
		sel.Excluded = append([]string(nil), f.Excluded...)
		if f.Geography != nil {
			sel.Geography = pod.Geography{Country: f.Geography.Country, Place: f.Geography.Place}.Normalize()
		}
		sel.Language = language.Normalize(f.Language)
		if f.Era != nil {
			// ⚠ **PRESENCE is the opt-in, and that is what makes "any era" reachable
			// (§10, V51f).** A present range means the operator has ANSWERED the era question —
			// including answering "any" with `{0,0}`. Only absence inherits. Keying the scope
			// default off `sel.Era == 0` would re-inherit on the very next derivation, leaving a
			// channel with a programming era no way to say "draw from the whole catalog". Same
			// pattern as `AutoCurate`, for the same reason.
			inheritEra = false
			sel.Era = pod.EraRange{From: f.Era.From, To: f.Era.To}
		} else if len(f.EraWindows) > 0 {
			inheritEra = false
			sel.EraWindows = rangesToFiller(f.EraWindows)
		}
	}
	// The "seed filler era from scope dates" default, applied live rather than only stamped at
	// create — so an existing channel benefits, and a channel whose scope later changes follows.
	if inheritEra {
		if era, windows := scope.FillerEra(); era != nil {
			sel.Era = pod.EraRange{From: era.From, To: era.To}
		} else if len(windows) > 0 {
			sel.EraWindows = rangesToFiller(windows)
		} else if derived.Era != nil {
			sel.Era = pod.EraRange{From: derived.Era.From, To: derived.Era.To}
		}
	}
	// ⚠ An empty audience is "operator said nothing" — the string has no explicit-any value, and
	// the seed always writes one — so it derives. A kids ceiling then reaches the ladder's
	// fail-closed audience rules instead of the open general pool.
	if sel.Audience == "" {
		sel.Audience = clips.Audience(derived.Audience)
	}
	return sel
}

func rangesToFiller(ranges []schedule.Range) []pod.EraRange {
	out := make([]pod.EraRange, len(ranges))
	for i, r := range ranges {
		out[i] = pod.EraRange{From: r.From, To: r.To}
	}
	return pod.NormalizeEraWindows(out)
}

// PodSeed derives a deterministic pod seed from the channel id (§10 seeded-
// deterministic — same channel rebuilds the same clip pool, so the filler-list
// attach is idempotent across reconciles). Exported for the same reason as PodEra:
// preview must seed from the identical value or it previews a different pod.
func PodSeed(channelID string) int64 {
	var h int64 = 1469598103934665603 // FNV-1a offset basis
	for _, b := range []byte(channelID) {
		h ^= int64(b)
		h *= 1099511628211
	}
	return h
}

// PodSeedAt derives the seed for the break STARTING AT a specific instant — what
// pod.Window has always documented ("channel + window start") and what real television
// does: consecutive breaks do not replay the same three adverts.
//
// PodSeed (channel-only) remains for the two callers that legitimately want ONE pool per
// channel rather than one pod per break:
//
//   - BuildFillerList attaches a Tunarr filler-list to the channel, and Tunarr picks from
//     that pool itself. There is no break to seed from at attach time.
//   - HasPool only asks "is there anything to play at all".
//
// Internal playout and the guide, which both resolve A SPECIFIC BREAK, use this instead. The
// two must agree — the hover card promises the clips that will actually air — and they do,
// because both derive the seed from the same (channel, break start).
func PodSeedAt(channelID string, breakStartMs int64) int64 {
	h := PodSeed(channelID)
	// Mix the start in with the same FNV-1a step, byte by byte, so nearby breaks land far
	// apart in the sequence rather than producing near-identical pods.
	for i := 0; i < 8; i++ {
		h ^= (breakStartMs >> (8 * i)) & 0xff
		h *= 1099511628211
	}
	return h
}
