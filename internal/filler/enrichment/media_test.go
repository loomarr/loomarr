package enrichment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
)

func TestPlanMediaWork_OnlyRequestsCapabilitiesThatCanCloseAnUnresolvedAxis(t *testing.T) {
	now := time.Unix(500, 0).UTC()
	operatorEmpty := enrichment.State{
		ClipHash: "clip", Axis: enrichment.AxisAudience, Status: enrichment.StatusComplete,
		Evidence: enrichment.Evidence{Kind: enrichment.EvidenceOperator, Reference: "operator",
			Confidence: 100, Producer: "operator", ProducerVersion: "1", ObservedAt: now},
	}
	product := enrichment.State{
		ClipHash: "clip", Axis: enrichment.AxisProduct, Status: enrichment.StatusComplete,
		Value: enrichment.Value{Tags: []string{"candy"}},
		Evidence: enrichment.Evidence{Kind: enrichment.EvidenceItem, Reference: "item.title",
			Confidence: 100, Producer: "metadata", ProducerVersion: "1", ObservedAt: now},
	}
	work := enrichment.PlanMediaWork(
		enrichment.Candidate{ClipHash: "clip", Transcript: "an older transcript"},
		[]enrichment.State{operatorEmpty, product},
		enrichment.MediaCapabilities{Transcript: true, Vision: true},
	)
	if !work.Transcript || !work.Vision {
		t.Fatalf("work = %+v, want a newly selected capability to revisit still-unresolved axes", work)
	}

	weakAudience := enrichment.State{
		ClipHash: "clip", Axis: enrichment.AxisAudience, Status: enrichment.StatusComplete,
		Value: enrichment.Value{Text: "general"},
		Evidence: enrichment.Evidence{Kind: enrichment.EvidenceInference,
			Reference: "text", Confidence: 35, Producer: "text", ProducerVersion: "1", ObservedAt: now},
	}
	work = enrichment.PlanMediaWork(enrichment.Candidate{ClipHash: "clip"},
		[]enrichment.State{weakAudience}, enrichment.MediaCapabilities{Vision: true})
	if !work.Vision {
		t.Fatalf("work = %+v, want grounded frames to be able to improve weak inference", work)
	}

	states := make([]enrichment.State, 0, len(enrichment.Axes()))
	for _, axis := range enrichment.Axes() {
		states = append(states, enrichment.State{
			ClipHash: "clip", Axis: axis, Status: enrichment.StatusComplete,
			Evidence: enrichment.Evidence{Kind: enrichment.EvidenceOperator, Reference: "operator",
				Confidence: 100, Producer: "operator", ProducerVersion: "1", ObservedAt: now},
		})
	}
	work = enrichment.PlanMediaWork(enrichment.Candidate{ClipHash: "clip"}, states,
		enrichment.MediaCapabilities{Transcript: true, Vision: true})
	if work.Transcript || work.Vision {
		t.Fatalf("operator-complete work = %+v, want no automated catch-up", work)
	}
}

