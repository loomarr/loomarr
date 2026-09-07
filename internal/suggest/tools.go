package suggest

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
)

// runTool executes a model tool call. Only catalog_search is honored; anything
// else returns an error result the model can react to (defense against a model
// inventing a tool). Returns the JSON result string AND the candidates (so the
// suggester can track what was surfaced for grounding).
func (s *Suggester) runTool(ctx context.Context, tc llm.ToolCall, intent Intent, feedback []FeedbackSignal) (string, []catalog.Candidate, DecisionTrace, bool) {
	ledger := newWorkLedger()
	ledger.beginGeneration()
	if !ledger.reserve(1) {
		return `{"error":"suggestion budget exhausted"}`, nil, DecisionTrace{Version: DecisionTraceVersion, Terminal: FailureBudgetExhausted}, false
	}
	result, candidates, trace, valid, _ := s.runToolWithDateMeaning(ctx, tc, intent, feedback, nil, ledger)
	return result, candidates, trace, valid
}

// runToolWithDateMeaning is the one tool boundary that decodes model JSON and
// returns its canonical interpretation to the invocation state. Keeping the
// accepted value out of the untrusted map prevents a later final response from
// silently changing what the earlier retrieval meant.
type preparedToolCall struct {
	arguments     map[string]any
	meaning       ValidatedDateMeaning
	collection    bool
	discovery     catalog.DiscoveryQuery
	discoveryMode bool
	queries       []catalog.DiscoveryQuery
}

// prepareToolCall performs every untrusted-input check and computes the exact
// extra window reservation without calling a source.
func prepareToolCall(tc llm.ToolCall, intent Intent, accepted *ValidatedDateMeaning) (preparedToolCall, string, DecisionTrace, bool) {
	if tc.Name != catalogToolName {
		return preparedToolCall{}, fmt.Sprintf(`{"error":"unknown tool %q; only %s is available"}`, tc.Name, catalogToolName), DecisionTrace{}, false
	}
	arguments := tc.Arguments
	meaning, dateErr := validatedToolDateMeaning(intent, arguments)
	if dateErr != nil {
		if accepted == nil && isDateMeaningConflict(dateErr) {
			return preparedToolCall{}, fmt.Sprintf(`{"error":%q}`, dateErr.Error()), DecisionTrace{Version: DecisionTraceVersion, Terminal: TerminalConstraintsConflict}, true
		}
		return preparedToolCall{}, fmt.Sprintf(`{"error":%q}`, dateErr.Error()), DecisionTrace{}, false
	}
	if accepted != nil && !accepted.Equal(meaning) {
		return preparedToolCall{}, `{"error":"dateMeaning does not match accepted tool interpretation"}`, DecisionTrace{}, false
	}
	if _, present := arguments["era"]; present {
		return preparedToolCall{}, `{"error":"era is retired"}`, DecisionTrace{}, false
	}
	if meaning.DateMeaning().Kind == DateMeaningAmbiguous {
		return preparedToolCall{meaning: meaning}, `{"error":"clarify_dates"}`, DecisionTrace{}, true
	}
	if rawMode, present := arguments["mode"]; present {
		mode, ok := rawMode.(string)
		if !ok || strings.TrimSpace(mode) != "collection" {
			return preparedToolCall{}, `{"error":"mode must be collection when provided"}`, DecisionTrace{}, false
		}
		for key := range arguments {
			if key != "mode" && key != "media_type" && key != "titles" && key != "dateMeaning" {
				return preparedToolCall{}, `{"error":"collection mode accepts only media_type and exact titles; discovery filters cannot prove membership"}`, DecisionTrace{}, false
			}
		}
		// Validate collection arguments now; execution can then be delayed until
		// after source initialization.
		if _, err := collectionTitleAnchors(arguments["titles"]); err != nil {
			return preparedToolCall{}, fmt.Sprintf(`{"error":%q}`, err.Error()), DecisionTrace{}, false
		}
		if !provision.MediaType(stringArg(arguments["media_type"])).Valid() {
			return preparedToolCall{}, `{"error":"collection mode requires media_type movie or series"}`, DecisionTrace{}, false
		}
		return preparedToolCall{arguments: arguments, meaning: meaning, collection: true}, "", DecisionTrace{}, true
	}
	discovery, discoveryMode, parseErr := parseDiscoveryQueryWithDateMeaning(arguments, meaning)
	if parseErr != nil {
		if projected, ok := projectCatalogArguments(tc.Arguments); ok {
			arguments = projected
			discovery, discoveryMode, parseErr = parseDiscoveryQueryWithDateMeaning(arguments, meaning)
		}
	}
	if parseErr != nil {
		return preparedToolCall{}, fmt.Sprintf(`{"error":%q}`, parseErr.Error()), DecisionTrace{}, false
	}
	prepared := preparedToolCall{arguments: arguments, meaning: meaning, discovery: discovery, discoveryMode: discoveryMode}
	if discoveryMode {
		prepared.queries = projectDiscoveryWindows(discovery, meaning)
	}
	return prepared, "", DecisionTrace{}, true
}

