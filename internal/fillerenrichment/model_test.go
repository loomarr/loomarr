package fillerenrichment_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/fillerenrichment"
)

func evidence(kind fillerenrichment.EvidenceKind, confidence int) fillerenrichment.Evidence {
	return fillerenrichment.Evidence{Kind: kind, Reference: "fixture", Confidence: confidence,
		Producer: "test", ProducerVersion: "1", ObservedAt: time.Unix(100, 0).UTC()}
}

func TestApply_PrefersEvidenceRankBeforeConfidence(t *testing.T) {
	current := fillerenrichment.State{ClipHash: "clip", Axis: fillerenrichment.AxisEra,
		Status: fillerenrichment.StatusComplete, Value: fillerenrichment.Value{Year: 1999}, Evidence: evidence(fillerenrichment.EvidenceItem, 80)}
	weak := fillerenrichment.State{ClipHash: "clip", Axis: fillerenrichment.AxisEra,
		Status: fillerenrichment.StatusComplete, Value: fillerenrichment.Value{Year: 1980}, Evidence: evidence(fillerenrichment.EvidenceInference, 100)}
	got, changed, err := fillerenrichment.Apply(current, weak)
	if err != nil || changed || got.Value.Year != 1999 {
		t.Fatalf("Apply() = %+v, changed %v, err %v; weak inference replaced item fact", got, changed, err)
	}
	operator := weak
	operator.Value.Year = 2000
	operator.Evidence = evidence(fillerenrichment.EvidenceOperator, 1)
	got, changed, err = fillerenrichment.Apply(current, operator)
	if err != nil || !changed || got.Value.Year != 2000 {
		t.Fatalf("Apply(operator) = %+v, changed %v, err %v", got, changed, err)
	}
}

func TestApply_IsIdempotentAndRefreshesStaleAtSameRank(t *testing.T) {
	state := fillerenrichment.State{ClipHash: "clip", Axis: fillerenrichment.AxisBrand,
		Status: fillerenrichment.StatusComplete, Value: fillerenrichment.Value{Text: "HP Sauce"}, Evidence: evidence(fillerenrichment.EvidenceItem, 100)}
	got, changed, err := fillerenrichment.Apply(state, state)
	if err != nil || changed || got.Value.Text != "HP Sauce" {
		t.Fatalf("idempotent Apply = %+v, changed %v, err %v", got, changed, err)
	}
	stale := state
	stale.Status = fillerenrichment.StatusStale
	refresh := state
	refresh.Evidence.ProducerVersion = "2"
	got, changed, err = fillerenrichment.Apply(stale, refresh)
	if err != nil || !changed || got.Evidence.ProducerVersion != "2" {
		t.Fatalf("stale refresh = %+v, changed %v, err %v", got, changed, err)
	}
}

func TestApply_RefreshesAProducerVersionWithoutDiscardingAKnownValue(t *testing.T) {
	current := fillerenrichment.State{ClipHash: "clip", Axis: fillerenrichment.AxisProduct,
		Status: fillerenrichment.StatusComplete, Value: fillerenrichment.Value{Tags: []string{"candy"}},
		Evidence: evidence(fillerenrichment.EvidenceTrustedMap, 100)}
	current.Evidence.Producer = "deterministic-metadata"
	current.Evidence.TaxonomyVersion = "seed-v2"
	next := current
	next.Value.Tags = []string{"food"}
	next.Evidence.ProducerVersion = "2"
	got, changed, err := fillerenrichment.Apply(current, next)
	if err != nil || !changed || len(got.Value.Tags) != 1 || got.Value.Tags[0] != "food" {
		t.Fatalf("version refresh = %+v, changed %v, err %v", got, changed, err)
	}
	empty := next
	empty.Value = fillerenrichment.Value{}
	empty.Evidence.ProducerVersion = "3"
	got, changed, err = fillerenrichment.Apply(next, empty)
	if err != nil || changed || len(got.Value.Tags) != 1 || got.Value.Tags[0] != "food" {
		t.Fatalf("empty refresh discarded known value: %+v, changed %v, err %v", got, changed, err)
	}
}

