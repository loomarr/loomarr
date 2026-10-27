// Package defects measures and judges real picture and sound defects in a filler clip (#2034). The
// floor rejects on what a clip actually shows and sounds like, against a bar set by its era: a 1985
// ad is held to good tape, a 2015 ad to real HD. Stated resolution and bitrate are never inputs.
//
// Readings come from one bounded ffmpeg pass (mediatools.MeasureDefects). Assess turns them into
// a verdict per check. Detectors decide clear passes and fails; a check between its two bars is
// borderline, and the borderline band is for a vision model to rule on (#2034 step 4b). Until the
// enforcement step (#2036) a verdict is only recorded. Everything here is pure.
package defects

import "math"

// MeasurementVersion versions what the readings mean: the detectors, their options and the frame
// sampling. A change gets a new key, and clips measured under the old one are measured again.
// v2: the picture is measured inside its active area, and a quarter-size upscale score is added.
const MeasurementVersion = "defects-v2-4fps-active"

// FloorVersion versions the bars. "provisional" marks bars set from synthetic measurements before
// the household labels calibrate them (#2034 step 4c). v2: clipping has no borderline band. v3:
// fitted to the synthetic corpus, with an upscale check for every era.
const FloorVersion = "floor-v3-provisional"

// SampleFPS is the frame rate the picture detectors sample at. Interlace detection is the
// exception: it needs consecutive fields, so it reads every frame.
const SampleFPS = 4

// minFrames is the least evidence a picture verdict needs: two seconds at SampleFPS.
const minFrames = 8

// Readings are one clip's measured defects. Shares are fractions of 0..1; levels are dBFS.
type Readings struct {
	Version string `json:"version"`
	// Unmeasurable says why the clip could not be measured (no picture, an undecodable stream).
	// Such a clip is Undecided and is measured again only under a new MeasurementVersion.
	Unmeasurable string `json:"unmeasurable,omitempty"`
	// Width and Height are the decoded frame size the crop is measured against.
	Width  int `json:"width"`
	Height int `json:"height"`
	// Frames is how many frames the picture detectors sampled.
	Frames int `json:"frames"`
	// BlockMean is blockdetect's mean: visible compression blocks. Clean encodes read 1-13; a
	// starved encode 60 or more.
	BlockMean float64 `json:"blockMean"`
	// BlurMean is blurdetect's mean edge width. Sharp video reads about 4-5.
	BlurMean float64 `json:"blurMean"`
	// OutlierShare is the mean share of temporal-outlier pixels (signalstats TOUT): tape dropouts,
	// sparkle and heavy noise.
	OutlierShare float64 `json:"outlierShare"`
	// RepeatedLineShare is the mean share of vertically repeated lines (signalstats VREP). Recorded
	// for calibration only: flat graphics read high on clean video.
	RepeatedLineShare float64 `json:"repeatedLineShare"`
	// ActiveWidth and ActiveHeight are the union of the picture's non-black area over the clip.
	// Every picture reading but interlace is taken inside it.
	ActiveWidth  int `json:"activeWidth"`
	ActiveHeight int `json:"activeHeight"`
	// InterlacedShare is how far one field order leads the other across the frames idet decided:
	// visible combing keeps one order throughout, a detector misreading splits evenly (see Parse).
	InterlacedShare float64 `json:"interlacedShare"`
	// UpscaleSSIM is the luma SSIM between each frame and itself halved and enlarged again. A frame
	// with no detail beyond half its size reads close to 1: it was upscaled.
	UpscaleSSIM float64 `json:"upscaleSsim"`
	// UpscaleSSIM4 is the same at a quarter size. A good tape copy has no detail at half size,
	// so for an older ad it is a tiny source enlarged that reads close to 1 here.
	UpscaleSSIM4 float64 `json:"upscaleSsim4"`
	HasAudio     bool    `json:"hasAudio"`
	PeakDB       float64 `json:"peakDb"`
	// FlatFactor and PeakCount come from astats over 16-bit samples: a clipped track flattens at full
	// scale, so both rise together.
	FlatFactor float64 `json:"flatFactor"`
	PeakCount  float64 `json:"peakCount"`
	// NoiseFloorDB is recorded for calibration only: tonal content reads as a high floor.
	NoiseFloorDB float64 `json:"noiseFloorDb"`
}

