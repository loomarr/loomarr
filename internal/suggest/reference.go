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
	"github.com/loomarr/loomarr/internal/tmdb"
)

const (
	maxReferenceTitleQueries   = 8
	maxMembershipSourceQueries = 8
)

type membershipSourceResolution struct {
	candidate catalog.Candidate
	found     bool
}

type membershipSourceState struct {
	byTitle  map[string]membershipSourceResolution
	searches int
}

func newMembershipSourceState() *membershipSourceState {
	return &membershipSourceState{byTitle: make(map[string]membershipSourceResolution)}
}

var (
	namedCollectionPhrasePattern   = regexp.MustCompile(`(?i:\bnamed\b.{0,80}\b(?:collection|line-?up|block)\b)`)
	properNamedSetPattern          = regexp.MustCompile(`\b[A-Z][[:alnum:]&'-]*(?:\s+[A-Z][[:alnum:]&'-]*){0,5}\s+(?i:collection|line-?up|block)\b`)
	acronymCuePattern              = regexp.MustCompile(`(?i:\b(?:for|from|based\s+on|like)\s+)([A-Z][A-Z0-9&]{2,9})\b`)
	acronymSetSuffixPattern        = regexp.MustCompile(`\b([A-Z][A-Z0-9&]{2,9})(?i:\s+(?:lineup|block|channel|like)\b)`)
	acronymSentenceEndPattern      = regexp.MustCompile(`\b([A-Z][A-Z0-9&]{2,9})\b\s*(?:[.!?]|$)`)
	directNetworkRoleBeforePattern = regexp.MustCompile(`(?i:\b(?:the\s+)?network\s+)$`)
	directNetworkRolePattern       = regexp.MustCompile(`(?i:^\s+(?:the\s+)?network\b)`)
	bareProperNamePattern          = regexp.MustCompile(`\b(?:The\s+)?[A-Z][[:alnum:]&'-]*(?:\s+[A-Z][[:alnum:]&'-]*){0,5}\b`)
)

type referenceGrounding struct {
	candidates []catalog.Candidate
	trace      DecisionTrace
	messages   []llm.Message
}

// sourceGroundingState owns source work for one Suggest invocation. URL
// detection is deliberately local; provider work waits for a canonical date
// interpretation so malformed or ambiguous first turns cannot touch a source.
type sourceGroundingState struct {
	hasReference bool
	presented    bool
	titleHints   []string
	initialized  bool
	result       sourceGroundingResult
}

type sourceGroundingResult struct {
	reference referenceGrounding
	curated   []catalog.Candidate
	explicit  []catalog.Candidate
}

func newSourceGroundingState(intent Intent) sourceGroundingState {
	_, hasReference := reference.URL(referenceIntentText(intent))
	return sourceGroundingState{hasReference: hasReference}
}

func (s *Suggester) initializeSources(ctx context.Context, intent *Intent, meaning ValidatedDateMeaning, state *sourceGroundingState) (sourceGroundingResult, error) {
	if state.initialized {
		return state.result, nil
	}
	if err := ctx.Err(); err != nil {
		return sourceGroundingResult{}, err
	}
	var result sourceGroundingResult
	if state.hasReference {
		referenceResult, _, err := s.groundReference(ctx, intent, meaning)
		if err != nil {
			return sourceGroundingResult{}, err
		}
		if len(referenceResult.candidates) == 0 {
			return sourceGroundingResult{}, ErrNoGroundedTitles
		}
		result.reference = referenceResult
	}
	curated, err := s.groundCuratedTitleSubject(ctx, intent)
	if err != nil {
		return sourceGroundingResult{}, err
	}
	explicit, err := s.groundExplicitMembershipAnchors(ctx, intent)
	if err != nil {
		return sourceGroundingResult{}, err
	}
	if !state.hasReference && len(intent.membershipKeys) == 0 {
		if finder, ok := s.references.(reference.Discoverer); ok {
			if label := namedBlockLabel(*intent); label != "" {
				evidence, discoveryErr := finder.Discover(ctx, label)
				if discoveryErr != nil {
					return sourceGroundingResult{}, fmt.Errorf("discover named source: %w", discoveryErr)
				}
				if evidence.URL != "" && len(evidence.TitleAnchors) > 0 {
					grounded, _, groundingErr := s.groundReferenceEvidence(ctx, intent, meaning, evidence, state.titleHints)
					if groundingErr != nil {
						return sourceGroundingResult{}, groundingErr
					}
					result.reference = grounded
					state.hasReference = true
				}
			}
		}
	}
	result.curated, result.explicit = curated, explicit
	state.result, state.initialized = result, true
	return result, nil
}

