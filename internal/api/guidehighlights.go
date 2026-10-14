package api

import (
	"cmp"
	"context"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// GET /v1/guide/highlights — the few airings worth calling out tonight, each with a typed reason
// (#1664). Home's Tonight list shows them; the client words the reason ("Season 4 premiere",
// "Back-to-back until 11") from the enum and its parameters, so no client ranks airings or
// hard-codes the phrasing.
//
// What counts is the maintainer's call (#1659 H3): premieres and marathons. Movies and "new on
// this channel" are not highlights.
//
//   - A RUN is one show's episodes airing back to back on a channel; breaks and dead air between
//     them don't end it, any other programme does.
//   - A run holding an episode 1 (season 1 or later; specials don't count) is a premiere: a series
//     premiere for season 1, a season premiere after. If three or more episodes follow from it,
//     the premiere carries the run ("Season 4 premiere, back-to-back until 11"). Only where the
//     channel airs episodes in order at that time (schedule.OrderingAt, sequential only): shuffle
//     and syndication deal a deck, where an episode 1 is chance and every deal would be full of
//     "premieres".
//   - Otherwise a run of three or more is a marathon.
//
// One highlight per run, and one per show on a channel (a looping channel's rerun is not news).
// Premieres outrank marathons, longer marathons outrank shorter; every channel gets one before
// any gets a second; the chosen few come back in airtime order.

type highlightReason string

const (
	highlightSeriesPremiere highlightReason = "series_premiere"
	highlightSeasonPremiere highlightReason = "season_premiere"
	highlightMarathon       highlightReason = "marathon"
)

// marathonMinEpisodes is the shortest back-to-back run called a marathon. Two in a row is
// ordinary scheduling; three is a block someone would plan an evening around.
const marathonMinEpisodes = 3

const (
	highlightsDefaultLimit  = 4
	highlightsMaxLimit      = 20
	highlightsDefaultWindow = 6 * time.Hour
	highlightsMaxWindow     = 24 * time.Hour
)

type channelAirings struct {
	channelID  string
	broadcasts []playout.Broadcast
	// shuffledAt reports whether the channel deals its episodes out of order at a time (shuffle or
	// syndication). An episode 1 turning up in a deal is chance, not a premiere. nil means in order.
	shuffledAt func(time.Time) bool
}

type highlightPick struct {
	channelID string
	airing    playout.Broadcast
	reason    highlightReason
	// episodes and until describe the back-to-back run from the airing, when it is a marathon
	// (or a premiere that opens one); zero otherwise.
	episodes int
	until    time.Time
}

func highlightRank(p highlightPick) int {
	switch p.reason {
	case highlightSeriesPremiere:
		return 0
	case highlightSeasonPremiere:
		return 1
	default:
		return 2
	}
}

// pickHighlights chooses up to `limit` highlights across the channels. Pure: the airings in, the
// picks out, so the rules above are tested without a schedule.
func pickHighlights(channels []channelAirings, limit int) []highlightPick {
	var candidates []highlightPick
	for _, ch := range channels {
		candidates = append(candidates, channelHighlights(ch)...)
	}
	slices.SortStableFunc(candidates, func(a, b highlightPick) int {
		return cmp.Or(
			cmp.Compare(highlightRank(a), highlightRank(b)),
			cmp.Compare(b.episodes, a.episodes),
			a.airing.Start.Compare(b.airing.Start),
			cmp.Compare(a.channelID, b.channelID),
		)
	})
	// A channel looping a short pool airs the same premiere or marathon every cycle; the rerun is
	// not news. One highlight per show on a channel: its best, the first in rank order.
	type channelShow struct {
		channelID string
		key       provision.Key
	}
	seen := map[channelShow]bool{}
	candidates = slices.DeleteFunc(candidates, func(c highlightPick) bool {
		show := channelShow{c.channelID, c.airing.Key}
		dup := seen[show]
		seen[show] = true
		return dup
	})
	picked := make([]bool, len(candidates))
	var out []highlightPick
	// First pass: the best highlight on each channel. Second: fill from the rest.
	onChannel := map[string]bool{}
	for i, c := range candidates {
		if len(out) == limit {
			break
		}
		if !onChannel[c.channelID] {
			onChannel[c.channelID] = true
			picked[i] = true
			out = append(out, c)
		}
	}
	for i, c := range candidates {
		if len(out) == limit {
			break
		}
		if !picked[i] {
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b highlightPick) int {
		return cmp.Or(a.airing.Start.Compare(b.airing.Start), cmp.Compare(a.channelID, b.channelID))
	})
	return out
}

// channelHighlights finds one channel's runs and turns each into at most one highlight.
func channelHighlights(ch channelAirings) []highlightPick {
	var out []highlightPick
	var run []playout.Broadcast
	flush := func() {
		if len(run) == 0 {
			return
		}
		if pick, ok := runHighlight(ch, run); ok {
			out = append(out, pick)
		}
		run = nil
	}
	for _, b := range ch.broadcasts {
		if b.Kind != schedule.SlotProgram || b.Nominal {
			if b.Nominal {
				flush() // a pending placeholder has no real airtime to be back to back with
			}
			continue // breaks and dead air don't end a run
		}
		if b.SeriesTitle == "" || b.Key == "" {
			flush() // a movie ends a run and is never one
			continue
		}
		if len(run) > 0 && run[0].Key != b.Key {
			flush()
		}
		run = append(run, b)
	}
	flush()
	return out
}

func runHighlight(ch channelAirings, run []playout.Broadcast) (highlightPick, bool) {
	channelID := ch.channelID
	for i, b := range run {
		if b.Episode != 1 || b.Season < 1 || (ch.shuffledAt != nil && ch.shuffledAt(b.Start)) {
			continue
		}
		pick := highlightPick{channelID: channelID, airing: b, reason: highlightSeasonPremiere}
		if b.Season == 1 {
			pick.reason = highlightSeriesPremiere
		}
		if rest := run[i:]; len(rest) >= marathonMinEpisodes {
			pick.episodes, pick.until = len(rest), rest[len(rest)-1].Stop
		}
		return pick, true
	}
	if len(run) >= marathonMinEpisodes {
		return highlightPick{channelID: channelID, airing: run[0], reason: highlightMarathon,
			episodes: len(run), until: run[len(run)-1].Stop}, true
	}
	return highlightPick{}, false
}

func (s *Server) registerGuideHighlights(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "guide-highlights", Method: http.MethodGet, Path: "/v1/guide/highlights",
		Summary: "Tonight's highlights across every channel",
		Description: "Any signed-in person. Up to `limit` airings in [from, to) worth calling out, each with a typed " +
			"`reason`: a series premiere, a season premiere, or a marathon (three or more episodes of one show back " +
			"to back). A premiere that opens such a run carries it too (`episodes`, `untilMs`). Premieres outrank " +
			"marathons and every channel gets one before any gets a second; the result is in airtime order. " +
			"The client words the reason; never rank airings client-side.",
		Tags: []string{"channels"},
	}, RoleMember), s.guideHighlights)
}