// Band is the era bar a clip is held to.
type Band string

const (
	// BandTape is broadcast tape: before 1995. Soft and a little noisy is authentic.
	BandTape Band = "tape"
	// BandSD is standard-definition digital: 1995-2009, and any clip of unknown era.
	BandSD Band = "sd"
	// BandHD is the high-definition era: 2010 on. An upscale is a defect.
	BandHD Band = "hd"
)

// BandFor places an era. An unknown era (0) is held to the middle bar rather than the most lenient.
func BandFor(era int) Band {
	switch {
	case era > 0 && era < 1995:
		return BandTape
	case era >= 2010:
		return BandHD
	default:
		return BandSD
	}
}

// Check names one defect.
type Check string

const (
	CheckBlocks    Check = "blocks"
	CheckBlur      Check = "blur"
	CheckDropouts  Check = "dropouts"
	CheckInterlace Check = "interlace"
	CheckWindowbox Check = "windowbox"
	CheckUpscale   Check = "upscale"
	CheckClipping  Check = "clipping"
)

// Outcome is a verdict, worst first: Fail, Borderline, Undecided (too little evidence to judge),
// Pass.
type Outcome string

const (
	Pass       Outcome = "pass"
	Borderline Outcome = "borderline"
	Fail       Outcome = "fail"
	Undecided  Outcome = "undecided"
)

// Finding is one check's verdict with the measurement and the bar it met.
type Finding struct {
	Check    Check   `json:"check"`
	Outcome  Outcome `json:"outcome"`
	Measured float64 `json:"measured"`
	// Bar is the borderline bar for a pass or borderline finding and the fail bar for a fail.
	Bar float64 `json:"bar"`
}

// Assessment is a clip's verdict under one floor version.
type Assessment struct {
	Version  string    `json:"version"`
	Band     Band      `json:"band"`
	Outcome  Outcome   `json:"outcome"`
	Findings []Finding `json:"findings"`
}

// bar is a check's two thresholds; a higher measurement is worse.
type bar struct{ borderline, fail float64 }

// bars are provisional (FloorVersion): set from synthetic measurements, calibrated in step 4c.
// Compression blocks, combing, windowboxing and clipping are rip damage rather than era texture,
// so one bar holds for every era. Softness and tape noise are era texture, and relax with age.
//
// floor-v3 fits them to the committed synthetic corpus (defects.CorpusVersion), whose picture
// reads like footage: clean 1.1 block, 5.7 blur. Each pair sits between the readings it must
// separate, named beside it; the household report recalibrates them on real clips.
var bars = map[Check]map[Band]bar{
	// MPEG-2 at quantiser 3 (DVD quality) reads 1.8; starved at 28, 14.5.
	CheckBlocks: allBands(bar{4, 8}),
	// Tape: a 1.5 px blur (good VHS) reads 7.06, 3 px 9.58. SD: 0.8 px reads 6.17, 1.5 px 7.06.
	// HD: 1.5 px fails; 0.8 px is left to the judge.
	CheckBlur: {BandTape: {8, 9}, BandSD: {6.5, 6.9}, BandHD: {6, 6.8}},
	// The outlier share is close to the share of the frame dropouts cover: twelve one-line specks
	// on every frame read 0.0034, two read 0.0005, clean video 0.0002.
	CheckDropouts:  {BandTape: {0.0015, 0.003}, BandSD: {0.001, 0.0025}, BandHD: {0.0007, 0.002}},
	CheckInterlace: allBands(bar{0.3, 0.6}),
	// A 5% border on every side is overscan-sized and passes; 25% fails.
	CheckWindowbox: allBands(bar{0.07, 0.12}),
	// HD on the half-size score: 0.8 px of blur reads 0.9875, a 2x enlargement 0.9964. Older eras
	// on the quarter-size score, where good tape is not "upscaled": a 2x enlargement reads 0.972,
	// a 4x one 0.994.
	CheckUpscale: {BandTape: {0.988, 0.992}, BandSD: {0.988, 0.992}, BandHD: {0.990, 0.994}},
	// No borderline band: the judge sees stills and cannot hear, so sound is settled by the
	// detector alone (floor-v2).
	CheckClipping: allBands(bar{5, 5}),
}

