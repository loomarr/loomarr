package store

import (
	"testing"

	"github.com/loomarr/loomarr/internal/filler"
)

func TestUnmarshalSplitProposal_AcceptsLegacyBareSegmentArray(t *testing.T) {
	var p filler.SplitProposal
	err := unmarshalSplitProposal(`[{"index":0,"startMs":0,"endMs":30000,"name":"legacy"}]`, &p)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Ready() || len(p.Segments) != 1 || p.Segments[0].Name != "legacy" {
		t.Fatalf("legacy proposal = %+v, want one ready segment", p)
	}
}
