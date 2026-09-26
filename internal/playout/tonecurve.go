package playout

import "strings"

// ToneCurve is the HDR→SDR tone curve, the operator's `playout.tone_curve` (G11, #1512). One curve
// runs on every family; each tone-mapper spells it its own way, and a tone-mapper without the curve
// is either skipped (the GPU ones) or substitutes its closest curve, declared in Pipeline.Fallbacks
// (the CPU one). Never black: every chain ends in a tone-mapper that exists on every build.
//
//	curve    tonemap_opencl   libplacebo   CPU tonemap          preferred GPU path
//	hable    hable            hable        hable                tonemap_opencl (Intel: zero-copy)
//	mobius   mobius           mobius       mobius               tonemap_opencl
//	reinhard reinhard         reinhard     reinhard             tonemap_opencl
//	bt2390   bt2390           bt.2390      mobius (substitute)  libplacebo (Intel: CPU hop)
//	bt2446a  (none)           bt.2446a     mobius (substitute)  libplacebo
//	spline   (none)           spline       mobius (substitute)  libplacebo
//
// Mobius is the CPU substitute for the three libplacebo curves because, like them, it keeps the
// in-range picture linear and only rolls the highlights off.
type ToneCurve string

const (
	ToneCurveHable    ToneCurve = "hable"
	ToneCurveMobius   ToneCurve = "mobius"
	ToneCurveReinhard ToneCurve = "reinhard"
	ToneCurveBT2390   ToneCurve = "bt2390"
	ToneCurveBT2446a  ToneCurve = "bt2446a"
	ToneCurveSpline   ToneCurve = "spline"
)

// DefaultToneCurve is Hable: the only curve with a zero-copy GPU path on Intel and NVIDIA and an
// exact CPU equivalent (spike 0b; maintainer decision, #1512).
const DefaultToneCurve = ToneCurveHable

// ToneCurves is every curve the setting offers, in the order it lists them.
var ToneCurves = []ToneCurve{ToneCurveHable, ToneCurveMobius, ToneCurveReinhard, ToneCurveBT2390, ToneCurveBT2446a, ToneCurveSpline}

// ParseToneCurve reads a setting or query value; anything unknown is the default.
func ParseToneCurve(s string) ToneCurve {
	c := ToneCurve(strings.ToLower(strings.TrimSpace(s)))
	for _, known := range ToneCurves {
		if c == known {
			return c
		}
	}
	return DefaultToneCurve
}

// placeboFirst: the curve exists only (or first) in libplacebo, so libplacebo is its preferred GPU
// tone-mapper even where that costs a CPU hop (Intel).
func (c ToneCurve) placeboFirst() bool {
	return c == ToneCurveBT2390 || c == ToneCurveBT2446a || c == ToneCurveSpline
}

// openCL is the curve's tonemap_opencl name, "" when tonemap_opencl lacks it.
func (c ToneCurve) openCL() string {
	switch c {
	case ToneCurveBT2446a, ToneCurveSpline:
		return ""
	}
	return string(c)
}

// placebo is the curve's libplacebo `tonemapping` name.
func (c ToneCurve) placebo() string {
	switch c {
	case ToneCurveBT2390:
		return "bt.2390"
	case ToneCurveBT2446a:
		return "bt.2446a"
	}
	return string(c)
}

// cpu is the CPU tonemap filter's curve and whether it is the curve asked for.
func (c ToneCurve) cpu() (ToneCurve, bool) {
	if c.placeboFirst() {
		return ToneCurveMobius, false
	}
	return c, true
}
