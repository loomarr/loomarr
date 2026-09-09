//go:build eval

package eval

import (
	"context"
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestV8BindingDuplicateFullBindingRejected(t *testing.T) {
	provider := testkit.NewLLM()
	_, _, err := newEmbeddedCertificationGenerator(v8MetadataFS(t, func(_, base, _ map[string]any) {
		caseByID(base, "date-movie-adjacent")["description"] = caseByID(base, "date-movie-disjoint")["description"]
		caseByID(base, "date-movie-adjacent")["mustInclude"] = caseByID(base, "date-movie-disjoint")["mustInclude"]
	}), provider)
	if err == nil {
		t.Fatal("duplicate full binding was accepted")
	}
	if provider.Calls != 0 {
		t.Fatal("duplicate binding reached the provider")
	}
}

func TestV8BindingSameDescriptionDistinctIncludesAccepted(t *testing.T) {
	files := v8MetadataFS(t, func(_, base, _ map[string]any) {
		caseByID(base, "date-movie-adjacent")["description"] = "shared"
		caseByID(base, "date-movie-disjoint")["description"] = "shared"
		caseByID(base, "date-movie-adjacent")["mustInclude"] = []any{"one"}
		caseByID(base, "date-movie-disjoint")["mustInclude"] = []any{"two"}
	})
	provider := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"Drama"}, "dateMeaning": map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","tmdbId":11001}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`),
		testkit.ToolCallResponse("catalog_search", map[string]any{"genres": []any{"Drama"}, "dateMeaning": map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","tmdbId":11003}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`),
	)
	g, observer, err := newEmbeddedCertificationGenerator(files, provider)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		include string
		id      int
	}{{"one", 11001}, {"two", 11003}} {
		observer.Begin()
		proposal, err := g.Suggest(context.Background(), suggest.Intent{Description: "shared", MustInclude: []string{tc.include}})
		if err != nil || len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != tc.id {
			t.Fatalf("include %q routed to lineup=%+v, err=%v, want %d", tc.include, proposal.Lineup, err, tc.id)
		}
	}
}

func TestV8BindingChangedIncludeRejectedBeforeProvider(t *testing.T) {
	cases, err := CertificationCases()
	if err != nil {
		t.Fatal(err)
	}
	c := certificationCaseByName(t, cases, "date-repair")
	provider := testkit.NewLLM()
	g, _, err := NewEmbeddedCertificationGenerator(provider)
	if err != nil {
		t.Fatal(err)
	}
	for _, includes := range [][]string{nil, {"changed"}, {c.Intent.MustInclude[0], "extra"}} {
		_, err := g.Suggest(context.Background(), suggest.Intent{Description: c.Intent.Description, MustInclude: includes})
		if err == nil || provider.Calls != 0 {
			t.Fatalf("changed include reached generator: err=%v provider calls=%d", err, provider.Calls)
		}
	}
}

func TestV8BindingEmptyRuntimeIncludeMatchesAbsentInclude(t *testing.T) {
	cases, err := CertificationCases()
	if err != nil {
		t.Fatal(err)
	}
	c := certificationCaseByName(t, cases, "date-title-year-none")
	provider := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "Synthetic Matrix", "dateMeaning": map[string]any{"kind": "none", "anchors": []any{}, "axes": []any{}}}),
		testkit.FinalResponse(`{"picks":[{"mediaType":"movie","tmdbId":10001}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`),
	)
	g, _, err := NewEmbeddedCertificationGenerator(provider)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := g.Suggest(context.Background(), suggest.Intent{Description: c.Intent.Description, MustInclude: []string{}})
	if err != nil || len(proposal.Lineup) != 1 || proposal.Lineup[0].TMDBID != 10001 || provider.Calls != 2 {
		t.Fatalf("empty include binding: lineup=%+v err=%v calls=%d", proposal.Lineup, err, provider.Calls)
	}
}

func TestV8BindingNilAndEmptyIncludesEquivalent(t *testing.T) {
	files := v8MetadataFS(t, func(_, base, _ map[string]any) {
		caseByID(base, "date-movie-adjacent")["description"] = "shared empty"
		caseByID(base, "date-movie-disjoint")["description"] = "shared empty"
		caseByID(base, "date-movie-adjacent")["mustInclude"] = []any{}
		caseByID(base, "date-movie-disjoint")["mustInclude"] = nil
	})
	_, _, err := newEmbeddedCertificationGenerator(files, testkit.NewLLM())
	if err == nil {
		t.Fatal("equivalent empty bindings were not rejected as duplicates")
	}
}

func TestV8MetadataActualCatalogDateFiltering(t *testing.T) {
	metadata, err := loadCertificationCorpus(certificationFiles)
	if err != nil {
		t.Fatal(err)
	}
	fixture := newEmbeddedCatalogFixture(metadata.fixture)
	catalogue := embeddedTMDBCatalog{fixture}
	for _, tc := range []struct {
		name, fixture string
		mediaType     provision.MediaType
		from, to      int
		ids           []int
	}{
		{"early window", "date-movie-disjoint", provision.Movie, 1990, 1991, []int{11003}},
		{"late window", "date-movie-disjoint", provision.Movie, 2000, 2001, []int{11004}},
		{"widened hull", "date-movie-disjoint", provision.Movie, 1990, 2001, []int{11003, 11004, 11005}},
		{"no dates", "date-movie-disjoint", provision.Movie, 0, 0, []int{11003, 11004, 11005}},
		{"wrong media without dates", "date-movie-disjoint", provision.Series, 0, 0, nil},
		{"premiere", "date-series-premiere-airing", provision.Series, 1980, 1980, []int{11006}},
		{"airing is not premiere", "date-series-premiere-airing", provision.Series, 2000, 2000, nil},
		{"historical response unchanged", "genre-discovery", provision.Series, 9998, 9999, []int{10002}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture.selectCase(tc.fixture)
			got, err := catalogue.Discover(context.Background(), catalog.DiscoveryQuery{Genres: []string{"Drama"}, MediaType: tc.mediaType, YearFrom: tc.from, YearTo: tc.to}, 10)
			var ids []int
			for _, candidate := range got {
				ids = append(ids, candidate.TMDBID)
			}
			if err != nil || !reflect.DeepEqual(ids, tc.ids) {
				t.Fatalf("ids=%v err=%v want=%v", ids, err, tc.ids)
			}
		})
	}
	fixture.selectCase("tool-error-recovery")
	if _, err := catalogue.Discover(context.Background(), catalog.DiscoveryQuery{Keywords: []string{"adventure"}}, 10); err == nil {
		t.Fatal("fixture discovery error was lost")
	}
}
