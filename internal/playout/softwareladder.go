package playout

import "strconv"

// The software degradation ladder (#1517). On a CPU-only host a title too heavy for the CPU never
// refuses, never shows a message and never stalls: the item's encoder steps down a quality ladder
// by measured speed (RungMonitor) and back up with headroom. Audio never degrades, and the output
// geometry, SAR, pixel format and cadence stay the channel's on every rung (pad/scale up, repeated
// frames), so the packager never sees a format change across a rung restart.
//
// The rungs are the maintainer's Decision 3 (project/SPIKE-4k-tonemap.md), on the phase 0b
// measurements (4 CPUs, no GPU, 4K HEVC 10-bit HDR, tone-mapped):
//
//	rung 0  full: every frame decoded; HDR tone-maps at no more than 720 lines   3.4–3.7 cores at 1x
//	rung 1  720-line working size, -skip_loop_filter all                         −4 to −7%
//	rung 2  rung 1 + -skip_frame noref: only where the source measurably gains    none (H1) to −60% (H2)
//	rung 3  keyframes only (-skip_frame nokey) at a 480-line working size         0.33–0.41 cores
//
// A 4K decode alone costs 2.3–2.6 cores, so only rung 3 reliably fits a 4-CPU host. Software hosts
// never produce a premium (4K) format; that is the premium format's own admission.

// SoftwareRung is one rung of the software ladder. The zero value is full quality.
type SoftwareRung int

const (
	RungFull SoftwareRung = iota
	RungLight
	RungNoRef
	RungKeyframes
)

// SoftwareRungs is the ladder, best first.
var SoftwareRungs = []SoftwareRung{RungFull, RungLight, RungNoRef, RungKeyframes}

func (r SoftwareRung) String() string {
	switch r {
	case RungFull:
		return "rung0-full"
	case RungLight:
		return "rung1-light"
	case RungNoRef:
		return "rung2-noref"
	case RungKeyframes:
		return "rung3-keyframes"
	}
	return "rung" + strconv.Itoa(int(r))
}

// workingLines is the height the rung decodes into before the scale to the channel geometry; 0
// means the channel's own height.
//
// Rungs 1–2 downscale only a heavy source (HDR or above 1080p), where the tone-map and the scale
// cost follow the pixel count. An SDR source up to 1080p takes their decoder shortcuts at its own
// size: a 1080→720→1080 round trip costs more than it saves (supervisor decision on the live
// numbers, #1517: rung 1 ran 1.38x against rung 0's 2.11x on a 1080p HEVC title).
func (r SoftwareRung) workingLines(heavy bool) int {
	switch r {
	case RungLight, RungNoRef:
		if !heavy {
			return 0
		}
		return 720
	case RungKeyframes:
		return 480
	}
	return 0
}

// decoderOptions are the rung's decoder shortcuts: input options, so they precede -i. The :v
// specifier scopes them to the video decoder, so the audio is decoded in full on every rung.
func (r SoftwareRung) decoderOptions() []string {
	switch r {
	case RungLight:
		return []string{"-skip_loop_filter:v", "all"}
	case RungNoRef:
		// "noref" is ffmpeg's spelling; "nonref" is rejected by the option parser (phase 0b).
		return []string{"-skip_loop_filter:v", "all", "-skip_frame:v", "noref"}
	case RungKeyframes:
		return []string{"-skip_loop_filter:v", "all", "-skip_frame:v", "nokey"}
	}
	return nil
}

// tailFill holds the last decoded frame to the item's end on a rung that skips frames. fps repeats
// frames only up to the last one decoded, so keyframes-only ends the video up to one source GOP
// before the audio (measured: 1 frame out of 50 for a 2 s single-GOP item). The clone covers GOPs
// up to 10 s; the item's own bound (-frames:v, -t) trims the surplus.
func (r SoftwareRung) tailFill() string {
	if r == RungNoRef || r == RungKeyframes {
		return "tpad=stop_mode=clone:stop_duration=10"
	}
	return ""
}

// heavySource is a source whose picture cost follows its pixel count on the CPU: HDR (tone-mapped)
// or above 1080p. Unknown geometry counts as not heavy.
func heavySource(src MediaFormat) bool {
	return src.HDR() || src.Height > 1080 || src.Width > 1920
}

// RungCosts is each rung's CPU at 1x relative to rung 0 for one class of source. The step-up
// threshold is their ratio (RungMonitor), and StartRung projects with them. Once the ResourceBudget's
// class probe measures the rungs per host (#1520), its ratios replace these.
type RungCosts [RungKeyframes + 1]float64

// Measured ratios, each rounded toward the conservative side for a step up (a cheap current rung, an
// expensive next one), so a projection never promises headroom the next rung does not have. Rung
// 2's gain depends on the source's GOP (none on phase 0b's H1, −60% on H2), so its prior is rung 1's
// and the monitor measures it live.
var (
	// heavyRungCosts: 4K HDR tone-mapped at 720 lines. Rung 1: phase 0b 3.48/3.66, 3.23/3.44
	// (the #1517 live runs on a loaded host scattered ±40% around it). Rung 3: phase 0b 0.33/3.66,
	// 0.32/3.44; #1517 live 0.73/4.89, 0.57/7.07.
	heavyRungCosts = RungCosts{RungFull: 1, RungLight: 0.95, RungNoRef: 0.95, RungKeyframes: 0.12}
	// sdrRungCosts: SDR up to 1080p, at its own size on rungs 1–2. Rung 1 keeps the phase 0b
	// loop-filter saving (4–7%); rung 3: #1517 live 0.38/1.74 cores on 1080p HEVC 10-bit (the 3-CPU
	// run's 0.92/1.42 is the other end; the lower one is the conservative ratio).
	sdrRungCosts = RungCosts{RungFull: 1, RungLight: 0.95, RungNoRef: 0.95, RungKeyframes: 0.22}
)

// RungCostsFor is the rung cost table for a source's class.
func RungCostsFor(src MediaFormat) RungCosts {
	if heavySource(src) {
		return heavyRungCosts
	}
	return sdrRungCosts
}

// minStartSpeed is the realtime multiple a start rung must be measured or projected to reach alone
// (the budget's per-stream bar, phase 0). It is also the monitor's step-up margin over the next
// rung's cost ratio, against 0.97 for a step down: the hysteresis between the two.
const minStartSpeed = 1.2

// RungCost is the measured cost of one stream of a class on this software host at rung 0, alone
// (the ResourceBudget's ClassCost for the class at the channel's output height). The zero value is
// unmeasured.
type RungCost struct {
	Speed    float64
	CPUCores float64
}

// StartRung is the rung a new item starts on: the best rung whose projected speed reaches 1.2x.
// Rung 2 is never chosen here: its gain is source-dependent, so it cannot be promised in advance.
//
// Unmeasured, SDR up to 1080p starts at full quality, and anything 4K or HDR (phase 0b: 2.3–3.7
// cores at 1x on 4 CPUs) starts on keyframes-only, the one rung that always fits, and steps up
// when the monitor sees headroom.
func StartRung(src MediaFormat, cost RungCost) SoftwareRung {
	if cost.Speed <= 0 {
		if heavySource(src) {
			return RungKeyframes
		}
		return RungFull
	}
	costs := RungCostsFor(src)
	for _, r := range []SoftwareRung{RungFull, RungLight} {
		if cost.Speed/costs[r] >= minStartSpeed {
			return r
		}
	}
	return RungKeyframes
}
