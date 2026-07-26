package api

import (
	"context"
	"errors"
	"time"

	"github.com/mantonx/loomarr/internal/store"
)

// "What's on?" — the viewer-facing airtime surfaces (§9 guide freshness, §12).
//
// Split out of channels.go. These read TUNARR's generated guide rather than computing
// airtimes: §6 says Tunarr owns playout for Tunarr-backed channels, so recomputing here would
// duplicate its scheduling math and eventually disagree with it.
//
// ⚠ Not to be confused with /v1/guide (guide.go), which is Loomarr's OWN time grid over
// internal playout, or with /playout/guide.xml, which is XMLTV for a media server's EPG. Three
// surfaces answer "what is on" for three different consumers; only this one delegates to Tunarr.
//
// A missing guide is NOT an error anywhere here: a channel that has never reconciled, or a
// Tunarr hiccup, yields an empty strip rather than blanking the page it sits on.

// NowNextEntry is one program on a channel's timeline (§9 guide freshness). Airtimes
// come from Tunarr, which owns playout — Loomarr owns the lineup (what should play), not
// when it plays, so recomputing these locally would duplicate Tunarr's scheduling math.
type NowNextEntry struct {
	Title   string `json:"title"`
	StartMs int64  `json:"startMs" doc:"Epoch ms; airtime as Tunarr scheduled it"`
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

// channelsNowNext answers the Channels LIST in one upstream call: Tunarr's guide is keyed
// by channel id, so N cards cost one request, not N. A channel with no generated guide
// simply has no entry — an empty guide is "nothing to show", not a failure (finding 4).
func (s *Server) channelsNowNext(ctx context.Context, _ *struct{}) (*nowNextOutput, error) {
	out := &nowNextOutput{}
	out.Body.Channels = []ChannelNowNext{}
	if s.guide == nil {
		return out, nil // guide reader not configured (unit tests, no Tunarr) — empty, not 501
	}
	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	byTunarr, err := s.guide.NowNext(ctx, time.Now())
	if err != nil {
		// The guide is a nicety on a list view; a Tunarr hiccup must not blank the page.
		return out, nil
	}
	for _, ch := range channels {
		if ch.TunarrID == "" {
			continue // never reconciled: nothing is airing yet
		}
		if nn, ok := byTunarr[ch.TunarrID]; ok {
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
// airing now followed by the next few, from Tunarr's generated guide (§6 airtimes). Any
// authenticated user may read it (the guide is viewer-facing, §8.1). A channel with no guide
// yet (never reconciled, or a Tunarr hiccup) returns an empty list, not an error — the strip
// simply shows "nothing scheduled" rather than blanking the page.
func (s *Server) channelUpcoming(ctx context.Context, in *upcomingInput) (*upcomingOutput, error) {
	out := &upcomingOutput{}
	out.Body.Upcoming = []NowNextEntry{}

	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}
	if s.guide == nil || ch.TunarrID == "" {
		return out, nil // no guide reader, or never reconciled — nothing airing yet
	}

	limit := in.Limit
	if limit <= 0 {
		limit = 6
	}
	if limit > 24 {
		limit = 24
	}
	entries, err := s.guide.Upcoming(ctx, ch.TunarrID, time.Now(), limit)
	if err != nil {
		return out, nil // a Tunarr hiccup must not blank the Overview
	}
	out.Body.Upcoming = entries
	return out, nil
}
