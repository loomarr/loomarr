package suggest_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

func epochCandidate() catalog.Candidate {
	return catalog.Candidate{MediaType: "series", TMDBID: 2, Name: "Modern Marvels", Year: 1993, InLibrary: true, Genres: []string{"Documentary"}, Networks: []string{"History"}, Overview: "History Channel documentaries about the history of technology."}
}

func epochFinal(t *testing.T, meaning map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{
		"picks":       []any{map[string]any{"mediaType": "series", "key": "series:tmdb:2", "name": "Modern Marvels"}},
		"dateMeaning": meaning,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestSuggest_NetworkEpochAllowsEditorialDecadeWithoutEpisodeCutoff(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{epochCandidate()}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "genres": []any{"Documentary"}, "media_type": "series", "dateMeaning": dateMeaningNone()}),
		testkit.FinalResponse(epochFinal(t, dateMeaningNone())),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "A channel like the History Channel from the 1990s"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].Name != "Modern Marvels" || proposal.Policy.Scope.Dates != nil {
		t.Fatalf("network epoch became a calendar cutoff: %+v", proposal)
	}
	if !strings.Contains(model.Prompt(), "NETWORK PROGRAMMING EPOCH") {
		t.Fatal("provider was not instructed to interpret the network's programming identity")
	}
}

func TestSuggest_NetworkEpochRepairsMistakenEpisodeAiringConstraintBeforeDiscovery(t *testing.T) {
	const description = "A channel like the History Channel from the 1990s"
	start := strings.Index(description, "1990s")
	wrong := dateMeaningAiring1990()
	wrong["anchors"] = []any{map[string]any{"field": "description", "start": start, "end": start + 5}}
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{epochCandidate()}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "media_type": "series", "dateMeaning": wrong}),
		testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "media_type": "series", "dateMeaning": dateMeaningNone()}),
		testkit.FinalResponse(epochFinal(t, dateMeaningNone())),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: description})
	if err != nil {
		t.Fatal(err)
	}
	if len(corpus.Discoveries()) != 1 || corpus.Discoveries()[0].Query.EditorialEpochEnd != 1999 || corpus.Discoveries()[0].Query.YearTo != 0 || corpus.Discoveries()[0].Query.YearFrom != 0 || proposal.Policy.Scope.Dates != nil {
		t.Fatalf("context date dispatched as a hard filter: discoveries=%+v scope=%+v", corpus.Discoveries(), proposal.Policy.Scope.Dates)
	}
}

func TestSuggest_NetworkEpochRepairsUnnecessaryDateClarification(t *testing.T) {
	const description = "A channel like the History Channel from the 1990s, when Modern Marvels was around"
	start := strings.Index(description, "1990s")
	ambiguous := map[string]any{"kind": "ambiguous", "anchors": []any{map[string]any{"field": "description", "start": start, "end": start + 5}}, "axes": []any{}}
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{epochCandidate()}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "media_type": "series", "dateMeaning": ambiguous}),
		testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "media_type": "series", "dateMeaning": dateMeaningNone()}),
		testkit.FinalResponse(epochFinal(t, dateMeaningNone())),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: description})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || len(corpus.Discoveries()) != 1 || proposal.Policy.Scope.Dates != nil {
		t.Fatalf("editorial epoch asked for date clarification: %+v", proposal)
	}
}