func (s *Suggester) executePreparedTool(ctx context.Context, prepared preparedToolCall, intent Intent, feedback []FeedbackSignal) (string, []catalog.Candidate, DecisionTrace, bool) {
	if prepared.collection {
		return s.runCollectionTool(ctx, prepared.arguments, intent, feedback)
	}
	arguments, discoveryMode := prepared.arguments, prepared.discoveryMode
	mtArg, _ := arguments["media_type"].(string)

	var cands []catalog.Candidate
	var err error
	var union catalog.DiscoveryUnion
	if discoveryMode {
		union, err = s.catalog.DiscoverUnion(ctx, prepared.queries)
		cands = union.Candidates
	} else {
		// KEYWORD: search both corpora by title.
		cands, err = s.catalog.Search(ctx, stringArg(arguments["query"]), catalog.ScopeAll, catalogSearchLimit)
	}
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil, DecisionTrace{Version: DecisionTraceVersion, Terminal: TerminalRetrievalFailure, WindowsCompleted: union.WindowsCompleted, SourceQueriesDispatched: union.SourceQueriesDispatched}, true
	}
	if !discoveryMode {
		query := stringArg(arguments["query"])
		for _, candidate := range cands {
			if sameExactTitle(query, candidate.Name) {
				cacheMembershipSourceResolution(intent, candidate.Name, cands)
			}
		}
	}
	for _, candidate := range cands {
		if resolveErr := s.resolveMembershipSource(ctx, intent, candidate.Name); resolveErr != nil {
			return fmt.Sprintf(`{"error":%q}`, resolveErr.Error()), nil, DecisionTrace{Version: DecisionTraceVersion, Terminal: TerminalRetrievalFailure}, true
		}
	}
	if mtArg != "" {
		cands = filterByMediaType(cands, mtArg) // narrow to the requested type
	}
	ranked := rankGroundedCandidatesWithTrace(decisionRankQuery(intent), cands, feedback)
	ranked.Trace.WindowsCompleted = union.WindowsCompleted
	ranked.Trace.SourceQueriesDispatched = union.SourceQueriesDispatched
	cands = ranked.Candidates
	blob, _ := json.Marshal(toolResult(cands))
	return string(blob), cands, ranked.Trace, true
}

func (s *Suggester) runToolWithDateMeaning(ctx context.Context, tc llm.ToolCall, intent Intent, feedback []FeedbackSignal, accepted *ValidatedDateMeaning, ledger *workLedger) (string, []catalog.Candidate, DecisionTrace, bool, *ValidatedDateMeaning) {
	prepared, result, trace, valid := prepareToolCall(tc, intent, accepted)
	if result != "" || !valid || prepared.meaning.DateMeaning().Kind == DateMeaningAmbiguous {
		return result, nil, trace, valid, func() *ValidatedDateMeaning {
			if valid {
				return &prepared.meaning
			}
			return nil
		}()
	}
	extra := 0
	if prepared.discoveryMode {
		extra = len(prepared.queries) - 1
	}
	if !ledger.reserve(extra) {
		return `{"error":"suggestion budget exhausted"}`, nil, DecisionTrace{Version: DecisionTraceVersion, Terminal: FailureBudgetExhausted}, true, &prepared.meaning
	}
	result, candidates, trace, valid := s.executePreparedTool(ctx, prepared, intent, feedback)
	return result, candidates, trace, valid, &prepared.meaning
}

