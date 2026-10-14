package suggest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/catalogfixture"
)

func TestCandidateKeyIsSoleModelIdentity(t *testing.T) {
	candidates := []catalog.Candidate{
		{MediaType: provision.Series, TMDBID: 90021502, Name: "Murmur Bay", InLibrary: true},
		{MediaType: provision.Series, TMDBID: 90000540, TVDBID: 90000762, Name: "Grand Oak Hotel", InLibrary: true},
		{MediaType: provision.Movie, Name: "Unidentified"},
	}
	results := toolResult(candidates)
	blob, err := json.Marshal(results)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].Key != "series:tmdb:90021502" || results[1].Key != "series:tvdb:90000762" ||
		strings.Contains(string(blob), "tmdbId") || strings.Contains(string(blob), "tvdbId") {
		t.Fatalf("model candidates must expose only canonical keys: %s", blob)
	}
	intent := Intent{Description: "series"}
	meaning, err := ValidateDateMeaning(intent, &DateMeaning{Kind: DateMeaningNone})
	if err != nil {
		t.Fatal(err)
	}
	surfaced := map[provision.Key]catalog.Candidate{
		"series:tmdb:90021502": candidates[0], "series:tvdb:90000762": candidates[1],
	}
	for _, tc := range []struct {
		name, key string
		want      string
	}{
		{"tmdb only", "series:tmdb:90021502", "Murmur Bay"},
		{"preferred tvdb", "series:tvdb:90000762", "Grand Oak Hotel"},
		{"copied numeric id in wrong namespace", "series:tvdb:90021502", ""},
		{"remembered tvdb id", "series:tvdb:90269680", ""},
		{"alternate known id was not surfaced", "series:tmdb:90000540", ""},
		{"wrong medium", "movie:tmdb:90021502", ""},
		{"fabricated", "series:tmdb:90999999", ""},
		{"noncanonical decimal", "series:tmdb:090021502", ""},
		{"whitespace", " series:tmdb:90021502", ""},
		{"malformed", "90021502", ""},
		{"missing", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Model descriptions and obsolete fields cannot replace the surfaced identity.
			out, err := parsePicks(fmt.Sprintf(`{"picks":[{"key":%q,"mediaType":"movie","name":"Invented name","tmdbId":90099999,"tvdbId":90269680}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`, tc.key))
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := (&Suggester{maxAcq: 3}).buildProposal(context.Background(), intent, out, surfaced, &DecisionTrace{}, meaning)
			if tc.want == "" {
				if !errors.Is(err, ErrNoGroundedTitles) || len(proposal.Lineup)+len(proposal.Acquisitions) != 0 {
					t.Fatalf("ungrounded key survived: %+v, err = %v", proposal, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if len(proposal.Lineup) != 1 || proposal.Lineup[0].Name != tc.want {
					t.Fatalf("proposal identity did not come from surfaced Catalog candidate: %+v", proposal.Lineup)
				}
				key, err := proposal.Lineup[0].Key()
				if err != nil || string(key) != tc.key {
					t.Fatalf("proposal key = %q, err = %v", key, err)
				}
			}
		})
	}
}

func TestNamesOnlyDirectFinalReceivesCatalogKey(t *testing.T) {
	corpus := &catalogfixture.Corpus{Candidates: []catalog.Candidate{{
		MediaType: provision.Series, TMDBID: 90021502, TVDBID: 90269680, Name: "Murmur Bay", Year: 2013, InLibrary: true,
	}}}
	model := testkit.NewLLM(testkit.FinalResponse(`{"picks":[{"mediaType":"series","name":"Murmur Bay","year":2013}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`))
	proposal, err := New(model, catalog.New(nil, corpus), nil, 3).Suggest(context.Background(), Intent{Description: "mysteries"})
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Lineup) != 1 || model.Calls != 1 || len(corpus.Searches()) != 1 {
		t.Fatalf("bounded names-only recovery = %+v; model calls %d; searches %d", proposal.Lineup, model.Calls, len(corpus.Searches()))
	}
	key, err := proposal.Lineup[0].Key()
	if err != nil || key != "series:tvdb:90269680" {
		t.Fatalf("resolved key = %q, err = %v", key, err)
	}
}
