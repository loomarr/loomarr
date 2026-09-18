package fillerenrichment

import (
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	DeterministicProducer        = "deterministic-metadata"
	DeterministicProducerVersion = "1"
	ControlledTaxonomyVersion    = "seed-v2"
)

var DeterministicAxes = []Axis{
	AxisEra, AxisBrand, AxisGeography, AxisProduct, AxisFormat,
	AxisSeasonal, AxisAudienceCue, AxisPresentation,
}

type Signals struct {
	ClipHash     string
	Kind         string
	Title        string
	Description  string
	OriginalName string
	// UploadDate is retained so tests pin the rule that upload time never becomes broadcast era.
	UploadDate string
	SourceID   string
	Source     Geography
	ObservedAt time.Time
}

type phraseMapping struct {
	phrase  string
	brand   string
	product string
}

var phraseMappings = []phraseMapping{
	{phrase: "hp sauce", brand: "HP Sauce", product: "condiments"},
	{phrase: "tootsie pop", brand: "Tootsie Pop", product: "candy"},
}

var explicitYear = regexp.MustCompile(`\b(19[3-9][0-9]|20[0-3][0-9])\b`)
var fiveNetwork = regexp.MustCompile(`(?i)\b(?:broadcast|shown|aired)\b[^.\n]{0,80}\b(?:on\s+(?:channel\s+)?five|on\s+channel\s+5)\b`)

// AnalyzeDeterministic performs the always-available free pass over preserved metadata. It emits
// independent candidates; Apply owns precedence against existing state.
func AnalyzeDeterministic(signals Signals) []State {
	at := signals.ObservedAt.UTC()
	if at.IsZero() {
		at = time.Unix(1, 0).UTC()
	}
	itemText := strings.TrimSpace(strings.Join([]string{signals.Title, signals.Description, signals.OriginalName}, "\n"))
	lower := strings.ToLower(itemText)
	byAxis := make(map[Axis]State, len(DeterministicAxes))
	itemEvidence := func(reference string, confidence int) Evidence {
		return Evidence{Kind: EvidenceItem, Reference: reference, Confidence: confidence,
			Producer: DeterministicProducer, ProducerVersion: DeterministicProducerVersion, ObservedAt: at}
	}
	appendState := func(axis Axis, value Value, evidence Evidence) {
		byAxis[axis] = State{ClipHash: signals.ClipHash, Axis: axis, Status: StatusComplete, Value: value, Evidence: evidence}
	}
	for _, axis := range DeterministicAxes {
		evidence := itemEvidence("item.metadata_checked", 100)
		if axis == AxisProduct || axis == AxisFormat || axis == AxisSeasonal || axis == AxisAudienceCue || axis == AxisPresentation {
			evidence.TaxonomyVersion = ControlledTaxonomyVersion
		}
		appendState(axis, Value{}, evidence)
	}

	if years := uniqueYears(explicitYear.FindAllString(itemText, -1)); len(years) == 1 {
		appendState(AxisEra, Value{Year: years[0]}, itemEvidence("item.title_description_or_original_name", 100))
	}
	for _, mapping := range phraseMappings {
		if !strings.Contains(lower, mapping.phrase) {
			continue
		}
		appendState(AxisBrand, Value{Text: mapping.brand}, itemEvidence("item.title_or_description", 100))
		evidence := itemEvidence("trusted.product_mapping:"+mapping.phrase, 100)
		evidence.Kind = EvidenceTrustedMap
		evidence.TaxonomyVersion = ControlledTaxonomyVersion
		appendState(AxisProduct, Value{Tags: []string{mapping.product}}, evidence)
		break
	}
	if kind := strings.ToLower(strings.TrimSpace(signals.Kind)); kind != "" {
		format := map[string]string{
			"commercial": "commercial", "psa": "psa", "promo": "promo", "bumper": "bumper",
			"station_id": "ident", "interstitial": "interstitial",
		}[kind]
		if format != "" {
			evidence := itemEvidence("clip.role", 100)
			evidence.TaxonomyVersion = ControlledTaxonomyVersion
			appendState(AxisFormat, Value{Tags: []string{format}}, evidence)
		}
	}
	if strings.Contains(lower, "animated") || strings.Contains(lower, "animation") || strings.Contains(lower, "cartoon") {
		evidence := itemEvidence("item.title_or_description", 100)
		evidence.TaxonomyVersion = ControlledTaxonomyVersion
		appendState(AxisPresentation, Value{Tags: []string{"animated"}}, evidence)
	}
	seasonal := []struct{ phrase, slug string }{
		{"christmas", "christmas"}, {"xmas", "christmas"}, {"halloween", "halloween"},
		{"thanksgiving", "thanksgiving"}, {"back to school", "back-to-school"},
	}
	for _, mapping := range seasonal {
		if strings.Contains(lower, mapping.phrase) {
			evidence := itemEvidence("item.title_or_description", 100)
			evidence.TaxonomyVersion = ControlledTaxonomyVersion
			appendState(AxisSeasonal, Value{Tags: []string{mapping.slug}}, evidence)
			break
		}
	}
	if strings.Contains(lower, "for kids") || strings.Contains(lower, "children's") || strings.Contains(lower, "childrens") {
		evidence := itemEvidence("item.title_or_description", 100)
		evidence.TaxonomyVersion = ControlledTaxonomyVersion
		appendState(AxisAudienceCue, Value{Tags: []string{"kids-cue"}}, evidence)
	}
	if fiveNetwork.MatchString(itemText) {
		appendState(AxisGeography, Value{Geography: Geography{Scope: "national", Country: "GB", Network: "Five"}}, Evidence{
			Kind: EvidenceTrustedMap, Reference: "trusted.network_mapping:five-gb", Confidence: 100,
			Producer: DeterministicProducer, ProducerVersion: DeterministicProducerVersion, ObservedAt: at,
		})
	} else if signals.Source != (Geography{}) {
		appendState(AxisGeography, Value{Geography: signals.Source}, Evidence{
			Kind: EvidenceSourceDefault, Reference: "registered_source:" + strings.TrimSpace(signals.SourceID), Confidence: 100,
			Producer: DeterministicProducer, ProducerVersion: DeterministicProducerVersion, ObservedAt: at,
		})
	}
	out := make([]State, 0, len(byAxis))
	for _, state := range byAxis {
		out = append(out, state)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Axis < out[j].Axis })
	return out
}

func uniqueYears(raw []string) []int {
	seen := map[string]bool{}
	var out []int
	for _, year := range raw {
		if seen[year] {
			continue
		}
		seen[year] = true
		var value int
		for _, r := range year {
			value = value*10 + int(r-'0')
		}
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}
