package filler

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

type capturedScreeningEvaluator struct {
	axis   SegmentScreeningAxis
	result SegmentScreeningResult
	err    error
	order  *[]string
	seen   []SegmentScreeningMedia
}

func (e *capturedScreeningEvaluator) Axis() SegmentScreeningAxis { return e.axis }

func (e *capturedScreeningEvaluator) Evaluate(_ context.Context, media SegmentScreeningMedia) (SegmentScreeningResult, error) {
	e.seen = append(e.seen, media)
	*e.order = append(*e.order, string(e.axis)+":"+spanLabel(media.StartMs, media.EndMs))
	return e.result, e.err
}

type capturedScreeningEvidenceRepository struct {
	order    *[]string
	evidence []SegmentScreeningEvidence
	errAt    int
	err      error
}

func (r *capturedScreeningEvidenceRepository) PutSegmentScreeningEvidence(_ context.Context, evidence SegmentScreeningEvidence) error {
	if r.err != nil && len(r.evidence) == r.errAt {
		return r.err
	}
	*r.order = append(*r.order, "persist:"+spanLabel(evidence.StartMs, evidence.EndMs))
	r.evidence = append(r.evidence, evidence)
	return nil
}

func TestExactSpanScreeningRuntimeCallsFourAxesSeriallyAndPersistsEachSpan(t *testing.T) {
	source := structureSource(60_000)
	order := []string{}
	evaluators := screeningEvaluatorFixtures(&order)
	repository := &capturedScreeningEvidenceRepository{order: &order}
	runtime, err := NewExactSpanScreeningRuntime([]SegmentScreeningEvaluator{
		evaluators[ScreenRights], evaluators[ScreenPlayback], evaluators[ScreenVisualSafety], evaluators[ScreenSpokenSafety],
	}, repository, func() time.Time { return time.Date(2026, time.September, 12, 3, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	media := StructureScreeningMedia{
		Source: source, FullPath: "/tmp/conditioned-compilation.mp4",
		Intervals: []StructurePlanSegment{
			{StartMs: 0, EndMs: 30_000, Disposition: StructureKeep},
			{StartMs: 30_000, EndMs: 60_000, Disposition: StructureKeep},
		},
	}
	got, err := runtime.Screen(t.Context(), media)
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := []string{
		"visual_safety:0..30000", "spoken_safety:0..30000", "rights:0..30000", "playback_integrity:0..30000", "persist:0..30000",
		"visual_safety:30000..60000", "spoken_safety:30000..60000", "rights:30000..60000", "playback_integrity:30000..60000", "persist:30000..60000",
	}
	if !slices.Equal(order, wantOrder) || len(got) != 2 || len(repository.evidence) != 2 {
		t.Fatalf("order=%v evidence=%+v", order, got)
	}
	for _, evaluator := range evaluators {
		if len(evaluator.seen) != 2 || evaluator.seen[0].Source != source || evaluator.seen[0].FullPath != media.FullPath {
			t.Fatalf("axis %q media=%+v", evaluator.axis, evaluator.seen)
		}
	}
}

func TestExactSpanScreeningRuntimeRejectsIncompleteOrDuplicateAxesBeforeCalls(t *testing.T) {
	order := []string{}
	evaluators := screeningEvaluatorFixtures(&order)
	repository := &capturedScreeningEvidenceRepository{order: &order}
	tests := []struct {
		name  string
		items []SegmentScreeningEvaluator
	}{
		{name: "missing", items: []SegmentScreeningEvaluator{evaluators[ScreenVisualSafety], evaluators[ScreenSpokenSafety], evaluators[ScreenRights]}},
		{name: "duplicate", items: []SegmentScreeningEvaluator{evaluators[ScreenVisualSafety], evaluators[ScreenSpokenSafety], evaluators[ScreenRights], evaluators[ScreenRights]}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewExactSpanScreeningRuntime(test.items, repository, time.Now); err == nil {
				t.Fatal("invalid evaluator set was accepted")
			}
		})
	}
	if len(order) != 0 {
		t.Fatalf("constructor called evaluators: %v", order)
	}
}

func TestExactSpanScreeningRuntimeFailsClosedOnAxisOrOperationalDrift(t *testing.T) {
	source := structureSource(30_000)
	tests := []struct {
		name   string
		mutate func(map[SegmentScreeningAxis]*capturedScreeningEvaluator)
	}{
		{name: "operational error", mutate: func(items map[SegmentScreeningAxis]*capturedScreeningEvaluator) {
			items[ScreenSpokenSafety].err = errors.New("settlement missing")
		}},
		{name: "axis drift", mutate: func(items map[SegmentScreeningAxis]*capturedScreeningEvaluator) {
			items[ScreenSpokenSafety].result.Axis = ScreenVisualSafety
		}},
		{name: "invalid authority", mutate: func(items map[SegmentScreeningAxis]*capturedScreeningEvaluator) {
			items[ScreenSpokenSafety].result.AuthoritySHA256 = ""
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			order := []string{}
			items := screeningEvaluatorFixtures(&order)
			test.mutate(items)
			repository := &capturedScreeningEvidenceRepository{order: &order}
			runtime := mustScreeningRuntime(t, items, repository)
			if _, err := runtime.Screen(t.Context(), screeningRuntimeMedia(source)); err == nil {
				t.Fatal("invalid axis result was accepted")
			}
			if len(repository.evidence) != 0 {
				t.Fatalf("invalid aggregate was persisted: %+v", repository.evidence)
			}
		})
	}
}

func TestExactSpanScreeningRuntimePersistsHoldAndStopsBeforeNextSpanOnPersistenceFailure(t *testing.T) {
	source := structureSource(90_000)
	order := []string{}
	items := screeningEvaluatorFixtures(&order)
	items[ScreenRights].result.Outcome = ScreenHold
	want := errors.New("evidence unavailable")
	repository := &capturedScreeningEvidenceRepository{order: &order, err: want, errAt: 1}
	runtime := mustScreeningRuntime(t, items, repository)
	media := screeningRuntimeMedia(source)
	media.Intervals = append(media.Intervals, StructurePlanSegment{StartMs: 30_000, EndMs: 60_000, Disposition: StructureKeep})
	media.Intervals = append(media.Intervals, StructurePlanSegment{StartMs: 60_000, EndMs: 90_000, Disposition: StructureKeep})
	if _, err := runtime.Screen(t.Context(), media); !errors.Is(err, want) {
		t.Fatalf("error=%v, want persistence failure", err)
	}
	if len(repository.evidence) != 1 || repository.evidence[0].Passes() || !slices.Equal(order[len(order)-5:], []string{
		"persist:0..30000", "visual_safety:30000..60000", "spoken_safety:30000..60000", "rights:30000..60000", "playback_integrity:30000..60000",
	}) {
		// The failed put does not append its own marker. The first aggregate is durable before the
		// second span starts, and the third span never executes after the failed second put.
		t.Fatalf("order=%v evidence=%+v", order, repository.evidence)
	}
}

func mustScreeningRuntime(t *testing.T, items map[SegmentScreeningAxis]*capturedScreeningEvaluator, repository StructureScreeningEvidenceRepository) *ExactSpanScreeningRuntime {
	t.Helper()
	runtime, err := NewExactSpanScreeningRuntime([]SegmentScreeningEvaluator{
		items[ScreenVisualSafety], items[ScreenSpokenSafety], items[ScreenRights], items[ScreenPlayback],
	}, repository, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func screeningEvaluatorFixtures(order *[]string) map[SegmentScreeningAxis]*capturedScreeningEvaluator {
	items := make(map[SegmentScreeningAxis]*capturedScreeningEvaluator, len(segmentScreeningAxisOrder))
	for index, axis := range segmentScreeningAxisOrder {
		items[axis] = &capturedScreeningEvaluator{
			axis: axis, order: order,
			result: SegmentScreeningResult{
				Axis: axis, Outcome: ScreenPass, AuthoritySHA256: strings.Repeat(string(rune('1'+index)), 64), ReasonCode: "authority_clear",
			},
		}
	}
	return items
}

func screeningRuntimeMedia(source SplitSourceAsset) StructureScreeningMedia {
	return StructureScreeningMedia{
		Source: source, FullPath: "/tmp/conditioned-compilation.mp4",
		Intervals: []StructurePlanSegment{{StartMs: 0, EndMs: 30_000, Disposition: StructureKeep}},
	}
}

func spanLabel(startMs, endMs int64) string {
	return fmt.Sprintf("%d..%d", startMs, endMs)
}
