//go:build eval

package eval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestReleaseGatePromptRevisionPreservesCasesAndGates(t *testing.T) {
	var contracts []map[string]any
	for _, path := range []string{"testdata/planner-release-gate-v9.json", releaseGateManifestPath} {
		blob, err := releaseGateFiles.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var contract map[string]any
		if err := json.Unmarshal(blob, &contract); err != nil {
			t.Fatal(err)
		}
		delete(contract, "version")
		delete(contract, "promptVersion")
		contracts = append(contracts, contract)
	}
	if !reflect.DeepEqual(contracts[0], contracts[1]) {
		t.Fatal("prompt binding changed release cases, fixtures or gates")
	}
}

func proposalWithKeys(t *testing.T, values ...string) suggest.Proposal {
	t.Helper()
	proposal := suggest.Proposal{}
	for _, value := range values {
		mediaType, provider, id, ok := provision.ParseKey(provision.Key(value))
		if !ok || provider != "tmdb" {
			t.Fatalf("test key %q is not a TMDB key", value)
		}
		proposal.Lineup = append(proposal.Lineup, suggest.ProposalItem{MediaType: mediaType, TMDBID: id, Name: value})
	}
	return proposal
}

func TestReleaseGateScorecardRecordsBoundedPhasesWithoutRawPrompt(t *testing.T) {
	c, err := ReleaseGateCase("known-title")
	if err != nil {
		t.Fatal(err)
	}
	provider := testkit.NewLLM(llm.Response{
		Content: `{"picks":[{"mediaType":"series","key":"series:tmdb:999997","name":"Full House"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`,
		Attribution: llm.Attribution{
			RequestedProvider: "ollama", RequestedModel: "fixture-model",
			ResolvedProvider: "ollama", ResolvedModel: "fixture-digest",
			Tokens: llm.TokenUsage{Prompt: 100, Completion: 20}, Attempts: 1, Latency: 10 * time.Millisecond,
		},
	})
	generator, observer, err := NewEmbeddedReleaseGateGenerator(provider)
	if err != nil {
		t.Fatal(err)
	}
	config, err := ReleaseGateRunnerConfig(RunnerConfig{
		Profile: "release_fixture", Generator: ModelIdentity{Provider: "ollama", Model: "fixture-model"},
	})
	if err != nil {
		t.Fatal(err)
	}
	card := NewRunner(generator, config).WithObserver(observer).Run(context.Background(), []Case{c})
	if !card.Certified || len(card.Results) != 1 {
		t.Fatalf("release-gate scorecard = %+v", card)
	}
	result := card.Results[0]
	if result.ModelCalls != 1 || result.CatalogOperations == 0 || result.CatalogLatencyNanos <= 0 ||
		result.CandidatesSurfaced != 1 || result.SelectedCount != 1 || result.EndToEndLatencyNanos <= 0 {
		t.Fatalf("release-gate phase evidence = %+v", result)
	}
	if len(result.GeneratorCalls) != 1 || result.GeneratorCalls[0].ResolvedModel != "fixture-digest" {
		t.Fatalf("release-gate provider attribution = %+v", result.GeneratorCalls)
	}
	blob, err := json.Marshal(card)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(blob), c.Intent.Description) || strings.Contains(string(blob), "Full House") {
		t.Fatalf("release-gate scorecard retained raw prompt or provider output: %s", blob)
	}
}

