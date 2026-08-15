package filler

import (
	"fmt"
	"time"
)

const (
	hardDeadAirPercent = int64(90)
	reviewBlackSilence = 5 * time.Second
	reviewFreeze       = 10 * time.Second
)

// mediaQualityVerdict turns measured intervals into the deliberately conservative V40 decision:
// near-total dead air is objective; a long span that may be an intentional creative choice is not.
func mediaQualityVerdict(q MediaQuality) (Verdict, RejectReason, string) {
	if q.DurationMs <= 0 {
		return VerdictContinue, "", ""
	}
	blackMs, blackLongest := intervalFacts(q.Black)
	silenceMs, silenceLongest := intervalFacts(q.Silence)
	_, freezeLongest := intervalFacts(q.Freeze)
	blackPct := blackMs * 100 / q.DurationMs
	silencePct := silenceMs * 100 / q.DurationMs

	switch {
	case blackPct >= hardDeadAirPercent:
		return VerdictReject, ReasonBlackContent,
			fmt.Sprintf("%d%% of the %.1fs clip is black", blackPct, float64(q.DurationMs)/1000)
	case silencePct >= hardDeadAirPercent:
		return VerdictReject, ReasonSilentContent,
			fmt.Sprintf("%d%% of the %.1fs audio is silent", silencePct, float64(q.DurationMs)/1000)
	case blackLongest >= reviewBlackSilence.Milliseconds():
		return VerdictReview, "", fmt.Sprintf("A %.1fs black span needs a look before this airs.", float64(blackLongest)/1000)
	case silenceLongest >= reviewBlackSilence.Milliseconds():
		return VerdictReview, "", fmt.Sprintf("A %.1fs silent span needs a look before this airs.", float64(silenceLongest)/1000)
	case freezeLongest >= reviewFreeze.Milliseconds():
		return VerdictReview, "", fmt.Sprintf("The picture is unchanged for %.1fs; this may be intentional.", float64(freezeLongest)/1000)
	default:
		return VerdictContinue, "", ""
	}
}

func intervalFacts(spans []Interval) (totalMs, longestMs int64) {
	for _, span := range spans {
		if d := span.EndMs - span.StartMs; d > 0 {
			totalMs += d
			if d > longestMs {
				longestMs = d
			}
		}
	}
	return totalMs, longestMs
}
