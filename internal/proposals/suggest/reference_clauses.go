package suggest

import (
	"regexp"
	"strings"
)

var (
	directIncludePattern = regexp.MustCompile(`(?i:\b(?:include|including|add|adding|keep|want|with)\b)`)
	directExcludePattern = regexp.MustCompile(`(?i:\b(?:exclude|excluding|avoid|omit|remove|without)\b|\bbut\s+not\b)`)
	directAndPattern     = regexp.MustCompile(`(?i:\s+and\s+)`)
	// exampleCuePattern introduces titles the user offers as models of the
	// channel ("adventure movies like Indiana Jones and The Goonies").
	// RE2's \b is ASCII-only, so "à la" carries no leading boundary. "think X"
	// is deliberately absent: TestSuggest_SoftDescriptionExampleRemainsOptional
	// pins a trailing "Think Full House." as a soft, optional hint.
	exampleCuePattern = regexp.MustCompile(`(?i:\b(?:like|such\s+as|similar\s+to|in\s+the\s+vein\s+of|in\s+the\s+spirit\s+of|inspired\s+by|along\s+the\s+lines\s+of|reminiscent\s+of|a\s+la)\b|à\s+la\b|\be\.g\.)`)
)

var exampleTrailerPattern = regexp.MustCompile(`(?i:\s+(?:for|from|to|that|who|which|when|where|on|but|so)\s+|\s*\()`)

// exampleCueIsDesire reports "I'd like a channel of…", where "like" is a verb
// and no title follows.
func exampleCueIsDesire(prefix string) bool {
	words := strings.Fields(strings.ToLower(prefix))
	if len(words) == 0 {
		return false
	}
	switch strings.Trim(words[len(words)-1], ",.;:") {
	case "would", "i'd", "we'd", "you'd", "id", "to", "feel", "feels", "look", "looks", "just", "really":
		return true
	}
	return false
}

// exampleTitles are the clause values after an example cue. They are anchors
// for the same deterministic Catalog resolution as direct inclusions, so prose
// after a cue ("like a rainy night") resolves to nothing and is dropped.
func exampleTitles(intent Intent) []string {
	if networkStyleRequest(intent) {
		return nil // "like the History Channel" names a network, not a title
	}
	var values []string
	for _, field := range []string{intent.Description, intent.RefineText} {
		for _, match := range exampleCuePattern.FindAllStringIndex(field, -1) {
			if exampleCueIsDesire(field[:match[0]]) || cueHasNearbyExclusion(field[:match[0]]) {
				continue
			}
			for _, title := range clauseTitles(field[match[1]:]) {
				// The last title runs into the rest of the sentence ("Gremlins
				// for a family night"); also try it cut at the first
				// preposition/relative word. Exact resolution keeps whichever is a title.
				if cut := exampleTrailerPattern.FindStringIndex(title); cut != nil && cut[0] > 0 {
					values = append(values, strings.TrimSpace(title[:cut[0]]))
				}
				values = append(values, title)
			}
		}
	}
	return values
}

// clauseTitles splits the bounded value of an inclusion or example clause into
// its comma/"and"-separated title candidates.
func clauseTitles(clause string) []string {
	if stop := directClauseEnd(clause); stop >= 0 {
		clause = clause[:stop]
	}
	clause = directAndPattern.ReplaceAllString(clause, ",")
	var titles []string
	for _, title := range strings.Split(clause, ",") {
		title = strings.Trim(strings.TrimSpace(title), "-–—:;.!?()[]{}\"'")
		if title != "" && !pronounLedProse(title) {
			titles = append(titles, title)
		}
	}
	return titles
}

// pronounLedProse reports a clause that is a sentence about the viewer, not a title: "like I
// watched as a kid", "like we used to watch", "keep it PG-13" (#1940). A subject pronoun followed
// by a lowercase word is prose, while "I Love Lucy" and "We Bought a Zoo" stay title-shaped. A
// lowercase object pronoun ("it", "them") leading the clause is prose too; the film is "It".
func pronounLedProse(clause string) bool {
	words := strings.Fields(clause)
	switch words[0] {
	case "it", "them", "me", "us":
		return true
	}
	switch strings.ToLower(words[0]) {
	case "i", "we", "you", "they", "he", "she":
		return len(words) > 1 && words[1] == strings.ToLower(words[1])
	}
	return false
}

// directIncludedTitles extracts only the bounded value of an explicit inclusion
// clause. It is intentionally small grammar, not a general title recognizer;
// exact Catalog resolution remains the authority and safely drops prose.
func directIncludedTitles(intent Intent) []string {
	values := append([]string(nil), intent.MustInclude...)
	for _, field := range []string{intent.Description, intent.RefineText} {
		matches := directIncludePattern.FindAllStringIndex(field, -1)
		for _, match := range matches {
			if cueHasNearbyExclusion(field[:match[0]]) {
				continue
			}
			values = append(values, clauseTitles(field[match[1]:])...)
		}
	}
	values = append(values, exampleTitles(intent)...)
	return boundedReferenceTitles(values)
}

func cueHasNearbyExclusion(prefix string) bool {
	words := evidenceWords(prefix)
	for i := max(0, len(words)-4); i < len(words); i++ {
		switch words[i] {
		case "exclude", "excluding", "avoid", "omit", "remove", "without", "not":
			return true
		}
	}
	return false
}

func directClauseEnd(clause string) int {
	end := len(clause)
	if match := directExcludePattern.FindStringIndex(clause); match != nil {
		end = min(end, match[0])
	}
	// A spaced dash starts an aside ("It Follows — slow and creepy"); titles do not contain one.
	for _, delimiter := range []string{";", "\n", ".", "!", "?", " — ", " – ", " -- "} {
		if index := strings.Index(clause, delimiter); index >= 0 {
			end = min(end, index)
		}
	}
	return end
}

func requiredIntentNamesTitle(intent Intent, title string) bool {
	if titleExplicitlyExcluded(intent, title) {
		return false
	}
	for _, included := range intent.MustInclude {
		if sameExactTitle(included, title) {
			return true
		}
	}
	return freeformTitlePolarity(intent.Description, title) > 1 ||
		freeformTitlePolarity(intent.RefineText, title) > 1
}