// projectDiscoveryWindows lowers only the accepted execution meaning. Model
// arguments never choose provider years: movie release and series premiere are
// independent title axes, while series airing constrains episode selection only.
func projectDiscoveryWindows(base catalog.DiscoveryQuery, meaning ValidatedDateMeaning) []catalog.DiscoveryQuery {
	var movie, series []DateYearRange
	for _, axis := range meaning.ExecutionWindows() {
		switch axis.Kind {
		case DateAxisMovieRelease:
			movie = axis.Windows
		case DateAxisSeriesPremiere:
			series = axis.Windows
		}
	}
	windowed := func(media provision.MediaType, windows []DateYearRange) []catalog.DiscoveryQuery {
		out := make([]catalog.DiscoveryQuery, 0, len(windows))
		for _, window := range windows {
			query := base
			query.MediaType, query.YearFrom, query.YearTo = media, window.Start, window.End
			out = append(out, query)
		}
		return out
	}
	switch base.MediaType {
	case provision.Movie:
		if len(movie) > 0 {
			return windowed(provision.Movie, movie)
		}
	case provision.Series:
		if len(series) > 0 {
			return windowed(provision.Series, series)
		}
	default:
		var out []catalog.DiscoveryQuery
		if len(movie) > 0 {
			out = append(out, windowed(provision.Movie, movie)...)
		} else if len(series) > 0 {
			out = append(out, catalog.DiscoveryQuery{MediaType: provision.Movie, Keywords: base.Keywords, Genres: base.Genres, OriginalLanguage: base.OriginalLanguage, OriginCountry: base.OriginCountry, RuntimeMin: base.RuntimeMin, RuntimeMax: base.RuntimeMax, VoteAverageMin: base.VoteAverageMin, VoteCountMin: base.VoteCountMin, Cast: base.Cast, Creators: base.Creators})
		}
		if len(series) > 0 {
			out = append(out, windowed(provision.Series, series)...)
		} else if len(movie) > 0 {
			out = append(out, catalog.DiscoveryQuery{MediaType: provision.Series, Keywords: base.Keywords, Genres: base.Genres, OriginalLanguage: base.OriginalLanguage, OriginCountry: base.OriginCountry, RuntimeMin: base.RuntimeMin, RuntimeMax: base.RuntimeMax, VoteAverageMin: base.VoteAverageMin, VoteCountMin: base.VoteCountMin, Network: base.Network})
		}
		if len(out) > 0 {
			return out
		}
	}
	return []catalog.DiscoveryQuery{base}
}

// runCollectionTool resolves only the exact constituent titles the model names.
// It intentionally does not use network, genre, era, or adjacent discovery as
// membership evidence: those are thematic evidence, not a named set's roster.
func (s *Suggester) runCollectionTool(ctx context.Context, arguments map[string]any, intent Intent, feedback []FeedbackSignal) (string, []catalog.Candidate, DecisionTrace, bool) {
	for key := range arguments {
		if key != "mode" && key != "media_type" && key != "titles" && key != "dateMeaning" {
			return `{"error":"collection mode accepts only media_type and exact titles; discovery filters cannot prove membership"}`, nil, DecisionTrace{}, false
		}
	}
	mediaType := provision.MediaType(stringArg(arguments["media_type"]))
	if !mediaType.Valid() {
		return `{"error":"collection mode requires media_type movie or series"}`, nil, DecisionTrace{}, false
	}
	titles, err := collectionTitleAnchors(arguments["titles"])
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil, DecisionTrace{}, false
	}
	candidates := make([]catalog.Candidate, 0, len(titles))
	for _, title := range titles {
		results, searchErr := s.catalog.Search(ctx, title.name, catalog.ScopeAll, catalogSearchLimit)
		if searchErr != nil {
			return fmt.Sprintf(`{"error":%q}`, searchErr.Error()), nil, DecisionTrace{Version: DecisionTraceVersion, Terminal: TerminalRetrievalFailure}, true
		}
		candidate, found := exactCandidateForPick(results, pick{MediaType: string(mediaType), Name: title.name, Year: title.year})
		if !found {
			continue
		}
		_, keyErr := candidate.Key()
		if keyErr != nil {
			continue
		}
		cacheMembershipSourceResolution(intent, title.name, results)
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		return `{"error":"No exact member titles matched; provide constituent titles, not a block or network name."}`, nil, DecisionTrace{}, true
	}
	ranked := rankGroundedCandidatesWithTrace(decisionRankQuery(intent), candidates, feedback)
	blob, _ := json.Marshal(toolResult(ranked.Candidates))
	return string(blob), ranked.Candidates, ranked.Trace, true
}

// validatedToolDateMeaning deliberately round-trips the JSON-shaped tool
// arguments before validation. Tool calls arrive as map[string]any, whereas the
// canonical validator owns all semantic and anchor checks.
func validatedToolDateMeaning(intent Intent, arguments map[string]any) (ValidatedDateMeaning, error) {
	raw, present := arguments["dateMeaning"]
	if !present {
		return ValidatedDateMeaning{}, fmt.Errorf("dateMeaning is required")
	}
	blob, err := json.Marshal(raw)
	if err != nil {
		return ValidatedDateMeaning{}, fmt.Errorf("dateMeaning: %w", err)
	}
	meaning, err := decodeDateMeaning(blob)
	if err != nil {
		return ValidatedDateMeaning{}, fmt.Errorf("dateMeaning: %w", err)
	}
	return ValidateDateMeaning(intent, &meaning)
}

