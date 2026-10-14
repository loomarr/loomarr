package fillerstore

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// testFillerEnrichmentCandidateSelection pins what the candidate read selects now that it picks
// identities here and loads them through the core's batch read (#1747): held and not-playable clips
// are enrichment work, removed clips and composites are not, the order is (created_at, hash), and
// each clip arrives with its tags.
func testFillerEnrichmentCandidateSelection(t *testing.T, newStore NewStoreFunc) {
	t.Helper()
	s := newStore(t)
	ctx := context.Background()
	at := time.Unix(1_700_000_000, 0).UTC()
	put := func(hash string, created time.Time, edit func(*Clip)) {
		t.Helper()
		clip := sampleClip(hash, hash, filler.Commercial, 0, "", "")
		clip.CreatedAt, clip.UpdatedAt = created, created
		if edit != nil {
			edit(&clip)
		}
		if err := s.UpsertClip(ctx, clip); err != nil {
			t.Fatal(err)
		}
	}
	put("c-late", at.Add(2*time.Second), nil)
	put("b-early", at, nil)
	put("a-early", at, func(c *Clip) { c.Held = true })
	put("d-unplayable", at.Add(3*time.Second), func(c *Clip) { c.Placement = filler.PlacementNotPlayable })
	put("e-composite", at, func(c *Clip) { c.IsComposite = true })
	put("f-removed", at, nil)
	removed, err := s.GetClip(ctx, "f-removed")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetClipsRemoved(ctx, []string{removed.Path}, at); err != nil {
		t.Fatal(err)
	}
	if err := s.SetClipTags(ctx, "b-early", []string{"condiments"}); err != nil {
		t.Fatal(err)
	}

	for name, list := range map[string]func(context.Context, string, string, string, int) ([]Clip, error){
		"current revision": s.ListFillerEnrichmentCandidates,
		"capability":       s.ListFillerEnrichmentCapabilityCandidates,
	} {
		candidates, err := list(ctx, "selection-fixture", "1", "seed-v2", 10)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		var hashes []string
		for _, clip := range candidates {
			hashes = append(hashes, clip.Hash)
			if clip.Hash == "b-early" && !slices.Contains(clip.AssertedTags, "condiments") {
				t.Fatalf("%s: b-early arrived without its tags: %+v", name, clip.AssertedTags)
			}
		}
		if want := []string{"a-early", "b-early", "c-late", "d-unplayable"}; !slices.Equal(hashes, want) {
			t.Fatalf("%s: candidates = %v, want %v", name, hashes, want)
		}
		limited, err := list(ctx, "selection-fixture", "1", "seed-v2", 2)
		if err != nil || len(limited) != 2 || limited[0].Hash != "a-early" || limited[1].Hash != "b-early" {
			t.Fatalf("%s: limit 2 = %+v, err %v", name, limited, err)
		}
	}
}
