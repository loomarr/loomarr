package main

import (
	"context"
	"testing"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
)

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
