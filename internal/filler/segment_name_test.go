package filler

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGroundSegmentNameRequiresExactContentEvidence(t *testing.T) {
	tests := []struct {
		name, candidate, evidence, wantName string
		wantOK                              bool
	}{
		{
			name: "grounded visible product", candidate: "Toys R Us commercial",
			evidence: "TOYS R US — THE WORLD'S BIGGEST TOY STORE", wantName: "Toys R Us commercial", wantOK: true,
		},
		{
			name: "grounded non-English product", candidate: "Café La Llave advert",
			evidence: "CAFÉ LA LLAVE — el sabor de nuestra tradición", wantName: "Café La Llave advert", wantOK: true,
		},
		{name: "unsupported identity", candidate: "Coca-Cola holiday commercial", evidence: "ENJOY THE HOLIDAYS"},
		{name: "generic label", candidate: "commercial", evidence: "COMMERCIAL"},
		{name: "wordless", candidate: "Acme advert", evidence: ""},
		{name: "malformed control", candidate: "Acme\u0007 advert", evidence: "ACME"},
		{name: "too many words", candidate: "one two three four five six seven eight nine ten eleven", evidence: "one two three four five six seven eight nine ten eleven"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotEvidence, ok := groundSegmentName(tt.candidate, tt.evidence)
			if ok != tt.wantOK || gotName != tt.wantName {
				t.Fatalf("groundSegmentName(%q, %q) = %q, %q, %v; want %q, _, %v",
					tt.candidate, tt.evidence, gotName, gotEvidence, ok, tt.wantName, tt.wantOK)
			}
			if ok && (gotEvidence == "" || len([]rune(gotEvidence)) > maxSegmentNameEvidenceRunes) {
				t.Fatalf("accepted evidence = %q; want non-empty and bounded", gotEvidence)
			}
		})
	}
}

func TestSplitNameProvenanceSurvivesProposalJSON(t *testing.T) {
	want := SplitProposal{Segments: []SplitSegment{{
		Index: 0, StartMs: 0, EndMs: 30_000, Name: "Acme commercial",
		NameOrigin: SplitNameModelProposed, NameEvidence: "ACME",
	}}}
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got SplitProposal
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Segments) != 1 || got.Segments[0].NameOrigin != SplitNameModelProposed || got.Segments[0].NameEvidence != "ACME" {
		t.Fatalf("round trip = %+v", got.Segments)
	}
}

func TestFallbackSegmentNameIsFriendlyAndBounded(t *testing.T) {
	if got := fallbackSegmentName("TV_Puls-commercial-break.mp4", 3); got != "Clip 3 from TV Puls commercial break" {
		t.Fatalf("friendly fallback = %q", got)
	}
	for _, source := range []string{
		strings.Repeat("a", 80) + ".mp4",
		"209e320a22cf99cfbdbe5ed968691ba1.mp4",
		"550e8400-e29b-41d4-a716-446655440000.mp4",
	} {
		if got := fallbackSegmentName(source, 1); got != "Clip 1 from this recording" {
			t.Errorf("opaque fallback for %q = %q", source, got)
		}
	}
}

func TestProposedSegmentNameFallsBackWhenTranscriptDoesNotSupportProduct(t *testing.T) {
	tests := []struct {
		candidate, evidence, wantName, wantOrigin string
	}{
		{"Swiffer", "[00:02] Swiffer cleans your floor fast.", "Swiffer", SplitNameModelProposed},
		{"Coca-Cola", "[00:02] Enjoy the holidays.", "Clip 2 from holiday reel", SplitNameFallback},
		{"unknown", "[00:02] Music plays.", "Clip 2 from holiday reel", SplitNameFallback},
	}
	for _, tt := range tests {
		name, origin, evidence := proposedSegmentName(tt.candidate, tt.evidence, "holiday-reel.mp4", 2)
		if name != tt.wantName || origin != tt.wantOrigin {
			t.Errorf("proposal %q from %q = %q / %q / %q; want %q / %q",
				tt.candidate, tt.evidence, name, origin, evidence, tt.wantName, tt.wantOrigin)
		}
		if origin == SplitNameFallback && evidence != "" {
			t.Errorf("fallback carried synthetic evidence %q", evidence)
		}
	}
}
