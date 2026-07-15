package suggest

import "testing"

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
			content:   `{"rationale":"r","picks":[{"mediaType":"movie","tmdbId":603,"name":"The Matrix"}]}`,
			wantPicks: 1, wantRat: "r",
		},
		{
			name:      "markdown-fenced json (claude)",
			content:   "```json\n{\"rationale\":\"r\",\"picks\":[{\"mediaType\":\"movie\",\"tmdbId\":603,\"name\":\"The Matrix\"}]}\n```",
			wantPicks: 1, wantRat: "r",
		},
		{
			name:      "prose before and after",
			content:   "Here is the channel:\n{\"rationale\":\"r\",\"picks\":[]}\nHope that helps!",
			wantPicks: 0, wantRat: "r",
		},
		{
			name:      "brace inside a string value is not the object end",
			content:   `{"rationale":"a {weird} title","picks":[{"mediaType":"movie","tmdbId":1,"name":"Brace } Face"}]}`,
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
