package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/schedule"
)

// GET /v1/guide?from=&to= — the time-grid endpoint (§12, V13b).
//
// ⚠ NOT the XMLTV guide. That is /playout/guide.xml: a different format, for a different
// consumer (a media server's EPG), under a different auth model (a device token, not a session).
// This one is JSON for Loomarr's OWN grid UI. The word "guide" means both things, and conflating
// them has already cost one review cycle.
//
// The two share their arithmetic and diverge only in projection, which is the point:
//
//	XMLTV   → advertisable() drops breaks and nominal blocks (decision #12)
//	the grid → shows breaks explicitly, and pending slots as placeholders
//
// Sharing the WALK is what keeps them honest; sharing the FILTER would make one of them wrong.

// GuideAiring is one block on a channel's timeline.
//
// `kind` is a discriminator, deliberately replacing NowNextEntry's `gap bool` (V13b's stated
// gate). A boolean could not tell a commercial pod from a pending acquisition — both were
// simply "not a program" — so the grid could not draw them differently, and "why is there a
// blank hour on my channel" had two completely different answers with the same rendering.
type GuideAiring struct {
	Kind    string `json:"kind" enum:"program,filler,pending,flex" doc:"What occupies this block: a real program, a commercial pod, an acquisition still in flight, or dead-time padding"`
	Title   string `json:"title" doc:"The episode's name for a series, or the film's name"`
	Series  string `json:"series,omitempty" doc:"The show's name for an episode block; absent for a movie"`
	Season  int    `json:"season,omitempty"`
	Episode int    `json:"episode,omitempty"`
	StartMs int64  `json:"startMs" doc:"Epoch ms, absolute wall-clock"`
	StopMs  int64  `json:"stopMs" doc:"Epoch ms, exclusive"`
	// Nominal marks a block whose times are a DISPLAY ESTIMATE rather than real airtime.
	// Today that is exactly the pending blocks: an acquisition has no known duration, so the
	// grid draws it at a fixed nominal width to hold its place in the timeline. A client must
	// not present a nominal block's times as a promise that something airs then.
	Nominal     bool     `json:"nominal,omitempty" doc:"Times are a display estimate, not scheduled airtime"`
	Description string   `json:"description,omitempty"`
	Genres      []string `json:"genres,omitempty"`
	Year        int      `json:"year,omitempty"`
	Rating      string   `json:"rating,omitempty"`
	ItemID      string   `json:"itemId,omitempty" doc:"Media-server item id, when the content is available"`
	// RuntimeMs is the ITEM's own runtime, distinct from stopMs-startMs (how long the block
	// occupies the schedule). They normally agree; where they differ — a 22m episode in a 30m
	// slot — the difference is what makes padding visible.
	RuntimeMs int64 `json:"runtimeMs,omitempty"`
	// Provenance is the one-line "why is this here, and is it real yet?" — "in library",
	// "acquiring · 62% · 8m left", "requested · 41h left". Assembled server-side because it
	// draws on provisioning state, download progress and a deadline measured against now;
	// a client reassembling that sentence would duplicate a decision and drift from it.
	Provenance string `json:"provenance,omitempty"`
	// Pod is the break's ACTUAL composition, for a filler block (§12 hover card): which clips
	// play, in order, and how far down the §10 fallback ladder assembly had to go.
	//
	// Per-airing, not per-channel: it comes from the same seeded assembler internal playout
	// uses for THIS break, so the hover card lists the clips that will really air here rather
	// than a representative sample. Absent for non-filler blocks.
	Pod *PodPoolDTO `json:"pod,omitempty"`
}

// GuideChannelTimeline is one channel's row in the grid.
type GuideChannelTimeline struct {
	ChannelID string        `json:"channelId"`
	Name      string        `json:"name"`
	Number    int           `json:"number"`
	Logo      string        `json:"logo,omitempty"`
	Airings   []GuideAiring `json:"airings" doc:"In airtime order. Contiguous for airable blocks; pending placeholders may overlap the block that follows them"`
}

type guideInput struct {
	FromMs int64 `query:"from" doc:"Window start, epoch ms. Defaults to now."`
	ToMs   int64 `query:"to" doc:"Window end, epoch ms. Defaults to 4 hours after the window start."`
}

type guideOutput struct {
	Body struct {
		FromMs int64 `json:"fromMs" doc:"The window actually served, after clamping"`
		ToMs   int64 `json:"toMs"`
		// Timezone is the IANA name the guide's times should be RENDERED in, or empty for
		// the viewer's own device timezone. The times themselves stay absolute epoch ms —
		// a timezone is a formatting choice, and putting instants on the wire in local time
		// would invite a client to reinterpret rather than merely format them.
		Timezone string                 `json:"timezone,omitempty"`
		Channels []GuideChannelTimeline `json:"channels"`
	}
}

// Window bounds for the grid.
//
// The endpoint walks a repeating cycle, so a huge window is not an error — it is just a lot of
// arithmetic and a lot of JSON. The cap keeps one request from producing a multi-megabyte
// document; the default is what the grid opens on.
const (
	guideDefaultWindow = 4 * time.Hour
	guideMaxWindow     = 7 * 24 * time.Hour
	// How far into the PAST a window may reach. The grid scrolls backwards to "what did I
	// miss", but a channel's cycle is computed from its CURRENT lineup — walk back far enough
	// and the answer is fiction, because the lineup has been reconciled since. A day is honest;
	// a month would be invention.
	//
	// The DEFAULT only; `guide.retention_hours` (§15) is the live value.
	guideMaxLookback = 24 * time.Hour
)

