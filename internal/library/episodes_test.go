package library

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

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

func TestListEpisodesBoundsEditorialEvidence(t *testing.T) {
	c, ms := newClient(t, Emby)
	tags := []string{" holiday ", "Holiday", "", strings.Repeat("x", 63) + " " + "tail"}
	for i := 0; i < 20; i++ {
		tags = append(tags, fmt.Sprintf("tag-%02d", i))
	}
	ms.SetEpisodeItems(testkit.EpisodeStub{
		LibraryItemID: "episode-1", Name: "Special", RunTimeMs: 1,
		Season: 1, Episode: 1, CommunityRating: 11,
		Overview: strings.Repeat("🎄", 2047) + " " + "tail", Tags: tags,
	})

	episodes, err := c.ListEpisodes(context.Background(), "series")
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 1 {
		t.Fatalf("episodes = %+v, want one", episodes)
	}
	got := episodes[0]
	if got.CommunityRating != 0 {
		t.Fatalf("out-of-domain rating = %v, want unavailable zero", got.CommunityRating)
	}
	if count := utf8.RuneCountInString(got.Overview); count > 2048 || got.Overview != strings.TrimSpace(got.Overview) {
		t.Fatalf("overview is not trimmed within cap: %q (%d runes)", got.Overview, count)
	}
	if len(got.Tags) != 16 {
		t.Fatalf("tags = %v (count %d), want 16 bounded distinct tags", got.Tags, len(got.Tags))
	}
	seen := make(map[string]bool)
	for _, tag := range got.Tags {
		if tag != strings.TrimSpace(tag) || tag == "" || utf8.RuneCountInString(tag) > 64 {
			t.Fatalf("unbounded tag %q in %v", tag, got.Tags)
		}
		folded := strings.ToLower(tag)
		if seen[folded] {
			t.Fatalf("duplicate tag %q in %v", tag, got.Tags)
		}
		seen[folded] = true
	}
}

func TestListEpisodesExcludesStructurallyUnplayableItems(t *testing.T) {
	c, ms := newClient(t, Emby)
	ms.SetEpisodeItems(
		testkit.EpisodeStub{LibraryItemID: "", Name: "Blank identity", RunTimeMs: 1, Season: 1, Episode: 1},
		testkit.EpisodeStub{LibraryItemID: "zero-runtime", Name: "Zero runtime", Season: 1, Episode: 1},
		testkit.EpisodeStub{LibraryItemID: "negative-runtime", Name: "Negative runtime", RunTimeMs: -1, Season: 1, Episode: 1},
		testkit.EpisodeStub{LibraryItemID: "missing-season", Name: "Missing season", RunTimeMs: 1, OmitSeason: true, Episode: 1},
		testkit.EpisodeStub{LibraryItemID: "negative-season", Name: "Negative season", RunTimeMs: 1, Season: -1, Episode: 1},
		testkit.EpisodeStub{LibraryItemID: "missing-episode", Name: "Missing episode", RunTimeMs: 1, Season: 1, OmitEpisode: true},
		testkit.EpisodeStub{LibraryItemID: "zero-episode", Name: "Zero episode", RunTimeMs: 1, Season: 1},
		testkit.EpisodeStub{LibraryItemID: "negative-episode", Name: "Negative episode", RunTimeMs: 1, Season: 1, Episode: -1},
		testkit.EpisodeStub{LibraryItemID: "reversed-span", Name: "Reversed span", RunTimeMs: 1, Season: 1, Episode: 2, EpisodeEnd: 1},
		testkit.EpisodeStub{LibraryItemID: "playable-special", Name: "Playable special", RunTimeMs: 1, Season: 0, Episode: 1},
	)

	episodes, err := c.ListEpisodes(context.Background(), "series")
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 1 || episodes[0].LibraryItemID != "playable-special" {
		t.Fatalf("live episode projection = %+v, want only the structurally playable special", episodes)
	}
}

func TestListEpisodesMakesMalformedOrDuplicateEditorialEvidenceUnavailable(t *testing.T) {
	c, ms := newClient(t, Emby)
	ms.SetRawEpisodeItems(
		`{"Id":"valid-before","Name":"Valid Before","RunTimeTicks":10000,"ParentIndexNumber":1,"IndexNumber":1,`+
			`"CommunityRating":8.1,"Overview":"A grounded neighbor","Tags":["family"]}`,
		`{"Id":"mixed-tags","Name":"Mixed Tags","RunTimeTicks":10000,"ParentIndexNumber":1,"IndexNumber":2,`+
			`"CommunityRating":8.2,"Overview":"Still playable","Tags":["christmas",42]}`,
		`{"Id":"duplicate-editorial","Name":"Duplicate Editorial","RunTimeTicks":10000,"ParentIndexNumber":1,"IndexNumber":3,`+
			`"CommunityRating":9.5,"communityrating":7.5,`+
			`"Overview":"Christmas","Overview":"Halloween",`+
			`"Tags":["christmas"],"tags":["halloween"]}`,
		`{"Id":"unicode-duplicate","Name":"Unicode Duplicate","RunTimeTicks":10000,"ParentIndexNumber":1,"IndexNumber":4,`+
			`"CommunityRating":8.4,"Overview":"Playable","Tags":["christmas"],"Tagſ":["halloween"]}`,
		`{"Id":"valid-after","Name":"Valid After","RunTimeTicks":10000,"ParentIndexNumber":1,"IndexNumber":5,`+
			`"CommunityRating":8.5,"Overview":"Another grounded neighbor","Tags":["holiday"]}`,
	)

	episodes, err := c.ListEpisodes(context.Background(), "series")
	if err != nil {
		t.Fatalf("editorial corruption failed the whole series response: %v", err)
	}
	if len(episodes) != 5 {
		t.Fatalf("episodes = %+v, want all five structurally playable neighbors", episodes)
	}
	if got := episodes[0]; got.CommunityRating != 8.1 || got.Overview == "" || len(got.Tags) != 1 {
		t.Fatalf("valid neighboring evidence was lost: %+v", got)
	}
	if got := episodes[1]; got.CommunityRating != 8.2 || got.Overview == "" || len(got.Tags) != 0 {
		t.Fatalf("mixed tag array was partially retained or damaged other fields: %+v", got)
	}
	if got := episodes[2]; got.CommunityRating != 0 || got.Overview != "" || len(got.Tags) != 0 {
		t.Fatalf("duplicate editorial members were laundered through last-member-wins: %+v", got)
	}
	if got := episodes[3]; got.CommunityRating != 8.4 || got.Overview != "Playable" || len(got.Tags) != 0 {
		t.Fatalf("Unicode case-fold duplicate tags remained available: %+v", got)
	}
	if got := episodes[4]; got.CommunityRating != 8.5 || got.Overview == "" || len(got.Tags) != 1 {
		t.Fatalf("valid trailing neighbor was lost: %+v", got)
	}
}
