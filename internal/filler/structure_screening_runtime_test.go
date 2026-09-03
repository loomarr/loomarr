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
	axis     SegmentScreeningAxis
	recorded RecordedSegmentScreeningAxisEvidence
	err      error
	order    *[]string
	seen     []SegmentScreeningMedia
}

func (e *capturedScreeningEvaluator) Axis() SegmentScreeningAxis { return e.axis }

func (e *capturedScreeningEvaluator) Evaluate(_ context.Context, media SegmentScreeningMedia) (RecordedSegmentScreeningAxisEvidence, error) {
	e.seen = append(e.seen, media)
	*e.order = append(*e.order, string(e.axis)+":"+spanLabel(media.StartMs, media.EndMs))
	recorded := e.recorded
	recorded.Evidence.Source = media.Source
	recorded.Evidence.StartMs = media.StartMs
	recorded.Evidence.EndMs = media.EndMs
	recorded.Evidence.SHA256 = SegmentScreeningAxisEvidenceSHA256(recorded.Evidence)
	return recorded, e.err
}

type capturedScreeningEvidenceRepository struct {
	order     *[]string
	axis      []RecordedSegmentScreeningAxisEvidence
	evidence  []SegmentScreeningEvidence
	axisErrAt int
	axisErr   error
	errAt     int
	err       error
}

func (r *capturedScreeningEvidenceRepository) PutSegmentScreeningAxisEvidence(_ context.Context, recorded RecordedSegmentScreeningAxisEvidence) error {
	if r.axisErr != nil && len(r.axis) == r.axisErrAt {
		return r.axisErr
	}
	*r.order = append(*r.order, "persist:"+string(recorded.Evidence.Profile.Axis)+":"+spanLabel(recorded.Evidence.StartMs, recorded.Evidence.EndMs))
	r.axis = append(r.axis, recorded)
	return nil
}

func TestExactSpanScreeningRuntimeDoesNotAdvancePastUnpersistedAxisEvidence(t *testing.T) {
	source := structureSource(30_000)
	order := []string{}
	items := screeningEvaluatorFixtures(&order)
	want := errors.New("axis evidence unavailable")
	repository := &capturedScreeningEvidenceRepository{order: &order, axisErr: want, axisErrAt: 1}
	runtime := mustScreeningRuntime(t, items, repository)
	if _, err := runtime.Screen(t.Context(), screeningRuntimeMedia(source)); !errors.Is(err, want) {
		t.Fatalf("error=%v, want axis persistence failure", err)
	}
	wantOrder := []string{
		"visual_safety:0..30000", "persist:visual_safety:0..30000", "spoken_safety:0..30000",
	}
	if !slices.Equal(order, wantOrder) || len(repository.axis) != 1 || len(repository.evidence) != 0 {
		t.Fatalf("unpersisted axis advanced runtime: order=%v axis=%d aggregates=%d", order, len(repository.axis), len(repository.evidence))
	}
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
		"visual_safety:0..30000", "persist:visual_safety:0..30000", "spoken_safety:0..30000", "persist:spoken_safety:0..30000",
		"rights:0..30000", "persist:rights:0..30000", "playback_integrity:0..30000", "persist:playback_integrity:0..30000", "persist:0..30000",
		"visual_safety:30000..60000", "persist:visual_safety:30000..60000", "spoken_safety:30000..60000", "persist:spoken_safety:30000..60000",
		"rights:30000..60000", "persist:rights:30000..60000", "playback_integrity:30000..60000", "persist:playback_integrity:30000..60000", "persist:30000..60000",
	}
	if !slices.Equal(order, wantOrder) || len(got) != 2 || len(repository.axis) != 8 || len(repository.evidence) != 2 {
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
			items[ScreenSpokenSafety].recorded.Evidence.Profile.Axis = ScreenVisualSafety
		}},
		{name: "invalid authority", mutate: func(items map[SegmentScreeningAxis]*capturedScreeningEvaluator) {
			items[ScreenSpokenSafety].recorded.RawEvidence = nil
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
	items[ScreenRights].recorded.Evidence.Outcome = ScreenHold
	items[ScreenRights].recorded.Evidence.SHA256 = SegmentScreeningAxisEvidenceSHA256(items[ScreenRights].recorded.Evidence)
	want := errors.New("evidence unavailable")
	repository := &capturedScreeningEvidenceRepository{order: &order, err: want, errAt: 1}
	runtime := mustScreeningRuntime(t, items, repository)
	media := screeningRuntimeMedia(source)
	media.Intervals = append(media.Intervals, StructurePlanSegment{StartMs: 30_000, EndMs: 60_000, Disposition: StructureKeep})
	media.Intervals = append(media.Intervals, StructurePlanSegment{StartMs: 60_000, EndMs: 90_000, Disposition: StructureKeep})
	if _, err := runtime.Screen(t.Context(), media); !errors.Is(err, want) {
		t.Fatalf("error=%v, want persistence failure", err)
	}
	if len(repository.evidence) != 1 || repository.evidence[0].Passes() || len(repository.axis) != 8 || slices.ContainsFunc(order, func(value string) bool { return strings.Contains(value, "60000..90000") }) {
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
		profile := screeningProfileFixture(axis, string(rune('1'+index)))
		recorded, err := NewSegmentScreeningAxisEvidence(
			structureSource(90_000), 0, 30_000, profile, ScreenPass, "authority_clear",
			[]byte("captured-"+string(axis)), time.Date(2026, time.September, 12, 2, 0, 0, 0, time.UTC),
		)
		if err != nil {
			panic(err)
		}
		items[axis] = &capturedScreeningEvaluator{
			axis: axis, order: order, recorded: recorded,
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
