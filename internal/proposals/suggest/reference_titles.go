package suggest

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/platform/textmatch"
)

func boundedReferenceTitles(values []string) []string {
	return boundedTitles(values, maxReferenceTitleQueries)
}

// referenceCatalogTitles keeps a source article's TV-series disambiguator as
// identity evidence while removing it from the Catalog query and display name.
// Without the type/year constraint, a namesake can incorrectly win or make an
// exact source member such as "Sabrina the Teenage Witch (1996 TV series)" ambiguous.
func referenceCatalogTitles(values []string) ([]string, map[string]provision.MediaType, map[string]int) {
	titles := make([]string, 0, len(values))
	mediaTypes := make(map[string]provision.MediaType)
	years := make(map[string]int)
	for _, value := range values {
		name := value
		mediaType := provision.MediaType("")
		year := 0
		if base, suffix, found := strings.Cut(value, " ("); found && strings.HasSuffix(suffix, ")") {
			disambiguator := strings.ToLower(strings.TrimSuffix(suffix, ")"))
			if disambiguator == "tv series" || disambiguator == "television series" {
				name = base
				mediaType = provision.Series
			} else {
				parts := strings.Fields(disambiguator)
				if len(parts) == 3 && (parts[1] == "tv" || parts[1] == "television") && parts[2] == "series" {
					if parsed, err := strconv.Atoi(parts[0]); err == nil && parsed >= 1800 && parsed <= 2200 {
						name = base
						mediaType = provision.Series
						year = parsed
					}
				}
			}
		}
		name = strings.Join(strings.Fields(name), " ")
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if _, found := mediaTypes[key]; !found {
			titles = append(titles, name)
		}
		if mediaType.Valid() {
			mediaTypes[key] = mediaType
		} else if _, found := mediaTypes[key]; !found {
			mediaTypes[key] = ""
		}
		if year > 0 {
			years[key] = year
		} else if _, found := years[key]; !found {
			years[key] = 0
		}
	}
	return titles, mediaTypes, years
}

func boundedTitles(values []string, limit int) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, min(len(values), limit))
	for _, value := range values {
		value = strings.Join(strings.Fields(value), " ")
		key := strings.ToLower(value)
		if value == "" || len(value) > 120 || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}

func sameExactTitle(candidate, anchor string) bool {
	return textmatch.ContainsPhrase(candidate, anchor) && textmatch.ContainsPhrase(anchor, candidate)
}

var acronymDaypartBlockPattern = regexp.MustCompile(`\b([A-Z][A-Z0-9&]{2,9})(?i:\s+(?:monday|tuesday|wednesday|thursday|friday|saturday|sunday)(?:[- ]night)?\s+(?:lineup|block|channel)\b)`)
var blockDaypartPattern = regexp.MustCompile(`(?i)^(?:monday|tuesday|wednesday|thursday|friday|saturday|sunday)(?:-night)?$`)

// namedBlockLabel extracts one label already present in the submitted request.
// Conflicting labels abstain; no provider-authored terms reach source discovery.
func namedBlockLabel(intent Intent) string {
	if !requiresMembershipEvidence(intent) {
		return ""
	}
	labels := make(map[string]string)
	add := func(label string) {
		label = strings.TrimSpace(label)
		if blockDaypartPattern.MatchString(label) {
			return
		}
		if label != "" && freeformTitlePolarity(referenceIntentText(intent), label) >= 0 {
			labels[strings.ToLower(label)] = label
		}
	}
	for _, field := range []string{intent.Description, intent.RefineText} {
		for _, pattern := range []*regexp.Regexp{acronymSetSuffixPattern, acronymPhasePattern, acronymDirectMembersPattern, acronymDaypartBlockPattern, acronymSentenceEndPattern, acronymCuePattern, acronymEditorialBlockPattern} {
			for _, match := range pattern.FindAllStringSubmatchIndex(field, -1) {
				if strings.HasPrefix(field[match[3]:], "'s") || directNetworkRoleBeforePattern.MatchString(field[:match[2]]) || directNetworkRolePattern.MatchString(field[match[3]:]) {
					continue
				}
				add(field[match[2]:match[3]])
			}
		}
	}
	for _, field := range []string{intent.Description, intent.RefineText} {
		for _, match := range properNamedSetPattern.FindAllString(field, -1) {
			words := strings.Fields(match)
			words = words[:len(words)-1]
			for len(words) > 1 && (words[0] == "Make" || words[0] == "Create" || words[0] == "Recreate" || words[0] == "Build" || words[0] == "A" || words[0] == "An" || words[0] == "The") {
				words = words[1:]
			}
			if len(labels) > 0 {
				filtered := make([]string, 0, len(words))
				for _, word := range words {
					if !strings.HasSuffix(word, "'s") && !blockDaypartPattern.MatchString(word) {
						filtered = append(filtered, word)
					}
				}
				if len(filtered) > 0 && labels[strings.ToLower(filtered[0])] != "" {
					continue
				}
			}
			add(strings.Join(words, " "))
		}
		if len(labels) == 0 && bareProperNameNamesSet(field) {
			add(strings.Trim(strings.TrimSpace(field), ".,;:!?()[]{}\"'"))
		}
	}
	if len(labels) != 1 {
		return ""
	}
	for _, label := range labels {
		return label
	}
	return ""
}

func prioritizedReferenceTitles(intent Intent, titles, hints []string) []string {
	ordered := make([]string, 0, len(titles))
	for _, title := range titles {
		if requiredIntentNamesTitle(intent, title) {
			ordered = append(ordered, title)
		}
	}
	for _, hint := range boundedReferenceTitles(hints) {
		for _, title := range titles {
			if sameExactTitle(title, hint) {
				ordered = append(ordered, title)
			}
		}
	}
	return boundedTitles(append(ordered, titles...), maxReferenceRosterQueries)
}

// prioritizeReferenceCandidatePool keeps the household's already-owned members
// visible in the bounded finalization context. Popularity orders only otherwise
// equivalent, independently proven external members; it never proves membership.
func prioritizeReferenceCandidatePool(candidates []titlesearch.Candidate) []titlesearch.Candidate {
	ordered := append([]titlesearch.Candidate(nil), candidates...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].InLibrary != ordered[j].InLibrary {
			return ordered[i].InLibrary
		}
		if ordered[i].VoteCount != ordered[j].VoteCount {
			return ordered[i].VoteCount > ordered[j].VoteCount
		}
		if ordered[i].VoteAverage != ordered[j].VoteAverage {
			return ordered[i].VoteAverage > ordered[j].VoteAverage
		}
		return false
	})
	return ordered
}
