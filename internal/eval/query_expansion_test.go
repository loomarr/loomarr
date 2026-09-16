//go:build eval

package eval

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestQueryExpansionV1ArtifactsRemainImmutable(t *testing.T) {
	want := map[string]string{
		"testdata/query-expansion-v1.json":         "1b78d1c288c1bf913b5391705a2f2054895bb8e07770b58102f911ce6fa36a83",
		"testdata/query-expansion-catalog-v1.json": "42fe1981f6d38256e08219044e17ca688436325bf012ba69db50a15a1f55d611",
		"testdata/query-expansion-sources-v1.json": "9f7a590b5db976d064f33d8e5a01bf33be0951548726a11f1ce79be1883ae849",
	}
	for path, expected := range want {
		blob, err := queryPilotFiles.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprintf("%x", sha256.Sum256(blob)); got != expected {
			t.Fatalf("%s digest = %s, want immutable %s", path, got, expected)
		}
	}
}

func TestQueryExpansionReviewedTGIFUsesProductionSuggestion(t *testing.T) {
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	provider := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{
			"media_type": "series", "mode": "collection",
			"titles":      []any{"Family Matters", "Step by Step", "Boy Meets World", "Sabrina the Teenage Witch"},
			"dateMeaning": map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}},
		}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"series","key":"series:tmdb:2685"},{"mediaType":"series","key":"series:tmdb:2617"},{"mediaType":"series","key":"series:tmdb:1777"},{"mediaType":"series","key":"series:tmdb:605"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`),
	)
	generator, observer, err := NewEmbeddedQueryExpansionGenerator(provider)
	if err != nil {
		t.Fatal(err)
	}
	card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), cases[:1])
	if !card.Results[0].Passed() || !card.Assessment.Passed || card.Certified || !card.DevelopmentCorpus {
		t.Fatalf("reviewed TGIF development answer: %+v", card.Results[0])
	}
}

func TestQueryExpansionReviewedMCUUsesProductionSuggestion(t *testing.T) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		t.Fatal(err)
	}
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	var selected []Case
	for _, c := range cases {
		if c.Name == "exp-mcu-minimum" {
			selected = append(selected, c)
		}
	}
	if len(selected) != 1 {
		t.Fatalf("MCU tracer cases = %d, want one", len(selected))
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	var authored QueryPilotCase
	for _, c := range corpus.Cases {
		if c.ID == "exp-mcu-minimum" {
			authored = c
		}
	}
	provider := testkit.NewLLM(queryExpansionResponses(t, authored)...)
	generator, observer, err := NewEmbeddedQueryExpansionGenerator(provider)
	if err != nil {
		t.Fatal(err)
	}
	card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), selected)
	if !card.Results[0].Passed() || !card.Assessment.Passed || card.Certified || !card.DevelopmentCorpus {
		t.Fatalf("reviewed MCU development answer: %+v", card.Results[0])
	}
}

func TestQueryExpansionIndependentCausalOutcomeControls(t *testing.T) {
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	ownedAndMissing := func(keys ...string) suggest.Proposal {
		proposal := proposalWithKeys(t, keys...)
		proposal.Acquisitions, proposal.Lineup = proposal.Lineup[2:], proposal.Lineup[:2]
		return proposal
	}
	positive := ownedAndMissing("series:tmdb:2685", "series:tmdb:2617", "series:tmdb:1777", "series:tmdb:605")
	missing := ownedAndMissing("series:tmdb:2685", "series:tmdb:2617", "series:tmdb:1777")
	wrong := ownedAndMissing("series:tmdb:2685", "series:tmdb:2617", "series:tmdb:1777", "series:tmdb:31183")
	duplicate := ownedAndMissing("series:tmdb:2685", "series:tmdb:2617", "series:tmdb:1777", "series:tmdb:605", "series:tmdb:605")
	wrongOwnership := proposalWithKeys(t, "series:tmdb:2685", "series:tmdb:2617", "series:tmdb:1777", "series:tmdb:605")
	wrongEditorialDate := positive
	wrongEditorialDate.Policy.Scope.Dates = &schedule.DateScope{SeriesAiring: []schedule.Range{{From: 1990, To: 1999}}}
	airing := positive
	airing.Policy.Scope.Dates = &schedule.DateScope{SeriesAiring: []schedule.Range{{From: 1990, To: 1999}}}
	wrongAxis := positive
	wrongAxis.Policy.Scope.Dates = &schedule.DateScope{SeriesPremiere: []schedule.Range{{From: 1990, To: 1999}}}
	controls := []struct {
		name, caseID, failure string
		positive, negative    suggest.Proposal
	}{
		{"missing-anchor", "exp-tgif-minimum", "605", positive, missing},
		{"wrong-member", "exp-tgif-minimum", "31183", positive, wrong},
		{"duplicate-padding", "exp-tgif-minimum", "duplicate", positive, duplicate},
		{"sparse-results", "exp-tgif-minimum", "grounded titles", positive, proposalWithKeys(t, "series:tmdb:605")},
		{"wrong-ownership", "exp-tgif-minimum", "acquisitions", positive, wrongOwnership},
		{"invented-playback-limit", "exp-tgif-editorial-epoch", "date", positive, wrongEditorialDate},
		{"outside-library", "exp-tgif-owned", "acceptable", proposalWithKeys(t, "series:tmdb:2685", "series:tmdb:2617"), positive},
		{"wrong-date-axis", "exp-tgif-airing", "date", airing, wrongAxis},
	}
	for _, control := range controls {
		t.Run(control.name, func(t *testing.T) {
			var selected []Case
			for _, c := range cases {
				if c.Name == control.caseID {
					selected = append(selected, c)
				}
			}
			if len(selected) != 1 {
				t.Fatal("causal control requires exactly one authored request")
			}
			good := NewRunner(scriptedGenerator{proposal: control.positive}, config).Run(context.Background(), selected)
			if !good.Results[0].Passed() || !good.Assessment.Passed || good.Certified {
				t.Fatalf("independently valid positive failed: %+v", good.Results[0])
			}
			bad := NewRunner(scriptedGenerator{proposal: control.negative}, config).Run(context.Background(), selected)
			if control.failure == "date" {
				if bad.Results[0].PolicyAccurate || bad.Assessment.Passed || bad.Certified {
					t.Fatalf("wrong date policy escaped strict assessment: %+v", bad)
				}
				return
			}
			if bad.Results[0].Passed() || !strings.Contains(strings.Join(bad.Results[0].Failures, " "), control.failure) || bad.Certified {
				t.Fatalf("deliberately wrong answer escaped its target: %+v", bad.Results[0])
			}
		})
	}
}

func TestQueryExpansionMCUIndependentCausalOutcomeControls(t *testing.T) {
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{})
	if err != nil {
		t.Fatal(err)
	}
	ownedAndMissing := func(owned int, keys ...string) suggest.Proposal {
		proposal := proposalWithKeys(t, keys...)
		proposal.Acquisitions, proposal.Lineup = proposal.Lineup[owned:], proposal.Lineup[:owned]
		return proposal
	}
	positive := ownedAndMissing(3, "movie:tmdb:1726", "movie:tmdb:10138", "movie:tmdb:10195", "movie:tmdb:1724", "movie:tmdb:1771", "movie:tmdb:24428")
	missingAlias := ownedAndMissing(3, "movie:tmdb:1726", "movie:tmdb:10138", "movie:tmdb:10195", "movie:tmdb:1724", "movie:tmdb:1771")
	wrongPhase := ownedAndMissing(3, "movie:tmdb:1726", "movie:tmdb:10138", "movie:tmdb:10195", "movie:tmdb:1724", "movie:tmdb:1771", "movie:tmdb:68721")
	wrongCollision := ownedAndMissing(3, "movie:tmdb:1726", "movie:tmdb:10138", "movie:tmdb:10195", "movie:tmdb:1724", "movie:tmdb:1771", "movie:tmdb:63736")
	duplicate := ownedAndMissing(3, "movie:tmdb:1726", "movie:tmdb:10138", "movie:tmdb:10195", "movie:tmdb:1724", "movie:tmdb:1771", "movie:tmdb:24428", "movie:tmdb:24428")
	wrongOwnership := proposalWithKeys(t, "movie:tmdb:1726", "movie:tmdb:10138", "movie:tmdb:10195", "movie:tmdb:1724", "movie:tmdb:1771", "movie:tmdb:24428")
	ownedOnly := proposalWithKeys(t, "movie:tmdb:1726", "movie:tmdb:10138", "movie:tmdb:10195")
	inRange := ownedAndMissing(2, "movie:tmdb:10138", "movie:tmdb:10195", "movie:tmdb:1771", "movie:tmdb:24428")
	inRange.Policy.Scope.Dates = &schedule.DateScope{MovieRelease: []schedule.Range{{From: 2010, To: 2012}}}
	wrongAxis := inRange
	wrongAxis.Policy.Scope.Dates = &schedule.DateScope{SeriesPremiere: []schedule.Range{{From: 2010, To: 2012}}}
	controls := []struct {
		name, caseID, failure string
		positive, negative    suggest.Proposal
	}{
		{"missing-regional-alias", "exp-mcu-minimum", "24428", positive, missingAlias},
		{"phase-two-padding", "exp-mcu-minimum", "68721", positive, wrongPhase},
		{"title-collision", "exp-mcu-minimum", "63736", positive, wrongCollision},
		{"duplicate-padding", "exp-mcu-minimum", "duplicate", positive, duplicate},
		{"sparse-results", "exp-mcu-minimum", "grounded titles", positive, proposalWithKeys(t, "movie:tmdb:24428")},
		{"wrong-ownership", "exp-mcu-minimum", "acquisitions", positive, wrongOwnership},
		{"outside-library", "exp-mcu-owned", "acceptable", ownedOnly, positive},
		{"wrong-date-axis", "exp-mcu-release-2010s", "date", inRange, wrongAxis},
	}
	for _, control := range controls {
		t.Run(control.name, func(t *testing.T) {
			var selected []Case
			for _, c := range cases {
				if c.Name == control.caseID {
					selected = append(selected, c)
				}
			}
			if len(selected) != 1 {
				t.Fatal("causal control requires exactly one authored MCU request")
			}
			good := NewRunner(scriptedGenerator{proposal: control.positive}, config).Run(context.Background(), selected)
			if !good.Results[0].Passed() || !good.Assessment.Passed || good.Certified {
				t.Fatalf("independently valid positive failed: %+v", good.Results[0])
			}
			bad := NewRunner(scriptedGenerator{proposal: control.negative}, config).Run(context.Background(), selected)
			if control.failure == "date" {
				if bad.Results[0].PolicyAccurate || bad.Assessment.Passed || bad.Certified {
					t.Fatalf("wrong MCU date policy escaped strict assessment: %+v", bad)
				}
				return
			}
			if bad.Results[0].Passed() || !strings.Contains(strings.Join(bad.Results[0].Failures, " "), control.failure) || bad.Certified {
				t.Fatalf("deliberately wrong MCU answer escaped its target: %+v", bad.Results[0])
			}
		})
	}
}

func TestQueryExpansionEveryAuthoredRequestUsesProductionSuggestion(t *testing.T) {
	corpus, err := LoadEmbeddedQueryExpansionCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if len(corpus.Cases) != 60 {
		t.Fatalf("cumulative executable corpus has %d requests, want 60", len(corpus.Cases))
	}
	cases, err := QueryExpansionCases()
	if err != nil {
		t.Fatal(err)
	}
	config, err := QueryExpansionRunnerConfig(RunnerConfig{Profile: "query_expansion_scripted", Generator: ModelIdentity{Provider: "fixture", Model: "cooperative"}})
	if err != nil {
		t.Fatal(err)
	}
	for i, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			provider := testkit.NewLLM(queryExpansionResponses(t, corpus.Cases[i])...)
			generator, observer, err := NewEmbeddedQueryExpansionGenerator(provider)
			if err != nil {
				t.Fatal(err)
			}
			card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), []Case{c})
			if !card.Results[0].Passed() || !card.Assessment.Passed || card.Certified || !card.DevelopmentCorpus {
				t.Fatalf("authored request: stage=%s failures=%v assessment=%v selected=%d", card.Results[0].FailureStage, card.Results[0].Failures, card.Assessment.Failures, card.Results[0].SelectedCount)
			}
			if corpus.Cases[i].FixtureCase == "mcu-reviewed" {
				sourcePresented := false
				for _, message := range provider.LastMessages {
					sourcePresented = sourcePresented || strings.Contains(message.Content, "UNTRUSTED REFERENCE DATA")
				}
				if !sourcePresented {
					t.Fatal("MCU request passed without reviewed source evidence reaching final selection")
				}
			}
		})
	}
}

// Provider replies are authored independently of the manifest's allowed keys,
// required anchors, minimum counts and date oracles. Only request text is read
// to locate the exact wire-protocol date span.
func queryExpansionResponses(t *testing.T, c QueryPilotCase) []llm.Response {
	t.Helper()
	if c.FixtureCase == "mcu-reviewed" {
		return queryExpansionMCUResponses(t, c)
	}
	ids := []int{2685, 2617, 1777, 605}
	args := map[string]any{"media_type": "series", "mode": "collection", "titles": []any{"Family Matters", "Step by Step", "Boy Meets World", "Sabrina the Teenage Witch"}}
	switch c.ID {
	case "exp-tgif-without-sabrina":
		ids = []int{2685, 2617, 1777}
	case "exp-tgif-without-family", "exp-tgif-without-family-text":
		ids = []int{2617, 1777, 605}
	case "exp-tgif-owned", "exp-tgif-no-additions":
		ids = []int{2685, 2617}
	case "exp-sabrina-minimum", "exp-sabrina-conversation", "exp-sabrina-sitcom", "exp-sabrina-typo", "exp-sabrina-airing":
		ids = []int{605}
		args = map[string]any{"media_type": "series", "query": "Sabrina the Teenage Witch"}
	case "exp-sabrina-and-boy":
		ids = []int{605, 1777}
	case "exp-family-only":
		ids = []int{2685}
		args = map[string]any{"media_type": "series", "query": "Family Matters"}
	case "exp-family-and-sabrina":
		ids = []int{2685, 605}
	case "exp-tgif-without-step":
		ids = []int{2685, 1777, 605}
	case "exp-tgif-without-two":
		ids = []int{1777, 605}
	}
	meaning := map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}
	marker, from, to := "", 0, 0
	switch c.ID {
	case "exp-tgif-airing", "exp-tgif-airing-conversation":
		marker, from, to = "1990s", 1990, 1999
	case "exp-tgif-airing-narrow":
		marker, from, to = "1996-1999", 1996, 1999
	case "exp-tgif-airing-inclusive":
		marker, from, to = "1989-2000", 1989, 2000
	case "exp-sabrina-airing":
		marker, from, to = "1996-2000", 1996, 2000
	}
	if marker != "" {
		start := strings.Index(c.Description, marker)
		if start < 0 {
			t.Fatal("independent provider script has no date span")
		}
		meaning = map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": start, "end": start + len(marker)}}, "axes": []any{map[string]any{"kind": "series_airing", "combine": "any", "intervals": []any{map[string]any{"anchor": 0, "start": from, "end": to}}}}}
	}
	args["dateMeaning"] = meaning
	picks := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		picks = append(picks, map[string]any{"mediaType": "series", "key": fmt.Sprintf("series:tmdb:%d", id)})
	}
	final, err := json.Marshal(map[string]any{"picks": picks, "dateMeaning": meaning})
	if err != nil {
		t.Fatal(err)
	}
	// Match provider-decoded JSON numbers/arrays, not Go-only wire shapes.
	wire, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wire, &args); err != nil {
		t.Fatal(err)
	}
	return []llm.Response{testkit.ToolCallResponse("catalog_search", args), testkit.FinalResponse(string(final))}
}

func queryExpansionMCUResponses(t *testing.T, c QueryPilotCase) []llm.Response {
	t.Helper()
	ids := []int{1726, 1724, 10138, 10195, 1771, 24428}
	switch c.ID {
	case "exp-mcu-without-hulk":
		ids = []int{1726, 10138, 10195, 1771, 24428}
	case "exp-mcu-without-iron-man-2":
		ids = []int{1726, 1724, 10195, 1771, 24428}
	case "exp-mcu-without-thor":
		ids = []int{1726, 1724, 10138, 1771, 24428}
	case "exp-mcu-without-avengers":
		ids = []int{1726, 1724, 10138, 10195, 1771}
	case "exp-mcu-owned", "exp-mcu-no-acquisitions":
		ids = []int{1726, 10138, 10195}
	case "exp-mcu-release-2010s":
		ids = []int{10138, 10195, 1771, 24428}
	}
	meaning := map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}
	marker, from, to := "", 0, 0
	switch c.ID {
	case "exp-mcu-release-years":
		marker, from, to = "2008 through 2012", 2008, 2012
	case "exp-mcu-release-years-conversation":
		marker, from, to = "2008 and 2012", 2008, 2012
	case "exp-mcu-release-2010s":
		marker, from, to = "2010 through 2012", 2010, 2012
	}
	if marker != "" {
		start := strings.Index(c.Description, marker)
		if start < 0 {
			t.Fatal("independent MCU provider script has no date span")
		}
		meaning = map[string]any{"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": start, "end": start + len(marker)}}, "axes": []any{map[string]any{"kind": "movie_release", "combine": "any", "intervals": []any{map[string]any{"anchor": 0, "start": from, "end": to}}}}}
	}
	args := map[string]any{
		"media_type": "movie", "mode": "collection",
		"titles":      []any{"Iron Man", "The Incredible Hulk", "Iron Man 2", "Thor", "Captain America: The First Avenger", "Avengers Assemble"},
		"dateMeaning": meaning,
	}
	picks := make([]map[string]any, 0, len(ids))
	for _, id := range ids {
		picks = append(picks, map[string]any{"mediaType": "movie", "key": fmt.Sprintf("movie:tmdb:%d", id)})
	}
	final, err := json.Marshal(map[string]any{"picks": picks, "dateMeaning": meaning})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(wire, &args); err != nil {
		t.Fatal(err)
	}
	return []llm.Response{testkit.ToolCallResponse("catalog_search", args), testkit.FinalResponse(string(final))}
}
