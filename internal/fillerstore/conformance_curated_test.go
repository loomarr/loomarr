package fillerstore

import (
	"slices"
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

	// A row holding the same target under another id trips the unique (kind, uri) index; the offer
	// keeps that row and still records the rest of the list.
	unprefixed := NewFillerSource("old_ads", "archive", "old_ads", "Old seed", at)
	if err := s.UpsertFillerSource(ctx, unprefixed); err != nil {
		t.Fatal(err)
	}
	oldAds := filler.CuratedSource{Kind: "archive", Target: "old_ads", Label: "Old ads", Notes: "n"}
	if _, err := s.OfferCuratedSources(ctx, filler.CuratedList{Version: "v3", Sources: []filler.CuratedSource{oldAds}}, at.Add(2*time.Hour)); err != nil {
		t.Fatalf("an offer whose target another id holds: %v", err)
	}
	if kept, ok := findSource(t, s, "old_ads"); !ok || !kept.Enabled || kept.Label != "Old seed" {
		t.Fatalf("the row already holding the target = %+v (listed %v), want it untouched", kept, ok)
	}
}

// #2059 decision 10: setting a place switches on the curated sources tagged with its stations, and
// moving away switches them back off. A source the operator had on, or switched off, stays theirs.
func testLocalCuratedSources(t *testing.T, newStore NewStoreFunc) {
	ctx := t.Context()
	s := newStore(t)
	at := time.Unix(1_800_000_000, 0).UTC()
	harbor := filler.CuratedSource{Kind: "archive", Target: "harbor_airchecks", Label: "Harbor", Notes: "n", Stations: []string{"fcc:1", "fcc:2"}}
	lakeside := filler.CuratedSource{Kind: "archive", Target: "lakeside_airchecks", Label: "Lakeside", Notes: "n", Stations: []string{"fcc:3"}}
	mixed := filler.CuratedSource{Kind: "archive", Target: "mixed_ads", Label: "Mixed", Notes: "n"}
	if _, err := s.OfferCuratedSources(ctx, filler.CuratedList{Version: "v1", Sources: []filler.CuratedSource{harbor, lakeside, mixed}}, at); err != nil {
		t.Fatal(err)
	}
	// The operator already switched lakeside on by hand.
	if err := s.SetFillerSourceEnabled(ctx, lakeside.SourceID(), true); err != nil {
		t.Fatal(err)
	}
	enabled := func(id string) bool {
		t.Helper()
		src, ok := findSource(t, s, id)
		if !ok {
			t.Fatalf("source %s is gone", id)
		}
		return src.Enabled
	}
	switchFor := func(near ...string) (on, off []string) {
		t.Helper()
		set := map[string]bool{}
		for _, id := range near {
			set[id] = true
		}
		on, off, err := s.SwitchLocalCuratedSources(ctx, set)
		if err != nil {
			t.Fatal(err)
		}
		return on, off
	}

	on, off := switchFor("fcc:2", "fcc:3")
	if !slices.Equal(on, []string{harbor.SourceID()}) || len(off) != 0 || !enabled(harbor.SourceID()) || enabled(mixed.SourceID()) {
		t.Fatalf("near harbor and lakeside: on %v off %v, want only harbor switched on and the untagged source left off", on, off)
	}
	// Moving away switches harbor back off; lakeside was the operator's and stays on.
	on, off = switchFor()
	if len(on) != 0 || !slices.Equal(off, []string{harbor.SourceID()}) || enabled(harbor.SourceID()) || !enabled(lakeside.SourceID()) {
		t.Fatalf("no place nearby: on %v off %v, want harbor back off and the operator's lakeside left on", on, off)
	}
	// Back near harbor: on again. The operator then switches it off, and it stays off.
	if on, _ = switchFor("fcc:1"); !slices.Equal(on, []string{harbor.SourceID()}) {
		t.Fatalf("back near harbor: on %v, want it switched on again", on)
	}
	if err := s.SetFillerSourceEnabled(ctx, harbor.SourceID(), false); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if on, off = switchFor("fcc:1"); len(on) != 0 || len(off) != 0 || enabled(harbor.SourceID()) {
			t.Fatalf("after the operator switched harbor off: on %v off %v, want it left off", on, off)
		}
	}
	// The next list version retags a source, an earlier offer included.
	lakeside.Stations = []string{"fcc:9"}
	if _, err := s.OfferCuratedSources(ctx, filler.CuratedList{Version: "v2", Sources: []filler.CuratedSource{harbor, lakeside, mixed}}, at); err != nil {
		t.Fatal(err)
	}
	if err := s.SetFillerSourceEnabled(ctx, mixed.SourceID(), false); err != nil {
		t.Fatal(err)
	}
	if on, _ = switchFor("fcc:3"); len(on) != 0 {
		t.Fatalf("near lakeside's old station: on %v, want the retagged source not to match", on)
	}
}
