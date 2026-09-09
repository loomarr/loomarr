package suggest

import (
	"encoding/json"
	"strings"
	"unicode"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/textmatch"
)

// score describes only surviving grounded picks. Date evidence comes from the
// validated interpretation used by admission; there is no second date parser.
func score(intent Intent, lineup, acquisitions []ProposalItem, meaning ValidatedDateMeaning) Scores {
	items := append(append([]ProposalItem{}, lineup...), acquisitions...)
	s := Scores{Version: 1}
	s.ThemeFit, s.Theme = themeFit(themeIntent(intent, meaning), items)
	s.EraBalance, s.Era = eraAdherence(items, meaning)
	if len(items) > 0 {
		s.AvailabilityRatio = float64(len(lineup)) / float64(len(items))
	}
	return s
}

func themeFit(intent Intent, items []ProposalItem) (*float64, ThemeEvidence) {
	e := ThemeEvidence{Status: "unassessed", Basis: "none", Qualifiers: []QualifierEvidence{}}
	if requiresMembershipEvidence(intent) && allItemsHaveMembershipEvidence(intent, items, nil) {
		e.Status, e.Basis, e.AssessedItems = "supported", "named_membership", len(items)
		return scoreValue(1), e
	}
	terms := themeTerms(intent)
	if len(terms) == 0 {
		return nil, e
	}
	e.Basis = "qualifiers"
	for _, term := range terms {
		e.Qualifiers = append(e.Qualifiers, QualifierEvidence{Term: term})
	}
	hits := 0
	for _, item := range items {
		// A catalog title alone is too weak to establish a requested theme.
		if strings.TrimSpace(item.Overview+strings.Join(item.Genres, " ")+strings.Join(item.Keywords, " ")) == "" {
			e.UnknownItems++
			continue
		}
		e.AssessedItems++
		for i, term := range terms {
			if supportsQualifier(item, term) {
				e.Qualifiers[i].SupportedItems++
				hits++
			}
		}
	}
	if e.AssessedItems == 0 || e.UnknownItems > 0 {
		return nil, e
	}
	value := float64(hits) / float64(len(terms)*e.AssessedItems)
	e.Status = "partial"
	if value == 1 {
		e.Status = "supported"
	}
	return scoreValue(value), e
}

func allItemsHaveMembershipEvidence(intent Intent, lineup, acquisitions []ProposalItem) bool {
	items := append(append([]ProposalItem{}, lineup...), acquisitions...)
	if len(items) == 0 || len(intent.membershipKeys) == 0 {
		return false
	}
	for _, item := range items {
		key, err := item.Key()
		if err != nil || !intent.membershipKeys[key] {
			return false
		}
	}
	return true
}

// Explicit lexical equivalents make this diagnostic reproducible; they do not
// pretend to resolve unrestricted semantics. Model rationale is never evidence.
var qualifierEquivalents = map[string][]string{
	"cozy":    {"cozy", "cosy"},
	"mystery": {"mystery", "mysteries", "whodunit", "whodunits"},
	"sitcom":  {"sitcom", "sitcoms", "situation comedy"},
	"sci-fi":  {"sci-fi", "scifi", "science fiction"},
}

func canonicalQualifier(term string) string {
	for canonical, equivalents := range qualifierEquivalents {
		for _, equivalent := range equivalents {
			if term == equivalent {
				return canonical
			}
		}
	}
	return term
}

func supportsQualifier(item ProposalItem, term string) bool {
	hay := strings.Join(append(append([]string{item.Name, item.Overview}, item.Genres...), item.Keywords...), " ")
	variants := qualifierEquivalents[term]
	if len(variants) == 0 {
		variants = []string{term}
	}
	for _, variant := range variants {
		if textmatch.ContainsPhrase(hay, variant) {
			return true
		}
	}
	if term == "british" {
		for _, country := range item.OriginCountries {
			if country == "GB" || country == "UK" {
				return true
			}
		}
	}
	return false
}

