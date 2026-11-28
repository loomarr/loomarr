package playout

import "github.com/loomarr/loomarr/internal/playout/media"

// The engine and its callers spell the media vocabulary through these names. The types and their
// behaviour live in the leaf package, which other packages can share without importing the engine.
type (
	MediaFormat  = media.MediaFormat
	FormatProber = media.FormatProber
	Rect         = media.Rect
	StreamClass  = media.StreamClass
	ToneCurve    = media.ToneCurve
)

const (
	ClassCopy      = media.ClassCopy
	ClassSDR       = media.ClassSDR
	ClassHEVC10    = media.ClassHEVC10
	ClassHDR4K     = media.ClassHDR4K
	ClassPremium4K = media.ClassPremium4K

	ToneCurveHable    = media.ToneCurveHable
	ToneCurveMobius   = media.ToneCurveMobius
	ToneCurveReinhard = media.ToneCurveReinhard
	ToneCurveBT2390   = media.ToneCurveBT2390
	ToneCurveBT2446a  = media.ToneCurveBT2446a
	ToneCurveSpline   = media.ToneCurveSpline
	DefaultToneCurve  = media.DefaultToneCurve
)

var (
	TranscodeClasses = media.TranscodeClasses
	ToneCurves       = media.ToneCurves
	ClassOf          = media.ClassOf
	ParseToneCurve   = media.ParseToneCurve
	IsHEVCCodec      = media.IsHEVCCodec
)
