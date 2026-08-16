package api

import (
	"context"
	"errors"
	"time"

	"github.com/mantonx/loomarr/internal/store"
)

// "What's on?" — the viewer-facing airtime surfaces (§9 guide freshness, §12).
//
// Split out of channels.go. The injected guide routes each Loomarr channel to the backend that
// actually streams it. Tunarr-backed channels read Tunarr's generated guide, while internal
// channels share the same timeline as Loomarr's encoder and XMLTV output.
//
// A missing guide is NOT an error anywhere here: a channel that has never reconciled, or a
// playout backend hiccup, yields an empty strip rather than blanking the page it sits on.

// NowNextEntry is one program on a channel's timeline (§9 guide freshness). Airtimes
// come from the selected playout backend. Keeping that backend-owned timeline intact avoids
// recomputing scheduling decisions at the API boundary.
type NowNextEntry struct {
	Title   string `json:"title"`
	StartMs int64  `json:"startMs" doc:"Epoch ms; airtime as the selected playout backend scheduled it"`
	StopMs  int64  `json:"stopMs"`
	Gap     bool   `json:"gap" doc:"A flex/commercial-pod gap rather than a program"`
	TMDBID  string `json:"tmdbId,omitempty" doc:"Joins to a provisioning key (movie:tmdb:<id>) when known"`
}

// ChannelNowNext is what a channel card shows: what is on, and what follows.
type ChannelNowNext struct {
	ChannelID string        `json:"channelId" doc:"Loomarr channel id"`
	Now       *NowNextEntry `json:"now,omitempty"`
	Next      *NowNextEntry `json:"next,omitempty"`
}

type nowNextOutput struct {
	Body struct {
		Channels []ChannelNowNext `json:"channels"`
	}
}

// channelsNowNext answers the Channels LIST in one routed call. The guide is keyed by Loomarr
// channel id, so internal-only channels do not need a remote identity to appear. A channel with
// no generated guide simply has no entry — an empty guide is "nothing to show", not a failure.
func (s *Server) channelsNowNext(ctx context.Context, _ *struct{}) (*nowNextOutput, error) {
	out := &nowNextOutput{}
	out.Body.Channels = []ChannelNowNext{}
	if s.guide == nil {
		return out, nil // guide reader not configured — empty, not 501
	}
	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	byChannel, err := s.guide.NowNext(ctx, time.Now())
	if err != nil {
		// The guide is a nicety on a list view; a backend hiccup must not blank the page.
		return out, nil
	}
	for _, ch := range channels {
		if nn, ok := byChannel[ch.ID]; ok {
			nn.ChannelID = ch.ID
			out.Body.Channels = append(out.Body.Channels, nn)
		}
	}
	return out, nil
}

type upcomingInput struct {
	ID    string `path:"id"`
	Limit int    `query:"limit" doc:"How many programs to return (now + upcoming), default 6, max 24"`
}

type upcomingOutput struct {
	Body struct {
		Upcoming []NowNextEntry `json:"upcoming" doc:"The program airing now (first, if any) then the next ones in airtime order; gaps skipped"`
	}
}

// channelUpcoming answers the Overview "what's on later" strip for ONE channel: the program
// airing now followed by the next few, routed to the backend that streams it. Any authenticated
// user may read it (the guide is viewer-facing, §8.1). A channel with no guide yet returns an
// empty list, not an error — the strip shows "nothing scheduled" rather than blanking the page.
func (s *Server) channelUpcoming(ctx context.Context, in *upcomingInput) (*upcomingOutput, error) {
	out := &upcomingOutput{}
	out.Body.Upcoming = []NowNextEntry{}

	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}
	if s.guide == nil {
		return out, nil // no guide reader — nothing airing yet
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 6
	}
	if limit > 24 {
		limit = 24
	}
	entries, err := s.guide.Upcoming(ctx, ch.ID, time.Now(), limit)
	if err != nil {
		return out, nil // a backend hiccup must not blank the Overview
	}
	out.Body.Upcoming = entries
	return out, nil
}
