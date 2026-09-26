package suggest_test

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

// exampleFillCorpus is the real library's shape for #1499: the two named
// titles, on-era family/comedy neighbours, and off-era titles that a title-word
// search for "back" or "future" drags in (Sister Act 2 1993, To Be or Not to Be
// 1942 were observed filling next to the 1980s anchors).
func exampleFillCorpus() *catalogfixture.Corpus {
	rows := []struct {
		name   string
		year   int
		genres []string
	}{
		{"Back to the Future", 1985, []string{"Adventure", "Comedy", "Science Fiction"}},
		{"Gremlins", 1984, []string{"Comedy", "Horror", "Fantasy"}},
		{"Ghostbusters", 1984, []string{"Comedy", "Fantasy"}},
		{"Honey, I Shrunk the Kids", 1989, []string{"Comedy", "Family", "Science Fiction"}},
		{"Beetlejuice", 1988, []string{"Comedy", "Fantasy"}},
		{"Big", 1988, []string{"Comedy", "Fantasy", "Family"}},
		{"Ferris Bueller's Day Off", 1986, []string{"Comedy"}},
		{"Coming to America", 1988, []string{"Comedy", "Romance"}},
		{"Short Circuit", 1986, []string{"Comedy", "Science Fiction", "Family"}},
		{"Teen Wolf", 1985, []string{"Comedy", "Fantasy"}},
		{"Weekend at Bernie's", 1989, []string{"Comedy"}},
		{"Sister Act 2: Back in the Habit", 1993, []string{"Comedy", "Family", "Music"}},
		{"To Be or Not to Be", 1942, []string{"Comedy", "War"}},
		{"Lover Come Back", 1961, []string{"Comedy", "Romance"}},
		{"Harry Potter and the Prisoner of Azkaban", 2004, []string{"Adventure", "Fantasy", "Family"}},
		{"Your Name.", 2016, []string{"Animation", "Romance", "Fantasy"}},
	}
	cands := make([]catalog.Candidate, 0, len(rows))
	for i, r := range rows {
		cands = append(cands, catalog.Candidate{
			MediaType: "movie", TMDBID: 3000 + i, Name: r.name, Year: r.year, InLibrary: true, Genres: r.genres,
			VoteCount: 5000 - i*100, Overview: "A film in the library.",
		})
	}
	corpus := &catalogfixture.Corpus{Candidates: cands}
	corpus.SearchFunc = func(_ context.Context, query string, limit int) ([]catalog.Candidate, error) {
		norm := func(s string) string {
			return strings.ToLower(strings.NewReplacer(":", " ", "-", " ", "&", " ", ",", " ").Replace(s))
		}
		var out []catalog.Candidate
	next:
		for _, c := range cands {
			for _, w := range strings.Fields(norm(query)) {
				if !strings.Contains(norm(c.Name), w) {
					continue next
				}
			}
			out = append(out, c)
		}
		if limit > 0 && len(out) > limit {
			out = out[:limit]
		}
		return out, nil
	}
	// Discovery honours the structured filters the way TMDB does: genres AND
	// together and the year range bounds the release.
	corpus.DiscoverFunc = func(_ context.Context, q catalog.DiscoveryQuery, limit int) ([]catalog.Candidate, error) {
		var out []catalog.Candidate
	next:
		for _, c := range cands {
			if q.YearFrom > 0 && c.Year < q.YearFrom || q.YearTo > 0 && c.Year > q.YearTo {
				continue
			}
			for _, g := range q.Genres {
				if !slices.Contains(c.Genres, g) {
					continue next
				}
			}
			out = append(out, c)
		}
		return out, nil
	}
	return corpus
}

func exampleFillNames(p suggest.Proposal) []string {
	var names []string
	for _, item := range append(append([]suggest.ProposalItem(nil), p.Lineup...), p.Acquisitions...) {
		names = append(names, item.Name)
	}
	return names
}

func exampleFillYears(p suggest.Proposal) map[string]int {
	years := map[string]int{}
	for _, item := range append(append([]suggest.ProposalItem(nil), p.Lineup...), p.Acquisitions...) {
		years[item.Name] = item.Year
	}
	return years
}

func pickJSON(corpus *catalogfixture.Corpus, names ...string) string {
	var picks []string
	for _, name := range names {
		for _, c := range corpus.Candidates {
			if c.Name == name {
				picks = append(picks, `{"mediaType":"movie","key":"movie:tmdb:`+strconv.Itoa(c.TMDBID)+`","name":`+strconv.Quote(name)+`}`)
			}
		}
	}
	return `{"picks":[` + strings.Join(picks, ",") + `]}`
}

