package playout

import "time"

// RungMonitor decides when a software item encoder steps down or up the degradation ladder
// (softwareladder.go, #1517), from the encoder's own -progress stream. It only decides; it never
// touches a process.
//
// # Contract for the caller (the phase 2 channel packager)
//
// Feed one SpeedSample per -progress block (ReadProgress) of the item encoder, with the wall clock
// of the block and, when the platform can read it, the encoder process's CPU time (user+sys). When
// Observe returns a Step, the monitor has already moved to the new rung. The caller then:
//
//  1. stops the item encoder and discards anything it writes past the cut: a stopped encoder
//     flushes, and on rungs 2–3 the flush includes the tail fill (up to 10 s of repeated frames,
//     measured 10.2 s on a real 4K title), which must not reach the channel;
//  2. restarts it at the item position the channel has reached (the packager's own clock, not the
//     encoder's out_time) with ProgramSpec.SoftwareRung set to the decision's rung. Every rung
//     produces identical output parameters, so the packager splices it like any other restart;
//  3. keeps feeding samples from the new process, whose out_time and CPU start again from zero.
//
// One monitor lives for one item on one channel: a new item starts a new monitor at StartRung.
//
// # Signals
//
//   - Speed is out_time advanced over wall time in a short window, not ffmpeg's speed= (a
//     cumulative average since start, which reacts minutes late). Down: below DownBelow
//     continuously for DownFor, or DownLag of accumulated lag, which catches a slow drift (0.98x)
//     that never looks slow in any window but stalls the packager eventually.
//   - Headroom needs CPU time. The live chain paces its input (-readrate 1.0), so a healthy
//     encoder's speed sits at 1.0x whatever its headroom. With CPU time the monitor estimates the
//     speed the CPU allowance would allow (speed × allowance / cores used). The headroom threshold
//     is the next rung's measured cost ratio (Costs: rung 1 costs ~8x keyframes-only on 4K HDR,
//     ~4.3x on 1080p SDR) times UpMargin (1.2x, against 0.97x down: the hysteresis), held for UpFor
//     (60 s, the minimum dwell). Without CPU time it never steps up: it holds rather than guesses.
//   - Rung 2 (noref) is kept only where it measurably gains: leaving it, the monitor compares its
//     capacity with rung 1's, and a gain under NoRefGain makes the ladder skip it from then on.
//   - A step up that is reversed within twice its wait doubles the wait (up to 8x), so a rung that
//     does not fit is not retried every minute.
type RungMonitor struct {
	cfg  RungMonitorConfig
	rung SoftwareRung

	upWait time.Duration // current UpFor, doubled by each reversed step up
	upAt   time.Time     // when the last step up happened (zero: none)

	// noRef: 0 unknown, 1 gains, -1 does not gain. lightCap is rung 1's mean capacity when it
	// stepped down to rung 2.
	noRef    int
	lightCap float64
	noRefCap float64 // rung 2's measured cost relative to rung 0, once known

	dwell dwell
}

// RungMonitorConfig tunes the monitor; zero fields take the defaults below.
type RungMonitorConfig struct {
	// CPUAllowance is the cores this item's encoder may use (the budget's per-stream share); 0 means
	// unknown, which disables stepping up.
	CPUAllowance float64
	Settle       time.Duration // ignored after each (re)start: probe, first GOP, readrate burst start
	Window       time.Duration // the short speed window
	DownBelow    float64
	DownFor      time.Duration
	DownLag      time.Duration
	// Costs are the rung cost ratios of this item's class (RungCostsFor); zero takes the 4K HDR
	// table, whose step-up bar is the higher one.
	Costs     RungCosts
	UpMargin  float64
	UpFor     time.Duration
	NoRefGain float64
	// Reprice, when set, is offered every step before it is taken (Lease.StepSoftware): a step down
	// always applies; a step up it refuses does not happen, and backs off like a reversed step.
	Reprice func(to SoftwareRung) bool
}

