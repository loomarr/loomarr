package api

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mantonx/loomarr/internal/playout"
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
// GuideAiring projection and preview resolver as the main Guide; this narrower endpoint simply
// returns the current programme plus the next few rather than the cross-channel grid window.

// timelineWindow is how far ahead the strip looks — "now + the next few". V60 also asks for the
// shared DVR horizon behind now so programme context follows a delayed viewer. The future bound
// remains a handful of blocks rather than the whole day.
const timelineWindow = 3 * time.Hour

// TimelineThumbResolver resolves a same-origin preview image path for one programme block (§9.1 V47). An
// interface so the api package need not import tmdb — the same abstraction IconService uses. nil ⇒
// the timeline still works, just with no images (the strip renders a fallback), because TMDB is
// optional and a missing image must never fail the strip.
type TimelineThumbResolver interface {
	// ThumbFor returns a browser-relative preview image path and the content hash behind it, best-effort: two empty
	// strings (never an error to the caller) when there is no key, TMDB is unconfigured, TMDB has
	// no image, or the bounded interactive image fetch fails. `season`/`episode`
	// are 0 for a movie; a series uses them to fetch the episode's own still. Movie previews use
	// their landscape backdrop so both programme shapes share the same frame.
	//
	// ⚠ The hash is returned SEPARATELY rather than parsed back out of the URL, so the handler can
	// resolve every block's record in one deduplicated pass instead of one lookup per block.
	ThumbFor(ctx context.Context, key string, season, episode int) (url, hash string)
}

// timelineThumbKey is the programme identity relevant to artwork. The same movie or episode may
// occur more than once in a Guide window; resolving it once is both faster and kinder to TMDB.
type timelineThumbKey struct {
	key             string
	season, episode int
}

type timelineThumbResult struct {
	url, hash string
}

// TMDB has no multi-title artwork endpoint, so "batch" here means one deduplicated work set with
// bounded parallel fan-out. Eight overlaps normal network latency without turning a large Guide
// window into an unbounded burst against the provider.
const timelineThumbConcurrency = 8

// resolveTimelineThumbs is the shared Guide/Watch artwork batch. Results are keyed by programme
// identity so callers can project them back onto repeated airings without another provider call.
func (s *Server) resolveTimelineThumbs(
	ctx context.Context, requested []timelineThumbKey,
) map[timelineThumbKey]timelineThumbResult {
	resolved := make(map[timelineThumbKey]timelineThumbResult)
	if s.timelineThumbs == nil {
		return resolved
	}

	unique := make([]timelineThumbKey, 0, len(requested))
	seen := make(map[timelineThumbKey]struct{}, len(requested))
	for _, key := range requested {
		if key.key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, key)
	}
	if len(unique) == 0 {
		return resolved
	}

	results := make([]timelineThumbResult, len(unique))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range min(timelineThumbConcurrency, len(unique)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				key := unique[i]
				results[i].url, results[i].hash = s.timelineThumbs.ThumbFor(
					ctx, key.key, key.season, key.episode,
				)
			}
		}()
	}
	for i := range unique {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	for i, key := range unique {
		resolved[key] = results[i]
	}
	return resolved
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
	bs, err := s.playoutGuide.BroadcastsBetween(
		ctx, ch.ID, now.Add(-playout.DVRHorizon), now.Add(timelineWindow),
	)
	if err != nil {
		return out, nil // a guide hiccup must not blank the player
	}

	airings := make([]GuideAiring, 0, len(bs))
	// ⚠ A PARALLEL slice rather than a field on GuideAiring. The hash is plumbing between the two
	// loops below, not part of the resource — and GuideAiring is the shared projection the main
	// guide grid also serves, so a field here would appear in a payload that has no use for it.
	thumbHashes := make([]string, 0, len(bs))
	thumbKeys := make([]timelineThumbKey, 0, len(bs))
	for _, b := range bs {
		a := guideAiringOf(b)
		key := timelineThumbKey{}
		// A preview image for PROGRAMME blocks only (a break/flex/pending block has no title image
		// to show). Best-effort and gated on a resolver being wired — an install without TMDB shows
		// the strip with fallbacks rather than failing.
		if b.Kind == schedule.SlotProgram && s.timelineThumbs != nil {
			key = timelineThumbKey{key: string(b.Key), season: b.Season, episode: b.Episode}
		}
		airings = append(airings, a)
		thumbKeys = append(thumbKeys, key)
	}
	thumbs := s.resolveTimelineThumbs(ctx, thumbKeys)
	for i, key := range thumbKeys {
		airings[i].ThumbURL = thumbs[key].url
		thumbHashes = append(thumbHashes, thumbs[key].hash)
	}

	// One deduplicated pass for every block's image record (§22) — the shape imageDTOsByHash
	// exists to enforce, rather than a lookup inside the loop above.
	byHash := s.imageDTOsByHash(ctx, thumbHashes)
	for i := range airings {
		if thumbHashes[i] != "" {
			airings[i].ThumbImage = byHash[thumbHashes[i]]
		}
	}

	out.Body.Airings = airings
	return out, nil
}