func TestApply_SelectedTextProviderMayRefreshOlderInference(t *testing.T) {
	current := fillerenrichment.State{ClipHash: "clip", Axis: fillerenrichment.AxisAudience,
		Status: fillerenrichment.StatusComplete, Value: fillerenrichment.Value{Text: "general"},
		Evidence: evidence(fillerenrichment.EvidenceInference, 80)}
	current.Evidence.Producer = "text-model:ollama"
	current.Evidence.ProducerVersion = "prompt:old-model"
	next := current
	next.Value.Text = "family"
	next.Evidence.Producer = "text-model:openrouter"
	next.Evidence.ProducerVersion = "prompt:new-model"
	got, changed, err := fillerenrichment.Apply(current, next)
	if err != nil || !changed || got.Value.Text != "family" {
		t.Fatalf("selected-provider refresh = %+v, changed %v, err %v", got, changed, err)
	}
}

func TestApply_LaterOperatorCorrectionReplacesEarlierOperatorValue(t *testing.T) {
	current := fillerenrichment.State{ClipHash: "clip", Axis: fillerenrichment.AxisBrand,
		Status: fillerenrichment.StatusComplete, Value: fillerenrichment.Value{Text: "Old"},
		Evidence: evidence(fillerenrichment.EvidenceOperator, 100)}
	next := current
	next.Value = fillerenrichment.Value{}
	next.Evidence.ObservedAt = current.Evidence.ObservedAt.Add(time.Second)
	got, changed, err := fillerenrichment.Apply(current, next)
	if err != nil || !changed || got.Value.Text != "" {
		t.Fatalf("operator correction = %+v, changed %v, err %v", got, changed, err)
	}
}

func TestState_CompleteMayRecordHonestEmpty(t *testing.T) {
	state := fillerenrichment.State{ClipHash: "clip", Axis: fillerenrichment.AxisPresentation,
		Status: fillerenrichment.StatusComplete, Evidence: evidence(fillerenrichment.EvidenceContent, 100)}
	if err := state.Validate(); err != nil {
		t.Fatalf("complete empty state rejected: %v", err)
	}
	invalid := state
	invalid.Status = fillerenrichment.StatusMissing
	if err := invalid.Validate(); !errors.Is(err, fillerenrichment.ErrInvalidState) {
		t.Fatalf("missing state with evidence error = %v, want ErrInvalidState", err)
	}
}

func TestState_CompleteGeographyRequiresALocation(t *testing.T) {
	state := fillerenrichment.State{ClipHash: "clip", Axis: fillerenrichment.AxisGeography,
		Status: fillerenrichment.StatusComplete, Evidence: evidence(fillerenrichment.EvidenceInference, 70)}
	if err := state.Validate(); !errors.Is(err, fillerenrichment.ErrInvalidState) {
		t.Fatalf("complete empty geography error = %v, want ErrInvalidState", err)
	}
	state.Value.Geography = fillerenrichment.Geography{Scope: "national", Country: "US"}
	if err := state.Validate(); err != nil {
		t.Fatalf("complete country rejected: %v", err)
	}
	state.Value.Geography = fillerenrichment.Geography{}
	state.Evidence = evidence(fillerenrichment.EvidenceOperator, 100)
	if err := state.Validate(); err != nil {
		t.Fatalf("intentional operator-empty geography rejected: %v", err)
	}
}

func TestState_RejectsUnknownKind(t *testing.T) {
	state := fillerenrichment.State{ClipHash: "clip", Axis: fillerenrichment.AxisKind,
		Status: fillerenrichment.StatusComplete, Value: fillerenrichment.Value{Text: "promo"},
		Evidence: evidence(fillerenrichment.EvidenceInference, 80)}
	if err := state.Validate(); !errors.Is(err, fillerenrichment.ErrInvalidState) {
		t.Fatalf("unknown kind error = %v, want ErrInvalidState", err)
	}
}

func TestPass_RejectsMissingState(t *testing.T) {
	pass := fillerenrichment.Pass{
		ClipHash: "clip", Producer: "test", ProducerVersion: "1", CompletedAt: time.Unix(100, 0).UTC(),
		States: []fillerenrichment.State{{ClipHash: "clip", Axis: fillerenrichment.AxisEra, Status: fillerenrichment.StatusMissing}},
	}
	if err := pass.Validate(); !errors.Is(err, fillerenrichment.ErrInvalidState) {
		t.Fatalf("missing pass state error = %v, want ErrInvalidState", err)
	}
}

type sidecarFixture struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	UploadDate  string `json:"upload_date"`
	Loomarr     struct {
		OriginalName string `json:"originalName"`
		SourceID     string `json:"sourceId"`
	} `json:"loomarr"`
}