// Defaults. DownBelow sits just under 1.0 because a paced encoder's windowed speed wobbles around
// 1.0x; the lag rule covers what the margin lets through. UpMargin is the budget's per-stream 1.2x
// bar; UpFor is the minimum dwell, long enough that a scene change does not trigger a restart.
const (
	defaultSettle    = 3 * time.Second
	defaultWindow    = 4 * time.Second
	defaultDownBelow = 0.97
	defaultDownFor   = 8 * time.Second
	defaultDownLag   = 3 * time.Second
	defaultUpMargin  = minStartSpeed
	defaultUpFor     = 60 * time.Second
	defaultNoRefGain = 1.15
	maxUpBackoff     = 8
)

// SpeedSample is one -progress block of the item encoder.
type SpeedSample struct {
	At      time.Time     // wall clock when the block arrived
	OutTime time.Duration // Progress.OutTimeMS: media produced by this encoder process
	CPU     time.Duration // this encoder process's CPU time; 0 when unknown
}

// RungDecision is what Observe asks of the caller.
type RungDecision struct {
	// Step is true when the caller must restart the encoder on Rung.
	Step bool
	Rung SoftwareRung
	// Reason is for the log line.
	Reason string
}

// dwell is the state of one encoder process on one rung.
type dwell struct {
	started     time.Time
	settled     bool
	base        SpeedSample // the first sample after Settle: the lag origin
	window      []SpeedSample
	belowSince  time.Time
	aboveSince  time.Time
	capSum      float64
	capN        int
	initialized bool
}

// NewRungMonitor starts a monitor for an item encoder running on start.
func NewRungMonitor(start SoftwareRung, cfg RungMonitorConfig) *RungMonitor {
	def := func(v *time.Duration, d time.Duration) {
		if *v <= 0 {
			*v = d
		}
	}
	deff := func(v *float64, d float64) {
		if *v <= 0 {
			*v = d
		}
	}
	def(&cfg.Settle, defaultSettle)
	def(&cfg.Window, defaultWindow)
	def(&cfg.DownFor, defaultDownFor)
	def(&cfg.DownLag, defaultDownLag)
	def(&cfg.UpFor, defaultUpFor)
	deff(&cfg.DownBelow, defaultDownBelow)
	deff(&cfg.UpMargin, defaultUpMargin)
	if cfg.Costs[RungFull] == 0 {
		cfg.Costs = heavyRungCosts
	}
	deff(&cfg.NoRefGain, defaultNoRefGain)
	return &RungMonitor{cfg: cfg, rung: start, upWait: cfg.UpFor}
}

// Rung is the rung the encoder should be running on now.
func (m *RungMonitor) Rung() SoftwareRung { return m.rung }

