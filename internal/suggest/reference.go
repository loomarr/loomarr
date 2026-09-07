package suggest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/reference"
	"github.com/loomarr/loomarr/internal/textmatch"
)

const maxReferenceTitleQueries = 8

var (
	namedCollectionPhrasePattern = regexp.MustCompile(`(?i:\bnamed\b.{0,80}\b(?:collection|line-?up|block)\b)`)
	properNamedSetPattern        = regexp.MustCompile(`\b[A-Z][[:alnum:]&'-]*(?:\s+[A-Z][[:alnum:]&'-]*){1,5}\s+(?i:collection|line-?up|block)\b`)
	acronymCuePattern            = regexp.MustCompile(`(?i:\b(?:for|from|based\s+on|like)\s+)([A-Z][A-Z0-9&]{2,9})\b`)
	acronymSetSuffixPattern      = regexp.MustCompile(`\b([A-Z][A-Z0-9&]{2,9})(?i:\s+(?:lineup|block|channel|like)\b)`)
	acronymSentenceEndPattern    = regexp.MustCompile(`\b([A-Z][A-Z0-9&]{2,9})\b\s*(?:[.!?]|$)`)
	directNetworkRolePattern     = regexp.MustCompile(`(?i:^\s+(?:the\s+)?network\b)`)
)

type referenceGrounding struct {
	candidates []catalog.Candidate
	trace      DecisionTrace
	messages   []llm.Message
}

// groundExplicitMembershipAnchors accepts user-supplied constituent titles only
// when Catalog resolves exactly one identity. Model-proposed collection rosters
// never reach this path: existence is not evidence of membership.
func (s *Suggester) groundExplicitMembershipAnchors(ctx context.Context, intent *Intent) ([]catalog.Candidate, error) {
	if !requiresMembershipEvidence(*intent) {
		return nil, nil
	}
	anchored := make([]catalog.Candidate, 0, len(intent.MustInclude))
	for _, title := range boundedReferenceTitles(intent.MustInclude) {
		if titleExplicitlyExcluded(*intent, title) {
			continue
		}
		candidates, err := s.catalog.Search(ctx, title, catalog.ScopeAll, catalogSearchLimit)
		if err != nil {
			return nil, fmt.Errorf("search explicit membership title %q: %w", title, err)
		}
		candidate, found := unambiguousMembershipCandidate(candidates, title)
		if !found {
			continue // no implicit media/year choice for an ambiguous user anchor
		}
		key, _ := candidate.Key()
		intent.membershipKeys[key] = true
		anchored = append(anchored, candidate)
	}
	return anchored, nil
}

