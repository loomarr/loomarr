package suggest

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestIntentCoordinatesPreserveSubmittedRuneSpans(t *testing.T) {
	intent := Intent{Description: "  🎬 café\u2003films from the 1990s", Era: "\t1995–2000\n", RefineText: "keep été", MustInclude: []string{"", "  1980s TV", "é 2000s"}, MustExclude: []string{"\n1991 films"}}
	prompt := userPrompt(intent)
	const marker = "Do not count the surrounding prompt labels.\n"
	_, raw, ok := strings.Cut(prompt, marker)
	if !ok {
		t.Fatal("submitted source coordinates missing from actual user prompt")
	}
	var fields []struct {
		Field  DateAnchorField `json:"field"`
		Index  *int            `json:"index"`
		Length int             `json:"runeLength"`
		Tokens []struct {
			Text  string `json:"text"`
			Start int    `json:"start"`
			End   int    `json:"end"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 6 {
		t.Fatalf("coordinate fields = %d, want 6", len(fields))
	}
	// Literal positions distinguish runes from bytes and preserve leading whitespace.
	want := []DateAnchor{{Field: DateAnchorDescription, Start: 24, End: 29}, {Field: DateAnchorEra, Start: 1, End: 10}, {Field: DateAnchorRefineText, Start: 5, End: 8}, {Field: DateAnchorMustInclude, Index: ptr(1), Start: 2, End: 7}, {Field: DateAnchorMustInclude, Index: ptr(2), Start: 2, End: 7}, {Field: DateAnchorMustExclude, Index: ptr(0), Start: 1, End: 5}}
	for j, f := range fields {
		found := false
		for _, token := range f.Tokens {
			anchor := DateAnchor{Field: f.Field, Index: f.Index, Start: token.Start, End: token.End}
			if _, err := ValidateDateMeaning(intent, &DateMeaning{Kind: DateMeaningAmbiguous, Anchors: []DateAnchor{anchor}, Axes: []DateAxis{}}); err != nil {
				t.Fatalf("invalid advertised coordinate: %+v: %v", anchor, err)
			}
			if anchor.Field == want[j].Field && anchor.Start == want[j].Start && anchor.End == want[j].End {
				found = true
			}
		}
		if !found {
			t.Errorf("missing expected raw span %+v in %+v", want[j], f)
		}
		if (f.Index == nil) != (want[j].Index == nil) || f.Index != nil && *f.Index != *want[j].Index {
			t.Errorf("field %s index=%v, want %v", f.Field, f.Index, want[j].Index)
		}
	}
	if prompt != userPrompt(intent) {
		t.Fatal("coordinates changed across identical rendering")
	}
}
