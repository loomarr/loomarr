package fillerstore

import (
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// #2035: an upgrade offers each curated source once, switched off. One the operator added by hand
// is left as theirs, and one they deleted is not offered again.
func testCuratedSourceOffers(t *testing.T, newStore NewStoreFunc) {
	ctx := t.Context()
	s := newStore(t)
	at := time.Unix(1_800_000_000, 0).UTC()
	mine := filler.CuratedSource{Kind: "archive", Target: "mine", Label: "Curated label", Notes: "n"}
	adviews := filler.CuratedSource{Kind: "archive", Target: "adviews", Label: "AdViews", Country: "us", Notes: "n"}
	hand := NewFillerSource(mine.SourceID(), "archive", "mine", "My label", at)
	if err := s.UpsertFillerSource(ctx, hand); err != nil {
		t.Fatal(err)
	}

	added, err := s.OfferCuratedSources(ctx, filler.CuratedList{Version: "v1", Sources: []filler.CuratedSource{mine, adviews}}, at)
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 {
		t.Fatalf("added %d, want only the source this install lacked", added)
	}
	got, ok := findSource(t, s, adviews.SourceID())
	if !ok || got.Enabled || got.Label != "AdViews" || got.URI != "adviews" || got.Geography.Country != "US" {
		t.Fatalf("offered source = %+v (listed %v), want AdViews switched off in the US", got, ok)
	}
	if kept, _ := findSource(t, s, mine.SourceID()); !kept.Enabled || kept.Label != "My label" {
		t.Fatalf("hand-added source = %+v, want it left as the operator made it", kept)
	}

	if err := s.DeleteFillerSource(ctx, adviews.SourceID()); err != nil {
		t.Fatal(err)
	}
	later := filler.CuratedSource{Kind: "youtube", Target: "https://www.youtube.com/channel/UCx/videos", Label: "Later", Notes: "n"}
	added, err = s.OfferCuratedSources(ctx, filler.CuratedList{Version: "v2", Sources: []filler.CuratedSource{mine, adviews, later}}, at.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if added != 1 {
		t.Fatalf("the next version added %d, want only its new entry", added)
	}
	if _, ok := findSource(t, s, adviews.SourceID()); ok {
		t.Fatal("a curated source the operator deleted was offered again")
	}
	if got, ok := findSource(t, s, later.SourceID()); !ok || got.Enabled || got.Kind != "youtube" {
		t.Fatalf("the next version's entry = %+v (listed %v), want it added switched off", got, ok)
	}
}
