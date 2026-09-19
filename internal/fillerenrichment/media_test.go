package fillerenrichment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerenrichment"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

func TestPlanMediaWork_OnlyRequestsCapabilitiesThatCanCloseAnUnresolvedAxis(t *testing.T) {
	now := time.Unix(500, 0).UTC()
	operatorEmpty := fillerenrichment.State{
		ClipHash: "clip", Axis: fillerenrichment.AxisAudience, Status: fillerenrichment.StatusComplete,
		Evidence: fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceOperator, Reference: "operator",
			Confidence: 100, Producer: "operator", ProducerVersion: "1", ObservedAt: now},
	}
	product := fillerenrichment.State{
		ClipHash: "clip", Axis: fillerenrichment.AxisProduct, Status: fillerenrichment.StatusComplete,
		Value: fillerenrichment.Value{Tags: []string{"candy"}},
		Evidence: fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceItem, Reference: "item.title",
			Confidence: 100, Producer: "metadata", ProducerVersion: "1", ObservedAt: now},
	}
	work := fillerenrichment.PlanMediaWork(
		fillerenrichment.Candidate{ClipHash: "clip"},
		[]fillerenrichment.State{operatorEmpty, product},
		fillerenrichment.MediaCapabilities{Transcript: true, Vision: true},
	)
	if !work.Transcript || !work.Vision {
		t.Fatalf("work = %+v, want both capabilities for the still-unresolved descriptive axes", work)
	}

	states := make([]fillerenrichment.State, 0, len(fillerenrichment.Axes()))
	for _, axis := range fillerenrichment.Axes() {
		states = append(states, fillerenrichment.State{
			ClipHash: "clip", Axis: axis, Status: fillerenrichment.StatusComplete,
			Evidence: fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceOperator, Reference: "operator",
				Confidence: 100, Producer: "operator", ProducerVersion: "1", ObservedAt: now},
		})
	}
	work = fillerenrichment.PlanMediaWork(fillerenrichment.Candidate{ClipHash: "clip"}, states,
		fillerenrichment.MediaCapabilities{Transcript: true, Vision: true})
	if work.Transcript || work.Vision {
		t.Fatalf("operator-complete work = %+v, want no automated catch-up", work)
	}
}

func TestCapabilityRunner_BoundsReadyCatchupAndStampsOnlyCompletedWork(t *testing.T) {
	now := time.Unix(600, 0).UTC()
	repository := &runnerRepository{
		candidates: []fillerenrichment.Candidate{
			{ClipHash: "first", Path: "first.mp4"},
			{ClipHash: "broken", Path: "broken.mp4"},
			{ClipHash: "outside-bound", Path: "outside.mp4"},
		},
		states: map[string][]fillerenrichment.State{},
	}
	var executed []string
	runner := fillerenrichment.NewCapabilityRunner(
		fillerenrichment.CapabilityTranscript,
		repository,
		func() fillerenrichment.CapabilitySelection {
			return fillerenrichment.CapabilitySelection{
				Available: true, Producer: "transcript:local", ProducerVersion: "whisper-v1:model-a",
			}
		},
		func(_ context.Context, candidate fillerenrichment.Candidate) (fillerenrichment.CapabilityResult, error) {
			executed = append(executed, candidate.ClipHash)
			if candidate.ClipHash == "broken" {
				return fillerenrichment.CapabilityResult{}, errors.New("provider unavailable")
			}
			transcript := "spoken words"
			return fillerenrichment.CapabilityResult{Observation: &fillerenrichment.MediaObservation{
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

func TestCapabilityRunner_IsQuietUntilCapabilityExists(t *testing.T) {
	repository := &runnerRepository{candidates: []fillerenrichment.Candidate{{ClipHash: "ready"}}}
	runner := fillerenrichment.NewCapabilityRunner(
		fillerenrichment.CapabilityVision, repository,
		func() fillerenrichment.CapabilitySelection { return fillerenrichment.CapabilitySelection{} },
		func(context.Context, fillerenrichment.Candidate) (fillerenrichment.CapabilityResult, error) {
			t.Fatal("vision executed while unavailable")
			return fillerenrichment.CapabilityResult{}, nil
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
		candidates: []fillerenrichment.Candidate{{ClipHash: "ready", Path: "ready.mp4"}},
		states:     map[string][]fillerenrichment.State{},
	}
	capability := fillerenrichment.NewCapabilityRunner(
		fillerenrichment.CapabilityTranscript, repository,
		func() fillerenrichment.CapabilitySelection {
			return fillerenrichment.CapabilitySelection{Available: true, Producer: "transcript:local", ProducerVersion: "v1"}
		},
		func(context.Context, fillerenrichment.Candidate) (fillerenrichment.CapabilityResult, error) {
			return fillerenrichment.CapabilityResult{}, nil
		},
		func() int { return 1 }, func() time.Time { return now },
	)
	deterministic := fillerenrichment.NewRunner(repository,
		func(context.Context, fillerenrichment.Candidate, time.Time) (fillerenrichment.Signals, error) {
			return fillerenrichment.Signals{}, nil
		}, func() int { return 1 }, func() time.Time { return now })
	coordinator := fillerenrichment.NewCoordinator(deterministic, repository,
		func() fillerenrichment.TextSelection { return fillerenrichment.TextSelection{} }, nil, nil, nil).
		WithCapabilities(capability)

	result, err := coordinator.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Considered != 2 {
		t.Fatalf("result = %+v", result)
	}
	if len(repository.queries) != 2 || repository.queries[0] != "transcript:local|v1|" ||
		repository.queries[1] != fillerenrichment.DeterministicProducer+"|"+
			fillerenrichment.DeterministicProducerVersion+"|"+fillerenrichment.ControlledTaxonomyVersion {
		t.Fatalf("query order = %#v", repository.queries)
	}
}

func TestCapabilityRunner_BindsVisionFactsToProviderAndLiveTaxonomy(t *testing.T) {
	now := time.Unix(800, 0).UTC()
	repository := &runnerRepository{
		candidates: []fillerenrichment.Candidate{{ClipHash: "ready", Path: "ready.mp4"}},
		states:     map[string][]fillerenrichment.State{},
		taxa:       []taxonomy.Taxon{{Slug: "animated", Label: "Animated", Axis: taxonomy.AxisPresentation}},
	}
	runner := fillerenrichment.NewCapabilityRunner(
		fillerenrichment.CapabilityVision, repository,
		func() fillerenrichment.CapabilitySelection {
			return fillerenrichment.CapabilitySelection{Available: true, Producer: "vision:openrouter", ProducerVersion: "vision-v2:model"}
		},
		func(context.Context, fillerenrichment.Candidate) (fillerenrichment.CapabilityResult, error) {
			return fillerenrichment.CapabilityResult{States: []fillerenrichment.State{{Axis: fillerenrichment.AxisPresentation,
				Status: fillerenrichment.StatusComplete, Value: fillerenrichment.Value{Tags: []string{"animated"}},
				Evidence: fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceContent,
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