func themeTerms(intent Intent) []string {
	text := strings.ToLower(strings.Join(append([]string{intent.Description, intent.Era, intent.Tone}, intent.MustInclude...), " "))
	// Normalize only the documented multiword equivalents before tokenization.
	text = strings.ReplaceAll(text, "science fiction", "sci-fi")
	text = strings.ReplaceAll(text, "situation comedy", "sitcom")
	stop := map[string]bool{"a": true, "an": true, "the": true, "of": true, "and": true, "for": true, "channel": true, "movie": true, "movies": true, "show": true, "shows": true, "series": true, "with": true, "from": true, "in": true}
	seen := map[string]bool{}
	var terms []string
	for _, word := range strings.FieldsFunc(text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' }) {
		term := canonicalQualifier(word)
		if stop[term] || seen[term] {
			continue
		}
		seen[term] = true
		terms = append(terms, term)
	}
	return terms
}

// Mask all date spans in place before extracting theme qualifiers. Rune offsets
// remain stable for overlapping anchors, and the submitted Intent is not mutated.
func themeIntent(intent Intent, meaning ValidatedDateMeaning) Intent {
	intent.MustInclude = append([]string(nil), intent.MustInclude...)
	mask := func(text string, anchor DateAnchor) string {
		runes := []rune(text)
		for i := anchor.Start; i < anchor.End; i++ {
			runes[i] = ' '
		}
		return string(runes)
	}
	for _, anchor := range meaning.DateMeaning().Anchors {
		switch anchor.Field {
		case DateAnchorDescription:
			intent.Description = mask(intent.Description, anchor)
		case DateAnchorEra:
			intent.Era = mask(intent.Era, anchor)
		case DateAnchorMustInclude:
			intent.MustInclude[*anchor.Index] = mask(intent.MustInclude[*anchor.Index], anchor)
		}
	}
	return intent
}

func eraAdherence(items []ProposalItem, meaning ValidatedDateMeaning) (*float64, EraEvidence) {
	e := EraEvidence{Status: "not_requested"}
	axes := meaning.ExecutionWindows()
	if len(axes) == 0 {
		return nil, e
	}
	e.Status = "unassessed"
	for _, item := range items {
		applicable, unknown, matches := false, false, true
		for _, axis := range axes {
			if axis.Kind == DateAxisMovieRelease && item.MediaType != provision.Movie || axis.Kind != DateAxisMovieRelease && item.MediaType != provision.Series {
				continue
			}
			applicable = true
			if axis.Kind == DateAxisSeriesAiring || item.Year <= 0 {
				unknown = true
				continue
			}
			inWindow := false
			for _, window := range axis.Windows {
				inWindow = inWindow || item.Year >= window.Start && item.Year <= window.End
			}
			matches = matches && inWindow
		}
		if !applicable {
			continue
		}
		if unknown {
			e.UnknownItems++
			continue
		}
		e.AssessedItems++
		if matches {
			e.MatchingItems++
		}
	}
	if e.AssessedItems == 0 || e.UnknownItems > 0 {
		return nil, e
	}
	e.Status = "partial"
	if e.MatchingItems == e.AssessedItems {
		e.Status = "supported"
	}
	return scoreValue(float64(e.MatchingItems) / float64(e.AssessedItems)), e
}

func scoreValue(value float64) *float64 { return &value }

// UnmarshalJSON applies the assessment contract at the persisted-domain boundary,
// shared by Proposal lists and Journey responses. Old percentages cannot acquire
// the new meaning merely by being decoded into fields with the same wire names.
func (s *Scores) UnmarshalJSON(data []byte) error {
	type wire Scores
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = Scores(decoded)
	if s.Version != 1 {
		s.ThemeFit, s.EraBalance = nil, nil
		s.Theme = ThemeEvidence{Status: "unassessed", Basis: "none", Qualifiers: []QualifierEvidence{}}
		s.Era = EraEvidence{Status: "unassessed"}
	}
	return nil
}
