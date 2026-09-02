package suggest

import (
	"context"
	"encoding/json"
	"fmt"

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
	genres := stringSlice(tc.Arguments["genres"])
	keywords := stringSlice(tc.Arguments["keywords"])

	var cands []catalog.Candidate
	var err error
	if len(keywords) > 0 {
		// THEMATIC DISCOVERY: holidays, motifs, franchises, and topics live in
		// TMDB's keyword corpus and need not occur in the title text.
		from, to := parseEra(stringArg(tc.Arguments["era"]))
		cands, err = s.catalog.DiscoverKeywords(ctx, mediaTypeArg(mtArg), keywords, genres, from, to, catalogSearchLimit)
	} else if len(genres) > 0 {
		// DISCOVERY: the model gave genres → find by theme (+ era) rather than title.
		// This is what lets an abstract intent surface real content instead of an
		// empty keyword result. Grounding is unaffected: discovered candidates are
		// keyed into `surfaced` by the caller exactly like search results.
		from, to := parseEra(stringArg(tc.Arguments["era"]))
		cands, err = s.catalog.Discover(ctx, mediaTypeArg(mtArg), genres, from, to, catalogSearchLimit)
	} else {
		// KEYWORD: search both corpora by title.
		cands, err = s.catalog.Search(ctx, stringArg(tc.Arguments["query"]), catalog.ScopeAll, catalogSearchLimit)
	}
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error()), nil, DecisionTrace{Version: DecisionTraceVersion, Terminal: TerminalProviderFailure}
	}
	if mtArg != "" {
		cands = filterByMediaType(cands, mtArg) // narrow to the requested type
	}
	ranked := rankGroundedCandidatesWithTrace(decisionRankQuery(intent), cands, feedback)
	cands = ranked.Candidates
	blob, _ := json.Marshal(toolResult(cands))
	return string(blob), cands, ranked.Trace
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
// whose terms need not occur in the title. Every mode returns real ids + genres +
// overview + source-backed discovery evidence + an inLibrary flag; it is the
// ONLY way to find titles. Omitted evidence is unknown, never a mismatch.
func catalogTool() llm.ToolSchema {
	return llm.ToolSchema{
		Name: catalogToolName,
		Description: "Find real titles from the library + TMDB. Provide `query` to search by title, `genres` " +
			"to discover genre/era matches, or `keywords` to discover holidays, motifs, franchises, and topics. " +
			"Returns real external ids, genres, a short overview, available language/country/runtime/vote/keyword evidence, " +
			"and an inLibrary flag. Missing fields mean unknown. This is the ONLY way to find titles.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query":      map[string]any{"type": "string", "description": "title keywords (for a known title)"},
				"keywords":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "TMDB thematic keywords, e.g. [\"Christmas\"] or [\"heist\"]"},
				"genres":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "genre names to discover by, e.g. [\"Action\",\"Science Fiction\"]"},
				"era":        map[string]any{"type": "string", "description": "decade or year range for discovery, e.g. \"1990s\" or \"1985-1995\""},
				"media_type": map[string]any{"type": "string", "enum": []string{"movie", "series"}},
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
