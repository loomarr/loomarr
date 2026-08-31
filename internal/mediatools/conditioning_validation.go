package mediatools

import (
	"fmt"
	"math"
)

// ConditioningEvidenceMode is the closed persisted-evidence position. Before-rewrite evidence
// must carry directly packet-matched cut edges; after-rewrite evidence preserves their explicit
// unavailability because re-encoding changes packet identity.
type ConditioningEvidenceMode string

const (
	ConditioningBeforeRewrite ConditioningEvidenceMode = "before_rewrite"
	ConditioningAfterRewrite  ConditioningEvidenceMode = "after_rewrite"
)

// ValidateConditioningEvidence validates the application-safe persisted subset of a measurement.
// MeasureConditioning can honestly describe more than one A/V pair, but the filler application
// currently supports exactly one presented video and one presented audio stream.
func ValidateConditioningEvidence(m ConditioningMeasurement, mode ConditioningEvidenceMode) error {
	if mode != ConditioningBeforeRewrite && mode != ConditioningAfterRewrite {
		return fmt.Errorf("%w: unknown conditioning evidence mode %q", ErrConditioningOutput, mode)
	}
	if m.ContainerDurationMs <= 0 || m.ContainerDurationMs > ConditioningMaxDurationMs {
		return fmt.Errorf("%w: container duration is unavailable or out of bounds", ErrConditioningOutput)
	}
	if len(m.Streams) != 2 {
		return fmt.Errorf("%w: persisted conditioning requires exactly one audio/video pair", ErrConditioningOutput)
	}
	type streamID struct {
		kind  StreamKind
		index int
	}
	identities := make(map[streamID]ConditioningStream, 2)
	indexes := make(map[int]struct{}, 2)
	var video, audio *ConditioningStream
	for i := range m.Streams {
		stream := m.Streams[i]
		if stream.Index < 0 {
			return fmt.Errorf("%w: stream index is invalid", ErrConditioningOutput)
		}
		if _, exists := indexes[stream.Index]; exists {
			return fmt.Errorf("%w: stream index %d is reused", ErrConditioningOutput, stream.Index)
		}
		indexes[stream.Index] = struct{}{}
		if stream.Kind != StreamVideo && stream.Kind != StreamAudio {
			return fmt.Errorf("%w: stream kind %q is invalid", ErrConditioningOutput, stream.Kind)
		}
		id := streamID{kind: stream.Kind, index: stream.Index}
		if _, exists := identities[id]; exists {
			return fmt.Errorf("%w: duplicate stream identity", ErrConditioningOutput)
		}
		if !stream.Start.Available || !validPositiveTiming(stream.Duration) {
			return fmt.Errorf("%w: stream timing is unavailable or invalid", ErrConditioningOutput)
		}
		end, ok := checkedMillisecondsAdd(stream.Start.Milliseconds, stream.Duration.Milliseconds)
		if !ok || end <= 0 || end > m.ContainerDurationMs {
			return fmt.Errorf("%w: stream timing is outside the container", ErrConditioningOutput)
		}
		switch stream.Kind {
		case StreamVideo:
			if video != nil || stream.Cadence == nil || stream.Cadence.Numerator <= 0 || stream.Cadence.Denominator <= 0 {
				return fmt.Errorf("%w: video cadence or cardinality is invalid", ErrConditioningOutput)
			}
			video = &m.Streams[i]
		case StreamAudio:
			if audio != nil {
				return fmt.Errorf("%w: audio cardinality is invalid", ErrConditioningOutput)
			}
			audio = &m.Streams[i]
		}
		identities[id] = stream
	}
	if video == nil || audio == nil || !m.AVSkew.Start.Available || !m.AVSkew.End.Available {
		return fmt.Errorf("%w: A/V skew is unavailable", ErrConditioningOutput)
	}
	videoEnd, videoOK := checkedMillisecondsAdd(video.Start.Milliseconds, video.Duration.Milliseconds)
	audioEnd, audioOK := checkedMillisecondsAdd(audio.Start.Milliseconds, audio.Duration.Milliseconds)
	startSkew, startOK := checkedMillisecondsSub(audio.Start.Milliseconds, video.Start.Milliseconds)
	endSkew, endOK := checkedMillisecondsSub(audioEnd, videoEnd)
	if !videoOK || !audioOK || !startOK || !endOK ||
		m.AVSkew.Start.Milliseconds != startSkew || m.AVSkew.End.Milliseconds != endSkew {
		return fmt.Errorf("%w: A/V skew does not match stream timing", ErrConditioningOutput)
	}
	if !m.Loudness.Available || math.IsNaN(m.Loudness.IntegratedLUFS) || math.IsInf(m.Loudness.IntegratedLUFS, 0) {
		return fmt.Errorf("%w: integrated loudness is unavailable or invalid", ErrConditioningOutput)
	}
	switch m.Loudness.TruePeak.State {
	case TruePeakFinite:
		if math.IsNaN(m.Loudness.TruePeak.DBTP) || math.IsInf(m.Loudness.TruePeak.DBTP, 0) {
			return fmt.Errorf("%w: true peak is invalid", ErrConditioningOutput)
		}
	case TruePeakNegativeInfinity:
		if m.Loudness.TruePeak.DBTP != 0 {
			return fmt.Errorf("%w: digital-silence true peak must not carry a finite value", ErrConditioningOutput)
		}
	default:
		return fmt.Errorf("%w: true peak is unavailable or invalid", ErrConditioningOutput)
	}
	if m.Quality.DurationMs != m.ContainerDurationMs {
		return fmt.Errorf("%w: media-quality duration does not match the container", ErrConditioningOutput)
	}
	if err := ValidateMediaQualityEvidence(m.Quality); err != nil {
		return err
	}
	if len(m.Cuts) != 1 || len(m.Cuts[0].Streams) != len(identities) {
		return fmt.Errorf("%w: cut evidence does not match presented streams", ErrConditioningOutput)
	}
	cutIDs := make(map[streamID]struct{}, len(identities))
	for _, edge := range m.Cuts[0].Streams {
		id := streamID{kind: edge.Kind, index: edge.Index}
		if _, exists := identities[id]; !exists {
			return fmt.Errorf("%w: cut stream identity is not presented", ErrConditioningOutput)
		}
		if _, exists := cutIDs[id]; exists {
			return fmt.Errorf("%w: duplicate cut stream identity", ErrConditioningOutput)
		}
		if mode == ConditioningBeforeRewrite && (!edge.StartError.Available || !edge.EndError.Available) {
			return fmt.Errorf("%w: directly matched cut edge is unavailable", ErrConditioningOutput)
		}
		if !validOptionalTiming(edge.StartError) || !validOptionalTiming(edge.EndError) {
			return fmt.Errorf("%w: unavailable cut edge carries a guessed value", ErrConditioningOutput)
		}
		cutIDs[id] = struct{}{}
	}
	return nil
}