// groundReference resolves a pasted public page before inference and
// exact-searches a bounded set of its title anchors. The synthesized assistant
// tool-call/result pair is an honest record of catalog work Loomarr already did;
// it lets the unchanged planner contract finalize from grounded ids in one turn.
func (s *Suggester) groundReference(ctx context.Context, intent *Intent) (referenceGrounding, bool, error) {
	rawURL, found := reference.URL(referenceIntentText(*intent))
	if !found {
		return referenceGrounding{}, false, nil
	}
	if s.references == nil {
		return referenceGrounding{}, true, errors.New("reference resolver is unavailable")
	}

	evidence, err := s.references.Lookup(ctx, reference.Lookup{URL: rawURL})
	if err != nil {
		return referenceGrounding{}, true, fmt.Errorf("resolve reference: %w", err)
	}
	titles := boundedReferenceTitles(evidence.TitleAnchors)
	if len(titles) == 0 {
		return referenceGrounding{}, true, errors.New("reference contains no title anchors")
	}

	intent.ReferenceResolved = true
	intent.ReferenceTitles = titles
	intent.referenceEvidence = evidence
	intent.referenceKeys = make(map[provision.Key]bool)
	if intent.membershipKeys == nil {
		intent.membershipKeys = make(map[provision.Key]bool)
	}

	byKey := make(map[provision.Key]catalog.Candidate)
	messages := make([]llm.Message, 0, len(titles)*2)
	for index, title := range titles {
		candidates, searchErr := s.catalog.Search(ctx, title, catalog.ScopeAll, catalogSearchLimit)
		if searchErr != nil {
			return referenceGrounding{}, true, fmt.Errorf("search reference title %q: %w", title, searchErr)
		}
		exact := make([]catalog.Candidate, 0, len(candidates))
		for _, candidate := range candidates {
			if sameExactTitle(candidate.Name, title) {
				exact = append(exact, candidate)
			}
		}
		callID := fmt.Sprintf("loomarr-reference-catalog-%d", index+1)
		result, _ := json.Marshal(toolResult(exact))
		messages = append(messages,
			llm.Message{Role: llm.Assistant, ToolCalls: []llm.ToolCall{{
				ID: callID, Name: catalogToolName, Arguments: map[string]any{"query": title},
			}}},
			llm.Message{Role: llm.Tool, ToolCallID: callID, Content: string(result)},
		)
		candidate, found := unambiguousMembershipCandidate(exact, title)
		if !found || titleExplicitlyExcluded(*intent, candidate.Name) {
			continue
		}
		key, _ := candidate.Key()
		byKey[key] = candidate
		intent.referenceKeys[key] = true
		intent.membershipKeys[key] = true
	}

	candidates := make([]catalog.Candidate, 0, len(byKey))
	for _, candidate := range byKey {
		candidates = append(candidates, candidate)
	}
	ranked := rankGroundedCandidatesWithTrace(decisionRankQuery(*intent), candidates, nil)
	intent.referenceCandidates = append([]catalog.Candidate(nil), ranked.Candidates...)

	return referenceGrounding{
		candidates: ranked.Candidates,
		trace:      ranked.Trace,
		messages:   messages,
	}, true, nil
}

func referenceIntentText(intent Intent) string {
	return strings.Join([]string{
		intent.Description,
		intent.RefineText,
		strings.Join(intent.MustInclude, " "),
	}, " ")
}

// requiresMembershipEvidence identifies requests whose defining quality is
// belonging to a named set, rather than a fuzzy theme. Ordinary genre/mood
// requests remain model-judged because lexical overlap cannot encode synonyms.
func requiresMembershipEvidence(intent Intent) bool {
	if intent.ReferenceResolved {
		return true
	}
	text := referenceIntentText(intent)
	lower := strings.ToLower(text)
	if strings.Contains(lower, "programming block") ||
		strings.Contains(lower, "lineup from") ||
		strings.Contains(lower, "line-up from") ||
		namedCollectionPhrasePattern.MatchString(text) {
		return true
	}
	return acronymNamesSet(text) || properNamedSetPattern.MatchString(text)
}

// positiveIntentOrReferenceNamesTitle verifies the provenance of a catalog title.
// The model may hypothesize the title to drive exact catalog lookup, but only a
// positively framed phrase actually submitted by the user or extracted from a
// fetched reference can promote the resulting identity to membership evidence.
func positiveIntentOrReferenceNamesTitle(intent Intent, title string) bool {
	if titleExplicitlyExcluded(intent, title) {
		return false
	}
	for _, included := range intent.MustInclude {
		if sameExactTitle(included, title) {
			return true
		}
	}
	if freeformTitlePolarity(intent.Description, title) > 0 || freeformTitlePolarity(intent.RefineText, title) > 0 {
		return true
	}
	for _, referenceTitle := range intent.ReferenceTitles {
		if sameExactTitle(referenceTitle, title) {
			return true
		}
	}
	return false
}