func TestReleaseGateCorpusIsFrozenAndReleaseFocused(t *testing.T) {
	corpus, err := LoadEmbeddedReleaseGateCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if corpus.Version != "planner-release-gate-v10" || corpus.PromptVersion != suggest.PlannerPromptVersion ||
		corpus.ToolSchemaVersion != suggest.PlannerToolSchemaVersion || corpus.Fixture.SHA256 == "" || corpus.SourcesFixture.SHA256 == "" {
		t.Fatalf("release-gate identity = %+v", corpus)
	}
	want := map[string]bool{
		"tgif-terse": false, "tgif-conversational": false, "tgif-explicit-anchors": false,
		"tgif-public-reference": false, "broad-1990s-comedy": false, "known-title": false,
		"genre-era": false, "named-collection": false, "known-franchise": false,
		"movie-person": false, "tv-network": false, "tv-network-genre": false, "mood-theme": false,
		"public-reference": false, "invented-id-no-tools": false, "empty-exact-title": false,
		"ambiguous-exact-title": false, "partial-malformed-intent": false,
	}
	for _, c := range corpus.Cases {
		if _, ok := want[c.ID]; ok {
			want[c.ID] = true
		}
	}
	for id, present := range want {
		if !present {
			t.Errorf("release-gate case %q is missing", id)
		}
	}
	if corpus.Thresholds.MaxP50EndToEndLatencyNanos != 8_000_000_000 ||
		corpus.Thresholds.MaxP95EndToEndLatencyNanos != 12_000_000_000 ||
		corpus.Thresholds.MaxSuccessfulEndToEndLatencyNanos != 20_000_000_000 {
		t.Fatalf("release-gate latency thresholds = %+v", corpus.Thresholds)
	}
}

func TestReleaseGateModelCanaryIsBoundedAndRepresentative(t *testing.T) {
	cases, err := ReleaseGateModelCanaryCases()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"tgif-terse", "tgif-conversational", "named-collection", "known-franchise",
		"movie-person", "tv-network-genre", "mood-theme", "public-reference",
	}
	if len(cases) != len(want) {
		t.Fatalf("model canary cases = %d, want %d", len(cases), len(want))
	}
	for index, c := range cases {
		if c.Name != want[index] {
			t.Fatalf("model canary case %d = %q, want %q", index, c.Name, want[index])
		}
		if !c.ExpectGroundedCompletion || c.MaxToolCalls != suggest.ProductionBounds().MaxToolCalls {
			t.Fatalf("model canary case %q lost live release gates: %+v", c.Name, c)
		}
	}
	budget, err := computeCallBudget(len(cases), 1)
	if err != nil {
		t.Fatal(err)
	}
	if budget.MaxGeneratorCalls != 192 || budget.MaxJudgeCalls != 8 || budget.Total != 200 {
		t.Fatalf("model canary call budget = %+v", budget)
	}
}

func TestReleaseGateModelFinalistHasExactThousandCallEnvelope(t *testing.T) {
	cases, err := ReleaseGateModelCanaryCases()
	if err != nil {
		t.Fatal(err)
	}
	budget, err := computeCallBudget(len(cases), 5)
	if err != nil {
		t.Fatal(err)
	}
	if budget.Cases != 8 || budget.Trials != 5 || budget.MaxGeneratorCalls != 960 || budget.MaxJudgeCalls != 40 || budget.Total != 1000 {
		t.Fatalf("model finalist call budget = %+v", budget)
	}
}

func TestReleaseGateStructuralCasesUseProductionGrounding(t *testing.T) {
	tests := []struct {
		name      string
		response  string
		wantError bool
		wantTitle string
	}{
		{
			name:      "invented-id-no-tools",
			response:  `{"picks":[{"mediaType":"series","key":"series:tmdb:999999","name":"Imaginary TGIF Show"}]}`,
			wantError: true,
		},
		{
			name:      "empty-exact-title",
			response:  `{"picks":[{"mediaType":"series","key":"series:tmdb:999998","name":"The Unfindable Exact Show"}]}`,
			wantError: true,
		},
		{
			name:      "ambiguous-exact-title",
			response:  `{"picks":[{"mediaType":"series","key":"series:tmdb:999997","name":"Full House"}]}`,
			wantTitle: "Full House",
		},
		{
			name:      "partial-malformed-intent",
			response:  `{"picks":[{"mediaType":"series","key":"series:tmdb:999999","name":"Banana Fish"}]}`,
			wantError: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := testkit.NewLLM(testkit.FinalResponse(withReleaseDateMeaning(t, tc.response, map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}})))
			generator, _, err := NewEmbeddedReleaseGateGenerator(provider)
			if err != nil {
				t.Fatal(err)
			}
			c, err := ReleaseGateCase(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			proposal, suggestErr := generator.Suggest(context.Background(), mapIntent(c.Intent))
			if tc.wantError {
				if !errors.Is(suggestErr, suggest.ErrNoGroundedTitles) || len(allItems(proposal)) != 0 {
					t.Fatalf("ungrounded response = (%+v, %v), want explicit abstention", proposal, suggestErr)
				}
				return
			}
			if suggestErr != nil || len(allItems(proposal)) != 1 || allItems(proposal)[0].Name != tc.wantTitle ||
				allItems(proposal)[0].TMDBID == 999997 {
				t.Fatalf("exact-title recovery = (%+v, %v), want catalog identity for %q", proposal, suggestErr, tc.wantTitle)
			}
		})
	}
}

