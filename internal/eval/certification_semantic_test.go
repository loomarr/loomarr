//go:build eval

package eval

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

// These responses script the model boundary only. Production code must validate
// the dates, issue the scalar catalog queries, ground the picks and schedule the
// digest-bound playable metadata against the loaded case's unchanged hard gates.
func TestV8SemanticProductionPlannerAndSchedule(t *testing.T) {
	cases, err := CertificationCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := CertificationRunnerConfig(RunnerConfig{
		Profile: "hermetic-bounded-v1", Generator: ModelIdentity{Provider: "ollama", Model: "fixture-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	materializer, err := NewFrozenCertificationScheduleMaterializer()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		if !strings.HasPrefix(c.Name, "date-") {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			family := strings.Split(c.Name, "--")[0]
			meaning := v8SemanticMeaning(t, c, family)
			mediaType := provision.Movie
			if strings.HasPrefix(family, "date-series-") {
				mediaType = provision.Series
			}
			arguments := map[string]any{"media_type": mediaType, "genres": []string{"Drama"}, "dateMeaning": meaning}
			if family == "date-title-year-none" {
				delete(arguments, "genres")
				arguments["query"] = "Synthetic Matrix"
			}
			picks := make([]map[string]any, 0, len(c.ExpectedProposalKeys))
			for _, key := range c.ExpectedProposalKeys {
				kind, _, id, ok := provision.ParseKey(key)
				if !ok {
					t.Fatalf("invalid fixture key %q", key)
				}
				picks = append(picks, map[string]any{"mediaType": kind, "tmdbId": id})
			}
			final, err := json.Marshal(map[string]any{"picks": picks, "dateMeaning": meaning})
			if err != nil {
				t.Fatal(err)
			}
			responses := []llm.Response{testkit.ToolCallResponse("catalog_search", v8WireArguments(t, arguments))}
			wantCalls := 2
			if family == "date-repair" {
				responses = append(responses, testkit.FinalResponse(`{"picks":`))
				wantCalls++
			}
			responses = append(responses, testkit.FinalResponse(string(final)))
			if family == "date-same-axis-conflict" {
				wantCalls = 1
			}
			provider := testkit.NewLLM(responses...)
			generator, observer, err := NewEmbeddedCertificationGenerator(provider)
			if err != nil {
				t.Fatal(err)
			}
			var queries [][3]any
			generator.(*embeddedCertificationGenerator).fixture.onDiscover = func(q catalog.DiscoveryQuery) {
				queries = append(queries, [3]any{q.MediaType, q.YearFrom, q.YearTo})
			}
			card := NewRunner(generator, config).WithObserver(observer).WithMaterializer(materializer).Run(context.Background(), []Case{c})
			if !card.Certified || len(card.Results) != 1 || !card.Results[0].Passed() {
				t.Fatalf("production failure: assessment=%+v; failures=%v; stage=%s; grounding=%s; queries=%v; calls=%d", card.Assessment, card.Results[0].Failures, card.Results[0].FailureStage, card.Results[0].GroundingStage, queries, provider.Calls)
			}
			if provider.Calls != wantCalls {
				t.Fatalf("model calls = %d, want %d", provider.Calls, wantCalls)
			}
			wantQueries := v8SemanticQueries(family)
			if !reflect.DeepEqual(queries, wantQueries) {
				t.Fatalf("actual source queries = %v, want %v", queries, wantQueries)
			}
		})
	}
}

func v8WireArguments(t *testing.T, arguments map[string]any) map[string]any {
	t.Helper()
	// Provider tool arguments arrive as decoded JSON, including []any arrays
	// and map objects rather than Go structs or typed slices.
	encoded, err := json.Marshal(arguments)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func TestV8SemanticIndexedAnchorRejectedBeforeDiscoveryAndRepaired(t *testing.T) {
	cases, err := CertificationCases()
	if err != nil {
		t.Fatal(err)
	}
	c := certificationCaseByName(t, cases, "date-repair")
	config, err := CertificationRunnerConfig(RunnerConfig{Profile: "hermetic-bounded-v1", Generator: ModelIdentity{Provider: "ollama", Model: "fixture-model"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*suggest.DateAnchor)
	}{
		{"missing index", func(a *suggest.DateAnchor) { a.Index = nil }},
		{"wrong index", func(a *suggest.DateAnchor) { index := 1; a.Index = &index }},
		{"wrong field", func(a *suggest.DateAnchor) { a.Field = suggest.DateAnchorDescription }},
		{"out of bounds span", func(a *suggest.DateAnchor) { a.End = 1000 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			good := v8SemanticMeaning(t, c, "date-repair")
			bad := v8SemanticMeaning(t, c, "date-repair")
			tc.mutate(&bad.Anchors[0])
			tool := func(m suggest.DateMeaning) llm.Response {
				return testkit.ToolCallResponse("catalog_search", v8WireArguments(t, map[string]any{"media_type": "movie", "genres": []string{"Drama"}, "dateMeaning": m}))
			}
			final, err := json.Marshal(map[string]any{"picks": []map[string]any{{"mediaType": "movie", "tmdbId": 11008}}, "dateMeaning": good})
			if err != nil {
				t.Fatal(err)
			}
			provider := testkit.NewLLM(tool(bad), tool(good), testkit.FinalResponse(string(final)))
			generator, observer, err := NewEmbeddedCertificationGenerator(provider)
			if err != nil {
				t.Fatal(err)
			}
			var queries [][3]any
			generator.(*embeddedCertificationGenerator).fixture.onDiscover = func(q catalog.DiscoveryQuery) {
				queries = append(queries, [3]any{q.MediaType, q.YearFrom, q.YearTo})
			}
			provider.OnChat = func() {
				if provider.Calls == 1 && len(queries) != 0 {
					t.Fatalf("invalid anchor dispatched before repair: %v", queries)
				}
			}
			card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), []Case{c})
			if !card.Certified || provider.Calls != 3 || !reflect.DeepEqual(queries, v8SemanticQueries("date-repair")) {
				t.Fatalf("repair result: certified=%v calls=%d queries=%v assessment=%+v", card.Certified, provider.Calls, queries, card.Assessment)
			}
		})
	}
}