type guideHighlightsInput struct {
	FromMs int64 `query:"from" doc:"Window start, epoch ms. Defaults to now."`
	ToMs   int64 `query:"to" doc:"Window end, epoch ms. Defaults to 6 hours after the start; at most 24 hours."`
	Limit  int   `query:"limit" minimum:"0" maximum:"20" doc:"How many highlights, at most. Defaults to 4."`
}

type guideHighlightDTO struct {
	Reason        highlightReason `json:"reason" enum:"series_premiere,season_premiere,marathon" doc:"Why this airing is a highlight; the client words it"`
	ChannelID     string          `json:"channelId"`
	ChannelName   string          `json:"channelName"`
	ChannelNumber int             `json:"channelNumber"`
	// Episodes and UntilMs describe the back-to-back run from this airing: always for a
	// marathon, and for a premiere that opens one.
	Episodes int         `json:"episodes,omitempty" doc:"Episodes of this show back to back from this airing (3 or more), when it opens a run"`
	UntilMs  int64       `json:"untilMs,omitempty" doc:"When that run ends, epoch ms"`
	Airing   GuideAiring `json:"airing" doc:"The highlighted airing; season and episode carry a premiere's numbers"`
}

type guideHighlightsOutput struct {
	Body struct {
		FromMs     int64               `json:"fromMs" doc:"The window actually searched, after clamping"`
		ToMs       int64               `json:"toMs"`
		Highlights []guideHighlightDTO `json:"highlights"`
	}
}

