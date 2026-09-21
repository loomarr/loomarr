package fillerenrichment_test

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerenrichment"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

type runnerRepository struct {
	candidates []fillerenrichment.Candidate
	passes     []fillerenrichment.Pass
	states     map[string][]fillerenrichment.State
	taxa       []taxonomy.Taxon
	queries    []string
}

func (r *runnerRepository) ListCandidates(_ context.Context, producer, version, taxonomyVersion string, _ int) ([]fillerenrichment.Candidate, error) {
	r.queries = append(r.queries, producer+"|"+version+"|"+taxonomyVersion)
	return r.candidates, nil
}
func (r *runnerRepository) ListCapabilityCandidates(_ context.Context, producer, version, taxonomyVersion string, _ int) ([]fillerenrichment.Candidate, error) {
	r.queries = append(r.queries, producer+"|"+version+"|"+taxonomyVersion)
	return r.candidates, nil
}
func (r *runnerRepository) ApplyPass(_ context.Context, pass fillerenrichment.Pass) (int, error) {
	r.passes = append(r.passes, pass)
	return len(pass.States), nil
}

func (r *runnerRepository) ListStates(_ context.Context, clipHash string) ([]fillerenrichment.State, error) {
	return append([]fillerenrichment.State(nil), r.states[clipHash]...), nil
}

func (r *runnerRepository) ListTaxa(context.Context) ([]taxonomy.Taxon, error) {
	return append([]taxonomy.Taxon(nil), r.taxa...), nil
}

type textProvider struct {
	responses []string
	errors    []error
	calls     [][]llm.Message
}

func (p *textProvider) Name() string { return "fixture" }

func (p *textProvider) Chat(_ context.Context, messages []llm.Message, _ llm.ChatOptions) (llm.Response, error) {
	p.calls = append(p.calls, append([]llm.Message(nil), messages...))
	if len(p.errors) > 0 {
		err := p.errors[0]
		p.errors = p.errors[1:]
		if err != nil {
			return llm.Response{}, err
		}
	}
	response := p.responses[0]
	p.responses = p.responses[1:]
	return llm.Response{Content: response}, nil
}

func TestCoordinator_OneBadModelAnswerDoesNotStarveLaterClips(t *testing.T) {
	now := time.Unix(400, 0).UTC()
	repository := &runnerRepository{
		candidates: []fillerenrichment.Candidate{{ClipHash: "bad", Name: "Bad"}, {ClipHash: "good", Name: "Good"}},
		states:     map[string][]fillerenrichment.State{},
	}
	provider := &textProvider{
		responses: []string{`{"items":[
			{"id":"bad","audience":"family","brand":"","product":[],"format":[],"presentation":[],"seasonal":[],"audienceCue":[],"confidence":"not-a-number"},
			{"id":"good","audience":"family","brand":"","product":[],"format":[],"presentation":[],"seasonal":[],"audienceCue":[],"confidence":70}
		]}`},
	}
	coordinator := fillerenrichment.NewCoordinator(nil, repository, func() fillerenrichment.TextSelection {
		return fillerenrichment.TextSelection{Provider: provider, ProviderName: "fixture", Model: "model"}
	}, func(_ context.Context, c fillerenrichment.Candidate, _ time.Time) (fillerenrichment.Signals, error) {
		return fillerenrichment.Signals{Title: c.Name}, nil
	}, func() int { return 10 }, func() time.Time { return now })
	result, err := coordinator.Run(context.Background())
	if err == nil || result.Failed != 1 || len(repository.passes) != 1 || repository.passes[0].ClipHash != "good" {
		t.Fatalf("result=%+v err=%v passes=%+v", result, err, repository.passes)
	}
}

func TestRunner_BoundsAndStampsTheDeterministicPass(t *testing.T) {
	repository := &runnerRepository{candidates: []fillerenrichment.Candidate{{ClipHash: "hp", Path: "hp.mp4", Name: "HP Sauce Advert", Kind: "commercial"}}}
	now := time.Unix(100, 0).UTC()
	runner := fillerenrichment.NewRunner(repository, func(_ context.Context, candidate fillerenrichment.Candidate, _ time.Time) (fillerenrichment.Signals, error) {
		return fillerenrichment.Signals{Description: "An advert for HP Sauce broadcast 1999 on Five."}, nil
	}, func() int { return 10 }, func() time.Time { return now })
	result, err := runner.Run(context.Background())
	if err != nil || result.Considered != 1 || result.Updated != len(fillerenrichment.DeterministicAxes) {
		t.Fatalf("Run() = %+v, err %v", result, err)
	}
	if len(repository.passes) != 1 {
		t.Fatalf("passes = %d", len(repository.passes))
	}
	pass := repository.passes[0]
	if pass.ClipHash != "hp" || pass.Producer != fillerenrichment.DeterministicProducer ||
		pass.ProducerVersion != fillerenrichment.DeterministicProducerVersion || !pass.CompletedAt.Equal(now) {
		t.Fatalf("pass = %+v", pass)
	}
}