type collectionTitleAnchor struct {
	name string
	year int
}

func collectionTitleAnchors(raw any) ([]collectionTitleAnchor, error) {
	values, ok := raw.([]any)
	if !ok || len(values) == 0 || len(values) > 8 {
		return nil, fmt.Errorf("collection titles must contain 1-8 exact names")
	}
	seen := make(map[string]bool, len(values))
	result := make([]collectionTitleAnchor, 0, len(values))
	for _, rawTitle := range values {
		name, year := "", 0
		switch value := rawTitle.(type) {
		case string:
			name = strings.Join(strings.Fields(value), " ")
		case map[string]any:
			for key := range value {
				if key != "name" && key != "year" {
					return nil, fmt.Errorf("collection title objects accept only name and year")
				}
			}
			rawName, valid := value["name"].(string)
			if !valid {
				return nil, fmt.Errorf("collection title object requires a string name")
			}
			name = strings.Join(strings.Fields(rawName), " ")
			if rawYear, exists := value["year"]; exists {
				floatYear, valid := rawYear.(float64)
				if !valid || math.Trunc(floatYear) != floatYear || floatYear < 1870 || floatYear > 2200 {
					return nil, fmt.Errorf("collection title year must be an integer from 1870 to 2200")
				}
				year = int(floatYear)
			}
		default:
			return nil, fmt.Errorf("collection titles must be strings or {name,year} objects")
		}
		key := fmt.Sprintf("%s:%d", strings.ToLower(name), year)
		if name == "" || len([]rune(name)) > 120 || seen[key] {
			return nil, fmt.Errorf("collection titles must be distinct non-empty names of at most 120 characters")
		}
		seen[key] = true
		result = append(result, collectionTitleAnchor{name: name, year: year})
	}
	return result, nil
}

const (
	maxDiscoveryRuntimeMinutes = 24 * 60
	maxDiscoveryVoteCount      = 100_000_000
	maxDiscoveryEntityTerms    = 4
	maxDiscoveryEntityRunes    = 100
)

func parseDiscoveryQuery(args map[string]any) (catalog.DiscoveryQuery, bool, error) {
	return parseDiscoveryQueryWithDateQualifier(args, false)
}

// parseDiscoveryQueryWithDateMeaning permits a date-only discovery only after
// dateMeaning has passed the canonical validator at the tool boundary. The
// ordinary parser intentionally cannot infer qualification from untrusted JSON.
func parseDiscoveryQueryWithDateMeaning(args map[string]any, meaning ValidatedDateMeaning) (catalog.DiscoveryQuery, bool, error) {
	return parseDiscoveryQueryWithDateQualifier(args, meaning.DateMeaning().Kind == DateMeaningConstraints)
}