func TestReleaseGateBroaderIntentFamiliesUseProductionRoutes(t *testing.T) {
	tests := []struct {
		name      string
		arguments map[string]any
		response  string
		want      int
	}{
		{
			name: "named-collection",
			arguments: map[string]any{"mode": "collection", "media_type": "movie", "titles": []any{
				"Iron Man", "The Incredible Hulk", "Iron Man 2", "Thor", "Captain America: The First Avenger", "The Avengers",
			}},
			response: `{"picks":[{"mediaType":"movie","key":"movie:tmdb:21101","name":"Iron Man"},{"mediaType":"movie","key":"movie:tmdb:21102","name":"The Incredible Hulk"},{"mediaType":"movie","key":"movie:tmdb:21103","name":"Iron Man 2"},{"mediaType":"movie","key":"movie:tmdb:21104","name":"Thor"},{"mediaType":"movie","key":"movie:tmdb:21105","name":"Captain America: The First Avenger"},{"mediaType":"movie","key":"movie:tmdb:21106","name":"The Avengers"}]}`,
			want:     6,
		},
		{
			name: "known-franchise",
			arguments: map[string]any{"mode": "collection", "media_type": "movie", "titles": []any{
				"The Matrix", "The Matrix Reloaded", "The Matrix Revolutions", "The Animatrix",
			}},
			response: `{"picks":[{"mediaType":"movie","key":"movie:tmdb:21201","name":"The Matrix"},{"mediaType":"movie","key":"movie:tmdb:21202","name":"The Matrix Reloaded"},{"mediaType":"movie","key":"movie:tmdb:21203","name":"The Matrix Revolutions"},{"mediaType":"movie","key":"movie:tmdb:21204","name":"The Animatrix"}]}`,
			want:     4,
		},
		{
			name:      "movie-person",
			arguments: map[string]any{"media_type": "movie", "cast": []any{"Tom Hanks"}, "era": "1990s"},
			response:  `{"picks":[{"mediaType":"movie","key":"movie:tmdb:21301","name":"Joe Versus the Volcano"},{"mediaType":"movie","key":"movie:tmdb:21302","name":"That Thing You Do!"},{"mediaType":"movie","key":"movie:tmdb:21303","name":"You've Got Mail"}],"policy":{"era":{"from":1990,"to":1999}}}`,
			want:      3,
		},
		{
			name:      "tv-network",
			arguments: map[string]any{"media_type": "series", "network": "HBO", "era": "2000s"},
			response:  `{"picks":[{"mediaType":"series","key":"series:tmdb:21401","name":"The Sopranos"},{"mediaType":"series","key":"series:tmdb:21402","name":"The Wire"},{"mediaType":"series","key":"series:tmdb:21403","name":"Six Feet Under"},{"mediaType":"series","key":"series:tmdb:21404","name":"Deadwood"}],"policy":{"era":{"from":2000,"to":2009}}}`,
			want:      4,
		},
		{
			name: "tv-network-genre",
			arguments: map[string]any{
				"mode": "network", "media_type": "series", "network": "Apple TV+", "genres": []any{"Science Fiction"},
			},
			response: `{"picks":[{"mediaType":"series","key":"series:tmdb:21701","name":"Foundation"},{"mediaType":"series","key":"series:tmdb:21702","name":"For All Mankind"},{"mediaType":"series","key":"series:tmdb:21703","name":"Silo"},{"mediaType":"series","key":"series:tmdb:21704","name":"Severance"}]}`,
			want:     4,
		},
		{
			name:      "mood-theme",
			arguments: map[string]any{"media_type": "series", "keywords": []any{"cozy", "mystery"}},
			response:  `{"picks":[{"mediaType":"series","key":"series:tmdb:21501","name":"Murder, She Wrote"},{"mediaType":"series","key":"series:tmdb:21502","name":"Father Brown"},{"mediaType":"series","key":"series:tmdb:21503","name":"Miss Fisher's Murder Mysteries"}]}`,
			want:      3,
		},
		{
			name: "public-reference",
			arguments: map[string]any{"mode": "collection", "media_type": "movie", "titles": []any{
				"E.T. the Extra-Terrestrial", "Hook", "Jurassic Park",
			}},
			response: `{"picks":[{"mediaType":"movie","key":"movie:tmdb:21601","name":"E.T. the Extra-Terrestrial"},{"mediaType":"movie","key":"movie:tmdb:21602","name":"Hook"},{"mediaType":"movie","key":"movie:tmdb:21603","name":"Jurassic Park"}]}`,
			want:     3,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := ReleaseGateCase(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			provider := testkit.NewLLM(releaseScriptedResponses(t, c, tc.arguments, tc.response)...)
			generator, _, err := NewEmbeddedReleaseGateGenerator(provider)
			if err != nil {
				t.Fatal(err)
			}

			proposal, suggestErr := generator.Suggest(context.Background(), mapIntent(c.Intent))
			if suggestErr != nil || len(allItems(proposal)) != tc.want {
				t.Fatalf("proposal = (%+v, %v), want %d grounded items", proposal, suggestErr, tc.want)
			}
			if failures := deterministicChecks(c, proposal, nil); len(failures) != 0 {
				t.Fatalf("release checks failed: %v", failures)
			}
		})
	}
}

