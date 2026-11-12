package suggest_test

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/library/tmdb"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

// #3365: a "cozy holiday movies" request kept a slasher the model's own rationale called "far
// too violent". A pick whose reason argues against it is dropped, and so is a Horror title for a
// soft-toned request, whatever the model wrote.
func TestProposal_DropsPicksTheRationaleRejectsAndHorrorForACozyRequest(t *testing.T) {
	lib := catalogfixture.NewScriptedLibrary()
	lib.SetSearchItems(
		catalogfixture.SearchStub{Terms: []string{"christmas"}, LibraryItemID: "lib-1", Name: "Snowed Inn", Type: "Movie", Year: 2004, TMDBID: 90006001, Genres: []string{"Comedy", "Family"}, OfficialRating: "PG"},
		catalogfixture.SearchStub{Terms: []string{"christmas"}, LibraryItemID: "lib-2", Name: "Sleigh Ride Slasher", Type: "Movie", Year: 2024, TMDBID: 90006002, Genres: []string{"Thriller"}, OfficialRating: "NR"},
		catalogfixture.SearchStub{Terms: []string{"christmas"}, LibraryItemID: "lib-3", Name: "Krampus Night", Type: "Movie", Year: 2015, TMDBID: 90006003, Genres: []string{"Horror", "Comedy"}, OfficialRating: "PG-13"},
	)
	llmMock := testkit.NewLLM(t,
		catalogSearchResponse(map[string]any{"query": "christmas"}),
		finalResponseWithNone(`{"rationale":"cozy holiday films","picks":[
			{"mediaType":"movie","key":"movie:tmdb:90006001","name":"Snowed Inn","rationale":"Warm holiday comedy"},
			{"mediaType":"movie","key":"movie:tmdb:90006002","name":"Sleigh Ride Slasher","rationale":"Holiday-set but far too violent"},
			{"mediaType":"movie","key":"movie:tmdb:90006003","name":"Krampus Night","rationale":"A festive family romp"}
		]}`),
	)
	mt := testkit.NewTMDB(t)
	s := suggest.New(llmMock, titlesearch.New(lib, tmdb.NewWithBase(mt.URL, "key")), tmdb.NewWithBase(mt.URL, "key"), 10)

	prop, err := s.Suggest(context.Background(), proposals.Intent{Description: "Cozy holiday movies"})
	if err != nil {
		t.Fatal(err)
	}
	if len(prop.Lineup) != 1 || prop.Lineup[0].TMDBID != 90006001 {
		t.Fatalf("lineup = %+v, want only the warm holiday comedy", prop.Lineup)
	}
	if !traceHas(prop.Trace, "movie:tmdb:90006002", proposals.DispositionValidationDropped, proposals.ReasonRationaleRejects) {
		t.Fatalf("trace omitted the rationale rejection: %+v", prop.Trace)
	}
	if !traceHas(prop.Trace, "movie:tmdb:90006003", proposals.DispositionValidationDropped, proposals.ReasonToneMismatch) {
		t.Fatalf("trace omitted the tone mismatch: %+v", prop.Trace)
	}
}

// A Horror title the request names itself stays: the household asked for it.
func TestProposal_KeepsAHorrorTitleACozyRequestNames(t *testing.T) {
	lib := catalogfixture.NewScriptedLibrary()
	lib.SetSearchItems(
		catalogfixture.SearchStub{Terms: []string{"christmas"}, LibraryItemID: "lib-1", Name: "Snowed Inn", Type: "Movie", Year: 2004, TMDBID: 90006001, Genres: []string{"Comedy", "Family"}, OfficialRating: "PG"},
		catalogfixture.SearchStub{Terms: []string{"christmas"}, LibraryItemID: "lib-3", Name: "Krampus Night", Type: "Movie", Year: 2015, TMDBID: 90006003, Genres: []string{"Horror", "Comedy"}, OfficialRating: "PG-13"},
	)
	llmMock := testkit.NewLLM(t,
		catalogSearchResponse(map[string]any{"query": "christmas"}),
		finalResponseWithNone(`{"rationale":"cozy holiday films","picks":[
			{"mediaType":"movie","key":"movie:tmdb:90006001","name":"Snowed Inn","rationale":"Warm holiday comedy"},
			{"mediaType":"movie","key":"movie:tmdb:90006003","name":"Krampus Night","rationale":"Requested by name"}
		]}`),
	)
	mt := testkit.NewTMDB(t)
	s := suggest.New(llmMock, titlesearch.New(lib, tmdb.NewWithBase(mt.URL, "key")), tmdb.NewWithBase(mt.URL, "key"), 10)

	prop, err := s.Suggest(context.Background(), proposals.Intent{Description: "Cozy holiday movies", MustInclude: []string{"Krampus Night"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(prop.Lineup) != 2 {
		t.Fatalf("lineup = %+v, want both picks including the named Horror title", prop.Lineup)
	}
}
