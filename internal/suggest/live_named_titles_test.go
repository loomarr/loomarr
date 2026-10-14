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

// liveNamedTitlesCorpus is namedTitlesCorpus with the real titles of the #1497
// repro: a live model's world knowledge of them is what this harness exercises.
func liveNamedTitlesCorpus() *catalogfixture.Corpus {
	adventure := []string{"Adventure", "Family"}
	return namedTitlesCorpusOf([]titleRow{
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
	})
}

// liveExampleFillCorpus is exampleFillCorpus with the real titles of the #1499 library.
func liveExampleFillCorpus() *catalogfixture.Corpus {
	return exampleFillCorpusOf([]titleRow{
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
	})
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
	if d := os.Getenv("LOOMARR_LIVE_DESC"); d != "" {
		descriptions = []string{d}
	}
	for _, desc := range descriptions {
		for i := 0; i < runs; i++ {
			rec := &recordingProvider{Provider: llm.NewOpenAI(url, os.Getenv("LOOMARR_LIVE_LLM_MODEL"), os.Getenv("LOOMARR_LIVE_LLM_KEY"))}
			corpus := liveNamedTitlesCorpus()
			if os.Getenv("LOOMARR_LIVE_CORPUS") == "era" {
				corpus = liveExampleFillCorpus() // #1499: on-era neighbours plus off-era distractors
			}
			s := suggest.New(rec, catalog.New(nil, corpus), referenceExistsValidator{}, 10)
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
