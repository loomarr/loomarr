package main

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/store"
)

func TestSeedClipsCreatesDistinctTaxonomyReadyCatalog(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+t.TempDir()+"/seed.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if err := seedClips(ctx, st); err != nil {
		t.Fatal(err)
	}
	clips, err := st.ListClips(ctx, store.ClipFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(clips) != 4 {
		t.Fatalf("seeded filler clips = %d, want 4 distinct rows", len(clips))
	}
	seen := map[string]bool{}
	for _, clip := range clips {
		if clip.Hash == "" || clip.Path == "" {
			t.Errorf("seed clip %q has empty identity: hash=%q path=%q", clip.Name, clip.Hash, clip.Path)
		}
		if seen[clip.Hash] {
			t.Errorf("seed clip hash %q is duplicated", clip.Hash)
		}
		seen[clip.Hash] = true
		if len(clip.AssertedTags) == 0 {
			t.Errorf("seed clip %q has no directly asserted taxonomy tags", clip.Name)
		}
	}
}

func TestSeedApprovalPreservesPendingAcquisitionsInChannel(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+t.TempDir()+"/seed.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	if err := seedTitlesAndChannel(ctx, st, "admin-test"); err != nil {
		t.Fatal(err)
	}
	channels, err := st.ListChannels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 1 {
		t.Fatalf("channels = %d, want 1", len(channels))
	}
	got := make(map[provision.Key]bool, len(channels[0].Lineup))
	for _, entry := range channels[0].Lineup {
		got[entry.Key] = true
	}
	for _, key := range []provision.Key{
		"movie:tmdb:78", "movie:tmdb:9738", "movie:tmdb:8009",
	} {
		if !got[key] {
			t.Errorf("approved acquisition %s was stripped from the seeded channel", key)
		}
	}
}
