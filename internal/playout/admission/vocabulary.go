package admission

import "github.com/loomarr/loomarr/internal/playout/media"

// The media names the ledger speaks, so its code and tests read without the package qualifier.
type (
	MediaFormat = media.MediaFormat
	StreamClass = media.StreamClass
	ToneCurve   = media.ToneCurve
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

// ClassOf is the cost class of a source; TranscodeClasses are the ones the probe measures.
var (
	ClassOf          = media.ClassOf
	TranscodeClasses = media.TranscodeClasses
)
