package schedule

import (
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
)

// Mid-roll: commercial breaks INSIDE a long programme, placed only at natural scene fades (§10
// "Break placement", #1512 plan addition 1). Loomarr measures each source once for places where
// black video and silent audio coincide (inventory.Analysis.Breaks, beta.8 G7); this file decides
// which of them become breaks. It is pure: the caller supplies the candidates.
//
// The rules are the maintainer's, fixed:
//   - on by default for internal playout; a channel may switch it off (ChannelPolicy.MidRoll);
//   - the breaks-per-hour cadence applies ACROSS mid-roll and between-programme breaks;
//   - never mid-scene: a due break with no measured fade near it is SKIPPED, never forced.

// BreakCandidate is one measured natural break in a source: AtMs is the middle of a black-and-silent
// span, so cutting there shows no picture and loses no dialogue. Confidence is in (0, 1].
type BreakCandidate struct {
	AtMs       int64
	Confidence float64
}

// NaturalBreakSource answers "where are the natural breaks in this library item?" for the
// scheduler. Nil candidates means unmeasured (or nothing found): the programme airs whole.
type NaturalBreakSource interface {
	NaturalBreaks(libraryItemID string) []BreakCandidate
}

// MidRollPolicy bounds where a mid-roll break may go. The defaults (DefaultMidRollPolicy) are the
// proposal justified in #1512's natural-breaks PR; they live here, not in settings, because they
// describe what a watchable break is rather than a household preference — the household's dials
// are breaks-per-hour, break length and the per-channel switch.
type MidRollPolicy struct {
	// IntervalMs is the runtime between breaks: 60 min / breaks-per-hour (15 min at the default 4).
	IntervalMs int64
	// MinProgrammeMs: shorter programmes are never split. A half-hour sitcom (≈22 min) airs whole
	// and gets today's between-programme break; an hour-long drama (≈42 min) and every film split.
	MinProgrammeMs int64
	// ToleranceMs is how far from its due point a fade may be and still take the break. It IS the
	// G7 search window (inventory.BreakSearchHalfWindow, one shared constant), so every fade the
	// measurement looked for is usable and nothing it did not look for is assumed.
	ToleranceMs int64
	// MinPartMs is the shortest part a cut may create — from the programme start, from the previous
	// cut, and to the programme end. A fade in the cold open or the closing credits never becomes a
	// break: the between-programme break already sits there.
	MinPartMs int64
	// MinConfidence drops the faintest coincidences (a sub-quarter-second dip is as likely a hard
	// cut inside a scene as a fade between scenes).
	MinConfidence float64
}

// DefaultMidRollPolicy is the placement for a channel breaking breaksPerHour times an hour; the
// zero policy (no mid-roll) when breaksPerHour is not positive.
func DefaultMidRollPolicy(breaksPerHour int) MidRollPolicy {
	if breaksPerHour <= 0 {
		return MidRollPolicy{}
	}
	return MidRollPolicy{
		IntervalMs:     time.Hour.Milliseconds() / int64(breaksPerHour),
		MinProgrammeMs: (40 * time.Minute).Milliseconds(),
		ToleranceMs:    inventory.BreakSearchHalfWindow.Milliseconds(),
		MinPartMs:      (8 * time.Minute).Milliseconds(),
		MinConfidence:  0.25,
	}
}

// PlaceMidRollCuts returns the source times (ms, ascending) at which a programme of durationMs is
// cut for a break. sinceBreakMs is the programme runtime already aired since the channel's last
// break when this programme begins, so the cadence runs across programme boundaries rather than
// restarting at every title.
//
// Each break falls due IntervalMs after the previous one; the first is pulled no earlier than
// MinPartMs into the programme. A due break takes the usable candidate nearest its due point
// within ±ToleranceMs (ties: higher confidence, then earlier). With none, that break is skipped and
// the next falls due IntervalMs later — never forced into a scene.
func PlaceMidRollCuts(durationMs, sinceBreakMs int64, candidates []BreakCandidate, p MidRollPolicy) []int64 {
	if p.IntervalMs <= 0 || durationMs < p.MinProgrammeMs || len(candidates) == 0 {
		return nil
	}
	var cuts []int64
	last := int64(0) // the previous cut, or the programme start
	due := max(p.IntervalMs-sinceBreakMs, p.MinPartMs)
	for due-p.ToleranceMs <= durationMs-p.MinPartMs {
		best, found := BreakCandidate{}, false
		for _, c := range candidates {
			if c.Confidence < p.MinConfidence || c.AtMs < due-p.ToleranceMs || c.AtMs > due+p.ToleranceMs ||
				c.AtMs-last < p.MinPartMs || durationMs-c.AtMs < p.MinPartMs {
				continue
			}
			if !found || closerCandidate(c, best, due) {
				best, found = c, true
			}
		}
		if !found {
			due += p.IntervalMs
			continue
		}
		cuts = append(cuts, best.AtMs)
		last = best.AtMs
		due = best.AtMs + p.IntervalMs
	}
	return cuts
}

// validCuts keeps a pinned split only while it still fits the programme: strictly ascending cuts
// inside (0, durationMs). A source replaced under the pin (a different runtime) airs whole rather
// than being cut at times measured on other bytes.
func validCuts(cuts []int64, durationMs int64) []int64 {
	prev := int64(0)
	for _, c := range cuts {
		if c <= prev || c >= durationMs {
			return nil
		}
		prev = c
	}
	return cuts
}

func closerCandidate(c, best BreakCandidate, due int64) bool {
	dc, db := absMs(c.AtMs-due), absMs(best.AtMs-due)
	if dc != db {
		return dc < db
	}
	if c.Confidence != best.Confidence {
		return c.Confidence > best.Confidence
	}
	return c.AtMs < best.AtMs
}

func absMs(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