func TestSuggest_NetworkStyleRejectsTitleLookupBeforeNetworkDiscovery(t *testing.T) {
	corpus := &catalogfixture.Corpus{
		SearchFunc: func(context.Context, string, int) ([]catalog.Candidate, error) {
			return []catalog.Candidate{{MediaType: "series", TMDBID: 1, Name: "History Channel Documentaries", InLibrary: true, Genres: []string{"Documentary"}}}, nil
		},
		Candidates: []catalog.Candidate{epochCandidate()},
	}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "History Channel", "media_type": "series", "dateMeaning": dateMeaningNone()}),
		testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "genres": []any{"Documentary"}, "media_type": "series", "dateMeaning": dateMeaningNone()}),
		testkit.FinalResponse(epochFinal(t, dateMeaningNone())),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "A channel like the history channel from the 90s"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != 2 || len(corpus.Searches()) != 0 || len(corpus.Discoveries()) != 2 || model.Calls != 3 {
		t.Fatalf("network request dispatched a misleading title lookup: proposal=%+v searches=%+v discoveries=%+v calls=%d", proposal, corpus.Searches(), corpus.Discoveries(), model.Calls)
	}
}

func TestSuggest_NetworkEpochDoesNotEraseSeparateExplicitDates(t *testing.T) {
	for name, intent := range map[string]suggest.Intent{
		"era field":                   {Description: "A channel like the History Channel from the 1990s", Era: "1990s"},
		"separate airing restriction": {Description: "A channel like the History Channel from the 1990s. Only episodes aired during the 2000s."},
		"ordinary dated series":       {Description: "1990s documentary episodes on the History Channel"},
	} {
		t.Run(name, func(t *testing.T) {
			corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{epochCandidate()}}
			model := testkit.NewLLM(testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "media_type": "series", "dateMeaning": dateMeaningNone()}))
			_, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), intent)
			if err == nil || len(corpus.Discoveries()) != 0 {
				t.Fatalf("explicit date was ignored: err=%v discoveries=%+v", err, corpus.Discoveries())
			}
		})
	}
}

func TestSuggest_NetworkStyleDirectFinalMustDiscoverInsteadOfGroundingTitleNames(t *testing.T) {
	corpus := &catalogfixture.Corpus{
		SearchFunc: func(context.Context, string, int) ([]catalog.Candidate, error) {
			return []catalog.Candidate{{MediaType: "series", TMDBID: 3, Name: "The Roosevelts", Year: 2014, InLibrary: true}}, nil
		}, Candidates: []catalog.Candidate{epochCandidate()},
	}
	model := testkit.NewLLM(
		finalResponseWithNone(`{"picks":[{"mediaType":"series","name":"The Roosevelts","key":"series:tmdb:3"}]}`),
		testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "media_type": "series", "dateMeaning": dateMeaningNone()}),
		testkit.FinalResponse(epochFinal(t, dateMeaningNone())),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "A channel like the History Channel from the 1990s"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != 2 || len(corpus.Searches()) != 0 || len(corpus.Discoveries()) != 1 {
		t.Fatalf("direct-final bypassed network discovery: proposal=%+v searches=%+v", proposal, corpus.Searches())
	}
}

func TestSuggest_NetworkEpochCoverageFindsChoicesBeyondExemplarTopic(t *testing.T) {
	corpus := &catalogfixture.Corpus{DiscoverFunc: func(_ context.Context, query catalog.DiscoveryQuery, _ int) ([]catalog.Candidate, error) {
		if len(query.Keywords) > 0 {
			return nil, nil
		}
		return []catalog.Candidate{epochCandidate()}, nil
	}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "keywords": []any{"engineering"}, "media_type": "series", "dateMeaning": dateMeaningNone()}),
		testkit.FinalResponse(epochFinal(t, dateMeaningNone())),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "A channel like the History Channel from the 1990s, when Modern Marvels was around"})
	if err != nil {
		t.Fatal(err)
	}
	queries := corpus.Discoveries()
	if len(proposal.Lineup) != 1 || len(queries) != 2 || len(queries[0].Query.Keywords) != 1 || len(queries[1].Query.Keywords) != 0 || queries[1].Query.EditorialEpochEnd != 1999 || queries[1].Query.Network != "History" || proposal.Policy.Scope.Dates != nil || proposal.Trace.SourceQueriesDispatched != 2 {
		t.Fatalf("exemplar topic starved network-era coverage: proposal=%+v queries=%+v", proposal, queries)
	}
}

