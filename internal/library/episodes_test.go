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
	requests := ms.Requests()
	if len(requests) == 0 || !strings.Contains(requests[len(requests)-1].RawQuery, "ProductionYear") {
		t.Fatalf("episode request did not ask for ProductionYear: %+v", requests)
	}
}
