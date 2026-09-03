package filler

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"time"
)

var segmentScreeningAxisOrder = []SegmentScreeningAxis{
	ScreenVisualSafety,
	ScreenSpokenSafety,
	ScreenRights,
	ScreenPlayback,
}

type SegmentScreeningMedia struct {
	Source   SplitSourceAsset
	FullPath string
	StartMs  int64
	EndMs    int64
}

// SegmentScreeningEvaluator owns one repeat-safe, authority-bound axis operation. Repeating the
// same source and span must replay its closed result rather than repeat a possibly billed call.
type SegmentScreeningEvaluator interface {
	Axis() SegmentScreeningAxis
	Evaluate(context.Context, SegmentScreeningMedia) (SegmentScreeningResult, error)
}

type StructureScreeningEvidenceRepository interface {
	PutSegmentScreeningEvidence(context.Context, SegmentScreeningEvidence) error
}

// ExactSpanScreeningRuntime is the deep screening module used by the split stage. It prevents an
// aggregate adapter from manufacturing a blanket pass by requiring one evaluator per named axis.
type ExactSpanScreeningRuntime struct {
	evaluators []SegmentScreeningEvaluator
	evidence   StructureScreeningEvidenceRepository
	now        func() time.Time
}

func NewExactSpanScreeningRuntime(evaluators []SegmentScreeningEvaluator, evidence StructureScreeningEvidenceRepository, now func() time.Time) (*ExactSpanScreeningRuntime, error) {
	if len(evaluators) != len(segmentScreeningAxisOrder) || evidence == nil || now == nil {
		return nil, fmt.Errorf("exact-span screening runtime requires four evaluators, evidence repository, and clock")
	}
	byAxis := make(map[SegmentScreeningAxis]SegmentScreeningEvaluator, len(evaluators))
	for _, evaluator := range evaluators {
		if evaluator == nil {
			return nil, fmt.Errorf("exact-span screening runtime contains a nil evaluator")
		}
		axis := evaluator.Axis()
		if validateSegmentScreeningAxis(axis) != nil {
			return nil, fmt.Errorf("exact-span screening runtime contains an unknown axis %q", axis)
		}
		if _, duplicate := byAxis[axis]; duplicate {
			return nil, fmt.Errorf("exact-span screening runtime repeats axis %q", axis)
		}
		byAxis[axis] = evaluator
	}
	ordered := make([]SegmentScreeningEvaluator, 0, len(segmentScreeningAxisOrder))
	for _, axis := range segmentScreeningAxisOrder {
		evaluator, exists := byAxis[axis]
		if !exists {
			return nil, fmt.Errorf("exact-span screening runtime is missing axis %q", axis)
		}
		ordered = append(ordered, evaluator)
	}
	return &ExactSpanScreeningRuntime{evaluators: ordered, evidence: evidence, now: now}, nil
}

func (r *ExactSpanScreeningRuntime) Screen(ctx context.Context, media StructureScreeningMedia) ([]SegmentScreeningEvidence, error) {
	if r == nil || len(r.evaluators) != len(segmentScreeningAxisOrder) || r.evidence == nil || r.now == nil {
		return nil, fmt.Errorf("exact-span screening runtime is unavailable")
	}
	if err := validateStructureScreeningMedia(media); err != nil {
		return nil, err
	}
	completed := make([]SegmentScreeningEvidence, 0, len(media.Intervals))
	for intervalIndex, interval := range media.Intervals {
		axisResults := make([]SegmentScreeningResult, 0, len(r.evaluators))
		input := SegmentScreeningMedia{
			Source: media.Source, FullPath: media.FullPath,
			StartMs: interval.StartMs, EndMs: interval.EndMs,
		}
		for axisIndex, evaluator := range r.evaluators {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			result, err := evaluator.Evaluate(ctx, input)
			if err != nil {
				return nil, fmt.Errorf("screen interval %d axis %q produced no authority: %w", intervalIndex, evaluator.Axis(), err)
			}
			if result.Axis != segmentScreeningAxisOrder[axisIndex] || validateSegmentScreeningResult(result) != nil {
				return nil, fmt.Errorf("screen interval %d axis %q returned invalid or drifted evidence", intervalIndex, evaluator.Axis())
			}
			axisResults = append(axisResults, result)
		}
		evidence, err := NewSegmentScreeningEvidence(media.Source, interval.StartMs, interval.EndMs, axisResults, r.now())
		if err != nil {
			return nil, fmt.Errorf("assemble screen interval %d: %w", intervalIndex, err)
		}
		if err := r.evidence.PutSegmentScreeningEvidence(ctx, evidence); err != nil {
			return nil, fmt.Errorf("persist screen interval %d: %w", intervalIndex, err)
		}
		completed = append(completed, evidence)
	}
	return completed, nil
}

func validateStructureScreeningMedia(media StructureScreeningMedia) error {
	if err := media.Source.validate(); err != nil || !filepath.IsAbs(media.FullPath) || filepath.Clean(media.FullPath) != media.FullPath || len(media.Intervals) == 0 {
		return fmt.Errorf("exact-span screening media is invalid")
	}
	previousEnd := int64(-1)
	for index, interval := range media.Intervals {
		if interval.Disposition != StructureKeep || interval.StartMs < 0 || interval.EndMs <= interval.StartMs || interval.EndMs > media.Source.DurationMs || interval.StartMs < previousEnd {
			return fmt.Errorf("exact-span screening interval %d is invalid or unordered", index)
		}
		previousEnd = interval.EndMs
	}
	return nil
}

func validateSegmentScreeningAxis(axis SegmentScreeningAxis) error {
	if !slices.Contains(segmentScreeningAxisOrder, axis) {
		return fmt.Errorf("segment screening axis is invalid")
	}
	return nil
}

var _ ExactSpanScreeningDecisioner = (*ExactSpanScreeningRuntime)(nil)