func parseDiscoveryQueryWithDateQualifier(args map[string]any, dateOnlyQualifier bool) (catalog.DiscoveryQuery, bool, error) {
	if _, present := args["era"]; present {
		return catalog.DiscoveryQuery{}, false, fmt.Errorf("era is retired; dateMeaning is the sole date authority")
	}
	titleQuery := ""
	if raw, exists := args["query"]; exists {
		value, ok := raw.(string)
		if !ok {
			return catalog.DiscoveryQuery{}, false, fmt.Errorf("query must be a string")
		}
		if value != "" {
			titleQuery = strings.TrimSpace(value)
			if titleQuery == "" {
				return catalog.DiscoveryQuery{}, false, fmt.Errorf("query must be empty or contain non-whitespace text")
			}
		}
	}
	keywords, err := optionalStringTerms(args, "keywords")
	if err != nil {
		return catalog.DiscoveryQuery{}, false, err
	}
	genres, err := optionalStringTerms(args, "genres")
	if err != nil {
		return catalog.DiscoveryQuery{}, false, err
	}
	query := catalog.DiscoveryQuery{
		MediaType: mediaTypeArg(stringArg(args["media_type"])),
		Keywords:  keywords,
		Genres:    genres,
	}
	if raw, exists := args["network"]; exists && raw != "" {
		value, ok := raw.(string)
		query.Network = strings.TrimSpace(value)
		if !ok || query.Network == "" || len([]rune(query.Network)) > maxDiscoveryEntityRunes {
			return catalog.DiscoveryQuery{}, false, fmt.Errorf("network must be a non-empty string of at most %d characters", maxDiscoveryEntityRunes)
		}
	}
	if query.Cast, err = boundedEntityTerms(args, "cast"); err != nil {
		return catalog.DiscoveryQuery{}, false, err
	}
	if query.Creators, err = boundedEntityTerms(args, "creators"); err != nil {
		return catalog.DiscoveryQuery{}, false, err
	}
	hasPeople := len(query.Cast) > 0 || len(query.Creators) > 0
	if query.Network != "" && hasPeople {
		return catalog.DiscoveryQuery{}, false, fmt.Errorf("network and person constraints cannot be combined")
	}
	if query.Network != "" && query.MediaType != provision.Series {
		return catalog.DiscoveryQuery{}, false, fmt.Errorf("network requires media_type series")
	}
	if hasPeople && query.MediaType != provision.Movie {
		return catalog.DiscoveryQuery{}, false, fmt.Errorf("cast and creators require media_type movie")
	}
	if rawValue, ok := args["original_language"]; ok && rawValue != "" {
		raw, stringOK := rawValue.(string)
		if !stringOK || strings.TrimSpace(raw) == "" {
			return catalog.DiscoveryQuery{}, false, fmt.Errorf("original_language: must be a two-letter code")
		}
		query.OriginalLanguage, err = discoveryCode(raw, false)
		if err != nil {
			return catalog.DiscoveryQuery{}, false, fmt.Errorf("original_language: %w", err)
		}
	}
	if rawValue, ok := args["origin_country"]; ok && rawValue != "" {
		raw, stringOK := rawValue.(string)
		if !stringOK || strings.TrimSpace(raw) == "" {
			return catalog.DiscoveryQuery{}, false, fmt.Errorf("origin_country: must be a two-letter code")
		}
		query.OriginCountry, err = discoveryCode(raw, true)
		if err != nil {
			return catalog.DiscoveryQuery{}, false, fmt.Errorf("origin_country: %w", err)
		}
	}
	if query.RuntimeMin, err = boundedIntArg(args, "runtime_min", maxDiscoveryRuntimeMinutes); err != nil {
		return catalog.DiscoveryQuery{}, false, err
	}
	if query.RuntimeMax, err = boundedIntArg(args, "runtime_max", maxDiscoveryRuntimeMinutes); err != nil {
		return catalog.DiscoveryQuery{}, false, err
	}
	if query.RuntimeMin > 0 && query.RuntimeMax > 0 && query.RuntimeMin > query.RuntimeMax {
		return catalog.DiscoveryQuery{}, false, fmt.Errorf("runtime_min must not exceed runtime_max")
	}
	if query.VoteCountMin, err = boundedIntArg(args, "vote_count_min", maxDiscoveryVoteCount); err != nil {
		return catalog.DiscoveryQuery{}, false, err
	}
	_, voteAverageSet := args["vote_average_min"]
	if raw, ok := args["vote_average_min"]; ok {
		query.VoteAverageMin, err = finiteNumber(raw)
		if err != nil || query.VoteAverageMin <= 0 || query.VoteAverageMin > 10 {
			return catalog.DiscoveryQuery{}, false, fmt.Errorf("vote_average_min must be greater than 0 and at most 10")
		}
	}

	discoveryMode := len(query.Keywords) > 0 || len(query.Genres) > 0 || (titleQuery == "" && dateOnlyQualifier) ||
		query.OriginalLanguage != "" || query.OriginCountry != "" || query.RuntimeMin > 0 ||
		query.RuntimeMax > 0 || voteAverageSet || query.VoteCountMin > 0 || query.Network != "" || hasPeople
	if discoveryMode && titleQuery != "" {
		return catalog.DiscoveryQuery{}, false, fmt.Errorf("query cannot be combined with discovery qualifiers")
	}
	if !discoveryMode && titleQuery == "" {
		return catalog.DiscoveryQuery{}, false, fmt.Errorf("provide query or a discovery qualifier")
	}
	return query, discoveryMode, nil
}

