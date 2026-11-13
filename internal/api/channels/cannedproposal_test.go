package channels

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/ideas"
	"github.com/loomarr/loomarr/internal/library/provision"
)

type fixedLibrary []ideas.Item

func (l fixedLibrary) IdeaItems(context.Context) ([]ideas.Item, error) { return l, nil }

func libraryOf(genre string, n, from int) fixedLibrary {
	out := make(fixedLibrary, 0, n)
	for i := 0; i < n; i++ {
		id := from + i
		out = append(out, ideas.Item{
			Keys:      []provision.Key{provision.Key(fmt.Sprintf("movie:tmdb:%d", id))},
			MediaType: provision.Movie, Name: fmt.Sprintf("%s film %d", genre, id), Year: 1990 + i, Genres: []string{genre},
		})
	}
	return out
}

// The demo has no AI, so a described channel gets a canned proposal built from the library
// (#2729): the genre the guest named when there is one, otherwise the biggest idea.
func TestCannedProposalFollowsTheGenreTheGuestNamed(t *testing.T) {
	lib := append(libraryOf("Comedy", 7, 100), libraryOf("Mystery", 9, 200)...)
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)

	got, err := CannedProposal(context.Background(), lib, "A channel of cosy comedy for Sunday afternoons", now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(got.ChannelName), "comedy") {
		t.Errorf("channel = %q, want the comedy idea", got.ChannelName)
	}
	if len(got.Lineup) != 7 || got.Scores.AvailabilityRatio != 1 || got.Rationale == "" {
		t.Errorf("proposal = %d titles, availability %v, rationale %q", len(got.Lineup), got.Scores.AvailabilityRatio, got.Rationale)
	}

	got, err = CannedProposal(context.Background(), lib, "Something to fall asleep to", now)
	if err != nil {
		t.Fatal(err)
	}
	// Every film is from the 1990s, so the decade is the biggest idea (all 16 titles).
	if !strings.Contains(got.ChannelName, "90s") || len(got.Lineup) != 16 {
		t.Errorf("channel = %q with %d titles, want the biggest idea when no genre is named", got.ChannelName, len(got.Lineup))
	}
}

func TestCannedProposalNeedsALibrary(t *testing.T) {
	if _, err := CannedProposal(context.Background(), fixedLibrary{}, "anything at all", time.Now()); err == nil {
		t.Fatal("want a refusal when the library has no titles to build a channel from")
	}
}