// referenceReadError marks a failed fetch of the supplied public page. Catalog
// failures after a successful fetch deliberately do not use it.
type referenceReadError struct{ err error }

func (e *referenceReadError) Error() string { return e.err.Error() }
func (e *referenceReadError) Unwrap() error { return e.err }

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
		cacheMembershipSourceResolution(*intent, title, candidates)
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

// groundReference resolves a pasted public page only after inference has
// supplied a valid date interpretation, then
// exact-searches a bounded set of its title anchors. The synthesized assistant
// tool-call/result pair is an honest record of catalog work Loomarr already did;
// it lets the unchanged planner contract finalize from grounded ids in one turn.
func (s *Suggester) groundReference(ctx context.Context, intent *Intent, meaning ValidatedDateMeaning) (referenceGrounding, bool, error) {
	rawURL, found := reference.URL(referenceIntentText(*intent))
	if !found {
		return referenceGrounding{}, false, nil
	}
	if s.references == nil {
		return referenceGrounding{}, true, errors.New("reference resolver is unavailable")
	}

	evidence, err := s.references.Lookup(ctx, reference.Lookup{URL: rawURL})
	if err != nil {
		return referenceGrounding{}, true, &referenceReadError{err: fmt.Errorf("resolve reference: %w", err)}
	}
	return s.groundReferenceEvidence(ctx, intent, meaning, evidence, nil)
}

func (s *Suggester) groundReferenceEvidence(ctx context.Context, intent *Intent, meaning ValidatedDateMeaning, evidence reference.Evidence, hints []string) (referenceGrounding, bool, error) {
	titles := boundedTitles(evidence.TitleAnchors, reference.MaxTitleAnchors)
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
	for index, title := range prioritizedReferenceTitles(titles, hints) {
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
				ID: callID, Name: catalogToolName, Arguments: map[string]any{"query": title, "dateMeaning": meaning.DateMeaning()},
			}}},
			llm.Message{Role: llm.Tool, ToolCallID: callID, Content: string(result)},
		)
		cacheMembershipSourceResolution(*intent, title, exact)
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
	if len(ranked.Candidates) > catalogSearchLimit {
		ranked.Candidates = ranked.Candidates[:catalogSearchLimit]
	}
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
	if _, found := curatedTitleSubject(intent); found {
		return true
	}
	return acronymNamesSet(text) || properNamedSetPattern.MatchString(text) ||
		bareProperNameNamesSet(intent.Description) || bareProperNameNamesSet(intent.RefineText)
}

// bareProperNameNamesSet fails closed only when the request field itself is an
// otherwise ambiguous title-cased label. It intentionally does not scan spans:
// title examples, exclusions, and ordinary prose remain fuzzy discovery.
// A direct exact catalog title can still become user-supplied membership
// evidence during grounding; a merely thematic catalog hit cannot.
func bareProperNameNamesSet(text string) bool {
	name := strings.Trim(strings.TrimSpace(text), ".,;:!?()[]{}\"'")
	if name == "" || name == "The" || name == strings.ToUpper(name) || tmdb.IsKnownGenre(name) ||
		curatedKnownGenreInField(name) ||
		bareProperNamePattern.FindString(name) != name {
		return false
	}
	return true
}

