package library

import (
	"context"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit"
)

func TestListEpisodesCarriesProductionYear(t *testing.T) {
	c, ms := newClient(t, Emby)
	ms.SetEpisodeItems(testkit.EpisodeStub{
		LibraryItemID: "episode-1", Name: "Homer at the Bat", RunTimeMs: 1_320_000,
		Season: 3, Episode: 17, ProductionYear: 1992, OfficialRating: "TV-PG",
		CommunityRating: 9.1, Overview: "Springfield plays ball at Christmas", Tags: []string{"holiday", "baseball"},
	})

	episodes, err := c.ListEpisodes(context.Background(), "the-simpsons")
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 1 {
		t.Fatalf("episodes = %+v, want one", episodes)
	}
	got := episodes[0]
	if got.ProductionYear != 1992 || got.Season != 3 || got.Episode != 17 {
		t.Fatalf("episode metadata = %+v, want S03E17 from 1992", got)
	}
	if got.CommunityRating != 9.1 || got.Overview == "" || len(got.Tags) != 2 {
		t.Fatalf("episode editorial evidence = %+v, want rating, overview, and tags", got)
	}
	requests := ms.Requests()
	if len(requests) == 0 || !strings.Contains(requests[len(requests)-1].RawQuery, "CommunityRating") {
		t.Fatalf("episode request did not ask for editorial evidence: %+v", requests)
	}
}