// Observe takes one sample and returns the decision.
func (m *RungMonitor) Observe(s SpeedSample) RungDecision {
	d := &m.dwell
	if !d.initialized {
		*d = dwell{started: s.At, initialized: true}
	}
	if !d.settled {
		if s.At.Sub(d.started) < m.cfg.Settle {
			return RungDecision{Rung: m.rung}
		}
		d.settled, d.base = true, s
	}
	d.window = append(d.window, s)
	for len(d.window) > 1 && s.At.Sub(d.window[1].At) >= m.cfg.Window {
		d.window = d.window[1:]
	}
	first := d.window[0]
	wall := s.At.Sub(first.At)
	if wall < m.cfg.Window {
		return RungDecision{Rung: m.rung}
	}
	speed := float64(s.OutTime-first.OutTime) / float64(wall)
	capacity := speed
	if m.cfg.CPUAllowance > 0 && s.CPU > first.CPU && first.CPU > 0 {
		cores := float64(s.CPU-first.CPU) / float64(wall)
		capacity = max(speed, speed*m.cfg.CPUAllowance/cores)
	}
	d.capSum += capacity
	d.capN++
	if m.rung == RungNoRef && m.lightCap > 0 {
		// Rung 2's cost is measured while it runs: projecting it up to rung 1 must use the gain this
		// source actually showed, not the prior.
		m.noRefCap = m.cfg.Costs[RungLight] * m.lightCap / (d.capSum / float64(d.capN))
	}

	lag := s.At.Sub(d.base.At) - (s.OutTime - d.base.OutTime)
	if speed < m.cfg.DownBelow {
		if d.belowSince.IsZero() {
			d.belowSince = s.At
		}
	} else {
		d.belowSince = time.Time{}
	}
	if down, ok := m.below(); ok {
		switch {
		case !d.belowSince.IsZero() && s.At.Sub(d.belowSince) >= m.cfg.DownFor:
			return m.step(s, down, "below realtime")
		case lag >= m.cfg.DownLag:
			return m.step(s, down, "fell behind")
		}
	}

	up, ok := m.above()
	if !ok || m.cfg.CPUAllowance <= 0 || s.CPU <= 0 {
		return RungDecision{Rung: m.rung}
	}
	if capacity >= m.cfg.UpMargin*m.cost(up)/m.cost(m.rung) && speed >= m.cfg.DownBelow {
		if d.aboveSince.IsZero() {
			d.aboveSince = s.At
		}
		if s.At.Sub(d.aboveSince) >= m.upWait {
			if m.cfg.Reprice != nil && !m.cfg.Reprice(up) {
				// The CPU is headroom this encoder has but the ledger has promised elsewhere.
				d.aboveSince = time.Time{}
				m.upWait = min(2*m.upWait, maxUpBackoff*m.cfg.UpFor)
				return RungDecision{Rung: m.rung}
			}
			m.upAt = s.At
			return m.step(s, up, "headroom")
		}
	} else {
		d.aboveSince = time.Time{}
	}
	return RungDecision{Rung: m.rung}
}

// below is the next rung down: rung 2 is skipped once it has proved not to gain.
func (m *RungMonitor) below() (SoftwareRung, bool) {
	switch m.rung {
	case RungFull:
		return RungLight, true
	case RungLight:
		if m.noRef < 0 {
			return RungKeyframes, true
		}
		return RungNoRef, true
	case RungNoRef:
		return RungKeyframes, true
	}
	return m.rung, false
}

// above is the next rung up: from keyframes-only, rung 2 only where it has proved to gain.
func (m *RungMonitor) above() (SoftwareRung, bool) {
	switch m.rung {
	case RungKeyframes:
		if m.noRef > 0 {
			return RungNoRef, true
		}
		return RungLight, true
	case RungNoRef:
		return RungLight, true
	case RungLight:
		return RungFull, true
	}
	return m.rung, false
}

// cost is a rung's CPU relative to rung 0: the phase 0b prior, or rung 2's measured one.
func (m *RungMonitor) cost(r SoftwareRung) float64 {
	if r == RungNoRef && m.noRefCap > 0 {
		return m.noRefCap
	}
	return m.cfg.Costs[r]
}

func (m *RungMonitor) step(s SpeedSample, to SoftwareRung, reason string) RungDecision {
	d := &m.dwell
	mean := d.capSum / float64(max(d.capN, 1))
	switch {
	case m.rung == RungLight && to == RungNoRef:
		m.lightCap = mean
	case m.rung == RungNoRef && m.lightCap > 0:
		if mean >= m.lightCap*m.cfg.NoRefGain {
			m.noRef, m.noRefCap = 1, m.cfg.Costs[RungLight]*m.lightCap/mean
		} else {
			m.noRef = -1
		}
	}
	if to > m.rung { // a step down
		if m.cfg.Reprice != nil {
			m.cfg.Reprice(to) // releases CPU; never refused
		}
		if !m.upAt.IsZero() && s.At.Sub(m.upAt) < 2*m.upWait {
			// The rung stepped up to did not fit: wait longer before trying again.
			m.upWait = min(2*m.upWait, maxUpBackoff*m.cfg.UpFor)
		}
		m.upAt = time.Time{}
	}
	m.rung = to
	m.dwell = dwell{}
	return RungDecision{Step: true, Rung: to, Reason: reason}
}