// curatedTitleSubject extracts a proper-name title adjacent to the existing
// curated-episode cue vocabulary. Description and refinement are considered
// independently so a cue in one field cannot rewrite another field's subject.
func curatedTitleSubject(intent Intent) (string, bool) {
	for _, field := range []string{intent.Description, intent.RefineText} {
		if subject, found := curatedTitleSubjectInField(field); found {
			return subject, true
		}
	}
	return "", false
}

func curatedTitleSubjectInField(field string) (string, bool) {
	subject, found := curatedCueSubjectInField(field)
	if !found || subject == "The" || tmdb.IsKnownGenre(subject) || bareProperNamePattern.FindString(subject) != subject {
		return "", false
	}
	return subject, true
}

// curatedKnownGenreInField recognizes the field-local genre form of a curated
// cue so it remains ordinary discovery rather than a named set.
func curatedKnownGenreInField(field string) bool {
	subject, found := curatedCueSubjectInField(field)
	return found && tmdb.IsKnownGenre(subject)
}

func curatedCueSubjectInField(field string) (string, bool) {
	words := strings.Fields(strings.Trim(strings.TrimSpace(field), ".,;:!?()[]{}\"'"))
	if len(words) < 2 {
		return "", false
	}
	if last := strings.ToLower(strings.Trim(words[len(words)-1], ".,;:!?()[]{}\"'")); last == "episode" || last == "episodes" {
		words = words[:len(words)-1]
	}
	if len(words) < 2 {
		return "", false
	}
	isCue := func(word string) bool {
		word = strings.ToLower(strings.Trim(word, ".,;:!?()[]{}\"'"))
		for _, cue := range curatedEpisodeCues {
			if word == cue {
				return true
			}
		}
		return false
	}
	var subjectWords []string
	if isCue(words[0]) {
		subjectWords = words[1:]
	} else if isCue(words[len(words)-1]) {
		subjectWords = words[:len(words)-1]
	} else {
		return "", false
	}
	subject := strings.Trim(strings.Join(subjectWords, " "), ".,;:!?()[]{}\"'")
	if subject == "" {
		return "", false
	}
	return subject, true
}

func sameCuratedTitleSubject(candidate, subject string) bool {
	if sameExactTitle(candidate, subject) {
		return true
	}
	trimThe := func(value string) string {
		fields := strings.Fields(value)
		if len(fields) > 1 && strings.EqualFold(fields[0], "the") {
			return strings.Join(fields[1:], " ")
		}
		return value
	}
	return sameExactTitle(trimThe(candidate), trimThe(subject))
}

