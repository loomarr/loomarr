package suggest

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/textmatch"
	"github.com/loomarr/loomarr/internal/tmdb"
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
	// Scope is an Intent property. Computing it per item/qualifier repeats Unicode
	// normalization and phrase matching without changing the answer.
	seriesTitleScope := intentUsesSeriesTitleScope(intent)
	hits := 0
	for _, item := range items {
		// A catalog title alone is too weak to establish a requested theme.
		if strings.TrimSpace(strings.Join([]string{
			item.Overview, strings.Join(item.Genres, " "), strings.Join(item.Keywords, " "),
			strings.Join(item.OriginCountries, " "), strings.Join(item.Networks, " "),
			strings.Join(item.Cast, " "), strings.Join(item.Creators, " "),
		}, " ")) == "" {
			e.UnknownItems++
			continue
		}
		e.AssessedItems++
		for i, term := range terms {
			if supportsQualifier(item, term) || seriesTitleScope && titleSupportsSubject(item.Name, term) {
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

var countryQualifierCodes = map[string]string{
	"british": "GB", "uk": "GB",
	"american": "US", "canadian": "CA", "australian": "AU",
	"japanese": "JP", "korean": "KR", "french": "FR", "german": "DE",
	"italian": "IT", "spanish": "ES", "indian": "IN", "irish": "IE",
	"new-zealand": "NZ",
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
	hay := strings.Join(append(append(append(append([]string{item.Overview}, item.Genres...), item.Keywords...), item.Networks...), append(item.Cast, item.Creators...)...), " ")
	variants := qualifierEquivalents[term]
	if len(variants) == 0 {
		variants = []string{term}
	}
	for _, variant := range variants {
		if textmatch.ContainsPhrase(hay, variant) {
			return true
		}
	}
	if code := countryQualifierCodes[term]; code != "" {
		for _, country := range item.OriginCountries {
			if strings.EqualFold(country, code) || code == "GB" && strings.EqualFold(country, "UK") {
				return true
			}
		}
	}
	if term == "sitcom" && item.MediaType == provision.Series {
		for _, genre := range item.Genres {
			if strings.EqualFold(genre, "comedy") {
				return true
			}
		}
	}
	return false
}

// A request for episodes of one named series is different from a theme that
// happens to resemble a catalog title. The grounded series name is evidence for
// the subject ("Simpsons"), while the episode-mode cues are executed by the
// deterministic selection policy and are not theme qualifiers.
func intentUsesSeriesTitleScope(intent Intent) bool {
	text := affirmativeIntentText(intent)
	return textmatch.ContainsPhrase(text, "episode") || textmatch.ContainsPhrase(text, "episodes") ||
		intentRequestsCuratedEpisodes(intent) || intentRequestsSequential(intent)
}

func titleSupportsSubject(title, term string) bool {
	return textmatch.ContainsPhrase(title, term)
}

func themeTerms(intent Intent) []string {
	fields := append([]string{intent.Description, intent.Era, intent.Tone, intent.RefineText}, intent.MustInclude...)
	// Direct include/exclude syntax controls which canonical titles survive; it
	// does not describe the surrounding theme. Remove those clauses before
	// measuring qualifier evidence so a required or rejected title cannot make
	// its own selection look more (or less) thematically grounded.
	directTitles := directIncludedTitles(intent)
	for index, field := range fields {
		if excluded := directExcludePattern.FindStringIndex(field); excluded != nil {
			field = field[:excluded[0]]
		}
		for _, title := range directTitles {
			field = regexp.MustCompile(`(?i)`+regexp.QuoteMeta(title)).ReplaceAllString(field, " ")
		}
		fields[index] = field
	}
	text := strings.ToLower(strings.Join(fields, " "))
	// Normalize only the documented multiword equivalents before tokenization.
	text = strings.ReplaceAll(text, "science fiction", "sci-fi")
	text = strings.ReplaceAll(text, "situation comedy", "sitcom")
	text = strings.ReplaceAll(text, "united kingdom", "british")
	text = strings.ReplaceAll(text, "united states", "american")
	text = strings.ReplaceAll(text, "new zealand", "new-zealand")
	stop := map[string]bool{
		"a": true, "an": true, "the": true, "of": true, "and": true, "or": true, "for": true,
		"channel": true, "movie": true, "movies": true, "show": true, "shows": true, "series": true,
		"with": true, "from": true, "in": true, "on": true, "by": true, "written": true,
		"directed": true, "created": true, "creator": true, "starring": true,
		"include": true, "including": true, "add": true, "adding": true, "keep": true,
		"want": true, "exclude": true, "excluding": true, "avoid": true, "omit": true,
		"remove": true, "without": true,
		// Editorial controls are evaluated by deriveIntentPolicy. Counting them
		// again as catalog themes makes a correctly grounded episode request look
		// like a loose match merely because TMDB does not label shows "best" or
		// "full binge".
		"classic": true, "classics": true, "best": true, "greatest": true,
		"favorite": true, "favorites": true, "favourite": true, "favourites": true,
		"rerun": true, "reruns": true, "curated": true, "highlight": true, "highlights": true,
		"episode": true, "episodes": true, "not": true, "full": true, "binge": true,
		"marathon": true, "chronological": true, "order": true, "beginning": true,
		"start": true, "finish": true,
	}
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
	// "murder mystery" is one established compound, not two independent
	// catalog demands. TMDB's Mystery genre is the source evidence; requiring a
	// second literal "murder" string makes canonical examples look partial.
	if seen["murder"] && seen["mystery"] {
		terms = slices.DeleteFunc(terms, func(term string) bool { return term == "murder" })
	}
	return terms
}

// candidateContradictsExplicitQualifiers distinguishes disproven hard metadata
// from sparse metadata. Only source-known country/genre facts can reject a pick.
func candidateContradictsExplicitQualifiers(intent Intent, candidate catalog.Candidate) bool {
	if intentRequiresLibraryOnly(intent) && !candidate.InLibrary {
		return true
	}
	item := fromCandidate(candidate, "", 0)
	terms := themeTerms(intent)
	requestedCountries := explicitOriginCountryCodes(intent)
	if len(requestedCountries) > 0 && len(candidate.OriginCountries) > 0 {
		matchesCountry := false
		for _, requested := range requestedCountries {
			for _, actual := range candidate.OriginCountries {
				matchesCountry = matchesCountry || strings.EqualFold(requested, actual) || requested == "GB" && strings.EqualFold(actual, "UK")
			}
		}
		if !matchesCountry {
			return true
		}
	}
	for _, term := range terms {
		if hardGenreQualifier(term) && len(candidate.Genres) > 0 && !supportsQualifier(item, term) {
			return true
		}
	}
	return false
}

func explicitOriginCountryCodes(intent Intent) []string {
	seen := make(map[string]bool)
	var codes []string
	for _, term := range themeTerms(intent) {
		if code := countryQualifierCodes[term]; code != "" && !seen[code] {
			seen[code] = true
			codes = append(codes, code)
		}
	}
	return codes
}

func explicitHardGenreTerms(intent Intent) []string {
	var genres []string
	for _, term := range themeTerms(intent) {
		if hardGenreQualifier(term) {
			genres = append(genres, term)
		}
	}
	return genres
}

func hardGenreQualifier(term string) bool {
	// Family/kids/cartoon aliases also express audience or medium and have their
	// own deterministic boundaries. Treating them as literal TMDB genres would
	// preempt the more informative audience refusal path.
	switch term {
	case "family", "kids", "cartoon", "cartoons", "anime":
		return false
	default:
		return tmdb.IsKnownGenre(term)
	}
}

var dateThemeSpanPattern = regexp.MustCompile(`(?i)\b(?:(?:from|between|before|after|since|until|during|throughout)\s+(?:the\s+)?)?(?:(?:early|mid|late)[ -]?)?(?:(?:19|20)[0-9]{2}\s*(?:-|–|—|to|through)\s*(?:19|20)[0-9]{2}|(?:19|20)[0-9]0s|(?:19|20)[0-9]{2})\b`)

// Mask all date spans in place before extracting theme qualifiers. Rune offsets
// remain stable for overlapping anchors, and the submitted Intent is not mutated.
// Providers occasionally overrun an anchor into adjacent theme words (for
// example, "1990s family sitcoms"). Within a constraint anchor, mask only the
// actual date phrase; the accepted intervals still own the date interpretation.
func themeIntent(intent Intent, meaning ValidatedDateMeaning) Intent {
	intent.MustInclude = append([]string(nil), intent.MustInclude...)
	mask := func(text string, anchor DateAnchor) string {
		runes := []rune(text)
		anchored := string(runes[anchor.Start:anchor.End])
		matches := dateThemeSpanPattern.FindAllStringIndex(anchored, -1)
		if len(matches) == 0 {
			// Another overlapping anchor may already have blanked the shared date
			// phrase. Never respond by erasing the provider's entire wider span.
			return string(runes)
		}
		for _, match := range matches {
			start := anchor.Start + len([]rune(anchored[:match[0]]))
			end := anchor.Start + len([]rune(anchored[:match[1]]))
			for i := start; i < end; i++ {
				runes[i] = ' '
			}
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