func TestReleaseGateRunsMoreThanOneThousandScriptedInputVariants(t *testing.T) {
	fixtures := []struct {
		caseID    string
		arguments map[string]any
		final     string
	}{
		{
			caseID: "tgif-conversational",
			arguments: map[string]any{"mode": "collection", "media_type": "series", "titles": []any{
				"Full House", "Family Matters", "Step by Step", "Perfect Strangers", "Boy Meets World", "Sabrina the Teenage Witch",
			}},
			final: `{"picks":[{"mediaType":"series","key":"series:tmdb:20101","name":"Full House"},{"mediaType":"series","key":"series:tmdb:20102","name":"Family Matters"},{"mediaType":"series","key":"series:tmdb:20103","name":"Step by Step"},{"mediaType":"series","key":"series:tmdb:20104","name":"Perfect Strangers"},{"mediaType":"series","key":"series:tmdb:20105","name":"Boy Meets World"},{"mediaType":"series","key":"series:tmdb:20106","name":"Sabrina the Teenage Witch"}]}`,
		},
		{
			caseID: "named-collection",
			arguments: map[string]any{"mode": "collection", "media_type": "movie", "titles": []any{
				"Iron Man", "The Incredible Hulk", "Iron Man 2",
			}},
			final: `{"picks":[{"mediaType":"movie","key":"movie:tmdb:21101","name":"Iron Man"},{"mediaType":"movie","key":"movie:tmdb:21102","name":"The Incredible Hulk"},{"mediaType":"movie","key":"movie:tmdb:21103","name":"Iron Man 2"}]}`,
		},
		{
			caseID: "known-franchise",
			arguments: map[string]any{"mode": "collection", "media_type": "movie", "titles": []any{
				"The Matrix", "The Matrix Reloaded", "The Matrix Revolutions",
			}},
			final: `{"picks":[{"mediaType":"movie","key":"movie:tmdb:21201","name":"The Matrix"},{"mediaType":"movie","key":"movie:tmdb:21202","name":"The Matrix Reloaded"},{"mediaType":"movie","key":"movie:tmdb:21203","name":"The Matrix Revolutions"}]}`,
		},
		{
			caseID:    "movie-person",
			arguments: map[string]any{"media_type": "movie", "cast": []any{"Tom Hanks"}, "era": "1990s"},
			final:     `{"picks":[{"mediaType":"movie","key":"movie:tmdb:21301","name":"Joe Versus the Volcano"},{"mediaType":"movie","key":"movie:tmdb:21302","name":"That Thing You Do!"},{"mediaType":"movie","key":"movie:tmdb:21303","name":"You've Got Mail"}],"policy":{"era":{"from":1990,"to":1999}}}`,
		},
		{
			caseID: "tv-network-genre",
			arguments: map[string]any{
				"mode": "network", "media_type": "series", "network": "Apple TV+", "genres": []any{"Science Fiction"},
			},
			final: `{"picks":[{"mediaType":"series","key":"series:tmdb:21701","name":"Foundation"},{"mediaType":"series","key":"series:tmdb:21702","name":"For All Mankind"},{"mediaType":"series","key":"series:tmdb:21703","name":"Silo"}]}`,
		},
		{
			caseID:    "mood-theme",
			arguments: map[string]any{"media_type": "series", "keywords": []any{"cozy", "mystery"}},
			final:     `{"picks":[{"mediaType":"series","key":"series:tmdb:21501","name":"Murder, She Wrote"},{"mediaType":"series","key":"series:tmdb:21502","name":"Father Brown"},{"mediaType":"series","key":"series:tmdb:21503","name":"Miss Fisher's Murder Mysteries"}]}`,
		},
		{
			caseID: "public-reference",
			arguments: map[string]any{"mode": "collection", "media_type": "movie", "titles": []any{
				"E.T. the Extra-Terrestrial", "Hook", "Jurassic Park",
			}},
			final: `{"picks":[{"mediaType":"movie","key":"movie:tmdb:21601","name":"E.T. the Extra-Terrestrial"},{"mediaType":"movie","key":"movie:tmdb:21602","name":"Hook"},{"mediaType":"movie","key":"movie:tmdb:21603","name":"Jurassic Park"}]}`,
		},
	}
	prefixes := []string{
		"", "Please ", "Could you ", "I'd like you to ", "Make me ", "Build ", "Create ", "How about ",
		"Let's try ", "I want ", "My family asked for ", "For tonight, ", "For weekends, ", "Can Loomarr make ",
		"Using my library, ", "Here is the idea: ",
	}
	suffixes := []string{
		"", " please", " with some surprises", " and keep it focused", " for weekends", " that feels curated",
		" with variety", " to review first", " using what I own first", " with acquisitions when useful",
	}

	corpus, err := LoadEmbeddedReleaseGateCorpus()
	if err != nil {
		t.Fatal(err)
	}
	fixtureByCase := make(map[string]string, len(corpus.Cases))
	for _, c := range corpus.Cases {
		fixtureByCase[c.ID] = c.FixtureCase
	}
	checked := 0
	for _, fixture := range fixtures {
		provider := testkit.NewLLM()
		rawGenerator, observer, generatorErr := NewEmbeddedReleaseGateGenerator(provider)
		if generatorErr != nil {
			t.Fatal(generatorErr)
		}
		generator := rawGenerator.(*releaseGateGenerator)
		base, caseErr := ReleaseGateCase(fixture.caseID)
		if caseErr != nil {
			t.Fatal(caseErr)
		}
		for _, prefix := range prefixes {
			for _, suffix := range suffixes {
				description := prefix + base.Intent.Description + suffix
				generator.caseByIntent[description] = fixtureByCase[fixture.caseID]
				variant := base
				variant.Intent.Description = description
				provider.SetResponses(releaseScriptedResponses(t, variant, fixture.arguments, fixture.final)...)
				observer.Begin()
				proposal, suggestErr := generator.Suggest(context.Background(), mapIntent(variant.Intent))
				if suggestErr != nil {
					t.Fatalf("%s variant %q failed: %v; cause=%v", fixture.caseID, description, suggestErr, errors.Unwrap(suggestErr))
				}
				if failures := deterministicChecks(variant, proposal, nil); len(failures) != 0 {
					t.Fatalf("%s variant %q: %v", fixture.caseID, description, failures)
				}
				checked++
			}
		}
	}
	if checked != 1120 {
		t.Fatalf("scripted input variants checked = %d, want 1120", checked)
	}
}