// unambiguousMembershipCandidate accepts only a single canonical Catalog identity
// for a source title. It deliberately runs before model-provided type/year or a
// ranked subset can select a remake; duplicate rows for the same key are harmless.
func unambiguousMembershipCandidate(candidates []catalog.Candidate, title string) (catalog.Candidate, bool) {
	byKey := make(map[provision.Key]catalog.Candidate)
	for _, candidate := range candidates {
		if !sameExactTitle(candidate.Name, title) {
			continue
		}
		if key, err := candidate.Key(); err == nil {
			byKey[key] = candidate
		}
	}
	if len(byKey) != 1 {
		return catalog.Candidate{}, false
	}
	for _, candidate := range byKey {
		return candidate, true
	}
	return catalog.Candidate{}, false
}

func promoteUnambiguousMembership(intent Intent, title string, candidates []catalog.Candidate) {
	if !requiresMembershipEvidence(intent) || !positiveIntentOrReferenceNamesTitle(intent, title) {
		return
	}
	if candidate, found := unambiguousMembershipCandidate(candidates, title); found {
		key, _ := candidate.Key()
		intent.membershipKeys[key] = true
	}
}

func titleExplicitlyExcluded(intent Intent, title string) bool {
	for _, excluded := range intent.MustExclude {
		if sameExactTitle(excluded, title) || textmatch.ContainsPhrase(excluded, title) {
			return true
		}
	}
	return freeformTitlePolarity(intent.Description, title) < 0 || freeformTitlePolarity(intent.RefineText, title) < 0
}

// freeformTitlePolarity recognizes a deliberately small cue vocabulary around
// an exact title mention. The closest cue in the preceding ten words wins, which
// handles "think A and B" and "include A, but not B" without treating arbitrary
// substring presence as positive intent.
func freeformTitlePolarity(text, title string) int {
	words := evidenceWords(text)
	titleWords := evidenceWords(title)
	if len(titleWords) == 0 || len(words) < len(titleWords) {
		return 0
	}
	positive := map[string]bool{"add": true, "adding": true, "example": true, "examples": true, "include": true, "including": true, "keep": true, "like": true, "think": true, "want": true, "with": true}
	negative := map[string]bool{"avoid": true, "but": true, "drop": true, "except": true, "exclude": true, "excluding": true, "no": true, "not": true, "omit": true, "remove": true, "without": true}
	polarity := 0
	for start := 0; start+len(titleWords) <= len(words); start++ {
		if strings.Join(words[start:start+len(titleWords)], " ") != strings.Join(titleWords, " ") {
			continue
		}
		for cue := start - 1; cue >= max(0, start-10); cue-- {
			if negative[words[cue]] {
				polarity = -1
				break
			}
			if positive[words[cue]] || (words[cue] == "as" && cue > 0 && words[cue-1] == "such") {
				polarity = 1
				break
			}
		}
		if polarity < 0 {
			return polarity
		}
	}
	return polarity
}

func evidenceWords(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func acronymNamesSet(text string) bool {
	for _, pattern := range []*regexp.Regexp{acronymSetSuffixPattern, acronymSentenceEndPattern} {
		for _, match := range pattern.FindAllStringSubmatchIndex(text, -1) {
			acronym := text[match[2]:match[3]]
			if freeformTitlePolarity(text, acronym) >= 0 {
				return true
			}
		}
	}
	for _, match := range acronymCuePattern.FindAllStringSubmatchIndex(text, -1) {
		acronymEnd := match[3]
		if directNetworkRolePattern.MatchString(text[acronymEnd:]) {
			continue
		}
		return true
	}
	return false
}

func boundedReferenceTitles(values []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, min(len(values), maxReferenceTitleQueries))
	for _, value := range values {
		value = strings.Join(strings.Fields(value), " ")
		key := strings.ToLower(value)
		if value == "" || len(value) > 120 || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
		if len(result) == maxReferenceTitleQueries {
			break
		}
	}
	return result
}

func sameExactTitle(candidate, anchor string) bool {
	return textmatch.ContainsPhrase(candidate, anchor) && textmatch.ContainsPhrase(anchor, candidate)
}
