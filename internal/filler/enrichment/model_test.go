package enrichment_test

import (
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/enrichment"
)

func evidence(kind enrichment.EvidenceKind, confidence int) enrichment.Evidence {
	return enrichment.Evidence{Kind: kind, Reference: "fixture", Confidence: confidence,
		Producer: "test", ProducerVersion: "1", ObservedAt: time.Unix(100, 0).UTC()}
}

func TestApply_PrefersEvidenceRankBeforeConfidence(t *testing.T) {
	current := enrichment.State{ClipHash: "clip", Axis: enrichment.AxisEra,
		Status: enrichment.StatusComplete, Value: enrichment.Value{Year: 1999}, Evidence: evidence(enrichment.EvidenceItem, 80)}
	weak := enrichment.State{ClipHash: "clip", Axis: enrichment.AxisEra,
		Status: enrichment.StatusComplete, Value: enrichment.Value{Year: 1980}, Evidence: evidence(enrichment.EvidenceInference, 100)}
	got, changed, err := enrichment.Apply(current, weak)
	if err != nil || changed || got.Value.Year != 1999 {
		t.Fatalf("Apply() = %+v, changed %v, err %v; weak inference replaced item fact", got, changed, err)
	}
	operator := weak
	operator.Value.Year = 2000
	operator.Evidence = evidence(enrichment.EvidenceOperator, 1)
	got, changed, err = enrichment.Apply(current, operator)
	if err != nil || !changed || got.Value.Year != 2000 {
		t.Fatalf("Apply(operator) = %+v, changed %v, err %v", got, changed, err)
	}
}

func TestApply_IsIdempotentAndRefreshesStaleAtSameRank(t *testing.T) {
	state := enrichment.State{ClipHash: "clip", Axis: enrichment.AxisBrand,
		Status: enrichment.StatusComplete, Value: enrichment.Value{Text: "Brambly Sauce"}, Evidence: evidence(enrichment.EvidenceItem, 100)}
	got, changed, err := enrichment.Apply(state, state)
	if err != nil || changed || got.Value.Text != "Brambly Sauce" {
		t.Fatalf("idempotent Apply = %+v, changed %v, err %v", got, changed, err)
	}
	stale := state
	stale.Status = enrichment.StatusStale
	refresh := state
	refresh.Evidence.ProducerVersion = "2"
	got, changed, err = enrichment.Apply(stale, refresh)
	if err != nil || !changed || got.Evidence.ProducerVersion != "2" {
		t.Fatalf("stale refresh = %+v, changed %v, err %v", got, changed, err)
	}
}

func TestApply_RefreshesAProducerVersionWithoutDiscardingAKnownValue(t *testing.T) {
	current := enrichment.State{ClipHash: "clip", Axis: enrichment.AxisProduct,
		Status: enrichment.StatusComplete, Value: enrichment.Value{Tags: []string{"candy"}},
		Evidence: evidence(enrichment.EvidenceTrustedMap, 100)}
	current.Evidence.Producer = "deterministic-metadata"
	current.Evidence.TaxonomyVersion = "seed-v2"
	next := current
	next.Value.Tags = []string{"food"}
	next.Evidence.ProducerVersion = "2"
	got, changed, err := enrichment.Apply(current, next)
	if err != nil || !changed || len(got.Value.Tags) != 1 || got.Value.Tags[0] != "food" {
		t.Fatalf("version refresh = %+v, changed %v, err %v", got, changed, err)
	}
	empty := next
	empty.Value = enrichment.Value{}
	empty.Evidence.ProducerVersion = "3"
	got, changed, err = enrichment.Apply(next, empty)
	if err != nil || changed || len(got.Value.Tags) != 1 || got.Value.Tags[0] != "food" {
		t.Fatalf("empty refresh discarded known value: %+v, changed %v, err %v", got, changed, err)
	}
}

func TestApply_SelectedTextProviderMayRefreshOlderInference(t *testing.T) {
	current := enrichment.State{ClipHash: "clip", Axis: enrichment.AxisAudience,
		Status: enrichment.StatusComplete, Value: enrichment.Value{Text: "general"},
		Evidence: evidence(enrichment.EvidenceInference, 80)}
	current.Evidence.Producer = "text-model:ollama"
	current.Evidence.ProducerVersion = "prompt:old-model"
	next := current
	next.Value.Text = "family"
	next.Evidence.Producer = "text-model:openrouter"
	next.Evidence.ProducerVersion = "prompt:new-model"
	got, changed, err := enrichment.Apply(current, next)
	if err != nil || !changed || got.Value.Text != "family" {
		t.Fatalf("selected-provider refresh = %+v, changed %v, err %v", got, changed, err)
	}
}