func readSignals(t *testing.T, name, hash string) fillerenrichment.Signals {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var fixture sidecarFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fillerenrichment.Signals{ClipHash: hash, Kind: "commercial", Title: fixture.Title,
		Description: fixture.Description, OriginalName: fixture.Loomarr.OriginalName,
		UploadDate: fixture.UploadDate, SourceID: fixture.Loomarr.SourceID, ObservedAt: time.Unix(100, 0).UTC()}
}

func statesByAxis(states []fillerenrichment.State) map[fillerenrichment.Axis]fillerenrichment.State {
	out := make(map[fillerenrichment.Axis]fillerenrichment.State, len(states))
	for _, state := range states {
		out[state.Axis] = state
	}
	return out
}

func TestAnalyzeDeterministic_TootsieUsesItemTextButNotUploadDate(t *testing.T) {
	got := statesByAxis(fillerenrichment.AnalyzeDeterministic(readSignals(t, "tootsie-pop.info.json", "tootsie")))
	if got[fillerenrichment.AxisKind].Value.Text != "commercial" {
		t.Fatalf("kind = %+v", got[fillerenrichment.AxisKind])
	}
	if got[fillerenrichment.AxisBrand].Value.Text != "Tootsie Pop" {
		t.Fatalf("brand = %+v", got[fillerenrichment.AxisBrand])
	}
	if tags := got[fillerenrichment.AxisProduct].Value.Tags; len(tags) != 1 || tags[0] != "candy" {
		t.Fatalf("product = %+v", got[fillerenrichment.AxisProduct])
	}
	if got[fillerenrichment.AxisEra].Value.Year != 0 || got[fillerenrichment.AxisEra].Status != fillerenrichment.StatusComplete {
		t.Fatalf("upload date became era or deterministic check was not recorded: %+v", got[fillerenrichment.AxisEra])
	}
	// The fixture's source declares no country: the pass records an empty withdrawal, never a
	// completed location, so later passes still look for one.
	if geo, exists := got[fillerenrichment.AxisGeography]; exists &&
		(geo.Status == fillerenrichment.StatusComplete || geo.Value.Geography != (fillerenrichment.Geography{})) {
		t.Fatalf("unknown geography was marked complete: %+v", geo)
	}
}

func TestAnalyzeDeterministic_RecognizesRoleFromItemText(t *testing.T) {
	got := statesByAxis(fillerenrichment.AnalyzeDeterministic(fillerenrichment.Signals{
		ClipHash: "advert", Kind: "unclassified", Title: "Local Toy Advert", ObservedAt: time.Unix(100, 0).UTC(),
	}))
	if got[fillerenrichment.AxisKind].Value.Text != "commercial" ||
		got[fillerenrichment.AxisKind].Evidence.Reference != "item.title_description_or_original_name" {
		t.Fatalf("item-text kind = %+v", got[fillerenrichment.AxisKind])
	}
	if tags := got[fillerenrichment.AxisFormat].Value.Tags; len(tags) != 1 || tags[0] != "commercial" {
		t.Fatalf("item-text format = %+v", got[fillerenrichment.AxisFormat])
	}
}

func TestAnalyzeDeterministic_HPKeepsBroadcastYearAndTrustedNetwork(t *testing.T) {
	got := statesByAxis(fillerenrichment.AnalyzeDeterministic(readSignals(t, "hp-sauce.info.json", "hp")))
	if got[fillerenrichment.AxisEra].Value.Year != 1999 || got[fillerenrichment.AxisBrand].Value.Text != "HP Sauce" {
		t.Fatalf("HP facts = era %+v, brand %+v", got[fillerenrichment.AxisEra], got[fillerenrichment.AxisBrand])
	}
	product := got[fillerenrichment.AxisProduct]
	if len(product.Value.Tags) != 1 || product.Value.Tags[0] != "condiments" {
		t.Fatalf("product = %+v", product)
	}
	geo := got[fillerenrichment.AxisGeography]
	if geo.Value.Geography.Country != "GB" || geo.Value.Geography.Network != "Five" || geo.Evidence.Kind != fillerenrichment.EvidenceTrustedMap {
		t.Fatalf("geography = %+v", geo)
	}
}

