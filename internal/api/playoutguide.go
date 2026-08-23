package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
)

// GET /playout/guide.xml — XMLTV listings (§9.1, V6b).
//
// The tuner M3U tells a media server WHICH channels exist; this tells it WHAT IS ON. Without it
// a channel appears in Live TV, tunes, plays — and shows "no information" forever, which reads
// as a broken integration rather than a missing feature.
//
// The listings come from the SAME cycle arithmetic the encoder uses (playout.BroadcastsBetween
// over the channel's computed lineup), so the guide cannot advertise something different from
// what plays. A guide with its own timeline would eventually drift, and "it says Heat but
// Predator is playing" is the kind of bug nobody can reproduce on demand.

// PlayoutGuide resolves a channel's programme timeline for the EPG. Implemented by an adapter
// over channels.Engine; abstracted so the api package need not import it.
type PlayoutGuide interface {
	// BroadcastsBetween returns the programmes airing on a channel across [from, to), with
	// absolute wall-clock times. Programmes already in progress at `from` report their REAL
	// start, so a media server draws the current show from its actual beginning.
	BroadcastsBetween(ctx context.Context, channelID string, from, to time.Time) ([]playout.Broadcast, error)

	// BroadcastsWithPending is the same walk plus pending acquisitions as nominal-width
	// placeholders, for Loomarr's own time grid (V13b). Separate from BroadcastsBetween
	// because those blocks must never reach a media server's EPG: their times are invented, so
	// advertising one promises an airing that will not happen.
	BroadcastsWithPending(ctx context.Context, channelID string, from, to time.Time) ([]playout.Broadcast, error)
}

// guideWindowHours is how far ahead the guide is published.
//
// 24 hours by default. Long enough that a household browsing tonight's listings sees a full
// evening, short enough that the document stays small and the walk stays cheap — Tunarr's own
// output for four channels over ~14 hours is 98 programmes, so a day across a handful of
// channels is a few hundred entries.
//
// The past matters too: media servers ask for a window that begins BEFORE now so the
// currently-airing programme has a start time to draw from. `guideLookbackHours` covers that.
const (
	guideWindowHours   = 24
	guideLookbackHours = 2
)

// guideHandler serves the XMLTV document.
func (s *Server) guideHandler(w http.ResponseWriter, r *http.Request) {
	if s.playoutGuide == nil {
		s.writeProblem(w, r, http.StatusNotImplemented, "Guide unavailable",
			"Internal playout isn't running on this instance.")
		return
	}

	channels, err := s.playoutChannels(r.Context())
	if err != nil {
		s.log.Warn("playout: guide channel list failed", "err", err)
		s.writeProblem(w, r, statusFromError(err, http.StatusInternalServerError), "Couldn't build the guide",
			"Something went wrong reading your channels.")
		return
	}

	now := time.Now()
	from := now.Add(-guideLookbackHours * time.Hour)
	to := now.Add(guideWindowHours * time.Hour)

	g := playout.Guide{Programmes: map[string][]playout.Broadcast{}}
	for _, ch := range channels {
		num, _ := strconv.Atoi(ch.Number) // the tuner already rendered it from an int
		g.Channels = append(g.Channels, playout.GuideChannel{
			ID: ch.ID, Name: ch.Name, Number: num, IconURL: ch.Logo,
		})

		bs, err := s.playoutGuide.BroadcastsBetween(r.Context(), ch.ID, from, to)
		if err != nil {
			// ONE channel failing must not empty the whole guide. A media server re-fetches
			// the document wholesale, so returning an error here would blank every channel's
			// listings because one had a problem. The channel still appears (see
			// RenderXMLTV) and shows "no information", which is the honest state.
			s.log.Warn("playout: guide timeline failed for one channel",
				"channel", ch.ID, "err", err)
			continue
		}
		g.Programmes[ch.ID] = bs
	}

	doc, err := playout.RenderXMLTV(g, now)
	if err != nil {
		s.log.Warn("playout: guide render failed", "err", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "Couldn't build the guide",
			"Something went wrong generating the listings.")
		return
	}

	// `application/xml` rather than `text/xml`: the latter defaults to US-ASCII per RFC 2376,
	// which would mangle a non-ASCII programme title.
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	// No caching. The guide changes as the wall clock advances — a cached copy served an hour
	// later shows programmes that already finished, which is worse than no guide at all.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(doc)
}