func unambiguousCuratedTitleCandidate(candidates []catalog.Candidate, subject string) (catalog.Candidate, bool) {
	byKey := make(map[provision.Key]catalog.Candidate)
	for _, candidate := range candidates {
		if !sameCuratedTitleSubject(candidate.Name, subject) {
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

func (s *Suggester) groundCuratedTitleSubject(ctx context.Context, intent *Intent) ([]catalog.Candidate, error) {
	subject, found := curatedTitleSubject(*intent)
	if !found {
		return nil, nil
	}
	intent.curatedTitleSet = true
	if intent.membershipSources.searches >= maxMembershipSourceQueries {
		return nil, nil
	}
	intent.membershipSources.searches++
	candidates, err := s.catalog.Search(ctx, subject, catalog.ScopeAll, catalogSearchLimit)
	if err != nil {
		return nil, fmt.Errorf("search curated title subject %q: %w", subject, err)
	}
	candidate, found := unambiguousCuratedTitleCandidate(candidates, subject)
	if !found {
		return nil, nil
	}
	key, _ := candidate.Key()
	intent.curatedTitleKey = key
	intent.membershipKeys[key] = true
	return []catalog.Candidate{candidate}, nil
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
	if sameExactTitle(strings.TrimSpace(intent.Description), title) || sameExactTitle(strings.TrimSpace(intent.RefineText), title) {
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

func cacheMembershipSourceResolution(intent Intent, title string, candidates []catalog.Candidate) {
	if !requiresMembershipEvidence(intent) || !positiveIntentOrReferenceNamesTitle(intent, title) {
		return
	}
	state := intent.membershipSources
	if state == nil {
		state = newMembershipSourceState()
	}
	key := strings.ToLower(strings.Join(strings.Fields(title), " "))
	resolution, cached := state.byTitle[key]
	if !cached {
		resolution.candidate, resolution.found = unambiguousMembershipCandidate(candidates, title)
		state.byTitle[key] = resolution
	}
	if resolution.found {
		candidate := resolution.candidate
		key, _ := candidate.Key()
		intent.membershipKeys[key] = true
	}
}

// resolveMembershipSource binds named-set membership to an unfiltered exact-title
// Catalog search. Model-selected discovery filters and later ranking may narrow the
// candidates offered to the model, but cannot turn an ambiguous source title into
// a unique identity. Each request performs at most eight additional, deduplicated
// source lookups; exact searches elsewhere seed the same cache.
func (s *Suggester) resolveMembershipSource(ctx context.Context, intent Intent, title string) error {
	if !requiresMembershipEvidence(intent) || !positiveIntentOrReferenceNamesTitle(intent, title) {
		return nil
	}
	state := intent.membershipSources
	if state == nil {
		state = newMembershipSourceState()
		intent.membershipSources = state
	}
	key := strings.ToLower(strings.Join(strings.Fields(title), " "))
	if resolution, cached := state.byTitle[key]; cached {
		if resolution.found {
			candidateKey, _ := resolution.candidate.Key()
			intent.membershipKeys[candidateKey] = true
		}
		return nil
	}
	if state.searches >= maxMembershipSourceQueries {
		return nil
	}
	state.searches++
	candidates, err := s.catalog.Search(ctx, title, catalog.ScopeAll, catalogSearchLimit)
	if err != nil {
		return fmt.Errorf("resolve membership source title %q: %w", title, err)
	}
	cacheMembershipSourceResolution(intent, title, candidates)
	return nil
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
			if directNetworkRoleBeforePattern.MatchString(text[:match[2]]) ||
				directNetworkRolePattern.MatchString(text[match[3]:]) {
				continue
			}
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
	return boundedTitles(values, maxReferenceTitleQueries)
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
		if label != "" && freeformTitlePolarity(referenceIntentText(intent), label) >= 0 {
			labels[strings.ToLower(label)] = label
		}
	}
	for _, field := range []string{intent.Description, intent.RefineText} {
		for _, pattern := range []*regexp.Regexp{acronymSetSuffixPattern, acronymDaypartBlockPattern, acronymSentenceEndPattern, acronymCuePattern} {
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
			for len(words) > 1 && (words[0] == "Make" || words[0] == "Create" || words[0] == "Build" || words[0] == "A" || words[0] == "An") {
				words = words[1:]
			}
			if len(labels) > 0 {
				filtered := make([]string, 0, len(words))
				for _, word := range words {
					if !strings.HasSuffix(word, "'s") && !blockDaypartPattern.MatchString(word) {
						filtered = append(filtered, word)
					}
				}
				if len(filtered) == 1 && labels[strings.ToLower(filtered[0])] != "" {
					continue
				}
			}
			add(strings.Join(words, " "))
		}
		if bareProperNameNamesSet(field) {
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

func prioritizedReferenceTitles(titles, hints []string) []string {
	ordered := make([]string, 0, len(titles))
	for _, hint := range boundedReferenceTitles(hints) {
		for _, title := range titles {
			if sameExactTitle(title, hint) {
				ordered = append(ordered, title)
			}
		}
	}
	return boundedReferenceTitles(append(ordered, titles...))
}
