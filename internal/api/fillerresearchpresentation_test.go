package api

import (
	"testing"

	"github.com/loomarr/loomarr/internal/fillerresearch"
)

func TestClipContextSuggestionUsesOnlyCitedPagesAndYieldsToVerifiedFacts(t *testing.T) {
	report := fillerresearch.Report{Suggestion: fillerresearch.Suggestion{Decade: 1970,
		CountryCode: "US", Country: "United States", Confidence: 70, CitationIDs: []int{2}},
		Packet: fillerresearch.Packet{Citations: []fillerresearch.Citation{
			{ID: 1, Title: "Uncited", URL: "https://en.wikipedia.org/wiki/Uncited"},
			{ID: 2, Title: "Tootsie Pop", URL: "https://en.wikipedia.org/wiki/Tootsie_Pop"},
		}}}
	detail := clipContextSuggestionDTO(report, 0, "")
	if detail == nil || detail.Decade != 1970 || detail.CountryCode != "US" ||
		len(detail.Sources) != 1 || detail.Sources[0].Title != "Tootsie Pop" {
		t.Fatalf("detail = %+v", detail)
	}
	detail = clipContextSuggestionDTO(report, 1999, "")
	if detail == nil || detail.Year != 0 || detail.Decade != 0 || detail.CountryCode != "US" {
		t.Fatalf("verified era did not suppress likely era: %+v", detail)
	}
	if detail := clipContextSuggestionDTO(report, 1999, "GB"); detail != nil {
		t.Fatalf("fully verified context left suggestion: %+v", detail)
	}
}