func TestCapabilityRunner_BoundsReadyCatchupAndStampsOnlyCompletedWork(t *testing.T) {
	now := time.Unix(600, 0).UTC()
	repository := &runnerRepository{
		candidates: []enrichment.Candidate{
			{ClipHash: "first", Path: "first.mp4"},
			{ClipHash: "broken", Path: "broken.mp4"},
			{ClipHash: "outside-bound", Path: "outside.mp4"},
		},
		states: map[string][]enrichment.State{},
	}
	var executed []string
	runner := enrichment.NewCapabilityRunner(
		enrichment.CapabilityTranscript,
		repository,
		func() enrichment.CapabilitySelection {
			return enrichment.CapabilitySelection{
				Available: true, Producer: "transcript:local", ProducerVersion: "whisper-v1:model-a",
			}
		},
		func(_ context.Context, candidate enrichment.Candidate) (enrichment.CapabilityResult, error) {
			executed = append(executed, candidate.ClipHash)
			if candidate.ClipHash == "broken" {
				return enrichment.CapabilityResult{}, errors.New("provider unavailable")
			}
			transcript := "spoken words"
			return enrichment.CapabilityResult{Observation: &enrichment.MediaObservation{
				Transcript: &transcript,
			}}, nil
		},
		func() int { return 2 }, func() time.Time { return now },
	)
	result, err := runner.Run(context.Background())
	if err == nil || result.Considered != 2 || result.Failed != 1 {
		t.Fatalf("Run() = %+v, err %v", result, err)
	}
	if len(executed) != 2 || executed[0] != "first" || executed[1] != "broken" {
		t.Fatalf("executed = %#v", executed)
	}
	if len(repository.passes) != 1 || repository.passes[0].ClipHash != "first" ||
		repository.passes[0].Producer != "transcript:local" || len(repository.passes[0].States) != 0 ||
		repository.passes[0].Observation == nil || repository.passes[0].Observation.Transcript == nil ||
		*repository.passes[0].Observation.Transcript != "spoken words" {
		t.Fatalf("completion passes = %+v", repository.passes)
	}
}

// Clips that keep failing back off instead of filling every slot of every pass (#2456): on the
// household install five such clips took all five slots for days and nothing else was enriched.
func TestCapabilityRunner_FailingClipsBackOffAndStopStarvingTheRest(t *testing.T) {
	clock := time.Unix(10_000, 0).UTC()
	repository := &runnerRepository{
		candidates: []enrichment.Candidate{
			{ClipHash: "bad-1"}, {ClipHash: "bad-2"}, {ClipHash: "good-1"}, {ClipHash: "good-2"},
		},
		states: map[string][]enrichment.State{},
	}
	var executed []string
	runner := enrichment.NewCapabilityRunner(enrichment.CapabilityTranscript, repository,
		func() enrichment.CapabilitySelection {
			return enrichment.CapabilitySelection{Available: true, Producer: "transcript:local", ProducerVersion: "v1"}
		},
		func(_ context.Context, c enrichment.Candidate) (enrichment.CapabilityResult, error) {
			executed = append(executed, c.ClipHash)
			if c.ClipHash == "bad-1" || c.ClipHash == "bad-2" {
				return enrichment.CapabilityResult{}, errors.New("ffmpeg semantic frame: exit status 161")
			}
			return enrichment.CapabilityResult{}, nil
		},
		func() int { return 2 }, func() time.Time { return clock },
	)
	pass := func(at time.Duration) []string {
		t.Helper()
		clock = time.Unix(10_000, 0).UTC().Add(at)
		executed = nil
		_, _ = runner.Run(context.Background())
		return executed
	}
	same := func(got []string, want ...string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	if got := pass(0); !same(got, "bad-1", "bad-2") {
		t.Fatalf("first pass ran %v, want the two listed first", got)
	}
	if got := pass(time.Minute); !same(got, "good-1", "good-2") {
		t.Fatalf("pass a minute later ran %v, want the failing clips skipped and the rest served", got)
	}
	if got := pass(11 * time.Minute); !same(got, "bad-1", "bad-2") {
		t.Fatalf("pass after 10 minutes ran %v, want the failing clips retried", got)
	}
	if got := pass(25 * time.Minute); !same(got, "good-1", "good-2") {
		t.Fatalf("pass 14 minutes after the second failure ran %v, want them still waiting (20 minutes)", got)
	}
	if got := pass(32 * time.Minute); !same(got, "bad-1", "bad-2") {
		t.Fatalf("pass after the doubled wait ran %v, want the failing clips retried", got)
	}
}

func TestCapabilityRunner_IsQuietUntilCapabilityExists(t *testing.T) {
	repository := &runnerRepository{candidates: []enrichment.Candidate{{ClipHash: "ready"}}}
	runner := enrichment.NewCapabilityRunner(
		enrichment.CapabilityVision, repository,
		func() enrichment.CapabilitySelection { return enrichment.CapabilitySelection{} },
		func(context.Context, enrichment.Candidate) (enrichment.CapabilityResult, error) {
			t.Fatal("vision executed while unavailable")
			return enrichment.CapabilityResult{}, nil
		},
		nil, nil,
	)
	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repository.queries) != 0 || len(repository.passes) != 0 {
		t.Fatalf("queries = %#v, passes = %#v", repository.queries, repository.passes)
	}
}

