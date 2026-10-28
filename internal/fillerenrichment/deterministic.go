package fillerenrichment

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/broadcaststation"
)

const (
	DeterministicProducer = "deterministic-metadata"
	// 4: undeclared sources no longer stamp the household's home country, and US source defaults
	// yield to British "advert" wording (#1670). The bump re-runs every clip so v3 stamps retract.
	// 5: a broadcast station the item names locates its station material (#2061), so Cleveland news
	// recorded off WEWS is local to Cleveland instead of national.
	DeterministicProducerVersion = "5"
	ControlledTaxonomyVersion    = "seed-v2"
)

var DeterministicAxes = []Axis{
	AxisKind, AxisEra, AxisBrand, AxisGeography, AxisProduct, AxisFormat,
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
	// Station is the one broadcast station the item's title, description, or visible text names on
	// the clip's date, resolved by the caller against the shipped directory; nil when none or several.
	Station     *broadcaststation.Match
	Transcript  string
	VisibleText string
	ObservedAt  time.Time
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

// britishAdvert is the standalone British word for a commercial ("Puma Advert", "TV adverts").
var britishAdvert = regexp.MustCompile(`(?i)\badverts?\b`)
var fiveNetwork = regexp.MustCompile(`(?i)\b(?:broadcast|shown|aired)\b[^.\n]{0,80}\b(?:on\s+(?:channel\s+)?five|on\s+channel\s+5)\b`)

// stationMaterial is a station's own programming: its news, weather, idents and sign-offs. A clip
// that is that is local to the station. Everything else credited to a station (a network promo, a
// national ad) only says where the recording was made (#2059 decision 6). The phrases are the
// segment names archive titles use for a station's own news; a bare "news", "weather" or "forecast"
// also appears in ad copy ("all-weather tyres", "network news tonight") and does not count, nor do
// network newscast names ("Nightly News", "Evening News") or a slogan's "sign off".
var stationMaterial = regexp.MustCompile(`(?i)\b(?:news\s?(?:update|brief|break|channel|center|centre)|(?:eyewitness|action|local)\s+news|weather\s+(?:forecast|update|report|segment)|forecast\s+update|meteorologist|station\s+(?:id|ident|identification|sign[- ]off))\b`)

// stationSubject is a title that leads with the station and names what of its own the clip is
// ("WEWS 5 Promo", "WJW 8 sign-off"): the station is the subject, not the place it was recorded.
// The call sign stays case-sensitive; the segment word is any case.
var stationSubject = regexp.MustCompile(`^\s*(?:(?:ABC|CBS|NBC|FOX|PBS|UPN|WB|CW|IND)/)?[KW][A-Z]{2,3}(?:-[A-Z]{2})?(?:\s+(?i:News\s?Channel\s?|Channel\s?)?[0-9]{1,2})?\s+(?i:news|weather|promo|ident|id|station id|sign[- ]?(?:on|off))\b`)

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
		// Unlike descriptive set axes, an empty geography is not a completed answer: it would
		// suppress later work while the household boundary still excludes the clip.
		if axis == AxisGeography {
			continue
		}
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
	kind := strings.ToLower(strings.TrimSpace(signals.Kind))
	kindReference := "clip.role"
	if kind == "" || kind == "unclassified" {
		kind = kindFromText(lower)
		kindReference = "item.title_description_or_original_name"
	}
	if kind != "" && kind != "unclassified" {
		appendState(AxisKind, Value{Text: kind}, itemEvidence(kindReference, 100))
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
	// withdrawGeography records "this pass has no location", as `unsupported` rather than an empty
	// `complete`, so later passes still look for one. It also retracts this producer's own earlier
	// answer when its rules change (Apply's withdrawal rule).
	withdrawGeography := func(evidence Evidence) {
		byAxis[AxisGeography] = State{ClipHash: signals.ClipHash, Axis: AxisGeography, Status: StatusUnsupported, Evidence: evidence}
	}
	sourceReference := "registered_source:" + strings.TrimSpace(signals.SourceID)
	station := signals.Station
	switch {
	case station != nil && (kind == "station_id" || stationMaterial.MatchString(itemText) ||
		stationSubject.MatchString(signals.Title) || stationSubject.MatchString(strings.ReplaceAll(signals.OriginalName, "_", " "))):
		appendState(AxisGeography, Value{Geography: Geography{Scope: "local", Country: station.Station.Country,
			Station: station.Station.ID, AirDate: station.AirDate}}, Evidence{
			Kind: EvidenceTrustedMap, Reference: "trusted.station_directory:" + station.Station.ID + ":" + station.CallSign, Confidence: 90,
			Producer: DeterministicProducer, ProducerVersion: DeterministicProducerVersion, ObservedAt: at,
		})
	case fiveNetwork.MatchString(itemText):
		appendState(AxisGeography, Value{Geography: Geography{Scope: "national", Country: "GB", Network: "Five"}}, Evidence{
			Kind: EvidenceTrustedMap, Reference: "trusted.network_mapping:five-gb", Confidence: 100,
			Producer: DeterministicProducer, ProducerVersion: DeterministicProducerVersion, ObservedAt: at,
		})
	case strings.EqualFold(signals.Source.Country, "US") && britishAdvert.MatchString(itemText):
		// "Advert" is British usage, so a US source default is contradicted by the item itself.
		// It is not evidence of GB either; the clip stays unknown for stronger evidence to settle.
		withdrawGeography(itemEvidence("item.british_wording:advert", 100))
	case station != nil && signals.Source == (Geography{}):
		// Recorded off the station, so it aired in the station's country; most such ads ran
		// nationwide. That is an inference, below any source or item fact, which research citing a
		// local advertiser can still overturn.
		appendState(AxisGeography, Value{Geography: Geography{Scope: "national", Country: station.Station.Country,
			Station: station.Station.ID, AirDate: station.AirDate}}, Evidence{
			Kind: EvidenceInference, Reference: "station_provenance:" + station.Station.ID + ":" + station.CallSign, Confidence: 60,
			Producer: DeterministicProducer, ProducerVersion: DeterministicProducerVersion, ObservedAt: at,
		})
	case signals.Source != (Geography{}):
		appendState(AxisGeography, Value{Geography: signals.Source}, Evidence{
			Kind: EvidenceSourceDefault, Reference: sourceReference, Confidence: 100,
			Producer: DeterministicProducer, ProducerVersion: DeterministicProducerVersion, ObservedAt: at,
		})
	case strings.TrimSpace(signals.SourceID) != "":
		// The source declares no coverage. Producer version 3 stamped the household's home country
		// here; this explicit withdrawal is what lets the re-run retract that stamp (see Apply).
		evidence := itemEvidence(sourceReference, 100)
		evidence.Kind = EvidenceSourceDefault
		withdrawGeography(evidence)
	}
	out := make([]State, 0, len(byAxis))
	for _, state := range byAxis {
		out = append(out, state)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Axis < out[j].Axis })
	return out
}

func kindFromText(lower string) string {
	patterns := []struct {
		kind    string
		phrases []string
	}{
		{"station_id", []string{"station id", "station ident"}},
		{"psa", []string{"public service announcement", " psa "}},
		{"bumper", []string{"bumper"}},
		{"trailer", []string{"trailer"}},
		{"interstitial", []string{"interstitial"}},
		{"commercial", []string{"commercial", "advertisement", " advert", " ad spot"}},
	}
	padded := " " + lower + " "
	for _, pattern := range patterns {
		for _, phrase := range pattern.phrases {
			if strings.Contains(padded, phrase) {
				return pattern.kind
			}
		}
	}
	return ""
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
