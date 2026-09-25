package fillerenrichment

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

type classifierProvider struct {
	content  string
	messages []llm.Message
	options  []llm.ChatOptions
}

func (p *classifierProvider) Name() string { return "fixture" }

func (p *classifierProvider) Chat(_ context.Context, messages []llm.Message, options llm.ChatOptions) (llm.Response, error) {
	if !options.JSONMode {
		panic("text enrichment did not request JSON mode")
	}
	p.messages = append([]llm.Message(nil), messages...)
	p.options = append(p.options, options)
	return llm.Response{Content: p.content}, nil
}

func TestClassifyText_GroundsIndependentAxesAndLiteralBrand(t *testing.T) {
	provider := &classifierProvider{content: `{
		"kind":"commercial","audience":"KIDS","brand":"tootsie pop","product":["sweets"],
		"format":["commercial"],"seasonal":["candy"],
		"audienceCue":["kids-cue"],"presentation":["animated"],"confidence":99
	}`}
	forest := taxonomy.New([]taxonomy.Taxon{
		{Slug: "candy", Label: "Candy", Axis: taxonomy.AxisProduct, Synonyms: []string{"sweets"}},
		{Slug: "commercial", Label: "Commercial", Axis: taxonomy.AxisFormat},
		{Slug: "kids-cue", Label: "Kids cue", Axis: taxonomy.AxisAudienceCue},
		{Slug: "animated", Label: "Animated", Axis: taxonomy.AxisPresentation},
	})
	observedAt := time.Unix(500, 0).UTC()
	states, err := classifyText(t.Context(), provider, forest, textAxes, Signals{
		ClipHash: "tootsie", Title: "Tootsie Pop Classic Commercial", ObservedAt: observedAt,
	}, "text-model:fixture", "prompt:model", "taxonomy:fixture")
	if err != nil {
		t.Fatal(err)
	}
	byAxis := make(map[Axis]State, len(states))
	for _, state := range states {
		byAxis[state.Axis] = state
		if state.Evidence.Confidence != 80 {
			t.Fatalf("uncapped model confidence on %s: %d", state.Axis, state.Evidence.Confidence)
		}
	}
	if got := byAxis[AxisBrand]; got.Value.Text != "tootsie pop" || got.Evidence.Kind != EvidenceItem {
		t.Fatalf("literal brand = %+v", got)
	}
	if got := byAxis[AxisProduct].Value.Tags; len(got) != 1 || got[0] != "candy" {
		t.Fatalf("synonym-grounded product = %#v", got)
	}
	if got := byAxis[AxisSeasonal].Value.Tags; len(got) != 0 {
		t.Fatalf("product slug crossed into seasonal axis: %#v", got)
	}
	if got := byAxis[AxisAudience].Value.Text; got != "kids" {
		t.Fatalf("audience = %q", got)
	}
	if got := byAxis[AxisKind].Value.Text; got != "commercial" {
		t.Fatalf("kind = %q", got)
	}
	if len(provider.messages) != 2 || strings.Contains(provider.messages[1].Content, "license") {
		t.Fatalf("unexpected prompt = %#v", provider.messages)
	}
	if len(provider.options) != 1 || provider.options[0].MaxTokens != textSingleMaxTokens ||
		provider.options[0].ReasoningEffort != "none" || provider.options[0].Temperature == nil || *provider.options[0].Temperature > 0.2 {
		t.Fatalf("single classification controls = %#v, want %d tokens and no reasoning", provider.options, textSingleMaxTokens)
	}
}

func TestClassifyTextBatch_BoundsTheCompleteEightClipResponse(t *testing.T) {
	provider := &classifierProvider{content: `{"items":[]}`}
	inputs := make([]textBatchInput, textModelBatchSize)
	for index := range inputs {
		inputs[index] = textBatchInput{
			Axes: []Axis{AxisKind},
			Signals: Signals{
				ClipHash: strings.Repeat(string(rune('a'+index)), 64),
				Title:    "Fixture",
			},
		}
	}
	_, _, err := classifyTextBatch(t.Context(), provider, taxonomy.New(nil), inputs,
		"text-model:fixture", "prompt:model", "taxonomy:fixture")
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.options) != 1 || provider.options[0].MaxTokens != textBatchMaxTokens ||
		provider.options[0].ReasoningEffort != "none" || provider.options[0].Temperature == nil || *provider.options[0].Temperature > 0.2 {
		t.Fatalf("batch classification controls = %#v, want %d tokens and no reasoning", provider.options, textBatchMaxTokens)
	}
}