func (s *Server) guideHighlights(ctx context.Context, in *guideHighlightsInput) (*guideHighlightsOutput, error) {
	out := &guideHighlightsOutput{}
	out.Body.Highlights = []guideHighlightDTO{}
	now := time.Now()
	if s.now != nil {
		now = s.now()
	}
	from, to := time.UnixMilli(in.FromMs), time.UnixMilli(in.ToMs)
	if in.FromMs == 0 {
		from = now
	}
	if earliest := now.Add(-s.guideLookback()); from.Before(earliest) {
		from = earliest
	}
	if in.ToMs == 0 || !to.After(from) {
		to = from.Add(highlightsDefaultWindow)
	}
	if to.Sub(from) > highlightsMaxWindow {
		to = from.Add(highlightsMaxWindow)
	}
	limit := in.Limit
	if limit <= 0 {
		limit = highlightsDefaultLimit
	}
	limit = min(limit, highlightsMaxLimit)
	out.Body.FromMs, out.Body.ToMs = from.UnixMilli(), to.UnixMilli()
	if s.playoutGuide == nil || s.store == nil {
		return out, nil
	}

	channels, err := s.store.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	// Nothing airs on a paused or removed channel.
	channels = slices.DeleteFunc(channels, func(ch store.Channel) bool {
		return ch.Status == schedule.StatusPaused || ch.Status == schedule.StatusDetached
	})
	// The same bounded fan-out as the guide: each timeline is CPU-bound schedule arithmetic.
	airings := make([]channelAirings, len(channels))
	sem := make(chan struct{}, guideConcurrency)
	var wg sync.WaitGroup
	for i, ch := range channels {
		airings[i].channelID = ch.ID
		airings[i].shuffledAt = func(at time.Time) bool {
			return !schedule.OrderingAt(ch.Policy, ch.Strategy, at).AirsInOrder()
		}
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			bs, err := s.playoutGuide.BroadcastsBetween(ctx, id, from, to)
			if err != nil {
				// One channel failing leaves it out rather than failing Home.
				s.log.Warn("guide highlights: timeline failed for one channel", "channel", id, "err", err)
				return
			}
			airings[i].broadcasts = bs
		}(i, ch.ID)
	}
	wg.Wait()

	picks := pickHighlights(airings, limit)
	byID := make(map[string]store.Channel, len(channels))
	for _, ch := range channels {
		byID[ch.ID] = ch
	}
	// Artwork for the chosen few only, in one batched lookup.
	keys := make([]timelineThumbKey, 0, len(picks))
	for _, p := range picks {
		keys = append(keys, timelineThumbKey{key: string(p.airing.Key), season: p.airing.Season, episode: p.airing.Episode})
	}
	thumbs := s.resolveTimelineThumbs(ctx, keys)
	hashes := make([]string, 0, len(picks))
	for _, k := range keys {
		hashes = append(hashes, thumbs[k].hash)
	}
	images := s.imageDTOsByHash(ctx, hashes)
	for i, p := range picks {
		a := guideAiringOf(p.channelID, p.airing)
		a.ThumbURL = thumbs[keys[i]].url
		if image := images[hashes[i]]; image != nil {
			a.ThumbImage = image
			if a.ThumbURL == "" {
				a.ThumbURL = image.Src
			}
		}
		dto := guideHighlightDTO{Reason: p.reason, ChannelID: p.channelID, ChannelName: byID[p.channelID].Name,
			ChannelNumber: byID[p.channelID].Number, Episodes: p.episodes, Airing: a}
		if !p.until.IsZero() {
			dto.UntilMs = p.until.UnixMilli()
		}
		out.Body.Highlights = append(out.Body.Highlights, dto)
	}
	return out, nil
}