func TestReleaseGateAcceptableMembersAndEndToEndLatencyAreHardGates(t *testing.T) {
	acceptable, err := parseProvisionKeys([]string{"series:tmdb:20101", "series:tmdb:20102", "series:tmdb:20103"})
	if err != nil {
		t.Fatal(err)
	}
	proposal := proposalWithKeys(t, "series:tmdb:20101", "series:tmdb:20102")
	failures := deterministicChecks(Case{AcceptableKeys: acceptable, MinAcceptableKeys: 3}, proposal, nil)
	if len(failures) != 1 || !strings.Contains(failures[0], "acceptable grounded members 2 < required 3") {
		t.Fatalf("acceptable-member failures = %v", failures)
	}

	assessment := assessCertification([]Result{
		{EndToEndLatencyNanos: 7_000_000_000},
		{EndToEndLatencyNanos: 9_000_000_000},
		{EndToEndLatencyNanos: 21_000_000_000},
	}, CertificationThresholds{
		MaxP50EndToEndLatencyNanos:        8_000_000_000,
		MaxP95EndToEndLatencyNanos:        12_000_000_000,
		MaxSuccessfulEndToEndLatencyNanos: 20_000_000_000,
	}, ResourceMeasurement{})
	if assessment.Passed || len(assessment.Failures) != 3 {
		t.Fatalf("latency assessment = %+v, want all three pre-registered failures", assessment)
	}
}