func optionalStringTerms(args map[string]any, key string) ([]string, error) {
	raw, exists := args[key]
	if !exists || raw == "" {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
	out := make([]string, 0, len(values))
	for _, rawValue := range values {
		value, ok := rawValue.(string)
		if !ok {
			return nil, fmt.Errorf("%s must be an array of strings", key)
		}
		if value == "" {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, fmt.Errorf("%s must be an array of non-empty strings", key)
		}
		out = append(out, value)
	}
	return out, nil
}

func boundedEntityTerms(args map[string]any, key string) ([]string, error) {
	raw, exists := args[key]
	if !exists {
		return nil, nil
	}
	values, ok := raw.([]any)
	if ok && (len(values) == 0 || allExactEmptyStrings(values)) {
		return nil, nil
	}
	if !ok || len(values) > maxDiscoveryEntityTerms {
		return nil, fmt.Errorf("%s must be an array of 1 to %d names", key, maxDiscoveryEntityTerms)
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, rawValue := range values {
		value, ok := rawValue.(string)
		value = strings.TrimSpace(value)
		if !ok || value == "" || len([]rune(value)) > maxDiscoveryEntityRunes {
			return nil, fmt.Errorf("%s must be an array of non-empty names of at most %d characters", key, maxDiscoveryEntityRunes)
		}
		normalized := strings.ToLower(value)
		if seen[normalized] {
			return nil, fmt.Errorf("%s contains duplicate name %q", key, value)
		}
		seen[normalized] = true
		out = append(out, value)
	}
	return out, nil
}

func allExactEmptyStrings(values []any) bool {
	for _, value := range values {
		if value != "" {
			return false
		}
	}
	return true
}

// projectCatalogArguments recovers the one compatibility route proven by
// #1021: a series request with a valid defining network. Providers sometimes
// fill the mutually exclusive title/person properties too. Those fields may be
// discarded only when their values are individually well-formed; every valid
// scalar discovery qualifier remains authoritative, and every malformed value
// still fails the strict parser rather than broadening the search.
func projectCatalogArguments(args map[string]any) (map[string]any, bool) {
	mediaType := strings.TrimSpace(stringArg(args["media_type"]))
	network := strings.TrimSpace(stringArg(args["network"]))
	if mediaType != string(provision.Series) || network == "" {
		return nil, false
	}
	if raw, exists := args["query"]; exists {
		value, ok := raw.(string)
		if !ok || value != "" && strings.TrimSpace(value) == "" {
			return nil, false
		}
	}
	if _, err := boundedEntityTerms(args, "cast"); err != nil {
		return nil, false
	}
	if _, err := boundedEntityTerms(args, "creators"); err != nil {
		return nil, false
	}
	return projectArgumentKeys(args,
		"media_type", "genres", "keywords", "original_language", "origin_country",
		"runtime_min", "runtime_max", "vote_average_min", "vote_count_min", "network",
	), true
}

func projectArgumentKeys(args map[string]any, keys ...string) map[string]any {
	projected := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, exists := args[key]; exists {
			projected[key] = value
		}
	}
	return projected
}

func discoveryCode(value string, upper bool) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) != 2 || !asciiLetter(value[0]) || !asciiLetter(value[1]) {
		return "", fmt.Errorf("must be a two-letter code")
	}
	if upper {
		return strings.ToUpper(value), nil
	}
	return strings.ToLower(value), nil
}