func TestCoordinator_UsesOneModelCallOnlyForStillEmptyAxes(t *testing.T) {
	now := time.Unix(200, 0).UTC()
	repository := &runnerRepository{
		candidates: []fillerenrichment.Candidate{{
			ClipHash: "tootsie", Name: "Tootsie Pop Classic Commercial", Kind: "commercial",
		}},
		states: map[string][]fillerenrichment.State{
			"tootsie": {{
				ClipHash: "tootsie", Axis: fillerenrichment.AxisBrand, Status: fillerenrichment.StatusComplete,
				Value: fillerenrichment.Value{Text: "Tootsie Pop"}, Evidence: fillerenrichment.Evidence{
					Kind: fillerenrichment.EvidenceItem, Reference: "item.title", Confidence: 100,
					Producer: fillerenrichment.DeterministicProducer, ProducerVersion: fillerenrichment.DeterministicProducerVersion,
					ObservedAt: now,
				},
			}},
		},
		taxa: []taxonomy.Taxon{
			{Slug: "candy", Label: "Candy", Axis: taxonomy.AxisProduct},
			{Slug: "commercial", Label: "Commercial", Axis: taxonomy.AxisFormat},
			{Slug: "animated", Label: "Animated", Axis: taxonomy.AxisPresentation},
			{Slug: "kids-cue", Label: "Kids-oriented cue", Axis: taxonomy.AxisAudienceCue},
		},
	}
	provider := &textProvider{responses: []string{`{"items":[{
		"id":"tootsie","audience":"kids","brand":"A made-up brand","product":["candy"],
		"format":["commercial"],"presentation":["animated"],
		"seasonal":["candy"],"audienceCue":["kids-cue"],"confidence":88
	}]}`}}
	deterministic := fillerenrichment.NewRunner(repository, func(_ context.Context, c fillerenrichment.Candidate, _ time.Time) (fillerenrichment.Signals, error) {
		return fillerenrichment.Signals{Title: c.Name, Kind: c.Kind}, nil
	}, func() int { return 10 }, func() time.Time { return now })
	coordinator := fillerenrichment.NewCoordinator(deterministic, repository,
		func() fillerenrichment.TextSelection {
			return fillerenrichment.TextSelection{Provider: provider, ProviderName: "openrouter", Model: "fixture/model"}
		},
		func(_ context.Context, c fillerenrichment.Candidate, _ time.Time) (fillerenrichment.Signals, error) {
			return fillerenrichment.Signals{Title: c.Name, Kind: c.Kind}, nil
		}, func() int { return 10 }, func() time.Time { return now })

	result, err := coordinator.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Considered != 2 || len(provider.calls) != 1 || len(repository.passes) != 2 {
		t.Fatalf("result=%+v calls=%d passes=%d", result, len(provider.calls), len(repository.passes))
	}
	modelPass := repository.passes[1]
	if modelPass.Producer != "text-model:openrouter" || modelPass.TaxonomyVersion == "" {
		t.Fatalf("model pass identity = %+v", modelPass)
	}
	byAxis := map[fillerenrichment.Axis]fillerenrichment.State{}
	for _, state := range modelPass.States {
		byAxis[state.Axis] = state
	}
	if _, ok := byAxis[fillerenrichment.AxisBrand]; ok {
		t.Fatal("the completed brand axis was sent to the model again")
	}
	if got := byAxis[fillerenrichment.AxisAudience].Value.Text; got != "kids" {
		t.Fatalf("audience = %q", got)
	}
	if got := byAxis[fillerenrichment.AxisProduct].Value.Tags; len(got) != 1 || got[0] != "candy" {
		t.Fatalf("product = %#v", got)
	}
	if got := byAxis[fillerenrichment.AxisSeasonal].Value.Tags; len(got) != 0 {
		t.Fatalf("off-axis taxonomy answer survived grounding: %#v", got)
	}
	if _, ok := byAxis[fillerenrichment.AxisEra]; ok {
		t.Fatal("the text model must not infer an era")
	}
}