func TestClassifyText_DropsInventedBrandAndDoesNotCreateEraOrGeography(t *testing.T) {
	provider := &classifierProvider{content: `{
		"audience":"general","brand":"Invented Corp","product":[],"format":[],
		"seasonal":[],"audienceCue":[],"presentation":[],"era":1970,"country":"US","confidence":70
	}`}
	states, err := classifyText(t.Context(), provider, taxonomy.New(nil), textAxes, Signals{
		ClipHash: "unknown", Title: "Classic commercial", ObservedAt: time.Unix(600, 0).UTC(),
	}, "text-model:fixture", "prompt:model", "taxonomy:fixture")
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range states {
		if state.Axis == AxisEra || state.Axis == AxisGeography {
			t.Fatalf("text model created forbidden axis: %+v", state)
		}
		if state.Axis == AxisBrand && state.Value.Text != "" {
			t.Fatalf("invented brand survived: %+v", state)
		}
	}
}

func TestClassifyText_InvalidJSONDoesNotProduceACompletedPass(t *testing.T) {
	provider := &classifierProvider{content: "not json"}
	_, err := classifyText(t.Context(), provider, taxonomy.New(nil), []Axis{AxisAudience}, Signals{
		ClipHash: "broken", Title: "Unknown", ObservedAt: time.Unix(700, 0).UTC(),
	}, "text-model:fixture", "prompt:model", "taxonomy:fixture")
	if err == nil || !strings.Contains(err.Error(), "not JSON") {
		t.Fatalf("invalid response error = %v", err)
	}
}

func TestClassifyText_AcceptsSingleTaxonomyValueFromJSONModeProvider(t *testing.T) {
	provider := &classifierProvider{content: `{
		"audience":["kids"],"brand":[],"product":"candy","format":"commercial",
		"seasonal":"","audienceCue":[],"presentation":"animated","confidence":0.72
	}`}
	forest := taxonomy.New([]taxonomy.Taxon{
		{Slug: "candy", Label: "Candy", Axis: taxonomy.AxisProduct},
		{Slug: "commercial", Label: "Commercial", Axis: taxonomy.AxisFormat},
		{Slug: "animated", Label: "Animated", Axis: taxonomy.AxisPresentation},
	})
	states, err := classifyText(t.Context(), provider, forest, textAxes, Signals{
		ClipHash: "scalar", Title: "Candy commercial", ObservedAt: time.Unix(650, 0).UTC(),
	}, "text-model:fixture", "prompt:model", "taxonomy:fixture")
	if err != nil {
		t.Fatal(err)
	}
	byAxis := make(map[Axis]State, len(states))
	for _, state := range states {
		byAxis[state.Axis] = state
	}
	if got := byAxis[AxisProduct].Value.Tags; len(got) != 1 || got[0] != "candy" {
		t.Fatalf("scalar product = %#v", got)
	}
	if got := byAxis[AxisPresentation].Value.Tags; len(got) != 1 || got[0] != "animated" {
		t.Fatalf("scalar presentation = %#v", got)
	}
	if got := byAxis[AxisAudience].Value.Text; got != "kids" {
		t.Fatalf("single-item audience = %q", got)
	}
	if got := byAxis[AxisAudience].Evidence.Confidence; got != 72 {
		t.Fatalf("fractional confidence = %d", got)
	}
}

func TestGroundedBrandToleratesFilenamePunctuationOnly(t *testing.T) {
	if !containsGroundedText("CampbellsSoupAdvert", "Campbell's") {
		t.Fatal("punctuation folding rejected a literal filename brand")
	}
	if containsGroundedText("CampbellsSoupAdvert", "Coca-Cola") {
		t.Fatal("punctuation folding accepted a brand whose letters are absent")
	}
}