func TestApply_LaterOperatorCorrectionReplacesEarlierOperatorValue(t *testing.T) {
	current := enrichment.State{ClipHash: "clip", Axis: enrichment.AxisBrand,
		Status: enrichment.StatusComplete, Value: enrichment.Value{Text: "Old"},
		Evidence: evidence(enrichment.EvidenceOperator, 100)}
	next := current
	next.Value = enrichment.Value{}
	next.Evidence.ObservedAt = current.Evidence.ObservedAt.Add(time.Second)
	got, changed, err := enrichment.Apply(current, next)
	if err != nil || !changed || got.Value.Text != "" {
		t.Fatalf("operator correction = %+v, changed %v, err %v", got, changed, err)
	}
}

func TestState_CompleteMayRecordHonestEmpty(t *testing.T) {
	state := enrichment.State{ClipHash: "clip", Axis: enrichment.AxisPresentation,
		Status: enrichment.StatusComplete, Evidence: evidence(enrichment.EvidenceContent, 100)}
	if err := state.Validate(); err != nil {
		t.Fatalf("complete empty state rejected: %v", err)
	}
	invalid := state
	invalid.Status = enrichment.StatusMissing
	if err := invalid.Validate(); !errors.Is(err, enrichment.ErrInvalidState) {
		t.Fatalf("missing state with evidence error = %v, want ErrInvalidState", err)
	}
}

func TestState_CompleteGeographyRequiresALocation(t *testing.T) {
	state := enrichment.State{ClipHash: "clip", Axis: enrichment.AxisGeography,
		Status: enrichment.StatusComplete, Evidence: evidence(enrichment.EvidenceInference, 70)}
	if err := state.Validate(); !errors.Is(err, enrichment.ErrInvalidState) {
		t.Fatalf("complete empty geography error = %v, want ErrInvalidState", err)
	}
	state.Value.Geography = enrichment.Geography{Scope: "national", Country: "US"}
	if err := state.Validate(); err != nil {
		t.Fatalf("complete country rejected: %v", err)
	}
	state.Value.Geography = enrichment.Geography{}
	state.Evidence = evidence(enrichment.EvidenceOperator, 100)
	if err := state.Validate(); err != nil {
		t.Fatalf("intentional operator-empty geography rejected: %v", err)
	}
}

func TestState_RejectsUnknownKind(t *testing.T) {
	state := enrichment.State{ClipHash: "clip", Axis: enrichment.AxisKind,
		Status: enrichment.StatusComplete, Value: enrichment.Value{Text: "promo"},
		Evidence: evidence(enrichment.EvidenceInference, 80)}
	if err := state.Validate(); !errors.Is(err, enrichment.ErrInvalidState) {
		t.Fatalf("unknown kind error = %v, want ErrInvalidState", err)
	}
}

func TestPass_RejectsMissingState(t *testing.T) {
	pass := enrichment.Pass{
		ClipHash: "clip", Producer: "test", ProducerVersion: "1", CompletedAt: time.Unix(100, 0).UTC(),
		States: []enrichment.State{{ClipHash: "clip", Axis: enrichment.AxisEra, Status: enrichment.StatusMissing}},
	}
	if err := pass.Validate(); !errors.Is(err, enrichment.ErrInvalidState) {
		t.Fatalf("missing pass state error = %v, want ErrInvalidState", err)
	}
}

func statesByAxis(states []enrichment.State) map[enrichment.Axis]enrichment.State {
	out := make(map[enrichment.Axis]enrichment.State, len(states))
	for _, state := range states {
		out[state.Axis] = state
	}
	return out
}

func TestAnalyzeDeterministic_RecognizesRoleFromItemText(t *testing.T) {
	got := statesByAxis(enrichment.AnalyzeDeterministic(enrichment.Signals{
		ClipHash: "advert", Kind: "unclassified", Title: "Local Toy Advert", ObservedAt: time.Unix(100, 0).UTC(),
	}))
	if got[enrichment.AxisKind].Value.Text != "commercial" ||
		got[enrichment.AxisKind].Evidence.Reference != "item.title_description_or_original_name" {
		t.Fatalf("item-text kind = %+v", got[enrichment.AxisKind])
	}
	if tags := got[enrichment.AxisFormat].Value.Tags; len(tags) != 1 || tags[0] != "commercial" {
		t.Fatalf("item-text format = %+v", got[enrichment.AxisFormat])
	}
}

// #1670: a source that declares no country is no evidence of where its clips aired.
func TestAnalyzeDeterministic_UndeclaredSourceWithdrawsGeography(t *testing.T) {
	got := statesByAxis(enrichment.AnalyzeDeterministic(enrichment.Signals{
		ClipHash: "clip", Title: "Westbridge Bank commercial", SourceID: "archive:tv_ads",
	}))
	geo, ok := got[enrichment.AxisGeography]
	if !ok || geo.Status != enrichment.StatusUnsupported || geo.Value.Geography != (enrichment.Geography{}) {
		t.Fatalf("undeclared source geography = %+v (present %v), want an empty unsupported withdrawal", geo, ok)
	}
	if err := geo.Validate(); err != nil {
		t.Fatalf("withdrawal does not validate: %v", err)
	}
}