func allBands(b bar) map[Band]bar { return map[Band]bar{BandTape: b, BandSD: b, BandHD: b} }

// checkOrder fixes the order findings are reported in.
var checkOrder = []Check{CheckBlocks, CheckBlur, CheckDropouts, CheckInterlace, CheckWindowbox, CheckUpscale, CheckClipping}

// Assess judges readings against the bars for the clip's era. The overall outcome is the worst
// finding. Picture checks are Undecided without minFrames of evidence; clipping is skipped for a
// clip with no sound.
func Assess(r Readings, era int) Assessment {
	band := BandFor(era)
	if r.Unmeasurable != "" {
		return Assessment{Version: FloorVersion, Band: band, Outcome: Undecided}
	}
	a := Assessment{Version: FloorVersion, Band: band, Outcome: Pass}
	pictureKnown := r.Frames >= minFrames && r.Width > 0 && r.Height > 0
	for _, check := range checkOrder {
		b, applies := bars[check][band]
		if !applies || (check == CheckClipping && !r.HasAudio) {
			continue
		}
		f := Finding{Check: check, Measured: measure(check, band, r), Outcome: Pass, Bar: b.borderline}
		switch {
		case check != CheckClipping && !pictureKnown:
			f.Outcome, f.Bar = Undecided, 0
		case f.Measured >= b.fail:
			f.Outcome, f.Bar = Fail, b.fail
		case f.Measured >= b.borderline:
			f.Outcome = Borderline
		}
		a.Findings = append(a.Findings, f)
		a.Outcome = worse(a.Outcome, f.Outcome)
	}
	return a
}

func measure(check Check, band Band, r Readings) float64 {
	switch check {
	case CheckBlocks:
		return r.BlockMean
	case CheckBlur:
		return r.BlurMean
	case CheckDropouts:
		return r.OutlierShare
	case CheckInterlace:
		return r.InterlacedShare
	case CheckWindowbox:
		return windowbox(r)
	case CheckUpscale:
		if band == BandHD {
			return r.UpscaleSSIM
		}
		return r.UpscaleSSIM4
	case CheckClipping:
		return r.FlatFactor
	}
	return 0
}

// windowbox is how much of the frame a border takes on BOTH axes. Letterbox (bars top and bottom)
// and pillarbox (bars at the sides) are how a different aspect ratio is meant to look, and read 0;
// a picture shrunk inside the frame on every side reads the smaller of its two border shares.
func windowbox(r Readings) float64 {
	if r.Width <= 0 || r.Height <= 0 || r.ActiveWidth <= 0 || r.ActiveHeight <= 0 {
		return 0
	}
	w := 1 - float64(r.ActiveWidth)/float64(r.Width)
	h := 1 - float64(r.ActiveHeight)/float64(r.Height)
	return math.Max(0, math.Min(w, h))
}

func worse(a, b Outcome) Outcome {
	if rank(b) > rank(a) {
		return b
	}
	return a
}

// rank orders outcomes for the overall verdict. Undecided outranks Pass: a clip whose picture
// could not be judged has not passed.
func rank(o Outcome) int {
	switch o {
	case Fail:
		return 4
	case Borderline:
		return 3
	case Undecided:
		return 2
	case Pass:
		return 1
	}
	return 0
}