// ValidateMediaQualityEvidence validates the closed, durable detector evidence shape without
// applying any quality threshold or verdict.
func ValidateMediaQualityEvidence(quality MediaQuality) error {
	if quality.EvidenceVersion != MediaQualityEvidenceV1 || quality.Provenance != MediaQualityProvenanceFFmpegDetectors {
		return fmt.Errorf("%w: media-quality evidence version or provenance is invalid", ErrConditioningOutput)
	}
	if quality.DurationMs <= 0 {
		return fmt.Errorf("%w: media-quality duration is unavailable", ErrConditioningOutput)
	}
	for _, spans := range [][]Interval{quality.Black, quality.Silence, quality.Freeze} {
		if err := validateCanonicalIntervals(spans, quality.DurationMs); err != nil {
			return err
		}
	}
	return nil
}

// ValidateConditioningPair requires independently valid measurements over the exact same stream
// identities. It deliberately does not manufacture post-rewrite direct cut evidence.
func ValidateConditioningPair(before, after ConditioningMeasurement) error {
	if err := ValidateConditioningEvidence(before, ConditioningBeforeRewrite); err != nil {
		return err
	}
	if err := ValidateConditioningEvidence(after, ConditioningAfterRewrite); err != nil {
		return err
	}
	type streamID struct {
		kind  StreamKind
		index int
	}
	beforeIDs := make(map[streamID]struct{}, len(before.Streams))
	for _, stream := range before.Streams {
		beforeIDs[streamID{kind: stream.Kind, index: stream.Index}] = struct{}{}
	}
	for _, stream := range after.Streams {
		if _, ok := beforeIDs[streamID{kind: stream.Kind, index: stream.Index}]; !ok {
			return fmt.Errorf("%w: before/after stream identities differ", ErrConditioningOutput)
		}
	}
	if before.Cuts[0].Intended != after.Cuts[0].Intended {
		return fmt.Errorf("%w: before/after cut interval identities differ", ErrConditioningOutput)
	}
	return nil
}

