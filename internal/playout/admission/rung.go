package admission

import (
	"errors"
	"strconv"
)

// ErrAtCapacity reports that a new channel packager would exceed the admission budget.
//
// A distinct error because the API must render it as 503 with an actionable message, not as
// a generic failure: the operator's fixes (wait for a slot, relax a lowering-only safety cap,
// or choose a lower quality tier) are only discoverable if we say which wall was hit.
var ErrAtCapacity = errors.New("playout: at channel capacity")

// SoftwareRung is one rung of the software ladder (playout's softwareladder.go). The zero value is
// full quality.
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

// RungCosts is each rung's CPU at 1x relative to rung 0 for one class of source. The step-up
// threshold is their ratio (RungMonitor), and playout's StartRung projects with them. Once the
// ResourceBudget's class probe measures the rungs per host (#1520), its ratios replace these.
type RungCosts [RungKeyframes + 1]float64

// Measured ratios, each rounded toward the conservative side for a step up (a cheap current rung, an
// expensive next one), so a projection never promises headroom the next rung does not have. Rung
// 2's gain depends on the source's GOP (none on phase 0b's H1, −60% on H2), so its prior is rung 1's
// and the monitor measures it live.
var (
	// HeavyRungCosts: 4K HDR tone-mapped at 720 lines. Rung 1: phase 0b 3.48/3.66, 3.23/3.44
	// (the #1517 live runs on a loaded host scattered ±40% around it). Rung 3: phase 0b 0.33/3.66,
	// 0.32/3.44; #1517 live 0.73/4.89, 0.57/7.07.
	HeavyRungCosts = RungCosts{RungFull: 1, RungLight: 0.95, RungNoRef: 0.95, RungKeyframes: 0.12}
	// SDRRungCosts: SDR up to 1080p, at its own size on rungs 1–2. Rung 1 keeps the phase 0b
	// loop-filter saving (4–7%); rung 3: #1517 live 0.38/1.74 cores on 1080p HEVC 10-bit (the 3-CPU
	// run's 0.92/1.42 is the other end; the lower one is the conservative ratio).
	SDRRungCosts = RungCosts{RungFull: 1, RungLight: 0.95, RungNoRef: 0.95, RungKeyframes: 0.22}
)

// MinStartSpeed is the realtime multiple a start rung must be measured or projected to reach alone
// (the budget's per-stream bar, phase 0). It is also the monitor's step-up margin over the next
// rung's cost ratio, against 0.97 for a step down: the hysteresis between the two.
const MinStartSpeed = 1.2