// These are explicit scripted protocol inputs, never injected into a live provider.
func releaseScriptedResponses(t *testing.T, c Case, arguments map[string]any, final string) []llm.Response {
	t.Helper()
	args := make(map[string]any, len(arguments))
	for key, value := range arguments {
		args[key] = value
	}
	delete(args, "era") // Current protocol carries dates only in validated dateMeaning.
	if args["mode"] == "network" {
		delete(args, "mode")
	}
	meaning := map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}
	if c.ExpectEraFrom > 0 {
		decade := fmt.Sprintf("%ds", c.ExpectEraFrom)
		start := strings.Index(c.Intent.Description, decade)
		if start < 0 {
			t.Fatalf("scripted date anchor %q missing in %q", decade, c.Intent.Description)
		}
		axis := "movie_release"
		if args["media_type"] == "series" {
			axis = "series_airing"
		}
		meaning = map[string]any{
			"kind": "constraints", "anchors": []any{map[string]any{"field": "description", "start": start, "end": start + len(decade)}},
			"axes": []any{map[string]any{"kind": axis, "combine": "any", "intervals": []any{map[string]any{"anchor": 0, "start": c.ExpectEraFrom, "end": c.ExpectEraTo}}}},
		}
	}
	args["dateMeaning"] = meaning
	responses := []llm.Response{testkit.ToolCallResponse("catalog_search", args), testkit.FinalResponse(withReleaseDateMeaning(t, final, meaning))}
	if strings.Contains(c.Intent.Description, "https://") {
		responses = append(responses, testkit.FinalResponse(withReleaseDateMeaning(t, final, meaning)))
	}
	return responses
}

func withReleaseDateMeaning(t *testing.T, final string, meaning map[string]any) string {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(final), &decoded); err != nil {
		t.Fatal(err)
	}
	decoded["dateMeaning"] = meaning
	// A historical model-authored scalar era cannot override current validated date semantics.
	if policy, ok := decoded["policy"].(map[string]any); ok {
		delete(policy, "era")
	}
	blob, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	return string(blob)
}