func TestCoordinator_RunsMediaCatchupBeforeRefreshingDeterministicSignals(t *testing.T) {
	now := time.Unix(700, 0).UTC()
	repository := &runnerRepository{
		candidates: []enrichment.Candidate{{ClipHash: "ready", Path: "ready.mp4"}},
		states:     map[string][]enrichment.State{},
	}
	capability := enrichment.NewCapabilityRunner(
		enrichment.CapabilityTranscript, repository,
		func() enrichment.CapabilitySelection {
			return enrichment.CapabilitySelection{Available: true, Producer: "transcript:local", ProducerVersion: "v1"}
		},
		func(context.Context, enrichment.Candidate) (enrichment.CapabilityResult, error) {
			return enrichment.CapabilityResult{}, nil
		},
		func() int { return 1 }, func() time.Time { return now },
	)
	deterministic := enrichment.NewRunner(repository,
		func(context.Context, enrichment.Candidate, time.Time) (enrichment.Signals, error) {
			return enrichment.Signals{}, nil
		}, func() int { return 1 }, func() time.Time { return now })
	coordinator := enrichment.NewCoordinator(deterministic, repository,
		func() enrichment.TextSelection { return enrichment.TextSelection{} }, nil, nil, nil).
		WithCapabilities(capability)

	result, err := coordinator.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Considered != 2 {
		t.Fatalf("result = %+v", result)
	}
	if len(repository.queries) != 2 || repository.queries[0] != "transcript:local|v1|" ||
		repository.queries[1] != enrichment.DeterministicProducer+"|"+
			enrichment.DeterministicProducerVersion+"|"+enrichment.ControlledTaxonomyVersion {
		t.Fatalf("query order = %#v", repository.queries)
	}
}

func TestCapabilityRunner_BindsVisionFactsToProviderAndLiveTaxonomy(t *testing.T) {
	now := time.Unix(800, 0).UTC()
	repository := &runnerRepository{
		candidates: []enrichment.Candidate{{ClipHash: "ready", Path: "ready.mp4"}},
		states:     map[string][]enrichment.State{},
		taxa:       []taxonomy.Taxon{{Slug: "animated", Label: "Animated", Axis: taxonomy.AxisPresentation}},
	}
	runner := enrichment.NewCapabilityRunner(
		enrichment.CapabilityVision, repository,
		func() enrichment.CapabilitySelection {
			return enrichment.CapabilitySelection{Available: true, Producer: "vision:openrouter", ProducerVersion: "vision-v2:model"}
		},
		func(context.Context, enrichment.Candidate) (enrichment.CapabilityResult, error) {
			return enrichment.CapabilityResult{States: []enrichment.State{{Axis: enrichment.AxisPresentation,
				Status: enrichment.StatusComplete, Value: enrichment.Value{Tags: []string{"animated"}},
				Evidence: enrichment.Evidence{Kind: enrichment.EvidenceContent,
					Reference: "frame.taxonomy_grounded", Confidence: 100}}}}, nil
		}, func() int { return 1 }, func() time.Time { return now },
	)
	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repository.passes) != 1 || len(repository.passes[0].States) != 1 {
		t.Fatalf("passes = %+v", repository.passes)
	}
	pass, state := repository.passes[0], repository.passes[0].States[0]
	if pass.TaxonomyVersion == "" || state.ClipHash != "ready" ||
		state.Evidence.Producer != pass.Producer || state.Evidence.ProducerVersion != pass.ProducerVersion ||
		state.Evidence.TaxonomyVersion != pass.TaxonomyVersion || !state.Evidence.ObservedAt.Equal(now) {
		t.Fatalf("bound pass/state = %+v / %+v", pass, state)
	}
}
