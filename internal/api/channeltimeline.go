package api

import (
	"context"
	"errors"
	"time"

	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// The Watch player's mini-guide timeline (§9.1 Watch, V47).
//
// The Watch player replaces a normal video scrubber (a live channel cannot seek) with a strip of
// the SCHEDULE: the current programme, the next few, and the commercial breaks between them, each
// hover-inspectable with a preview image. This endpoint feeds that strip.
//
// It reads INTERNAL PLAYOUT's guide (playoutGuide.BroadcastsBetween), not Tunarr's — because the
// Watch feature is internal playout, and that reader carries the rich per-block data the strip needs
// (episode name, series, season/episode, the filler blocks as explicit breaks). It reuses the SAME
// GuideAiring projection the main grid uses (guideAiringOf), and adds one thing the grid does not
// need: a TMDB preview image per programme block, resolved server-side from the provisioning key.

// timelineWindow is how far ahead the strip looks — "now + the next few". Long enough to show the
// current programme plus a couple of upcoming items and their breaks; short enough that it is a
// handful of blocks, not the whole day. The reader clips the first block to its real start, so the
// current programme's true beginning anchors the strip.
const timelineWindow = 3 * time.Hour

// TimelineThumbResolver resolves a preview image URL for one programme block (§9.1 V47). An
// interface so the api package need not import tmdb — the same abstraction IconService uses. nil ⇒
// the timeline still works, just with no images (the strip renders a fallback), because TMDB is
// optional and a missing image must never fail the strip.
type TimelineThumbResolver interface {
	// ThumbFor returns a preview image URL for a block, best-effort: "" (never an error to the
	// caller) when there is no key, TMDB is unconfigured, or TMDB has no image. `season`/`episode`
	// are 0 for a movie; a series uses them to fetch the episode's own still.
	ThumbFor(ctx context.Context, key string, season, episode int) string
}

type timelineOutput struct {
	Body struct {
		Airings []GuideAiring `json:"airings" doc:"The current programme (first) then the next few and the breaks between them, in airtime order, with a preview image per programme block"`
	}
}

// channelTimeline is registered inside registerChannels (channels.go), alongside channel-upcoming —
// it is a channel read surface, and grouping it there keeps the register-list parity simple (no new
// register* function to track in api.go/export.go).
func (s *Server) channelTimeline(ctx context.Context, in *upcomingInput) (*timelineOutput, error) {
	out := &timelineOutput{}
	out.Body.Airings = []GuideAiring{}

	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}
	// No internal-playout guide (a Tunarr-only install, or never reconciled) ⇒ an empty strip, not
	// an error: the player falls back to its status line, exactly as the poster does.
	if s.playoutGuide == nil {
		return out, nil
	}

	now := time.Now()
	bs, err := s.playoutGuide.BroadcastsBetween(ctx, ch.ID, now, now.Add(timelineWindow))
	if err != nil {
		return out, nil // a guide hiccup must not blank the player
	}

	airings := make([]GuideAiring, 0, len(bs))
	for _, b := range bs {
		a := guideAiringOf(b)
		// A preview image for PROGRAMME blocks only (a break/flex/pending block has no title image
		// to show). Best-effort and gated on a resolver being wired — an install without TMDB shows
		// the strip with fallbacks rather than failing.
		if b.Kind == schedule.SlotProgram && s.timelineThumbs != nil {
			a.ThumbURL = s.timelineThumbs.ThumbFor(ctx, string(b.Key), b.Season, b.Episode)
		}
		airings = append(airings, a)
	}
	out.Body.Airings = airings
	return out, nil
}