// ValidateConditioningCutEvidence requires the pre-rewrite measurement to carry complete,
// packet-matched edge evidence for the exact reviewed interval. It validates evidence shape only;
// callers remain responsible for deciding whether any measured error is acceptable.
func ValidateConditioningCutEvidence(m ConditioningMeasurement, intended Interval) error {
	if err := ValidateConditioningEvidence(m, ConditioningBeforeRewrite); err != nil {
		return err
	}
	if err := ValidateConditioningCutIdentities(m.Cuts, []Interval{intended}); err != nil {
		return err
	}
	if len(m.Cuts[0].Streams) != len(m.Streams) {
		return fmt.Errorf("%w: measured child cut evidence is incomplete", ErrConditioningOutput)
	}
	for _, edge := range m.Cuts[0].Streams {
		if !edge.StartError.Available || !edge.EndError.Available {
			return fmt.Errorf("%w: measured child cut edge is unavailable", ErrConditioningOutput)
		}
	}
	return nil
}

// ValidateConditioningCutIdentities validates the bounded, ordered identity packet persisted for
// each measured cut. It deliberately accepts the existing cut slice rather than introducing a
// second interval model; callers decide whether the surrounding measurement is otherwise usable.
func ValidateConditioningCutIdentities(cuts []ConditioningCutMeasurement, intended []Interval) error {
	if len(cuts) != len(intended) || len(cuts) == 0 || len(cuts) > 8 {
		return fmt.Errorf("%w: measured cut identity cardinality is invalid", ErrConditioningOutput)
	}
	var previousEnd int64
	for i, want := range intended {
		if want.StartMs < 0 || want.EndMs <= want.StartMs {
			return fmt.Errorf("%w: intended conditioning interval is invalid", ErrConditioningOutput)
		}
		if i > 0 && want.StartMs < previousEnd {
			return fmt.Errorf("%w: intended conditioning intervals are reordered or overlapping", ErrConditioningOutput)
		}
		if cuts[i].Intended != want {
			return fmt.Errorf("%w: measured conditioning interval identity differs", ErrConditioningOutput)
		}
		previousEnd = want.EndMs
	}
	return nil
}

