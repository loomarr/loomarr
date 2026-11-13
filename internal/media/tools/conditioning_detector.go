package tools

import (
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

var (
	integratedLoudnessPattern        = regexp.MustCompile(`(?m)^\s*I:\s*(-?[0-9]+(?:\.[0-9]+)?)\s+LUFS\s*$`)
	truePeakPattern                  = regexp.MustCompile(`(?mi)^\s*Peak:\s*(-?(?:[0-9]+(?:\.[0-9]+)?|inf))\s+dBFS\s*$`)
	conditioningParsedDetectorLine   = regexp.MustCompile(`^\s*\[Parsed_(blackdetect|silencedetect|freezedetect)_([0-9]+) @ (0x[0-9A-Fa-f]+)\]\s+(.+?)\s*$`)
	conditioningLegacyDetectorLine   = regexp.MustCompile(`^\s*\[(blackdetect|silencedetect|freezedetect) @ (0x[0-9A-Fa-f]+)\]\s+(.+?)\s*$`)
	conditioningDetectorToken        = regexp.MustCompile(`(?:black_(?:start|end|duration)|silence_(?:start|end|duration)|freeze_(?:start|end|duration)):`)
	conditioningBlackPayload         = regexp.MustCompile(`^black_start:(-?[0-9]+(?:\.[0-9]+)?) black_end:(-?[0-9]+(?:\.[0-9]+)?) black_duration:(-?[0-9]+(?:\.[0-9]+)?)$`)
	conditioningSilenceStart         = regexp.MustCompile(`^silence_start: (-?[0-9]+(?:\.[0-9]+)?)$`)
	conditioningSilenceEnd           = regexp.MustCompile(`^silence_end: (-?[0-9]+(?:\.[0-9]+)?) \| silence_duration: (-?[0-9]+(?:\.[0-9]+)?)$`)
	conditioningFreezeStartModern    = regexp.MustCompile(`^lavfi\.freezedetect\.freeze_start: (-?[0-9]+(?:\.[0-9]+)?)$`)
	conditioningFreezeDurationModern = regexp.MustCompile(`^lavfi\.freezedetect\.freeze_duration: (-?[0-9]+(?:\.[0-9]+)?)$`)
	conditioningFreezeEndModern      = regexp.MustCompile(`^lavfi\.freezedetect\.freeze_end: (-?[0-9]+(?:\.[0-9]+)?)$`)
	conditioningFreezeStartLegacy    = regexp.MustCompile(`^freeze_start: (-?[0-9]+(?:\.[0-9]+)?)$`)
	conditioningFreezeDurationLegacy = regexp.MustCompile(`^freeze_duration: (-?[0-9]+(?:\.[0-9]+)?)$`)
	conditioningFreezeEndLegacy      = regexp.MustCompile(`^freeze_end: (-?[0-9]+(?:\.[0-9]+)?)$`)
)

type conditioningDetectorScalar struct {
	seconds *big.Rat
	ms      int64
}

// conditioningDetectorTimeline bounds one stream's detector events on the container timeline. eof
// is the stream's decoded end, which may pass the container end: an AAC stream decodes its final
// frame's padding (#1719). Evidence stops at the earlier of the two, boundMs.
type conditioningDetectorTimeline struct {
	eof     conditioningDetectorScalar
	tailMs  int64
	boundMs int64
}

func (t conditioningDetectorTimeline) maximumSeconds() *big.Rat {
	return new(big.Rat).Add(t.eof.seconds, big.NewRat(t.tailMs, 1_000))
}

type conditioningOpenDetectorEvent struct {
	start    conditioningDetectorScalar
	duration *conditioningDetectorScalar
}

type conditioningDetectorIntervalKey struct {
	kind       string
	start, end int64
}

type conditioningDetectorIdentity struct {
	kind     string
	instance string
	address  string
	modern   bool
}

func parseConditioningDetectorEvents(raw string, containerDurationMs int64, streams conditioningDetectorStreams) (MediaQuality, error) {
	quality := MediaQuality{
		EvidenceVersion: MediaQualityEvidenceV1,
		Provenance:      MediaQualityProvenanceFFmpegDetectors,
		DurationMs:      containerDurationMs,
	}
	zero := conditioningDetectorScalar{seconds: new(big.Rat)}
	videoTimeline := conditioningDetectorTimeline{eof: zero}
	audioTimeline := conditioningDetectorTimeline{eof: zero}
	if streams.video != nil {
		videoTimeline.eof = streams.videoEOF
		videoTimeline.tailMs = streams.videoTailMs
	}
	if streams.audio != nil {
		audioTimeline.eof = streams.audioEOF
	}
	videoTimeline.boundMs = min(videoTimeline.eof.ms, containerDurationMs)
	audioTimeline.boundMs = min(audioTimeline.eof.ms, containerDurationMs)
	var silence, freeze *conditioningOpenDetectorEvent
	seen := make(map[conditioningDetectorIntervalKey]struct{})
	identities := make(map[string]conditioningDetectorIdentity)
	for _, line := range strings.Split(raw, "\n") {
		identity, payload, matched := conditioningDetectorLine(line)
		if !matched {
			if conditioningDetectorToken.MatchString(line) {
				return MediaQuality{}, fmt.Errorf("%w: detector line does not match an observed ffmpeg prefix", ErrConditioningOutput)
			}
			continue
		}
		if existing, found := identities[identity.kind]; found && existing != identity {
			return MediaQuality{}, fmt.Errorf("%w: detector identity or grammar changed within one measurement", ErrConditioningOutput)
		}
		identities[identity.kind] = identity
		switch identity.kind {
		case "blackdetect":
			match := conditioningBlackPayload.FindStringSubmatch(payload)
			if match == nil {
				return MediaQuality{}, fmt.Errorf("%w: blackdetect payload does not match its grammar", ErrConditioningOutput)
			}
			interval, err := completeConditioningDetectorInterval("black", match[1], match[2], match[3], videoTimeline, seen)
			if err != nil {
				return MediaQuality{}, err
			}
			if interval != nil {
				quality.Black = append(quality.Black, *interval)
			}
		case "silencedetect":
			if match := conditioningSilenceStart.FindStringSubmatch(payload); match != nil {
				if silence != nil {
					return MediaQuality{}, fmt.Errorf("%w: repeated silencedetect start", ErrConditioningOutput)
				}
				start, err := parseConditioningDetectorTime(match[1], audioTimeline.maximumSeconds())
				if err != nil {
					return MediaQuality{}, err
				}
				silence = &conditioningOpenDetectorEvent{start: start}
				continue
			}
			match := conditioningSilenceEnd.FindStringSubmatch(payload)
			if match == nil || silence == nil {
				return MediaQuality{}, fmt.Errorf("%w: malformed or repeated silencedetect start", ErrConditioningOutput)
			}
			interval, err := completeConditioningDetectorIntervalFromStart("silence", silence.start, match[1], match[2], audioTimeline, seen)
			if err != nil {
				return MediaQuality{}, err
			}
			if interval != nil {
				quality.Silence = append(quality.Silence, *interval)
			}
			silence = nil
		case "freezedetect":
			startPattern, durationPattern, endPattern := conditioningFreezeStartLegacy, conditioningFreezeDurationLegacy, conditioningFreezeEndLegacy
			if identity.modern {
				startPattern, durationPattern, endPattern = conditioningFreezeStartModern, conditioningFreezeDurationModern, conditioningFreezeEndModern
			}
			if match := startPattern.FindStringSubmatch(payload); match != nil {
				if freeze != nil {
					return MediaQuality{}, fmt.Errorf("%w: repeated freezedetect start", ErrConditioningOutput)
				}
				start, err := parseConditioningDetectorTime(match[1], videoTimeline.maximumSeconds())
				if err != nil {
					return MediaQuality{}, err
				}
				freeze = &conditioningOpenDetectorEvent{start: start}
				continue
			}
			if match := durationPattern.FindStringSubmatch(payload); match != nil {
				if freeze == nil || freeze.duration != nil {
					return MediaQuality{}, fmt.Errorf("%w: unmatched or repeated freezedetect duration", ErrConditioningOutput)
				}
				duration, err := parseConditioningDetectorDuration(match[1], videoTimeline.maximumSeconds())
				if err != nil {
					return MediaQuality{}, err
				}
				freeze.duration = &duration
				continue
			}
			match := endPattern.FindStringSubmatch(payload)
			if match == nil || freeze == nil || freeze.duration == nil {
				return MediaQuality{}, fmt.Errorf("%w: malformed or incomplete freezedetect end", ErrConditioningOutput)
			}
			interval, err := completeConditioningDetectorIntervalFromScalars("freeze", freeze.start, match[1], *freeze.duration, videoTimeline, seen)
			if err != nil {
				return MediaQuality{}, err
			}
			if interval != nil {
				quality.Freeze = append(quality.Freeze, *interval)
			}
			freeze = nil
		default:
			return MediaQuality{}, fmt.Errorf("%w: unknown conditioning detector", ErrConditioningOutput)
		}
	}
	if silence != nil || freeze != nil {
		return MediaQuality{}, fmt.Errorf("%w: detector event has no matching end", ErrConditioningOutput)
	}
	quality.Black = normaliseIntervals(quality.Black, videoTimeline.boundMs)
	quality.Silence = normaliseIntervals(quality.Silence, audioTimeline.boundMs)
	quality.Freeze = normaliseIntervals(quality.Freeze, videoTimeline.boundMs)
	return quality, nil
}

func conditioningDetectorLine(line string) (identity conditioningDetectorIdentity, payload string, matched bool) {
	if values := conditioningParsedDetectorLine.FindStringSubmatch(line); values != nil {
		return conditioningDetectorIdentity{kind: values[1], instance: values[2], address: values[3], modern: true}, values[4], true
	}
	if values := conditioningLegacyDetectorLine.FindStringSubmatch(line); values != nil {
		return conditioningDetectorIdentity{kind: values[1], address: values[2]}, values[3], true
	}
	return conditioningDetectorIdentity{}, "", false
}

func completeConditioningDetectorInterval(kind, startRaw, endRaw, durationRaw string, timeline conditioningDetectorTimeline, seen map[conditioningDetectorIntervalKey]struct{}) (*Interval, error) {
	start, err := parseConditioningDetectorTime(startRaw, timeline.maximumSeconds())
	if err != nil {
		return nil, err
	}
	duration, err := parseConditioningDetectorDuration(durationRaw, timeline.maximumSeconds())
	if err != nil {
		return nil, err
	}
	return completeConditioningDetectorIntervalFromScalars(kind, start, endRaw, duration, timeline, seen)
}

func completeConditioningDetectorIntervalFromStart(kind string, start conditioningDetectorScalar, endRaw, durationRaw string, timeline conditioningDetectorTimeline, seen map[conditioningDetectorIntervalKey]struct{}) (*Interval, error) {
	duration, err := parseConditioningDetectorDuration(durationRaw, timeline.maximumSeconds())
	if err != nil {
		return nil, err
	}
	return completeConditioningDetectorIntervalFromScalars(kind, start, endRaw, duration, timeline, seen)
}

func completeConditioningDetectorIntervalFromScalars(kind string, start conditioningDetectorScalar, endRaw string, detectorDuration conditioningDetectorScalar, timeline conditioningDetectorTimeline, seen map[conditioningDetectorIntervalKey]struct{}) (*Interval, error) {
	end, err := parseConditioningDetectorEndTime(endRaw, timeline.maximumSeconds())
	if err != nil || end.seconds.Cmp(start.seconds) <= 0 || end.ms <= start.ms {
		return nil, fmt.Errorf("%w: detector interval is inverted or out of range", ErrConditioningOutput)
	}
	delta := new(big.Rat).Sub(end.seconds, start.seconds)
	disagreement := new(big.Rat).Sub(delta, detectorDuration.seconds)
	disagreement.Abs(disagreement)
	if disagreement.Cmp(big.NewRat(1, 1_000)) > 0 {
		return nil, fmt.Errorf("%w: detector duration does not equal end minus start", ErrConditioningOutput)
	}
	key := conditioningDetectorIntervalKey{kind: kind, start: start.ms, end: end.ms}
	if _, duplicate := seen[key]; duplicate {
		return nil, fmt.Errorf("%w: duplicate complete detector interval", ErrConditioningOutput)
	}
	seen[key] = struct{}{}
	clampedStart := min(start.ms, timeline.boundMs)
	clampedEnd := min(end.ms, timeline.boundMs)
	if clampedEnd <= clampedStart {
		return nil, nil
	}
	return &Interval{StartMs: clampedStart, EndMs: clampedEnd}, nil
}

func parseConditioningDetectorTime(raw string, maximumSeconds *big.Rat) (conditioningDetectorScalar, error) {
	value, err := parseConditioningDetectorScalar(raw)
	if err != nil || value.seconds.Cmp(maximumSeconds) > 0 {
		return conditioningDetectorScalar{}, fmt.Errorf("%w: detector time %q is outside the permitted timeline", ErrConditioningOutput, raw)
	}
	return value, nil
}

func parseConditioningDetectorEndTime(raw string, maximumSeconds *big.Rat) (conditioningDetectorScalar, error) {
	value, err := parseConditioningDetectorScalar(raw)
	if err != nil {
		return conditioningDetectorScalar{}, err
	}
	if value.seconds.Cmp(maximumSeconds) <= 0 {
		return value, nil
	}
	excess := new(big.Rat).Sub(value.seconds, maximumSeconds)
	if excess.Cmp(big.NewRat(conditioningDetectorEndToleranceMs, 1_000)) <= 0 {
		return value, nil
	}
	return conditioningDetectorScalar{}, fmt.Errorf("%w: detector end time %q is outside the permitted timeline", ErrConditioningOutput, raw)
}

func parseConditioningDetectorDuration(raw string, maximumSeconds *big.Rat) (conditioningDetectorScalar, error) {
	value, err := parseConditioningDetectorScalar(raw)
	if err != nil || value.seconds.Sign() <= 0 || value.seconds.Cmp(maximumSeconds) > 0 || value.ms <= 0 {
		return conditioningDetectorScalar{}, fmt.Errorf("%w: invalid detector duration %q", ErrConditioningOutput, raw)
	}
	return value, nil
}

func parseConditioningDetectorScalar(raw string) (conditioningDetectorScalar, error) {
	seconds, ok := new(big.Rat).SetString(raw)
	if !ok || seconds.Sign() < 0 {
		return conditioningDetectorScalar{}, fmt.Errorf("%w: invalid detector scalar %q", ErrConditioningOutput, raw)
	}
	scaled := new(big.Rat).Mul(seconds, big.NewRat(1_000, 1))
	ms, ok := roundPositiveConditioningRational(scaled)
	if !ok {
		return conditioningDetectorScalar{}, fmt.Errorf("%w: detector scalar overflows milliseconds", ErrConditioningOutput)
	}
	return conditioningDetectorScalar{seconds: seconds, ms: ms}, nil
}

func roundPositiveConditioningRational(value *big.Rat) (int64, bool) {
	quotient := new(big.Int)
	remainder := new(big.Int)
	quotient.QuoRem(value.Num(), value.Denom(), remainder)
	if new(big.Int).Lsh(remainder, 1).Cmp(value.Denom()) >= 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() {
		return 0, false
	}
	return quotient.Int64(), true
}

func conditioningDetectorTailAllowance(stream ConditioningStream) int64 {
	if stream.Cadence == nil || stream.Cadence.Numerator <= 0 || stream.Cadence.Denominator <= 0 {
		return 0
	}
	numerator := big.NewInt(stream.Cadence.Numerator)
	scaledDenominator := new(big.Int).Mul(big.NewInt(stream.Cadence.Denominator), big.NewInt(1_000))
	frameMs := new(big.Int).Add(scaledDenominator, new(big.Int).Sub(numerator, big.NewInt(1)))
	frameMs.Quo(frameMs, numerator)
	if !frameMs.IsInt64() || frameMs.Int64() >= ConditioningMaxDurationMs {
		return ConditioningMaxDurationMs
	}
	return max(int64(1), frameMs.Int64()) + 1
}

func conditioningVideoDetectorTailAllowance(index int, frames []conditioningProbeFrameJSON, stream ConditioningStream) (int64, error) {
	fallback := conditioningDetectorTailAllowance(stream)
	var latest, previous *big.Rat
	for _, frame := range frames {
		if frame.StreamIndex == nil || *frame.StreamIndex != index {
			continue
		}
		ptsRaw := frame.BestEffortTimestamp
		if ptsRaw == "" || strings.EqualFold(ptsRaw, "N/A") {
			ptsRaw = frame.PTS
		}
		pts, ok := new(big.Rat).SetString(ptsRaw)
		if !ok || pts.Sign() < 0 {
			return 0, fmt.Errorf("%w: selected video stream %d has invalid frame timestamp", ErrConditioningOutput, index)
		}
		switch {
		case latest == nil || pts.Cmp(latest) > 0:
			previous = latest
			latest = pts
		case pts.Cmp(latest) < 0 && (previous == nil || pts.Cmp(previous) > 0):
			previous = pts
		}
	}
	if latest == nil || previous == nil {
		return fallback, nil
	}
	deltaMs := new(big.Rat).Mul(new(big.Rat).Sub(latest, previous), big.NewRat(1_000, 1))
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(deltaMs.Num(), deltaMs.Denom(), remainder)
	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if !quotient.IsInt64() || quotient.Int64() >= ConditioningMaxDurationMs {
		return ConditioningMaxDurationMs, nil
	}
	return max(fallback, quotient.Int64()+1), nil
}

func parseConditioningLoudness(raw string) (ConditioningLoudness, error) {
	for _, line := range strings.Split(raw, "\n") {
		if strings.Contains(line, "I:") && strings.Contains(line, "LUFS") && integratedLoudnessPattern.FindStringSubmatch(line) == nil {
			return ConditioningLoudness{}, fmt.Errorf("%w: integrated loudness summary does not match its grammar", ErrConditioningOutput)
		}
		if strings.Contains(line, "Peak:") && strings.Contains(line, "dBFS") && truePeakPattern.FindStringSubmatch(line) == nil {
			return ConditioningLoudness{}, fmt.Errorf("%w: true-peak summary does not match its grammar", ErrConditioningOutput)
		}
	}
	integratedMatches := integratedLoudnessPattern.FindAllStringSubmatch(raw, -1)
	peakMatches := truePeakPattern.FindAllStringSubmatch(raw, -1)
	if len(integratedMatches) == 0 && len(peakMatches) == 0 {
		return ConditioningLoudness{TruePeak: ConditioningTruePeak{State: TruePeakUnavailable}}, nil
	}
	if len(integratedMatches) != 1 || len(peakMatches) != 1 {
		return ConditioningLoudness{}, fmt.Errorf("%w: loudness summary must contain exactly one integrated and one peak value", ErrConditioningOutput)
	}
	integrated, peak := integratedMatches[0], peakMatches[0]
	lufs, err := strconv.ParseFloat(integrated[1], 64)
	if err != nil || math.IsNaN(lufs) || math.IsInf(lufs, 0) {
		return ConditioningLoudness{}, fmt.Errorf("%w: invalid integrated loudness", ErrConditioningOutput)
	}
	measurement := ConditioningLoudness{IntegratedLUFS: lufs, Available: true}
	if strings.EqualFold(peak[1], "-inf") {
		measurement.TruePeak.State = TruePeakNegativeInfinity
		return measurement, nil
	}
	dbtp, err := strconv.ParseFloat(peak[1], 64)
	if err != nil || math.IsNaN(dbtp) || math.IsInf(dbtp, 0) {
		return ConditioningLoudness{}, fmt.Errorf("%w: invalid true peak", ErrConditioningOutput)
	}
	measurement.TruePeak = ConditioningTruePeak{State: TruePeakFinite, DBTP: dbtp}
	return measurement, nil
}
