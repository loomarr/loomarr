package app

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	channelsapi "github.com/loomarr/loomarr/internal/api/channels"
	"github.com/loomarr/loomarr/internal/proposals"
)

// demoSuggest is the demo site's suggestion service (#2729). The demo has no AI, so a described
// channel does not wait for a model: its proposal is built from the demo library, the way a channel
// idea is, and goes through the same review and approval as any other. Every other call is the real
// service's.
type demoSuggest struct {
	api.SuggestService
	library channelsapi.IdeaLibrary
	now     func() time.Time
}

// Canned tells the proposal route this submitter needs no AI or TMDB settings.
func (demoSuggest) Canned() bool { return true }

func (d demoSuggest) Submit(ctx context.Context, intent proposals.Intent, createdBy string) (string, error) {
	built, err := channelsapi.CannedProposal(ctx, d.library, intent.Description, d.now())
	if err != nil {
		return "", err
	}
	return d.SubmitBuilt(ctx, intent, built, createdBy)
}
