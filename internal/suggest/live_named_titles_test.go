package suggest_test

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/llm"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

// namedTitlesCorpus is a library that CONTAINS the titles a request names plus
// the "Sunday afternoon" family/kids distractors seen in the #1497 repro.
// Search behaves like a title search: every query word must appear in the name.
func namedTitlesCorpus() *catalogfixture.Corpus {
	adventure := []string{"Adventure", "Family"}
	rows := []struct {
		name   string
		year   int
		genres []string
	}{
		{"Raiders of the Lost Ark", 1981, []string{"Adventure", "Action"}},
		{"Indiana Jones and the Temple of Doom", 1984, []string{"Adventure", "Action"}},
		{"Indiana Jones and the Last Crusade", 1989, []string{"Adventure", "Action"}},
		{"The Goonies", 1985, adventure},
		{"Romancing the Stone", 1984, []string{"Adventure", "Comedy", "Romance"}},
		{"The Princess Bride", 1987, adventure},
		{"Labyrinth", 1986, adventure},
		{"Willow", 1988, []string{"Adventure", "Fantasy"}},
		{"The NeverEnding Story", 1984, adventure},
		{"Stand by Me", 1986, []string{"Adventure", "Drama"}},
		{"Castle in the Sky", 1986, []string{"Animation", "Family", "Adventure"}},
		{"Benji the Hunted", 1987, []string{"Family", "Adventure"}},
		{"He-Man and She-Ra: The Secret of the Sword", 1985, []string{"Animation", "Family"}},
		{"ThunderCats Ho!", 1988, []string{"Animation", "Family"}},
		{"Good Old Boy", 1988, []string{"Family", "Drama"}},
		{"The Land Before Time", 1988, []string{"Animation", "Family"}},
		{"Oliver & Company", 1988, []string{"Animation", "Family"}},
		// The real library holds a modern franchise entry, a namesake of a named
		// title, and more than one title named in a request (#1498 review).
		{"Indiana Jones and the Dial of Destiny", 2023, []string{"Adventure", "Action"}},
		{"The Goonies", 2019, []string{"Documentary"}},
		{"Back to the Future", 1985, []string{"Adventure", "Comedy", "Science Fiction"}},
		{"Gremlins", 1984, []string{"Comedy", "Horror", "Fantasy"}},
		{"Toy Story 5", 2026, []string{"Animation", "Family"}},
	}
	cands := make([]catalog.Candidate, 0, len(rows))
	for i, r := range rows {
		votes := 1000
		if r.year == 2019 {
			votes = 3 // the documentary namesake
		}
		cands = append(cands, catalog.Candidate{
			MediaType: "movie", TMDBID: 2000 + i, Name: r.name, Year: r.year, InLibrary: true, Genres: r.genres,
			VoteCount: votes, Overview: "An 1980s film in the library.",
		})
	}
	corpus := &catalogfixture.Corpus{Candidates: cands}
	corpus.SearchFunc = func(_ context.Context, query string, limit int) ([]catalog.Candidate, error) {
		words := strings.Fields(strings.ToLower(strings.NewReplacer(":", " ", "-", " ", "&", " ").Replace(query)))
		var out []catalog.Candidate
	next:
		for _, c := range cands {
			name := strings.ToLower(strings.NewReplacer(":", " ", "-", " ", "&", " ").Replace(c.Name))
			for _, w := range words {
				if !strings.Contains(name, w) {
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
	return corpus
}

// TestLive_NamedTitles is the #1497 dev harness (skips without LOOMARR_LIVE_LLM_URL):
// the real Suggester against a live model, printing the titles returned per request.
//
//	LOOMARR_LIVE_LLM_URL=http://ai-server:8080/v1 LOOMARR_LIVE_LLM_KEY=... LOOMARR_LIVE_LLM_MODEL=flash-next \
//	LOOMARR_LIVE_RUNS=5 LOOMARR_LIVE_DUMP=1 go test ./internal/suggest -run TestLive_NamedTitles -count=1 -v
func TestLive_NamedTitles(t *testing.T) {
	url := os.Getenv("LOOMARR_LIVE_LLM_URL")
	if url == "" {
		t.Skip("LOOMARR_LIVE_LLM_URL not set")
	}
	runs, _ := strconv.Atoi(os.Getenv("LOOMARR_LIVE_RUNS"))
	if runs < 1 {
		runs = 1
	}
	descriptions := []string{
		"A Sunday-afternoon channel of 1980s adventure movies like Indiana Jones and The Goonies",
		"Classic 1980s family adventure films",
		"80s treasure-hunt and quest movies",
	}
	for _, desc := range descriptions {
		for i := 0; i < runs; i++ {
			rec := &recordingProvider{Provider: llm.NewOpenAI(url, os.Getenv("LOOMARR_LIVE_LLM_MODEL"), os.Getenv("LOOMARR_LIVE_LLM_KEY"))}
			s := suggest.New(rec, catalog.New(nil, namedTitlesCorpus()), referenceExistsValidator{}, 10)
			start := time.Now()
			prop, err := s.Suggest(context.Background(), suggest.Intent{Description: desc})
			var names []string
			if err == nil {
				for _, p := range prop.Lineup {
					names = append(names, p.Name)
				}
				for _, p := range prop.Acquisitions {
					names = append(names, "(acq) "+p.Name)
				}
			}
			var rows []string
			if os.Getenv("LOOMARR_LIVE_DUMP") != "" {
				for n, c := range rec.calls {
					rows = append(rows, fmt.Sprintf("    call%d %s", n+1, c.body))
				}
			}
			t.Logf("%q run %d %.1fs err=%v\n  titles=%v\n%s", desc, i+1, time.Since(start).Seconds(), err, names, strings.Join(rows, "\n"))
		}
	}
}