func v8SemanticQueries(family string) [][3]any {
	switch family {
	case "date-movie-adjacent":
		return [][3]any{{provision.Movie, 1990, 1992}}
	case "date-movie-disjoint":
		return [][3]any{{provision.Movie, 1990, 1991}, {provision.Movie, 2000, 2001}}
	case "date-series-premiere-airing":
		return [][3]any{{provision.Series, 1980, 1980}}
	case "date-series-pre-era-airing":
		return [][3]any{{provision.Series, 1900, 1989}}
	case "date-repair":
		return [][3]any{{provision.Movie, 1990, 1990}}
	default:
		return nil
	}
}

func v8SemanticMeaning(t *testing.T, c Case, family string) suggest.DateMeaning {
	t.Helper()
	m := suggest.DateMeaning{Kind: suggest.DateMeaningConstraints, Anchors: []suggest.DateAnchor{}, Axes: []suggest.DateAxis{}}
	anchor := func(text string) int {
		start := strings.Index(c.Intent.Description, text)
		if start < 0 {
			t.Fatalf("anchor %q absent from %q", text, c.Intent.Description)
		}
		m.Anchors = append(m.Anchors, suggest.DateAnchor{Field: suggest.DateAnchorDescription, Start: start, End: start + len(text)})
		return len(m.Anchors) - 1
	}
	interval := func(text string, start, end int) suggest.DateInterval {
		return suggest.DateInterval{Anchor: anchor(text), Start: start, End: end}
	}
	axis := func(kind suggest.DateAxisKind, combine suggest.DateCombine, intervals ...suggest.DateInterval) {
		m.Axes = append(m.Axes, suggest.DateAxis{Kind: kind, Combine: combine, Intervals: intervals})
	}
	switch family {
	case "date-movie-adjacent":
		axis(suggest.DateAxisMovieRelease, suggest.DateCombineAny, interval("1990", 1990, 1990), interval("1991 through 1992", 1991, 1992))
	case "date-movie-disjoint":
		axis(suggest.DateAxisMovieRelease, suggest.DateCombineAny, interval("1990-1991", 1990, 1991), interval("2000-2001", 2000, 2001))
	case "date-same-axis-conflict":
		axis(suggest.DateAxisMovieRelease, suggest.DateCombineAll, interval("1990", 1990, 1990), interval("2000", 2000, 2000))
	case "date-series-premiere-airing":
		axis(suggest.DateAxisSeriesPremiere, suggest.DateCombineAny, interval("1980", 1980, 1980))
		axis(suggest.DateAxisSeriesAiring, suggest.DateCombineAny, interval("2000", 2000, 2000))
	case "date-series-pre-era-airing":
		axis(suggest.DateAxisSeriesPremiere, suggest.DateCombineAny, interval("1990", 1900, 1989))
		axis(suggest.DateAxisSeriesAiring, suggest.DateCombineAny, interval("2000", 2000, 2000))
	case "date-title-year-none":
		m.Kind = suggest.DateMeaningNone
	case "date-repair":
		index := 0
		start := strings.Index(c.Intent.MustInclude[index], "1990")
		if start < 0 {
			t.Fatal("indexed include date is missing")
		}
		m.Anchors = append(m.Anchors, suggest.DateAnchor{Field: suggest.DateAnchorMustInclude, Index: &index, Start: start, End: start + 4})
		axis(suggest.DateAxisMovieRelease, suggest.DateCombineAny, suggest.DateInterval{Anchor: 0, Start: 1990, End: 1990})
	default:
		t.Fatalf("unhandled semantic family %q", family)
	}
	return m
}
