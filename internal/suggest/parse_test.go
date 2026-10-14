package suggest

import (
	"reflect"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/catalog"
	"github.com/loomarr/loomarr/internal/provision"
)

// The external-LLM smoke found Claude wraps its final JSON in a ```json fence even
// when told "ONLY JSON". parsePicks must read the object out of that wrapping — but
// must NOT be fooled by a brace inside a string, and must still reject true garbage.
func TestParsePicks_UnwrapsAndValidates(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantErr   bool
		wantPicks int
		wantRat   string
	}{
		{
			name:      "bare json (gpt-4o-mini)",
			content:   `{"rationale":"r","picks":[{"mediaType":"movie","key":"movie:tmdb:90000603","name":"Marigold Protocol"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`,
			wantPicks: 1, wantRat: "r",
		},
		{
			name:      "markdown-fenced json (claude)",
			content:   "```json\n{\"rationale\":\"r\",\"picks\":[{\"mediaType\":\"movie\",\"key\":\"movie:tmdb:90000603\",\"name\":\"Marigold Protocol\"}],\"dateMeaning\":{\"kind\":\"none\",\"anchors\":[],\"axes\":[]}}\n```",
			wantPicks: 1, wantRat: "r",
		},
		{
			name:      "prose before and after",
			content:   "Here is the channel:\n{\"rationale\":\"r\",\"picks\":[],\"dateMeaning\":{\"kind\":\"none\",\"anchors\":[],\"axes\":[]}}\nHope that helps!",
			wantPicks: 0, wantRat: "r",
		},
		{
			name:      "brace inside a string value is not the object end",
			content:   `{"rationale":"a {weird} title","picks":[{"mediaType":"movie","key":"movie:tmdb:90000001","name":"Brace } Face"}],"dateMeaning":{"kind":"none","anchors":[],"axes":[]}}`,
			wantPicks: 1, wantRat: "a {weird} title",
		},
		{
			name:    "no json object at all is still an error",
			content: "I could not find anything.",
			wantErr: true,
		},
		{
			name:    "truncated/unbalanced json still errors",
			content: `{"rationale":"r","picks":[`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := parsePicks(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got picks=%v rat=%q", out.Picks, out.Rationale)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(out.Picks) != tt.wantPicks {
				t.Errorf("picks = %d, want %d", len(out.Picks), tt.wantPicks)
			}
			if out.Rationale != tt.wantRat {
				t.Errorf("rationale = %q, want %q", out.Rationale, tt.wantRat)
			}
		})
	}
}

func TestToolResultCarriesGroundedDiscoveryEvidence(t *testing.T) {
	candidates := []catalog.Candidate{{
		MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol", OriginalLanguage: "en",
		OriginCountries: []string{"US"}, RuntimeMinutes: 136, VoteAverage: 8.2, VoteCount: 26000,
		Keywords: []string{"artificial reality"}, Networks: []string{"KBX"},
		Cast: []string{"Calder Rook"}, Creators: []string{"Lena Strand", "Lise Strand"},
	}}

	got := toolResult(candidates)
	if len(got) != 1 || got[0].OriginalLanguage != "en" || got[0].RuntimeMinutes != 136 ||
		got[0].VoteAverage != 8.2 || got[0].VoteCount != 26000 ||
		!reflect.DeepEqual(got[0].OriginCountries, []string{"US"}) ||
		!reflect.DeepEqual(got[0].Keywords, []string{"artificial reality"}) ||
		!reflect.DeepEqual(got[0].Networks, []string{"KBX"}) ||
		!reflect.DeepEqual(got[0].Cast, []string{"Calder Rook"}) ||
		!reflect.DeepEqual(got[0].Creators, []string{"Lena Strand", "Lise Strand"}) {
		t.Fatalf("tool discovery evidence = %+v", got)
	}
}

// userPrompt switches to a REFINE framing when the intent carries refine inputs: it
// leads with "already exists" + the current lineup + the change, and re-asserts the
// grounding requirement — rather than the fresh "Build a channel" framing.
func TestUserPrompt_RefineFraming(t *testing.T) {
	fresh := userPrompt(Intent{Description: "90s action"})
	if !strings.Contains(fresh, "Build a channel:") {
		t.Errorf("fresh prompt missing the build framing:\n%s", fresh)
	}

	refine := userPrompt(Intent{
		Description:   "90s action",
		RefineText:    "add more Juno Castellan",
		CurrentLineup: []LineupContext{{Name: "Marigold Protocol", Year: 1999}, {Name: "The Seventh Satellite Choir", Year: 1991}},
	})
	for _, want := range []string{
		"already exists",
		"Marigold Protocol (1999)",
		"The Seventh Satellite Choir (1991)",
		"add more Juno Castellan",
		"catalog tool", // grounding re-asserted for refine picks
	} {
		if !strings.Contains(refine, want) {
			t.Errorf("refine prompt missing %q:\n%s", want, refine)
		}
	}
	if strings.Contains(refine, "Build a channel:") {
		t.Error("refine prompt should NOT use the fresh build framing")
	}
}

func TestUserPrompt_FindMoreRequestsANewBatchWithoutRepeatingTheLineup(t *testing.T) {
	prompt := userPrompt(Intent{
		Description:   "90s action",
		RefineText:    "Find 6–8 additional titles that match this brief. Do not repeat or replace the selected lineup.",
		CurrentLineup: []LineupContext{{Name: "The Velvet Cartographer", Year: 1995}, {Name: "The Seventh Satellite Choir", Year: 1991}},
	})

	for _, want := range []string{
		"6-8 new well-matched titles",
		"Do not return titles from the current lineup",
		"selected lineup will be preserved separately",
		"catalog tool",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("find-more prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Keep the titles that still fit") {
		t.Errorf("find-more prompt must not spend the bounded result set repeating selected titles:\n%s", prompt)
	}
}
