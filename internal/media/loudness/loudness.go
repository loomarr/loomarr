// Package loudness is the one rule for correcting a filler clip's level (#2032). Playout applies
// the correction as a static gain at air time; ingest uses the same rule to refuse a clip that
// needs more boost than it allows. Keeping both on one function keeps the floor and the air-time
// gain on the same boost limit.
package loudness

import "math"

// TruePeakCeilingDBTP is the highest true peak a corrected clip may reach. A boost never pushes
// a clip's measured peak past it, which is what keeps a raised quiet clip from clipping.
const TruePeakCeilingDBTP = -1.0

// MaxBoostDB is the most a quiet clip is raised. A clip needing more is refused at ingest
// (too_quiet): the boost would raise its hiss and tape noise along with the programme, which is
// the picture-equivalent of brightening a dim VHS transfer. Starting value; calibrated in #2034.
const MaxBoostDB = 6.0

// MaxCutDB bounds how far a loud clip is lowered. Cuts carry no noise penalty, so this is only a
// guard against a broken measurement muting a clip.
const MaxCutDB = 20.0

// Correction is the static gain, in dB, that moves a clip measured at measuredLUFS toward
// targetLUFS without its true peak exceeding TruePeakCeilingDBTP. truePeakDBTP is nil when the
// peak was not measured. reachable reports whether the clip can reach the target within
// MaxBoostDB; a clip that cannot is refused at ingest rather than aired quiet. reachable ignores
// the peak ceiling: it is about boost raising noise, and a clip the ceiling holds short airs a
// little quiet rather than clipped.
func Correction(targetLUFS, measuredLUFS float64, truePeakDBTP *float64) (gainDB float64, reachable bool) {
	need := targetLUFS - measuredLUFS
	gain := math.Max(-MaxCutDB, math.Min(MaxBoostDB, need))
	if truePeakDBTP != nil && gain > 0 {
		gain = math.Min(gain, math.Max(0, TruePeakCeilingDBTP-*truePeakDBTP))
	}
	return gain, need <= MaxBoostDB
}