func asciiLetter(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func boundedIntArg(args map[string]any, key string, maxValue int) (int, error) {
	raw, ok := args[key]
	if !ok {
		return 0, nil
	}
	value, err := finiteNumber(raw)
	if err != nil || value != math.Trunc(value) || value < 1 || value > float64(maxValue) {
		return 0, fmt.Errorf("%s must be an integer from 1 to %d", key, maxValue)
	}
	return int(value), nil
}

func finiteNumber(value any) (float64, error) {
	var number float64
	switch value := value.(type) {
	case float64:
		number = value
	case float32:
		number = float64(value)
	case int:
		number = float64(value)
	case int64:
		number = float64(value)
	default:
		return 0, fmt.Errorf("not a number")
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("not a finite number")
	}
	return number, nil
}

func mergeDecisionTrace(dst, src *DecisionTrace) {
	if src == nil || src.Version == 0 {
		return
	}
	dst.Version = src.Version
	known := make(map[string]int, len(dst.Candidates))
	for i, candidate := range dst.Candidates {
		if candidate.Key != "" {
			known[candidate.Key] = i
		}
	}
	surfacedTotal := dst.SurfacedTotal + src.SurfacedTotal
	recordedTotal := dst.RecordedTotal + src.RecordedTotal
	dst.Truncated = dst.Truncated || src.Truncated || surfacedTotal > DecisionTraceMaxTotal || recordedTotal > DecisionTraceMaxTotal
	dst.SurfacedTotal = min(surfacedTotal, DecisionTraceMaxTotal)
	dst.RecordedTotal = min(recordedTotal, DecisionTraceMaxTotal)
	dst.WindowsCompleted += src.WindowsCompleted
	dst.SourceQueriesDispatched += src.SourceQueriesDispatched
	if src.Terminal != "" {
		dst.Terminal = src.Terminal
	} else if src.SurfacedTotal > 0 {
		dst.Terminal = ""
	}
	for _, c := range src.Candidates {
		if i, exists := known[c.Key]; c.Key != "" && exists {
			dst.Candidates[i] = c
			continue
		}
		if len(dst.Candidates) >= DecisionTraceMaxCandidates {
			dst.Truncated = true
			continue
		}
		dst.Candidates = append(dst.Candidates, c)
		if c.Key != "" {
			known[c.Key] = len(dst.Candidates) - 1
		}
	}
}

func filterAdjacentFeedback(adjacent []AdjacentContext, signals []FeedbackSignal) []AdjacentContext {
	never := make(map[provision.Key]bool)
	for _, signal := range signals {
		if signal.Action == FeedbackNever {
			never[signal.Target] = true
		}
	}
	out := make([]AdjacentContext, 0, len(adjacent))
	for _, candidate := range adjacent {
		if !never[provision.Key(candidate.Key)] {
			out = append(out, candidate)
		}
	}
	return out
}

// mediaTypeArg maps the tool's media_type string to provision's; "" ⇒ both.
func mediaTypeArg(mt string) provision.MediaType {
	switch provision.MediaType(mt) {
	case provision.Movie:
		return provision.Movie
	case provision.Series:
		return provision.Series
	default:
		return "" // both
	}
}

// stringArg safely reads a tool-call argument (untyped JSON).
func stringArg(v any) string { s, _ := v.(string); return s }

// filterByMediaType keeps only candidates matching the requested type ("movie" or
// "series"); an unrecognized value is ignored (returns all — never hides content
// from the model on a bad hint).
func filterByMediaType(cands []catalog.Candidate, mt string) []catalog.Candidate {
	want := provision.MediaType(mt)
	if want != provision.Movie && want != provision.Series {
		return cands
	}
	out := cands[:0:0]
	for _, c := range cands {
		if c.MediaType == want {
			out = append(out, c)
		}
	}
	return out
}

// catalogTool is the provider-neutral tool schema the model may call (§8). It does
// three modes: `query` runs title search; `genres` discovers
// genre themes; `keywords` discovers holidays, motifs, franchises, and topics
// whose terms need not occur in the title. Structured discovery may add scalar,
// movie-person, or TV-network qualifiers. Every mode returns real ids + genres +
// overview + source-backed discovery evidence + an inLibrary flag; it is the ONLY
// way to find titles. Omitted evidence is unknown, never a mismatch.
func catalogTool() llm.ToolSchema {
	return llm.ToolSchema{
		Name: catalogToolName,
		Description: "Find real titles from the library + TMDB. Provide `query` to search by title, `genres` " +
			"to discover genre matches, or `keywords` to discover holidays, motifs, franchises, and topics. " +
			"For a named collection, set mode=collection with media_type and 1-8 exact constituent titles; no discovery filters are allowed. " +
			"Discovery may also use explicitly requested country, original-language, runtime, vote, movie cast/creator, and TV network filters. " +
			"Returns real external ids, genres, a short overview, available language/country/runtime/vote/keyword/network/person evidence, " +
			"and an inLibrary flag. Missing fields mean unknown. This is the ONLY way to find titles.",
		Parameters: map[string]any{
			"type":     "object",
			"required": []string{"dateMeaning"},
			"properties": map[string]any{
				"dateMeaning": dateMeaningSchema(),
				"mode":        map[string]any{"type": "string", "enum": []string{"collection"}, "description": "collection requires media_type and titles; omit for ordinary title or discovery search"},
				"titles": map[string]any{"type": "array", "minItems": 1, "maxItems": 8, "items": map[string]any{"oneOf": []any{
					map[string]any{"type": "string", "minLength": 1, "maxLength": 120},
					map[string]any{"type": "object", "properties": map[string]any{
						"name": map[string]any{"type": "string", "minLength": 1, "maxLength": 120},
						"year": map[string]any{"type": "integer", "minimum": 1870, "maximum": 2200},
					}, "required": []string{"name"}, "additionalProperties": false},
				}}, "description": "exact collection members as title strings or {name,year} anchors"},
				"query":             map[string]any{"type": "string", "description": "title keywords (for a known title)"},
				"keywords":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "TMDB thematic keywords, e.g. [\"Christmas\"] or [\"heist\"]"},
				"genres":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "genre names to discover by, e.g. [\"Action\",\"Science Fiction\"]"},
				"media_type":        map[string]any{"type": "string", "enum": []string{"movie", "series"}},
				"original_language": map[string]any{"type": "string", "description": "explicit ISO 639-1 original-language code, e.g. \"ja\""},
				"origin_country":    map[string]any{"type": "string", "description": "explicit ISO 3166-1 origin-country code, e.g. \"GB\""},
				"runtime_min":       map[string]any{"type": "integer", "minimum": 1, "maximum": maxDiscoveryRuntimeMinutes},
				"runtime_max":       map[string]any{"type": "integer", "minimum": 1, "maximum": maxDiscoveryRuntimeMinutes},
				"vote_average_min":  map[string]any{"type": "number", "exclusiveMinimum": 0, "maximum": 10},
				"vote_count_min":    map[string]any{"type": "integer", "minimum": 1, "maximum": maxDiscoveryVoteCount},
				"network":           map[string]any{"type": "string", "maxLength": maxDiscoveryEntityRunes, "description": "exact TV network name; requires media_type=series"},
				"cast":              map[string]any{"type": "array", "minItems": 1, "maxItems": maxDiscoveryEntityTerms, "items": map[string]any{"type": "string", "maxLength": maxDiscoveryEntityRunes}, "description": "exact cast names; requires media_type=movie"},
				"creators":          map[string]any{"type": "array", "minItems": 1, "maxItems": maxDiscoveryEntityTerms, "items": map[string]any{"type": "string", "maxLength": maxDiscoveryEntityRunes}, "description": "exact director/writer/crew names; requires media_type=movie"},
			},
			"allOf": []any{map[string]any{
				"if":   map[string]any{"properties": map[string]any{"mode": map[string]any{"const": "collection"}}, "required": []string{"mode"}},
				"then": map[string]any{"required": []string{"media_type", "titles"}},
			}},
		},
	}
}

