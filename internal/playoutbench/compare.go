package playoutbench

import (
	"fmt"
	"math"
	"strings"
)

// Delta statuses.
const (
	StatusOK         = "ok"
	StatusImproved   = "improved"
	StatusRegression = "REGRESSION"
	StatusMissing    = "REGRESSION (missing)"
	StatusSkipped    = "skipped"
	StatusNew        = "new"
)

// Tolerance is how far a metric may drift from the accepted baseline before it is a regression. A
// change must exceed BOTH the relative band and the per-unit absolute slack: a 15 ms move on a 40 ms
// start is +37% but one scheduler tick, and must not fail a build.
type Tolerance struct {
	Relative float64
	Slack    map[string]float64
}

// DefaultTolerance is 15% with slack sized to each unit's timer and rusage granularity. Shared CI
// runners are noisy; the committed baselines come from supervised runs on quiet hardware.
func DefaultTolerance() Tolerance {
	return Tolerance{Relative: 0.15, Slack: map[string]float64{"ms": 25, "x": 0.3, "cores": 0.01, "LU": 0.2, "vmaf": 1, "streams": 0}}
}

// Delta is one metric's movement against the baseline.
type Delta struct {
	Metric            string
	Baseline, Current float64
	Change            float64 // relative to the baseline; absolute when the baseline is 0
	Status            string
}

// Compare diffs a fresh report against the accepted baseline of the same family. regressed is true
// when any metric moved beyond tolerance in its worse direction, changed at all if Exact, or vanished
// without the host declaring it skipped.
func Compare(base, cur *Report, tol Tolerance) (deltas []Delta, regressed bool, err error) {
	if base.Family != cur.Family {
		return nil, false, fmt.Errorf("baseline is %q, report is %q: numbers from different hardware families are not comparable", base.Family, cur.Family)
	}
	for _, name := range base.Names() {
		b := base.Metrics[name]
		c, ok := cur.Metrics[name]
		if !ok {
			d := Delta{Metric: name, Baseline: b.Value, Status: StatusMissing}
			if _, skipped := cur.Skipped[name]; skipped {
				d.Status = StatusSkipped
			} else {
				regressed = true
			}
			deltas = append(deltas, d)
			continue
		}
		d := Delta{Metric: name, Baseline: b.Value, Current: c.Value, Change: change(b.Value, c.Value)}
		d.Status = classify(b, c, tol)
		if d.Status == StatusRegression {
			regressed = true
		}
		deltas = append(deltas, d)
	}
	for _, name := range cur.Names() {
		if _, ok := base.Metrics[name]; !ok {
			deltas = append(deltas, Delta{Metric: name, Current: cur.Metrics[name].Value, Status: StatusNew})
		}
	}
	return deltas, regressed, nil
}

func change(base, cur float64) float64 {
	if base == 0 {
		return cur
	}
	return (cur - base) / math.Abs(base)
}

func classify(b, c Metric, tol Tolerance) string {
	diff := c.Value - b.Value
	if b.Better == Exact {
		if diff != 0 {
			return StatusRegression
		}
		return StatusOK
	}
	worse := diff // Lower is better: growth is worse.
	if b.Better == Higher {
		worse = -diff
	}
	if worse <= 0 {
		if worse < 0 {
			return StatusImproved
		}
		return StatusOK
	}
	limit := math.Abs(b.Value) * tol.Relative
	if worse > limit && worse > tol.Slack[b.Unit] {
		return StatusRegression
	}
	return StatusOK
}

// Mode picks which thresholds bind. Correctness mode is for hosts whose speed is not the product's
// (a virtualised macOS runner): they must still not tear, drift or change the SPS.
type Mode string

const (
	ModeFull        Mode = "full"
	ModeCorrectness Mode = "correctness"
)

// Verdict is one threshold's outcome.
type Verdict struct {
	Metric   string
	Limit    string
	Measured string
	Pass     bool
}

type threshold struct {
	metric      string
	max         bool // limit is an upper bound; otherwise a lower bound
	limit       float64
	correctness bool
}

// The beta.8 thresholds (#1512, phase 0 checkpoint 1 and the maintainer's revisions). Every
// number is the issue's; none is derived from a bench run.
var (
	// Any family: the commercial-break sequence and the bar of sustaining one stream at 1.2x.
	commonThresholds = []threshold{
		{metric: "concurrency/max_streams", limit: 1},
		{metric: "break/first_packet_ms", max: true, limit: 250},
		{metric: "break/pts_gaps", max: true, limit: 0, correctness: true},
		{metric: "break/audio_off_grid", max: true, limit: 0, correctness: true},
		{metric: "break/sps_variants", max: true, limit: 1, correctness: true},
		{metric: "break/loudness_dev_lu", max: true, limit: 1, correctness: true},
	}
	// Hardware families: start latency, speed, and the total CPU allowance at full concurrency.
	hardwareThresholds = []threshold{
		{metric: "start_p95_ms/h264-1080p", max: true, limit: 400},
		{metric: "start_p95_ms/hevc-1080p", max: true, limit: 500},
		{metric: "start_p95_ms/hdr-4k", max: true, limit: 1800},
		{metric: "speed_x/h264-1080p", limit: 15},
		{metric: "speed_x/hevc-1080p", limit: 15},
		{metric: "speed_x/hdr-4k", limit: 2},
		{metric: "concurrency/total_cores", max: true, limit: 1},
	}
)

// Judge measures a report against the thresholds of its family. A threshold whose metric is absent
// fails, unless the host declared it skipped: silence is never a pass.
func Judge(r *Report, mode Mode) []Verdict {
	set := commonThresholds
	switch r.Family {
	case "vaapi", "nvenc", "videotoolbox":
		set = append(append([]threshold{}, commonThresholds...), hardwareThresholds...)
	}
	var out []Verdict
	for _, t := range set {
		if mode == ModeCorrectness && !t.correctness {
			continue
		}
		v := Verdict{Metric: t.metric, Limit: limitText(t)}
		m, ok := r.Metrics[t.metric]
		switch {
		case ok:
			v.Measured = fmt.Sprintf("%.4g %s", m.Value, m.Unit)
			v.Pass = (t.max && m.Value <= t.limit) || (!t.max && m.Value >= t.limit)
		case r.Skipped[t.metric] != "":
			v.Measured, v.Pass = "skipped: "+r.Skipped[t.metric], true
		default:
			v.Measured = "not measured"
		}
		out = append(out, v)
	}
	return out
}

func limitText(t threshold) string {
	if t.max {
		return fmt.Sprintf("≤ %.4g", t.limit)
	}
	return fmt.Sprintf("≥ %.4g", t.limit)
}

// Failures returns the verdicts that did not pass, rendered for an error message.
func Failures(vs []Verdict) string {
	var lines []string
	for _, v := range vs {
		if !v.Pass {
			lines = append(lines, fmt.Sprintf("%s: %s (limit %s)", v.Metric, v.Measured, v.Limit))
		}
	}
	return strings.Join(lines, "\n")
}