// DeriveConditioningParentEdges projects directly matched pre-rewrite parent edges through the
// measured timeline shift of the same closed stream identities. It never changes AfterRewrite's
// raw, unavailable cut evidence.
func DeriveConditioningParentEdges(before, after ConditioningMeasurement) (ConditioningCutMeasurement, error) {
	if err := ValidateConditioningPair(before, after); err != nil {
		return ConditioningCutMeasurement{}, err
	}
	type streamID struct {
		kind  StreamKind
		index int
	}
	beforeStreams := make(map[streamID]ConditioningStream, len(before.Streams))
	beforeEdges := make(map[streamID]ConditioningCutStream, len(before.Cuts[0].Streams))
	for _, stream := range before.Streams {
		beforeStreams[streamID{kind: stream.Kind, index: stream.Index}] = stream
	}
	for _, edge := range before.Cuts[0].Streams {
		beforeEdges[streamID{kind: edge.Kind, index: edge.Index}] = edge
	}
	derived := ConditioningCutMeasurement{Intended: before.Cuts[0].Intended, Streams: make([]ConditioningCutStream, 0, len(after.Streams))}
	for _, stream := range after.Streams {
		id := streamID{kind: stream.Kind, index: stream.Index}
		prior, ok := beforeStreams[id]
		edge, edgeOK := beforeEdges[id]
		if !ok || !edgeOK {
			return ConditioningCutMeasurement{}, fmt.Errorf("%w: post-rewrite stream has no pre-rewrite edge", ErrConditioningOutput)
		}
		startDelta, ok := checkedMillisecondsSub(stream.Start.Milliseconds, prior.Start.Milliseconds)
		if !ok {
			return ConditioningCutMeasurement{}, fmt.Errorf("%w: derived start delta overflows", ErrConditioningOutput)
		}
		beforeEnd, beforeOK := checkedMillisecondsAdd(prior.Start.Milliseconds, prior.Duration.Milliseconds)
		afterEnd, afterOK := checkedMillisecondsAdd(stream.Start.Milliseconds, stream.Duration.Milliseconds)
		if !beforeOK || !afterOK {
			return ConditioningCutMeasurement{}, fmt.Errorf("%w: derived stream end overflows", ErrConditioningOutput)
		}
		endDelta, ok := checkedMillisecondsSub(afterEnd, beforeEnd)
		if !ok {
			return ConditioningCutMeasurement{}, fmt.Errorf("%w: derived end delta overflows", ErrConditioningOutput)
		}
		startError, startOK := checkedMillisecondsAdd(edge.StartError.Milliseconds, startDelta)
		endError, endOK := checkedMillisecondsAdd(edge.EndError.Milliseconds, endDelta)
		if !startOK || !endOK {
			return ConditioningCutMeasurement{}, fmt.Errorf("%w: derived parent edge overflows", ErrConditioningOutput)
		}
		derived.Streams = append(derived.Streams, ConditioningCutStream{
			Kind: stream.Kind, Index: stream.Index,
			StartError: OptionalMilliseconds{Milliseconds: startError, Available: true},
			EndError:   OptionalMilliseconds{Milliseconds: endError, Available: true},
		})
	}
	return derived, nil
}

func validPositiveTiming(value OptionalMilliseconds) bool {
	return value.Available && value.Milliseconds > 0
}

func validOptionalTiming(value OptionalMilliseconds) bool {
	return value.Available || value.Milliseconds == 0
}

func validateCanonicalIntervals(spans []Interval, durationMs int64) error {
	var previous Interval
	for i, span := range spans {
		if span.StartMs < 0 || span.EndMs <= span.StartMs || span.EndMs > durationMs {
			return fmt.Errorf("%w: detector interval is out of bounds", ErrConditioningOutput)
		}
		if i > 0 && span.StartMs <= previous.EndMs {
			return fmt.Errorf("%w: detector intervals are not normalized", ErrConditioningOutput)
		}
		previous = span
	}
	return nil
}

func checkedMillisecondsAdd(a, b int64) (int64, bool) {
	if (b > 0 && a > math.MaxInt64-b) || (b < 0 && a < math.MinInt64-b) {
		return 0, false
	}
	return a + b, true
}

func checkedMillisecondsSub(a, b int64) (int64, bool) {
	if b == math.MinInt64 {
		return 0, false
	}
	return checkedMillisecondsAdd(a, -b)
}