func TestCoordinator_BatchesSeveralClipsIntoOneModelRequest(t *testing.T) {
	now := time.Unix(250, 0).UTC()
	repository := &runnerRepository{
		candidates: []fillerenrichment.Candidate{
			{ClipHash: "first", Name: "First commercial"},
			{ClipHash: "second", Name: "Second commercial"},
		},
		states: map[string][]fillerenrichment.State{},
	}
	provider := &textProvider{responses: []string{
		`{"items":[
			{"id":"first","kind":"commercial","audience":"general","brand":"","product":[],"format":[],"seasonal":[],"audienceCue":[],"presentation":[],"confidence":70},
			{"id":"second","kind":"commercial","audience":"family","brand":"","product":[],"format":[],"seasonal":[],"audienceCue":[],"presentation":[],"confidence":70}
		]}`,
		`{"kind":"commercial","audience":"family","brand":"","product":[],"format":[],"seasonal":[],"audienceCue":[],"presentation":[],"confidence":70}`,
	}}
	coordinator := fillerenrichment.NewCoordinator(nil, repository, func() fillerenrichment.TextSelection {
		return fillerenrichment.TextSelection{Provider: provider, ProviderName: "fixture", Model: "model"}
	}, func(_ context.Context, candidate fillerenrichment.Candidate, _ time.Time) (fillerenrichment.Signals, error) {
		return fillerenrichment.Signals{Title: candidate.Name}, nil
	}, func() int { return 10 }, func() time.Time { return now })

	result, err := coordinator.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.calls) != 1 || len(repository.passes) != 2 || result.Failed != 0 {
		t.Fatalf("calls=%d passes=%d result=%+v, want one model batch and two clip passes", len(provider.calls), len(repository.passes), result)
	}
}

func TestCoordinator_IsQuietWithoutAConfiguredTextProvider(t *testing.T) {
	repository := &runnerRepository{}
	runner := fillerenrichment.NewRunner(repository, func(context.Context, fillerenrichment.Candidate, time.Time) (fillerenrichment.Signals, error) {
		return fillerenrichment.Signals{}, nil
	}, nil, nil)
	coordinator := fillerenrichment.NewCoordinator(runner, repository, func() fillerenrichment.TextSelection { return fillerenrichment.TextSelection{} }, nil, nil, nil)
	if _, err := coordinator.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(repository.queries) != 1 {
		t.Fatalf("candidate queries = %#v; absent provider should not create a model backlog query", repository.queries)
	}
}

func TestTextPass_OperatorEmptyAnswerIsNotReopened(t *testing.T) {
	now := time.Unix(300, 0).UTC()
	repository := &runnerRepository{
		candidates: []fillerenrichment.Candidate{{ClipHash: "operator", Name: "Unknown advert"}},
		states: map[string][]fillerenrichment.State{
			"operator": {{
				ClipHash: "operator", Axis: fillerenrichment.AxisAudience, Status: fillerenrichment.StatusComplete,
				Evidence: fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceOperator, Reference: "operator", Confidence: 100,
					Producer: "operator", ProducerVersion: "1", ObservedAt: now},
			}},
		},
		taxa: []taxonomy.Taxon{{Slug: "commercial", Label: "Commercial", Axis: taxonomy.AxisFormat}},
	}
	provider := &textProvider{responses: []string{`{"items":[{"id":"operator","audience":"kids","brand":"","product":[],"format":["commercial"],"presentation":[],"seasonal":[],"audienceCue":[],"confidence":80}]}`}}
	coordinator := fillerenrichment.NewCoordinator(nil, repository, func() fillerenrichment.TextSelection {
		return fillerenrichment.TextSelection{Provider: provider, ProviderName: "ollama", Model: "local"}
	}, func(_ context.Context, c fillerenrichment.Candidate, _ time.Time) (fillerenrichment.Signals, error) {
		return fillerenrichment.Signals{Title: c.Name}, nil
	}, func() int { return 10 }, func() time.Time { return now })
	if _, err := coordinator.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, state := range repository.passes[0].States {
		if state.Axis == fillerenrichment.AxisAudience {
			t.Fatal("an intentional operator-empty answer was reopened")
		}
	}
}