// #1670: a source that declares no country is no evidence of where its clips aired.
func TestAnalyzeDeterministic_UndeclaredSourceWithdrawsGeography(t *testing.T) {
	got := statesByAxis(fillerenrichment.AnalyzeDeterministic(fillerenrichment.Signals{
		ClipHash: "clip", Title: "NatWest commercial", SourceID: "archive:tv_ads",
	}))
	geo, ok := got[fillerenrichment.AxisGeography]
	if !ok || geo.Status != fillerenrichment.StatusUnsupported || geo.Value.Geography != (fillerenrichment.Geography{}) {
		t.Fatalf("undeclared source geography = %+v (present %v), want an empty unsupported withdrawal", geo, ok)
	}
	if err := geo.Validate(); err != nil {
		t.Fatalf("withdrawal does not validate: %v", err)
	}
}

func TestAnalyzeDeterministic_DeclaredSourceLabelsAndAdvertWordingDoesNot(t *testing.T) {
	us := fillerenrichment.Geography{Scope: "national", Country: "US"}
	declared := statesByAxis(fillerenrichment.AnalyzeDeterministic(fillerenrichment.Signals{
		ClipHash: "clip", Title: "Tootsie Pop commercial", SourceID: "archive:us", Source: us,
	}))
	if geo := declared[fillerenrichment.AxisGeography]; geo.Status != fillerenrichment.StatusComplete || geo.Value.Geography != us {
		t.Fatalf("declared US source geography = %+v", geo)
	}
	for _, title := range []string{"Puma Advert", "Bird's jelly TV adverts 1989", "ADVERT - Rumbelows"} {
		advert := statesByAxis(fillerenrichment.AnalyzeDeterministic(fillerenrichment.Signals{
			ClipHash: "clip", Title: title, SourceID: "archive:us", Source: us,
		}))
		if geo := advert[fillerenrichment.AxisGeography]; geo.Status != fillerenrichment.StatusUnsupported || geo.Value.Geography.Country != "" {
			t.Fatalf("%q geography = %+v; British wording must not keep a US label", title, geo)
		}
	}
	// "advertisement" is ordinary US usage and must not trip the British cue.
	american := statesByAxis(fillerenrichment.AnalyzeDeterministic(fillerenrichment.Signals{
		ClipHash: "clip", Title: "Coca-Cola advertisement", SourceID: "archive:us", Source: us,
	}))
	if geo := american[fillerenrichment.AxisGeography]; geo.Value.Geography != us {
		t.Fatalf("\"advertisement\" geography = %+v, want the declared US source", geo)
	}
}

func TestApply_WithdrawalRetractsOnlyItsOwnProducersEarlierAnswer(t *testing.T) {
	at := time.Unix(100, 0).UTC()
	stamp := fillerenrichment.State{ClipHash: "clip", Axis: fillerenrichment.AxisGeography,
		Status: fillerenrichment.StatusComplete,
		Value:  fillerenrichment.Value{Geography: fillerenrichment.Geography{Scope: "national", Country: "US"}},
		Evidence: fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceSourceDefault, Reference: "registered_source:a",
			Confidence: 100, Producer: fillerenrichment.DeterministicProducer, ProducerVersion: "3", ObservedAt: at}}
	withdrawal := fillerenrichment.State{ClipHash: "clip", Axis: fillerenrichment.AxisGeography,
		Status: fillerenrichment.StatusUnsupported,
		Evidence: fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceSourceDefault, Reference: "registered_source:a",
			Confidence: 100, Producer: fillerenrichment.DeterministicProducer, ProducerVersion: "4", ObservedAt: at.Add(time.Hour)}}
	got, changed, err := fillerenrichment.Apply(stamp, withdrawal)
	if err != nil || !changed || got.Status != fillerenrichment.StatusUnsupported {
		t.Fatalf("own earlier stamp = %+v, changed %v, err %v; want retracted", got, changed, err)
	}
	for name, other := range map[string]fillerenrichment.Evidence{
		"content":   {Kind: fillerenrichment.EvidenceContent, Reference: "frame", Confidence: 100, Producer: "vision", ProducerVersion: "1", ObservedAt: at},
		"inference": {Kind: fillerenrichment.EvidenceInference, Reference: "context", Confidence: 90, Producer: "research", ProducerVersion: "1", ObservedAt: at},
		"operator":  {Kind: fillerenrichment.EvidenceOperator, Reference: "operator", Confidence: 100, Producer: fillerenrichment.DeterministicProducer, ProducerVersion: "3", ObservedAt: at},
	} {
		current := stamp
		current.Evidence = other
		got, changed, err := fillerenrichment.Apply(current, withdrawal)
		if err != nil || changed || got.Value.Geography.Country != "US" {
			t.Fatalf("%s answer = %+v, changed %v, err %v; a withdrawal must not displace it", name, got, changed, err)
		}
	}
}