func TestAnalyzeDeterministic_DeclaredSourceLabelsAndAdvertWordingDoesNot(t *testing.T) {
	us := enrichment.Geography{Scope: "national", Country: "US"}
	declared := statesByAxis(enrichment.AnalyzeDeterministic(enrichment.Signals{
		ClipHash: "clip", Title: "Twirlpop commercial", SourceID: "archive:us", Source: us,
	}))
	if geo := declared[enrichment.AxisGeography]; geo.Status != enrichment.StatusComplete || geo.Value.Geography != us {
		t.Fatalf("declared US source geography = %+v", geo)
	}
	for _, title := range []string{"Lynxa Advert", "Wren's jelly TV adverts 1989", "ADVERT - Tumbleworths"} {
		advert := statesByAxis(enrichment.AnalyzeDeterministic(enrichment.Signals{
			ClipHash: "clip", Title: title, SourceID: "archive:us", Source: us,
		}))
		if geo := advert[enrichment.AxisGeography]; geo.Status != enrichment.StatusUnsupported || geo.Value.Geography.Country != "" {
			t.Fatalf("%q geography = %+v; British wording must not keep a US label", title, geo)
		}
	}
	// "advertisement" is ordinary US usage and must not trip the British cue.
	american := statesByAxis(enrichment.AnalyzeDeterministic(enrichment.Signals{
		ClipHash: "clip", Title: "Sparkra-Cola advertisement", SourceID: "archive:us", Source: us,
	}))
	if geo := american[enrichment.AxisGeography]; geo.Value.Geography != us {
		t.Fatalf("\"advertisement\" geography = %+v, want the declared US source", geo)
	}
}

func TestApply_WithdrawalRetractsOnlyItsOwnProducersEarlierAnswer(t *testing.T) {
	at := time.Unix(100, 0).UTC()
	stamp := enrichment.State{ClipHash: "clip", Axis: enrichment.AxisGeography,
		Status: enrichment.StatusComplete,
		Value:  enrichment.Value{Geography: enrichment.Geography{Scope: "national", Country: "US"}},
		Evidence: enrichment.Evidence{Kind: enrichment.EvidenceSourceDefault, Reference: "registered_source:a",
			Confidence: 100, Producer: enrichment.DeterministicProducer, ProducerVersion: "3", ObservedAt: at}}
	withdrawal := enrichment.State{ClipHash: "clip", Axis: enrichment.AxisGeography,
		Status: enrichment.StatusUnsupported,
		Evidence: enrichment.Evidence{Kind: enrichment.EvidenceSourceDefault, Reference: "registered_source:a",
			Confidence: 100, Producer: enrichment.DeterministicProducer, ProducerVersion: "4", ObservedAt: at.Add(time.Hour)}}
	got, changed, err := enrichment.Apply(stamp, withdrawal)
	if err != nil || !changed || got.Status != enrichment.StatusUnsupported {
		t.Fatalf("own earlier stamp = %+v, changed %v, err %v; want retracted", got, changed, err)
	}
	for name, other := range map[string]enrichment.Evidence{
		"content":   {Kind: enrichment.EvidenceContent, Reference: "frame", Confidence: 100, Producer: "vision", ProducerVersion: "1", ObservedAt: at},
		"inference": {Kind: enrichment.EvidenceInference, Reference: "context", Confidence: 90, Producer: "research", ProducerVersion: "1", ObservedAt: at},
		"operator":  {Kind: enrichment.EvidenceOperator, Reference: "operator", Confidence: 100, Producer: enrichment.DeterministicProducer, ProducerVersion: "3", ObservedAt: at},
	} {
		current := stamp
		current.Evidence = other
		got, changed, err := enrichment.Apply(current, withdrawal)
		if err != nil || changed || got.Value.Geography.Country != "US" {
			t.Fatalf("%s answer = %+v, changed %v, err %v; a withdrawal must not displace it", name, got, changed, err)
		}
	}
}

func TestApply_AnEmptyAnswerNeverErasesAKnownValueWhateverItsConfidence(t *testing.T) {
	current := enrichment.State{ClipHash: "clip", Axis: enrichment.AxisKind,
		Status: enrichment.StatusComplete, Value: enrichment.Value{Text: "commercial"},
		Evidence: evidence(enrichment.EvidenceInference, 60)}
	current.Evidence.Producer = "text-model:fixture"
	current.Evidence.ProducerVersion = "3"
	empty := current
	empty.Value = enrichment.Value{}
	empty.Evidence.ProducerVersion = "4"
	empty.Evidence.Confidence = 80
	got, changed, err := enrichment.Apply(current, empty)
	if err != nil || changed || got.Value.Text != "commercial" {
		t.Fatalf("Apply() = %+v, changed %v, err %v; a confident empty answer erased a known value", got, changed, err)
	}
	empty.Evidence.ProducerVersion = "3"
	got, changed, err = enrichment.Apply(current, empty)
	if err != nil || changed || got.Value.Text != "commercial" {
		t.Fatalf("same-version Apply() = %+v, changed %v, err %v", got, changed, err)
	}
}
