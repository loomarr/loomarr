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
	if got[fillerenrichment.AxisBrand].Value.Text != "Tootsie Pop" {
		t.Fatalf("brand = %+v", got[fillerenrichment.AxisBrand])
	}
	if tags := got[fillerenrichment.AxisProduct].Value.Tags; len(tags) != 1 || tags[0] != "candy" {
		t.Fatalf("product = %+v", got[fillerenrichment.AxisProduct])
	}
	if got[fillerenrichment.AxisEra].Value.Year != 0 || got[fillerenrichment.AxisEra].Status != fillerenrichment.StatusComplete {
		t.Fatalf("upload date became era or deterministic check was not recorded: %+v", got[fillerenrichment.AxisEra])
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
