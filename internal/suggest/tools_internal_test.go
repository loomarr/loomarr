package suggest

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

func TestSuggestCuratedTitleSubjectRejectsModelSteeredAmbiguity(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{
		{MediaType: provision.Series, Name: "Simpsons", Year: 2020, TMDBID: 9001},
		{MediaType: provision.Series, Name: "The Simpsons", Year: 1989, TMDBID: 456},
	}}
	s := New(testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{
			"mode": "collection", "media_type": "series",
			"titles": []any{map[string]any{"name": "The Simpsons", "year": float64(1989)}}, "dateMeaning": map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}},
		}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"series","tmdbId":456,"name":"The Simpsons"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`),
	), catalog.New(nil, corpus), nil, 10)

	_, err := s.Suggest(context.Background(), Intent{Description: "Classic Simpsons"})
	if !errors.Is(err, ErrNoGroundedTitles) {
		t.Fatalf("model type/year steering admitted ambiguous curated subject: %v", err)
	}
	searches := corpus.Searches()
	if len(searches) < 2 || searches[0].Query != "Simpsons" || searches[1].Query != "The Simpsons" {
		t.Fatalf("source searches = %+v, want raw user subject before canonical model query", searches)
	}
}

func TestGroundCuratedTitleSubjectTreatsDuplicateKeyRowsAsOneIdentity(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{
		{MediaType: provision.Series, Name: "The Simpsons", Year: 1989, TMDBID: 456},
		{MediaType: provision.Series, Name: "The Simpsons", Year: 1989, TMDBID: 456},
	}}
	s := New(nil, catalog.New(nil, corpus), nil, 10)
	intent := Intent{Description: "Classic Simpsons", membershipKeys: make(map[provision.Key]bool), membershipSources: newMembershipSourceState()}

	candidates, err := s.groundCuratedTitleSubject(context.Background(), &intent)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || intent.curatedTitleKey != provision.Key("series:tmdb:456") || len(intent.membershipKeys) != 1 {
		t.Fatalf("duplicate-key grounding = candidates %+v intent %+v", candidates, intent)
	}
	if searches := corpus.Searches(); len(searches) != 1 || searches[0].Query != "Simpsons" {
		t.Fatalf("source searches = %+v, want one raw-subject lookup", searches)
	}
}

func TestParseDiscoveryQueryValidatesAndNormalizesScalarQualifiers(t *testing.T) {
	got, discovery, err := parseDiscoveryQuery(map[string]any{
		"genres":            []any{"Drama"},
		"original_language": " EN ",
		"origin_country":    "gb",
		"runtime_min":       float64(20),
		"runtime_max":       90,
		"vote_average_min":  7.5,
		"vote_count_min":    int64(100),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !discovery || got.OriginalLanguage != "en" || got.OriginCountry != "GB" ||
		got.RuntimeMin != 20 || got.RuntimeMax != 90 || got.VoteAverageMin != 7.5 || got.VoteCountMin != 100 {
		t.Fatalf("normalized discovery query = %+v discovery=%v", got, discovery)
	}
}

func TestCollectionTitleAnchorsAcceptsExactNamesAndRemakeYears(t *testing.T) {
	anchors, err := collectionTitleAnchors([]any{
		" The Thing ", map[string]any{"name": "The Thing", "year": float64(1982)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(anchors) != 2 || anchors[0].name != "The Thing" || anchors[0].year != 0 ||
		anchors[1].name != "The Thing" || anchors[1].year != 1982 {
		t.Fatalf("anchors = %+v", anchors)
	}
}

func TestCollectionTitleAnchorsRejectsMalformedItems(t *testing.T) {
	tests := []struct {
		name  string
		value []any
		want  string
	}{
		{name: "non string", value: []any{12}, want: "strings or"},
		{name: "missing name", value: []any{map[string]any{"year": float64(1982)}}, want: "requires a string name"},
		{name: "unknown object member", value: []any{map[string]any{"name": "The Thing", "edition": "director's cut"}}, want: "only name and year"},
		{name: "bad year", value: []any{map[string]any{"name": "The Thing", "year": 1982.5}}, want: "integer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := collectionTitleAnchors(tt.value); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestRunToolRejectsUnknownCollectionModeAndQualifiers(t *testing.T) {
	s := &Suggester{}
	for _, arguments := range []map[string]any{
		{"mode": "unsupported", "query": "The Thing"},
		{"mode": 1, "query": "The Thing"},
		{"mode": "collection", "media_type": "movie", "titles": []any{"The Thing"}, "era": "1980s"},
	} {
		result, _, _, _ := s.runTool(context.Background(), llm.ToolCall{Name: catalogToolName, Arguments: arguments}, Intent{}, nil)
		if !strings.Contains(result, `"error"`) {
			t.Fatalf("arguments %v unexpectedly succeeded: %s", arguments, result)
		}
	}
}

func TestCatalogToolCollectionSchemaMatchesParser(t *testing.T) {
	properties := catalogTool().Parameters["properties"].(map[string]any)
	titles := properties["titles"].(map[string]any)
	items := titles["items"].(map[string]any)
	oneOf := items["oneOf"].([]any)
	if titles["minItems"] != 1 || titles["maxItems"] != 8 || len(oneOf) != 2 {
		t.Fatalf("titles schema = %#v", titles)
	}
	anchor := oneOf[1].(map[string]any)
	if anchor["additionalProperties"] != false || anchor["required"].([]string)[0] != "name" {
		t.Fatalf("anchor schema = %#v", anchor)
	}
}

func TestParseDiscoveryQueryValidatesAndNormalizesGroundedEntityQualifiers(t *testing.T) {
	movie, discovery, err := parseDiscoveryQuery(map[string]any{
		"media_type": "movie",
		"cast":       []any{" Tom Hanks ", "Meg Ryan"},
		"creators":   []any{" Nora Ephron "},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !discovery || movie.MediaType != "movie" || len(movie.Cast) != 2 || movie.Cast[0] != "Tom Hanks" ||
		len(movie.Creators) != 1 || movie.Creators[0] != "Nora Ephron" {
		t.Fatalf("movie entity query = %+v discovery=%v", movie, discovery)
	}

	series, discovery, err := parseDiscoveryQuery(map[string]any{
		"media_type": "series", "network": " HBO ", "origin_country": "us",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !discovery || series.MediaType != "series" || series.Network != "HBO" || series.OriginCountry != "US" {
		t.Fatalf("series entity query = %+v discovery=%v", series, discovery)
	}
}

func TestParseDiscoveryQueryIgnoresProviderEmptyOptionalPlaceholders(t *testing.T) {
	got, discovery, err := parseDiscoveryQuery(map[string]any{
		"cast":              []any{""},
		"creators":          []any{""},
		"genres":            []any{},
		"keywords":          []any{},
		"media_type":        "series",
		"network":           "ABC",
		"origin_country":    "",
		"original_language": "",
		"query":             "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !discovery || got.Network != "ABC" || got.MediaType != "series" {
		t.Fatalf("normalized provider placeholders = %+v discovery=%v", got, discovery)
	}
	if len(got.Cast) != 0 || len(got.Creators) != 0 || got.OriginalLanguage != "" || got.OriginCountry != "" {
		t.Fatalf("empty optional placeholders survived normalization: %+v", got)
	}
}

func TestProjectCatalogArgumentsKeepsAuthoritativeNetworkQualifiers(t *testing.T) {
	got, ok := projectCatalogArguments(map[string]any{
		"cast": []any{"Tiffani Thiessen"}, "creators": []any{"Jeff Franklin"},
		"genres": []any{"Comedy", "Family"}, "keywords": []any{"TGIF"},
		"media_type": "series", "network": "ABC", "origin_country": "US",
		"original_language": "en", "query": "TGIF", "runtime_min": float64(20),
		"runtime_max": float64(60), "vote_average_min": 6.5, "vote_count_min": float64(100),
	})
	if !ok {
		t.Fatal("valid series/network route was not projected")
	}
	query, discovery, err := parseDiscoveryQuery(got)
	if err != nil || !discovery {
		t.Fatalf("projected network route = (%+v, %v, %v), want valid discovery", query, discovery, err)
	}
	if query.Network != "ABC" || query.OriginalLanguage != "en" || query.OriginCountry != "US" ||
		query.RuntimeMin != 20 || query.RuntimeMax != 60 || query.VoteAverageMin != 6.5 || query.VoteCountMin != 100 {
		t.Fatalf("projected network route lost authoritative qualifiers: %+v", query)
	}
}

func TestProjectCatalogArgumentsRejectsMalformedDiscardedFieldsAndOtherRoutes(t *testing.T) {
	tests := []map[string]any{
		{"media_type": "series", "network": "ABC", "genres": []any{"Drama"}, "cast": []any{17}},
		{"media_type": "series", "network": "ABC", "genres": []any{"Drama"}, "query": 17},
		{"media_type": "series", "network": 17, "genres": []any{"Drama"}},
		{"media_type": "movie", "query": "The Matrix", "cast": []any{"Keanu Reeves"}},
		{"media_type": "movie", "cast": []any{"Tom Hanks"}, "genres": []any{"Comedy"}},
	}
	for _, args := range tests {
		if got, ok := projectCatalogArguments(args); ok {
			t.Fatalf("unsafe or unrelated arguments %#v projected to %#v", args, got)
		}
	}
}

func TestRunToolKeepsAllQualifiersOnAlreadyValidStrictCall(t *testing.T) {
	corpus := &catalogfixture.Corpus{}
	s := New(nil, catalog.New(nil, corpus), nil, 10)
	_, _, _, _ = s.runTool(context.Background(), llm.ToolCall{
		Name: catalogToolName,
		Arguments: map[string]any{
			"media_type": "series", "network": "ABC", "genres": []any{"Comedy"},
			"keywords": []any{"TGIF"}, "dateMeaning": map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}, "origin_country": "US",
			"original_language": "en", "runtime_min": float64(20), "runtime_max": float64(60),
			"vote_average_min": 6.5, "vote_count_min": float64(100),
		},
	}, Intent{}, nil)
	discoveries := corpus.Discoveries()
	if len(discoveries) != 1 {
		t.Fatalf("catalog discoveries = %d, want one strict call", len(discoveries))
	}
	got := discoveries[0].Query
	if got.OriginalLanguage != "en" || got.RuntimeMin != 20 || got.RuntimeMax != 60 ||
		got.VoteAverageMin != 6.5 || got.VoteCountMin != 100 {
		t.Fatalf("strict call lost compatible qualifiers: %+v", got)
	}
}

func TestRunToolDoesNotTurnAnOrdinaryPositiveExampleIntoMembershipProof(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{
		{MediaType: "series", Name: "Full House", TVDBID: 762},
		{MediaType: "series", Name: "Family Matters", TVDBID: 767},
	}}
	intent := Intent{
		Description:    "90s family comedies like Full House",
		membershipKeys: make(map[provision.Key]bool),
	}
	s := New(nil, catalog.New(nil, corpus), nil, 10)
	_, candidates, _, _ := s.runTool(context.Background(), llm.ToolCall{
		Name: catalogToolName, Arguments: map[string]any{"query": "family comedies", "dateMeaning": map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}},
	}, intent, nil)
	if len(candidates) != 2 || len(corpus.Searches()) != 1 {
		t.Fatalf("broad search = candidates %+v searches %+v", candidates, corpus.Searches())
	}
	if len(intent.membershipKeys) != 0 {
		t.Fatalf("ordinary example manufactured membership proof: %+v", intent.membershipKeys)
	}
}

func TestRunToolSourceResolutionIsBoundedAndDeduplicatedPerRequest(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{
		{MediaType: "series", Name: "Full House", TVDBID: 762},
		{MediaType: "series", Name: "Full House", TVDBID: 762},
		{MediaType: "series", Name: "Family Matters", TVDBID: 767},
	}}
	intent := Intent{
		Description:    "A named programming block with Full House and Family Matters",
		membershipKeys: make(map[provision.Key]bool), membershipSources: newMembershipSourceState(),
	}
	s := New(nil, catalog.New(nil, corpus), nil, 10)
	call := llm.ToolCall{Name: catalogToolName, Arguments: map[string]any{"query": "family night", "dateMeaning": map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}}}
	_, _, _, _ = s.runTool(context.Background(), call, intent, nil)
	_, _, _, _ = s.runTool(context.Background(), call, intent, nil)
	searches := corpus.Searches()
	if len(searches) != 4 || searches[0].Query != "family night" || searches[1].Query != "Full House" ||
		searches[2].Query != "Family Matters" || searches[3].Query != "family night" {
		t.Fatalf("catalog searches = %+v, want two base calls plus one deduplicated exact lookup per source title", searches)
	}
	if len(intent.membershipKeys) != 2 {
		t.Fatalf("membership keys = %+v, want both unambiguous source identities", intent.membershipKeys)
	}
}

func TestRunCollectionToolUsesUnfilteredSourceIdentityBeforeModelYear(t *testing.T) {
	tests := []struct {
		name       string
		candidates []catalog.Candidate
		wantKeys   int
	}{
		{name: "different canonical identities stay ambiguous", candidates: []catalog.Candidate{
			{MediaType: "series", Name: "Full House", Year: 1987, TVDBID: 762},
			{MediaType: "series", Name: "Full House", Year: 2020, TVDBID: 999762},
		}},
		{name: "duplicate rows for one key remain unambiguous", candidates: []catalog.Candidate{
			{MediaType: "series", Name: "Full House", Year: 1987, TVDBID: 762},
			{MediaType: "series", Name: "Full House", Year: 1987, TVDBID: 762},
		}, wantKeys: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			corpus := &catalogfixture.Corpus{Candidates: tt.candidates}
			intent := Intent{Description: "A named programming block with Full House", membershipKeys: make(map[provision.Key]bool), membershipSources: newMembershipSourceState()}
			s := New(nil, catalog.New(nil, corpus), nil, 10)
			_, candidates, _, _ := s.runCollectionTool(context.Background(), map[string]any{
				"mode": "collection", "media_type": "series", "titles": []any{map[string]any{"name": "Full House", "year": float64(1987)}},
			}, intent, nil)
			if len(candidates) != 1 || len(intent.membershipKeys) != tt.wantKeys || len(corpus.Searches()) != 1 {
				t.Fatalf("candidates=%+v membership=%+v searches=%+v", candidates, intent.membershipKeys, corpus.Searches())
			}
		})
	}
}

func TestRunToolDoesNotProjectMalformedNonEmptyFields(t *testing.T) {
	tests := []map[string]any{
		{"media_type": "series", "network": 17, "genres": []any{"Drama"}},
		{"media_type": "series", "network": "ABC", "query": 17},
		{"media_type": "series", "network": "ABC", "query": "   "},
		{"media_type": "series", "network": "ABC", "genres": []any{"Drama"}, "cast": []any{17}},
		{"media_type": "series", "network": "ABC", "runtime_min": 20.5},
		{"media_type": "series", "network": "ABC", "cast": []any{"Tiffani Thiessen"}, "genres": []any{17}},
		{"media_type": "series", "network": "ABC", "cast": []any{"Tiffani Thiessen"}, "keywords": []any{17}},
		{"media_type": "series", "network": "ABC", "cast": []any{"Tiffani Thiessen"}, "genres": []any{"   "}},
		{"media_type": "series", "network": "ABC", "creators": []any{"Jeff Franklin"}, "keywords": []any{"\t"}},
		{"media_type": "series", "network": "ABC", "cast": []any{"Tiffani Thiessen"}, "era": 1990},
	}
	for _, arguments := range tests {
		corpus := &catalogfixture.Corpus{}
		s := New(nil, catalog.New(nil, corpus), nil, 10)
		result, candidates, _, _ := s.runTool(context.Background(), llm.ToolCall{
			Name: catalogToolName, Arguments: arguments,
		}, Intent{}, nil)
		if !strings.Contains(result, `"error"`) || len(candidates) != 0 {
			t.Fatalf("malformed arguments %#v returned (%s, %+v), want bounded tool error", arguments, result, candidates)
		}
		if len(corpus.Searches()) != 0 || len(corpus.Discoveries()) != 0 {
			t.Fatalf("malformed arguments %#v reached the Catalog", arguments)
		}
	}
}

func TestParseDiscoveryQueryRejectsMalformedOrBroadeningInputs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{name: "bad language", args: map[string]any{"genres": []any{"Drama"}, "original_language": "english"}, want: "two-letter"},
		{name: "non-string query", args: map[string]any{"network": "ABC", "media_type": "series", "query": 17}, want: "query must be"},
		{name: "blank non-empty query", args: map[string]any{"network": "ABC", "media_type": "series", "query": "   "}, want: "query must be"},
		{name: "non-string genre", args: map[string]any{"network": "ABC", "media_type": "series", "genres": []any{17}}, want: "genres must be"},
		{name: "blank non-empty genre", args: map[string]any{"network": "ABC", "media_type": "series", "genres": []any{"   "}}, want: "genres must be"},
		{name: "non-string keyword", args: map[string]any{"network": "ABC", "media_type": "series", "keywords": []any{17}}, want: "keywords must be"},
		{name: "blank non-empty keyword", args: map[string]any{"network": "ABC", "media_type": "series", "keywords": []any{"\t"}}, want: "keywords must be"},
		{name: "retired era", args: map[string]any{"era": "whenever"}, want: "era is retired"},
		{name: "non-string retired era", args: map[string]any{"network": "ABC", "media_type": "series", "era": 1990}, want: "era is retired"},
		{name: "non-string country", args: map[string]any{"genres": []any{"Drama"}, "origin_country": 44}, want: "two-letter"},
		{name: "fractional runtime", args: map[string]any{"genres": []any{"Drama"}, "runtime_min": 20.5}, want: "integer"},
		{name: "inverted runtime", args: map[string]any{"genres": []any{"Drama"}, "runtime_min": 90, "runtime_max": 20}, want: "must not exceed"},
		{name: "vote average above scale", args: map[string]any{"genres": []any{"Drama"}, "vote_average_min": 10.1}, want: "at most 10"},
		{name: "zero vote average", args: map[string]any{"genres": []any{"Drama"}, "vote_average_min": 0}, want: "greater than 0"},
		{name: "mixed search modes", args: map[string]any{"query": "Alien", "origin_country": "US"}, want: "cannot be combined"},
		{name: "network without series type", args: map[string]any{"network": "HBO"}, want: "requires media_type series"},
		{name: "network on movie", args: map[string]any{"media_type": "movie", "network": "HBO"}, want: "requires media_type series"},
		{name: "cast without movie type", args: map[string]any{"cast": []any{"Tom Hanks"}}, want: "require media_type movie"},
		{name: "creator on series", args: map[string]any{"media_type": "series", "creators": []any{"David Simon"}}, want: "require media_type movie"},
		{name: "network and people", args: map[string]any{"media_type": "series", "network": "HBO", "cast": []any{"Idris Elba"}}, want: "cannot be combined"},
		{name: "malformed cast", args: map[string]any{"media_type": "movie", "cast": []any{"Tom Hanks", 31}}, want: "cast must be"},
		{name: "duplicate creator", args: map[string]any{"media_type": "movie", "creators": []any{"Nora Ephron", " nora ephron "}}, want: "duplicate"},
		{name: "blank network", args: map[string]any{"media_type": "series", "network": "  "}, want: "network must be"},
		{name: "empty request", args: map[string]any{}, want: "provide query"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := parseDiscoveryQuery(tt.args)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("parse error = %v, want substring %q", err, tt.want)
			}
		})
	}
}
