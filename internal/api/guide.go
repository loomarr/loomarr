package api

import (
	"context"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// guideConcurrency bounds how many channel timelines resolve at once. The work is CPU-bound
// (ComputeDesiredAt), not I/O-bound, so more goroutines than cores buys nothing and an install
// with fifty channels must not starve the rest of the process. Floored at 2 so a single-core
// container still overlaps the little I/O there is.
var guideConcurrency = max(2, runtime.NumCPU())

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
	Kind            string `json:"kind" enum:"program,filler,pending,flex" doc:"What occupies this block: a real program, a commercial pod, an acquisition still in flight, or dead-time padding"`
	ScheduleBlockID string `json:"scheduleBlockId" doc:"Opaque server-authored identity shared with diagnostics and Process runs for this scheduled block"`
	Title           string `json:"title" doc:"The episode's name for a series, or the film's name"`
	Series          string `json:"series,omitempty" doc:"The show's name for an episode block; absent for a movie"`
	Season          int    `json:"season,omitempty"`
	Episode         int    `json:"episode,omitempty"`
	StartMs         int64  `json:"startMs" doc:"Epoch ms, absolute wall-clock"`
	StopMs          int64  `json:"stopMs" doc:"Epoch ms, exclusive"`
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
	// ThumbURL is a preview image for this block: an episode still for a series, a backdrop for a
	// movie, or the first artwork-bearing clip in an actual filler pod. Empty for pending/flex or
	// when no image is available — the client renders a fallback. Both the Guide hover card (§12)
	// and Watch timeline (§9.1 V47) populate it.
	//
	// ⚠ **It points at THIS instance's image service, not TMDB, since V52 phase 7.** The image
	// still originates on TMDB; it is adopted, fetched server-side and served from our own disk, so
	// the Watch timeline stops loading third-party images in the operator's browser (§22).
	ThumbURL string `json:"thumbUrl,omitempty" doc:"Same-origin image-service preview path; empty when unavailable"`
	// ThumbImage is the image record behind ThumbURL — width/height, ThumbHash and both srcsets.
	// A programme's bounded interactive fetch returns real bytes before this is emitted; failure
	// omits both fields cleanly rather than exposing an adopted placeholder row.
	ThumbImage *ImageDTO `json:"thumbImage,omitempty" doc:"The preview image's record, for srcset, animation and placeholder rendering"`
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

// guideArtworkRef is request-local plumbing between schedule projection and the one batched image
// lookup after every row has resolved. Programme images already arrive as image hashes from the
// TMDB adopter; filler entries carry clip identities, which are resolved to their hover/still image
// hashes in one store query rather than one query per break.
type guideArtworkRef struct {
	program          timelineThumbKey
	programImageHash string
	fillerClipHashes []string
}

// GuideChannelTimeline is one channel's row in the grid.
//
// Carries the channel's STATUS and pending count alongside its timeline so a row can show its
// health chip and on-air dot without a second request. The grid is a whole-fleet view — asking
// for /v1/channels as well would double every load to assemble one row, and the two responses
// could disagree mid-render.
type GuideChannelTimeline struct {
	ChannelID string `json:"channelId"`
	Name      string `json:"name"`
	Number    int    `json:"number"`
	Logo      string `json:"logo,omitempty"`
	// Status is the channel's lifecycle state, the SAME vocabulary ChannelDTO uses — the grid
	// derives its health chip and on-air dot from it through the one shared mapping
	// (channel-health.ts), so "is this channel OK?" cannot mean one thing on the list and
	// another here.
	Status string `json:"status" enum:"building,live,empty,drifted,detached,paused"`
	// PendingCount is how many titles are still being acquired. Drives the "Filling in" chip:
	// a live channel with acquisitions in flight is on air but not yet what was asked for.
	PendingCount int           `json:"pendingCount"`
	Airings      []GuideAiring `json:"airings" doc:"In airtime order. Contiguous for airable blocks; pending placeholders may overlap the block that follows them"`
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

	// Channels resolve CONCURRENTLY. Each row's timeline is an independent computation —
	// BroadcastsWithPending re-runs the whole ComputeDesiredAt for that channel, which is
	// hundreds of milliseconds of CPU — and resolving them one after another simply added
	// those up. Measured on the maintainer's 4-channel install: ~490ms of the request was
	// this loop, with only 64ms of it actual library I/O.
	//
	// Bounded rather than unbounded: an install with fifty channels must not open fifty
	// concurrent lineup computations and starve the rest of the process. The cap is small
	// because the work is CPU-bound, not I/O-bound — past the core count more goroutines only
	// add scheduling overhead.
	//
	// Results are written BY INDEX into a pre-sized slice, never appended: the grid's row
	// order is the channel order the store returned (number-sorted), and an append-as-you-
	// finish would reorder the guide by whichever channel happened to compute fastest.
	rows := make([]GuideChannelTimeline, len(channels))
	artworkRefs := make([][]guideArtworkRef, len(channels))
	sem := make(chan struct{}, guideConcurrency)
	var wg sync.WaitGroup

	for i, ch := range channels {
		// EVERY channel, not just internally-played ones. /playout/tuner.m3u filters to the
		// internal backend because a media server must not see two tuners offering the same
		// channel — but this is Loomarr's own UI, where a Tunarr-backed channel is still one of
		// the user's channels and hiding it would look like it had been deleted.
		logo := ch.Logo
		if s.images != nil {
			logo = browserLogoURL(ch.Logo, s.images.PathFor)
		}
		rows[i] = GuideChannelTimeline{
			ChannelID: ch.ID, Name: ch.Name, Number: ch.Number, Logo: logo,
			Status: string(ch.Status),
			// The SAME derivation the channel list uses (DesiredLineup.PendingCount), so a
			// channel cannot read "Filling in" on one surface and healthy on the other.
			PendingCount: schedule.DesiredLineup{Slots: ch.Desired}.PendingCount(),
			Airings:      []GuideAiring{},
		}

		wg.Add(1)
		go func(i int, ch store.Channel) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			bs, err := s.playoutGuide.BroadcastsWithPending(ctx, ch.ID, from, to)
			if err != nil {
				// ONE channel failing must not empty the grid — the same posture as the XMLTV
				// document. The row still appears, with no blocks, which reads as "nothing
				// scheduled here" rather than as a missing channel.
				s.log.Warn("guide: timeline failed for one channel", "channel", ch.ID, "err", err)
				return
			}
			airings := make([]GuideAiring, 0, len(bs))
			refs := make([]guideArtworkRef, 0, len(bs))
			for _, b := range bs {
				a := guideAiringOf(ch.ID, b)
				ref := guideArtworkRef{}
				if b.Kind == schedule.SlotProgram && s.timelineThumbs != nil {
					ref.program = timelineThumbKey{
						key: string(b.Key), season: b.Season, episode: b.Episode,
					}
				}
				// A break's composition, resolved per-airing. Only for filler, and only when the
				// assembler is wired: an install without filler configured shows breaks with no
				// hover detail rather than failing the request.
				if b.Kind == schedule.SlotFiller && s.pods != nil {
					if pod, perr := s.pods.PreviewAt(ctx, ch.ID, b.Start.UnixMilli()); perr == nil && len(pod.Entries) > 0 {
						dto := podToPoolDTO(pod)
						a.Pod = &dto
						for _, entry := range pod.Entries {
							if entry.Hash != "" {
								ref.fillerClipHashes = append(ref.fillerClipHashes, entry.Hash)
							}
						}
					}
				}
				airings = append(airings, a)
				refs = append(refs, ref)
			}
			// The only cross-goroutine write, and it is to this goroutine's OWN index — no two
			// goroutines share an element, so this needs no mutex.
			rows[i].Airings = airings
			artworkRefs[i] = refs
		}(i, ch)
	}
	wg.Wait()

	// TMDB has no multi-title endpoint. Resolve one deduplicated, bounded-concurrent work set for
	// the whole Guide instead of serializing provider latency inside every channel row. Projection
	// back onto the parallel refs preserves repeated airings without repeating their lookup/fetch.
	programmeKeys := []timelineThumbKey{}
	for _, refs := range artworkRefs {
		for _, ref := range refs {
			if ref.program.key != "" {
				programmeKeys = append(programmeKeys, ref.program)
			}
		}
	}
	programmeThumbs := s.resolveTimelineThumbs(ctx, programmeKeys)
	for i := range rows {
		for j := range rows[i].Airings {
			thumb := programmeThumbs[artworkRefs[i][j].program]
			rows[i].Airings[j].ThumbURL = thumb.url
			artworkRefs[i][j].programImageHash = thumb.hash
		}
	}

	// Resolve filler clip identities to artwork in ONE catalog query for the entire Guide. A
	// four-hour window may contain dozens of two-minute pods; looking each clip up inside the
	// airing loop would turn a hover enhancement into a large N+1 on every page load.
	clipHashes := []string{}
	seenClipHashes := map[string]struct{}{}
	for _, refs := range artworkRefs {
		for _, ref := range refs {
			for _, hash := range ref.fillerClipHashes {
				if _, seen := seenClipHashes[hash]; seen {
					continue
				}
				seenClipHashes[hash] = struct{}{}
				clipHashes = append(clipHashes, hash)
			}
		}
	}
	clipsByHash := map[string]store.Clip{}
	if s.images != nil && len(clipHashes) > 0 {
		clips, clipErr := s.store.ListClips(ctx, store.ClipFilter{Hashes: clipHashes})
		if clipErr != nil {
			s.log.Warn("guide: filler artwork lookup failed", "err", clipErr)
		} else {
			for _, clip := range clips {
				clipsByHash[clip.Hash] = clip
			}
		}
	}

	// Pick one preview per airing, preserving pod order. A filler clip's animation is the richest
	// hover preview; its still is the honest fallback when no loop was rendered.
	selectedHashes := make([][]string, len(rows))
	allImageHashes := []string{}
	for i, refs := range artworkRefs {
		selectedHashes[i] = make([]string, len(refs))
		for j, ref := range refs {
			hash := ref.programImageHash
			if hash == "" {
				for _, clipHash := range ref.fillerClipHashes {
					clip, ok := clipsByHash[clipHash]
					if !ok {
						continue
					}
					hash = clip.HoverImageHash
					if hash == "" {
						hash = clip.ThumbImageHash
					}
					if hash != "" {
						break
					}
				}
			}
			selectedHashes[i][j] = hash
			allImageHashes = append(allImageHashes, hash)
		}
	}
	byHash := s.imageDTOsByHash(ctx, allImageHashes)
	for i := range rows {
		for j := range rows[i].Airings {
			if image := byHash[selectedHashes[i][j]]; image != nil {
				rows[i].Airings[j].ThumbImage = image
				if rows[i].Airings[j].ThumbURL == "" {
					rows[i].Airings[j].ThumbURL = image.Src
				}
			}
		}
	}

	out.Body.Channels = rows
	return out, nil
}