// guideLookback is how far back the window may reach, from `guide.retention_hours` (§15).
//
// Falls back to the compiled default when settings are unwired (unit tests) or the value is
// nonsensical: a zero or negative retention would pin the guide to "now" and leave the grid
// unable to show the programme currently airing, which needs its real start.
func (s *Server) guideLookback() time.Duration {
	if s.liveConfigInt == nil {
		return guideMaxLookback
	}
	if h := s.liveConfigInt("guide.retention_hours"); h > 0 {
		return time.Duration(h) * time.Hour
	}
	return guideMaxLookback
}

// channelGuide serves every channel's timeline over a window.
//
// Read-only and visible to any authenticated user: the guide is viewer-facing (§8.1), the same
// posture as the Overview strip it supersedes.
func (s *Server) channelGuide(ctx context.Context, in *guideInput) (*guideOutput, error) {
	out := &guideOutput{}
	out.Body.Channels = []GuideChannelTimeline{}

	now := time.Now()
	from := time.UnixMilli(in.FromMs)
	if in.FromMs == 0 {
		from = now
	}
	to := time.UnixMilli(in.ToMs)
	if in.ToMs == 0 {
		to = from.Add(guideDefaultWindow)
	}

	// Clamp rather than reject. A grid that scrolls fast can outrun the bounds, and answering
	// "here is the window I could serve" (echoed in the response) keeps it rendering; a 400
	// would blank the page for what is really just an over-eager scroll.
	if earliest := now.Add(-s.guideLookback()); from.Before(earliest) {
		from = earliest
	}
	if !to.After(from) {
		to = from.Add(guideDefaultWindow)
	}
	if to.Sub(from) > guideMaxWindow {
		to = from.Add(guideMaxWindow)
	}
	out.Body.FromMs = from.UnixMilli()
	out.Body.ToMs = to.UnixMilli()
	if s.liveConfig != nil {
		out.Body.Timezone = strings.TrimSpace(s.liveConfig("guide.timezone"))
	}

	if s.playoutGuide == nil {
		// No timeline resolver wired (unit tests, or an install with playout off). An empty
		// grid is the honest answer, not a 501: the page renders its "nothing scheduled" state.
		return out, nil
	}

	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		return nil, err
	}

	for _, ch := range channels {
		// EVERY channel, not just internally-played ones. /playout/tuner.m3u filters to the
		// internal backend because a media server must not see two tuners offering the same
		// channel — but this is Loomarr's own UI, where a Tunarr-backed channel is still one of
		// the user's channels and hiding it would look like it had been deleted.
		row := GuideChannelTimeline{
			ChannelID: ch.ID, Name: ch.Name, Number: ch.Number, Logo: ch.Logo,
			Airings: []GuideAiring{},
		}

		bs, err := s.playoutGuide.BroadcastsWithPending(ctx, ch.ID, from, to)
		if err != nil {
			// ONE channel failing must not empty the grid — the same posture as the XMLTV
			// document. The row still appears, with no blocks, which reads as "nothing
			// scheduled here" rather than as a missing channel.
			s.log.Warn("guide: timeline failed for one channel", "channel", ch.ID, "err", err)
			out.Body.Channels = append(out.Body.Channels, row)
			continue
		}
		for _, b := range bs {
			a := guideAiringOf(b)
			// A break's composition, resolved per-airing. Only for filler, and only when the
			// assembler is wired: an install without filler configured shows breaks with no
			// hover detail rather than failing the request.
			if b.Kind == schedule.SlotFiller && s.pods != nil {
				if pod, perr := s.pods.PreviewAt(ctx, ch.ID, b.Start.UnixMilli()); perr == nil && len(pod.Entries) > 0 {
					dto := podToPoolDTO(pod)
					a.Pod = &dto
				}
			}
			row.Airings = append(row.Airings, a)
		}
		out.Body.Channels = append(out.Body.Channels, row)
	}
	return out, nil
}

// guideAiringOf projects a playout.Broadcast onto the wire shape.
//
// GAPS ARE PRESERVED. Upcoming() drops them (`if e.IsGap() { continue }`), which is right for a
// "what's on next" strip and fatal for a grid: a dropped block leaves a hole that every later
// block silently slides into, so the timeline stops matching the clock. V13b's gate names this
// explicitly — "Upcoming's gap-filtering not reintroduced".
func guideAiringOf(b playout.Broadcast) GuideAiring {
	kind := string(b.Kind)
	if kind == "" {
		kind = string(schedule.SlotFlex)
	}
	return GuideAiring{
		Kind:        kind,
		Title:       b.Title,
		Series:      b.SeriesTitle,
		Season:      b.Season,
		Episode:     b.Episode,
		StartMs:     b.Start.UnixMilli(),
		StopMs:      b.Stop.UnixMilli(),
		Nominal:     b.Nominal,
		Description: b.Description,
		Genres:      b.Genres,
		Year:        b.Year,
		Rating:      b.Rating,
		ItemID:      b.LibraryItemID,
		RuntimeMs:   b.RuntimeMs,
		Provenance:  b.Provenance,
	}
}

// registerGuide mounts /v1/guide (§12, V13b).
func (s *Server) registerGuide(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "channel-guide", Method: http.MethodGet, Path: "/v1/guide",
		Summary: "Every channel's timeline over a window",
		Description: "Per-channel programme timelines across [from, to), for Loomarr's own time grid. " +
			"Commercial pods, pending acquisitions and flex are all returned with a `kind` discriminator " +
			"rather than being filtered or collapsed to a boolean, so the grid can draw them distinctly. " +
			"Times are absolute epoch ms; a block already in progress reports its real start. " +
			"NOT the XMLTV guide a media server reads — that is /playout/guide.xml. " +
			"Read-only: any authenticated user (§8.1 viewer-facing).",
		Tags: []string{"channels"},
	}, s.channelGuide)
}
