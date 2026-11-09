package channels

import (
	"context"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/ideas"
	"github.com/loomarr/loomarr/internal/proposals"
)

// CannedProposal is the demo site's stand-in for the AI (#2729): the demo has no model, so a
// described channel gets a proposal built from the library alone, the way a channel idea is. It
// picks the idea whose genre the description names, or the biggest idea when it names none, so a
// guest sees the same review card a real suggestion would produce.
func CannedProposal(ctx context.Context, lib IdeaLibrary, description string, now time.Time) (proposals.Proposal, error) {
	items, err := lib.IdeaItems(ctx)
	if err != nil {
		return proposals.Proposal{}, httpcore.ProblemWithCause(502, "Couldn't read the library",
			"The demo library didn't answer. Try again in a moment.", err)
	}
	built := ideas.Build(ideas.Input{Library: items, Now: now})
	if len(built) == 0 {
		return proposals.Proposal{}, httpcore.BadRequest("Nothing to build a channel from",
			"The demo library has too few titles in one genre to make a channel.")
	}
	chosen := built[0]
	words := strings.ToLower(description)
	for _, idea := range built {
		if idea.Facet == ideas.FacetGenre && strings.Contains(words, strings.ToLower(idea.Value)) {
			chosen = idea
			break
		}
	}
	name, pitch := ideas.Describe(chosen, "")
	return proposals.Proposal{
		ChannelName: name, Lineup: IdeaLineup(chosen.Titles), Rationale: pitch,
		Scores: proposals.Scores{AvailabilityRatio: 1},
	}, nil
}
