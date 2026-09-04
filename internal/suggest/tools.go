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
func (s *Suggester) runTool(ctx context.Context, tc llm.ToolCall, intent Intent, feedback []FeedbackSignal) (string, []catalog.Candidate, DecisionTrace) {
	if tc.Name != catalogToolName {
		return fmt.Sprintf(`{"error":"unknown tool %q; only %s is available"}`, tc.Name, catalogToolName), nil, DecisionTrace{}
	}
	mtArg, _ := tc.Arguments["media_type"].(string)
	discovery, discoveryMode, parseErr := parseDiscoveryQuery(tc.Arguments)
	if parseErr != nil {
		return fmt.Sprintf(`{"error":%q}`, parseErr.Error()), nil, DecisionTrace{}
	}

	var cands []catalog.Candidate
	var err error
	if discoveryMode {
		// Structured discovery covers genres, thematic keywords, era, and the
		// validated scalar qualifiers. Grounding is unchanged: every returned row
		// is keyed into `surfaced` exactly like a title-search result.
		cands, err = s.catalog.Discover(ctx, discovery, catalogSearchLimit)
	} else {
		// KEYWORD: search both corpora by title.
		cands, err = s.catalog.Search(ctx, stringArg(tc.Arguments["query"]), catalog.ScopeAll, catalogSearchLimit)
	}
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil, DecisionTrace{Version: DecisionTraceVersion, Terminal: TerminalRetrievalFailure}
	}
	if mtArg != "" {
		cands = filterByMediaType(cands, mtArg) // narrow to the requested type
	}
	ranked := rankGroundedCandidatesWithTrace(decisionRankQuery(intent), cands, feedback)
	cands = ranked.Candidates
	blob, _ := json.Marshal(toolResult(cands))
	return string(blob), cands, ranked.Trace
}

const (
	maxDiscoveryRuntimeMinutes = 24 * 60
	maxDiscoveryVoteCount      = 100_000_000
	maxDiscoveryEntityTerms    = 4
	maxDiscoveryEntityRunes    = 100
)

func parseDiscoveryQuery(args map[string]any) (catalog.DiscoveryQuery, bool, error) {
	query := catalog.DiscoveryQuery{
		MediaType: mediaTypeArg(stringArg(args["media_type"])),
		Keywords:  stringSlice(args["keywords"]),
		Genres:    stringSlice(args["genres"]),
	}
	if raw, exists := args["network"]; exists {
		value, ok := raw.(string)
		query.Network = strings.TrimSpace(value)
		if !ok || query.Network == "" || len([]rune(query.Network)) > maxDiscoveryEntityRunes {
			return catalog.DiscoveryQuery{}, false, fmt.Errorf("network must be a non-empty string of at most %d characters", maxDiscoveryEntityRunes)
		}
	}
	var err error
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
	rawEra := strings.TrimSpace(stringArg(args["era"]))
	query.YearFrom, query.YearTo = parseEra(rawEra)
	if rawEra != "" && query.YearFrom == 0 {
		return catalog.DiscoveryQuery{}, false, fmt.Errorf("era must be a year, decade, or year range")
	}

	if rawValue, ok := args["original_language"]; ok {
		raw, stringOK := rawValue.(string)
		if !stringOK || strings.TrimSpace(raw) == "" {
			return catalog.DiscoveryQuery{}, false, fmt.Errorf("original_language: must be a two-letter code")
		}
		query.OriginalLanguage, err = discoveryCode(raw, false)
		if err != nil {
			return catalog.DiscoveryQuery{}, false, fmt.Errorf("original_language: %w", err)
		}
	}
	if rawValue, ok := args["origin_country"]; ok {
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

	discoveryMode := len(query.Keywords) > 0 || len(query.Genres) > 0 || rawEra != "" ||
		query.OriginalLanguage != "" || query.OriginCountry != "" || query.RuntimeMin > 0 ||
		query.RuntimeMax > 0 || voteAverageSet || query.VoteCountMin > 0 || query.Network != "" || hasPeople
	if discoveryMode && strings.TrimSpace(stringArg(args["query"])) != "" {
		return catalog.DiscoveryQuery{}, false, fmt.Errorf("query cannot be combined with discovery qualifiers")
	}
	if !discoveryMode && strings.TrimSpace(stringArg(args["query"])) == "" {
		return catalog.DiscoveryQuery{}, false, fmt.Errorf("provide query or a discovery qualifier")
	}
	return query, discoveryMode, nil
}

func boundedEntityTerms(args map[string]any, key string) ([]string, error) {
	raw, exists := args[key]
	if !exists {
		return nil, nil
	}
	values, ok := raw.([]any)
	if !ok || len(values) == 0 || len(values) > maxDiscoveryEntityTerms {
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

// stringArg / stringSlice safely read tool-call arguments (untyped JSON).
func stringArg(v any) string { s, _ := v.(string); return s }

func stringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		if s, ok := e.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

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
// three modes: `query` runs title search; `genres` (+ optional `era`) discovers
// genre themes; `keywords` discovers holidays, motifs, franchises, and topics
// whose terms need not occur in the title. Structured discovery may add scalar,
// movie-person, or TV-network qualifiers. Every mode returns real ids + genres +
// overview + source-backed discovery evidence + an inLibrary flag; it is the ONLY
// way to find titles. Omitted evidence is unknown, never a mismatch.
func catalogTool() llm.ToolSchema {
	return llm.ToolSchema{
		Name: catalogToolName,
		Description: "Find real titles from the library + TMDB. Provide `query` to search by title, `genres` " +
			"to discover genre/era matches, or `keywords` to discover holidays, motifs, franchises, and topics. " +
			"Discovery may also use explicitly requested country, original-language, runtime, vote, movie cast/creator, and TV network filters. " +
			"Returns real external ids, genres, a short overview, available language/country/runtime/vote/keyword/network/person evidence, " +
			"and an inLibrary flag. Missing fields mean unknown. This is the ONLY way to find titles.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":             map[string]any{"type": "string", "description": "title keywords (for a known title)"},
				"keywords":          map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "TMDB thematic keywords, e.g. [\"Christmas\"] or [\"heist\"]"},
				"genres":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "genre names to discover by, e.g. [\"Action\",\"Science Fiction\"]"},
				"era":               map[string]any{"type": "string", "description": "decade or year range for discovery, e.g. \"1990s\" or \"1985-1995\""},
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
		},
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