func dateMeaningSchema() map[string]any {
	return map[string]any{
		"type": "object", "required": []string{"kind", "anchors", "axes"}, "additionalProperties": false,
		"properties": map[string]any{
			"kind":    map[string]any{"type": "string", "enum": []string{"none", "constraints", "ambiguous"}},
			"anchors": map[string]any{"type": "array", "items": dateAnchorSchema()},
			"axes":    map[string]any{"type": "array", "items": dateAxisSchema()},
		},
	}
}

func dateAnchorSchema() map[string]any {
	return map[string]any{
		"type": "object", "required": []string{"field", "start", "end"},
		"properties": map[string]any{
			"field": map[string]any{"type": "string", "enum": []string{"description", "era", "refineText", "mustInclude", "mustExclude"}},
			"index": map[string]any{"type": "integer", "minimum": 0},
			"start": map[string]any{"type": "integer", "minimum": 0},
			"end":   map[string]any{"type": "integer", "minimum": 1},
		},
		"additionalProperties": false,
	}
}

func dateAxisSchema() map[string]any {
	return map[string]any{
		"type": "object", "required": []string{"kind", "combine", "intervals"},
		"properties": map[string]any{
			"kind":      map[string]any{"type": "string", "enum": []string{"movie_release", "series_premiere", "series_airing"}},
			"combine":   map[string]any{"type": "string", "enum": []string{"any", "all"}},
			"intervals": map[string]any{"type": "array", "minItems": 1, "maxItems": 4, "items": dateIntervalSchema()},
		},
		"additionalProperties": false,
	}
}

func dateIntervalSchema() map[string]any {
	return map[string]any{
		"type": "object", "required": []string{"anchor", "start", "end"},
		"properties": map[string]any{
			"anchor": map[string]any{"type": "integer", "minimum": 0},
			"start":  map[string]any{"type": "integer", "minimum": 1900, "maximum": 2099},
			"end":    map[string]any{"type": "integer", "minimum": 1900, "maximum": 2099},
		},
		"additionalProperties": false,
	}
}

// adjacentVotesOf reports the consensus an offered adjacency candidate carried (§8.3), or 0
// for a key that did not come from that corpus.
//
// Linear over intent.Adjacent, which is bounded by adjacentLimit (12) — a map would be more
// code than the scan it replaces at this size.
func adjacentVotesOf(intent Intent, key provision.Key) int {
	for _, a := range intent.Adjacent {
		if provision.Key(a.Key) == key {
			return a.Votes
		}
	}
	return 0
}