func TestSuggest_NetworkEpochExposesOnlyRelevantLookupInterface(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{epochCandidate()}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "media_type": "series", "dateMeaning": dateMeaningNone()}),
		testkit.FinalResponse(epochFinal(t, dateMeaningNone())),
	)
	var initial llm.ChatOptions
	model.OnChat = func() {
		if model.Calls == 1 {
			initial = model.LastOpts
		}
	}
	_, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "A channel like the History Channel from the 1990s"})
	if err != nil {
		t.Fatal(err)
	}
	if len(initial.Tools) != 1 {
		t.Fatal("network discovery tool missing")
	}
	schema := initial.Tools[0].Parameters
	if _, present := schema["allOf"]; present {
		t.Fatal("network-only lookup still exposes collection mode/wrapper requirements")
	}
	properties := schema["properties"].(map[string]any)
	for _, field := range []string{"query", "mode", "titles", "cast", "creators", "runtime_min", "runtime_max", "vote_count_min", "vote_average_min"} {
		if _, present := properties[field]; present {
			t.Errorf("irrelevant lookup field remains exposed: %s", field)
		}
	}
	dateProperties := properties["dateMeaning"].(map[string]any)["properties"].(map[string]any)
	kinds := dateProperties["kind"].(map[string]any)["enum"].([]string)
	if len(kinds) != 1 || kinds[0] != "none" {
		t.Fatalf("editorial-only request still exposes playback date constraints: %v", kinds)
	}
	for _, field := range []string{"anchors", "axes"} {
		if dateProperties[field].(map[string]any)["maxItems"] != 0 {
			t.Errorf("editorial-only dateMeaning permits %s", field)
		}
	}
}

func TestSuggest_NetworkStyleRejectsUnrequestedPopularityThresholdBeforeDispatch(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{epochCandidate()}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "media_type": "series", "vote_count_min": 50, "dateMeaning": dateMeaningNone()}),
		testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "media_type": "series", "dateMeaning": dateMeaningNone()}),
		testkit.FinalResponse(epochFinal(t, dateMeaningNone())),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "A channel like the History Channel from the 1990s"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || len(corpus.Discoveries()) != 1 || corpus.Discoveries()[0].Query.VoteCountMin != 0 {
		t.Fatalf("unrequested popularity threshold starved discovery: %+v", corpus.Discoveries())
	}
}

func TestSuggest_NetworkStyleRejectsGenericDiscovery(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{epochCandidate()}}
	model := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"Documentary"}, "media_type": "series", "dateMeaning": dateMeaningNone()}),
		testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "media_type": "series", "dateMeaning": dateMeaningNone()}),
		testkit.FinalResponse(epochFinal(t, dateMeaningNone())),
	)
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "A channel like the history channel from the 90s"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || len(corpus.Discoveries()) != 1 || corpus.Discoveries()[0].Query.Network != "History" {
		t.Fatalf("generic history titles passed network discovery: proposal=%+v discoveries=%+v", proposal, corpus.Discoveries())
	}
}

func TestSuggest_NetworkStyleDoesNotReopenToolsOnJSONRepair(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{epochCandidate()}}
	lookup := testkit.ToolCallResponse("catalog_search", map[string]any{"network": "History", "media_type": "series", "dateMeaning": dateMeaningNone()})
	model := testkit.NewLLM(lookup, testkit.FinalResponse("{"), lookup, testkit.FinalResponse(epochFinal(t, dateMeaningNone())))
	proposal, err := dateExecutionSuggester(model, corpus).Suggest(context.Background(), suggest.Intent{Description: "A channel like the History Channel from the 1990s"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || len(corpus.Discoveries()) != 1 || len(model.LastOpts.Tools) != 0 {
		t.Fatalf("JSON repair reopened retrieval: proposal=%+v discoveries=%+v opts=%+v", proposal, corpus.Discoveries(), model.LastOpts)
	}
}