// #1499, off-era fill: the request names two 1980s titles and no era. The
// model's title-word search surfaced Sister Act 2 (1993) and To Be or Not to Be
// (1942) and it picked them. Off-era picks are dropped and the lineup is
// filled from on-era neighbours of the anchors, up to the 8-pick target.
func TestSuggest_ExampleAnchorsFillOnEraAndOnGenre(t *testing.T) {
	corpus := exampleFillCorpus()
	none := dateMeaningNone()
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "back", "media_type": "movie", "dateMeaning": none}),
		finalResponseWithDateMeaning(pickJSON(corpus, "Sister Act 2: Back in the Habit", "To Be or Not to Be"), none),
	)
	s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10)
	proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: "Movies like Back to the Future and Gremlins for a family night"})
	if err != nil {
		t.Fatal(err)
	}
	names, years := exampleFillNames(proposal), exampleFillYears(proposal)
	for _, want := range []string{"Back to the Future", "Gremlins"} {
		if !slices.Contains(names, want) {
			t.Errorf("named anchor %q dropped: %v", want, names)
		}
	}
	for name, year := range years {
		if year < 1979 || year > 1990 {
			t.Errorf("%q (%d) is outside the anchors' era: %v", name, year, names)
		}
	}
	if len(names) != 8 {
		t.Errorf("lineup has %d titles, want the 8-pick target: %v", len(names), names)
	}
}

// #1499, thin fill: the model finalizes with only the anchors. The lineup is
// topped up the same way.
func TestSuggest_ExampleAnchorsOnlyFinalIsToppedUp(t *testing.T) {
	corpus := exampleFillCorpus()
	none := dateMeaningNone()
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "back", "media_type": "movie", "dateMeaning": none}),
		finalResponseWithDateMeaning(pickJSON(corpus, "Back to the Future", "Gremlins"), none),
	)
	s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10)
	proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: "Movies like Back to the Future and Gremlins for a family night"})
	if err != nil {
		t.Fatal(err)
	}
	if names := exampleFillNames(proposal); len(names) != 8 {
		t.Fatalf("lineup has %d titles, want the 8-pick target: %v", len(names), names)
	}
}

// Control: anchors from different eras carry no clear era, so nothing is
// derived from them and no extra discovery runs.
func TestSuggest_ExampleAnchorsWithoutClearEraAreNotFilled(t *testing.T) {
	corpus := exampleFillCorpus()
	corpus.Candidates = append(corpus.Candidates,
		catalog.Candidate{MediaType: "movie", TMDBID: 3900, Name: "Casablanca", Year: 1942, InLibrary: true, Genres: []string{"Drama", "Romance"}, VoteCount: 900},
		catalog.Candidate{MediaType: "movie", TMDBID: 3901, Name: "Avatar", Year: 2009, InLibrary: true, Genres: []string{"Science Fiction", "Adventure"}, VoteCount: 900},
	)
	search := corpus.SearchFunc
	corpus.SearchFunc = func(ctx context.Context, q string, limit int) ([]catalog.Candidate, error) {
		var out []catalog.Candidate
		for _, c := range corpus.Candidates {
			if strings.EqualFold(c.Name, q) {
				out = append(out, c)
			}
		}
		if len(out) == 0 {
			return search(ctx, q, limit)
		}
		return out, nil
	}
	none := dateMeaningNone()
	model := testkit.NewLLM(
		catalogSearchResponse(map[string]any{"query": "Casablanca", "media_type": "movie", "dateMeaning": none}),
		finalResponseWithDateMeaning(pickJSON(corpus, "Casablanca", "Avatar"), none),
	)
	s := suggest.New(model, catalog.New(nil, corpus), referenceExistsValidator{}, 10)
	proposal, err := s.Suggest(context.Background(), suggest.Intent{Description: "Movies like Casablanca and Avatar"})
	if err != nil {
		t.Fatal(err)
	}
	if names := exampleFillNames(proposal); len(names) != 2 {
		t.Errorf("anchors across eras were padded: %v", names)
	}
	if n := len(corpus.Discoveries()); n != 0 {
		t.Errorf("ran %d discovery queries for anchors with no clear era", n)
	}
}