// guideAiringOf projects a playout.Broadcast onto the wire shape.
//
// GAPS ARE PRESERVED. Upcoming() drops them (`if e.IsGap() { continue }`), which is right for a
// "what's on next" strip and fatal for a grid: a dropped block leaves a hole that every later
// block silently slides into, so the timeline stops matching the clock. V13b's gate names this
// explicitly — "Upcoming's gap-filtering not reintroduced".
func guideAiringOf(channelID string, b playout.Broadcast) GuideAiring {
	kind := string(b.Kind)
	if kind == "" {
		kind = string(schedule.SlotFlex)
	}
	return GuideAiring{
		Kind:            kind,
		ScheduleBlockID: b.ScheduleBlockID(channelID),
		Title:           b.Title,
		Series:          b.SeriesTitle,
		Season:          b.Season,
		Episode:         b.Episode,
		StartMs:         b.Start.UnixMilli(),
		StopMs:          b.Stop.UnixMilli(),
		Nominal:         b.Nominal,
		Description:     b.Description,
		Genres:          b.Genres,
		Year:            b.Year,
		Rating:          b.Rating,
		ItemID:          b.LibraryItemID,
		RuntimeMs:       b.RuntimeMs,
		Provenance:      b.Provenance,
	}
}

// registerGuide mounts /v1/guide (§12, V13b).
func (s *Server) registerGuide(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "channel-guide", Method: http.MethodGet, Path: "/v1/guide",
		Summary: "Every channel's timeline over a window",
		Description: "Per-channel programme timelines across [from, to), for Loomarr's own time grid. " +
			"Commercial pods, pending acquisitions and flex are all returned with a `kind` discriminator " +
			"rather than being filtered or collapsed to a boolean, so the grid can draw them distinctly. " +
			"Times are absolute epoch ms; a block already in progress reports its real start. " +
			"NOT the XMLTV guide a media server reads — that is /playout/guide.xml. " +
			"Read-only: any authenticated user (§8.1 viewer-facing).",
		Tags: []string{"channels"},
	}, RoleMember), s.channelGuide)
}
