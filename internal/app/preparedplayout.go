package app

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/prepared"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

const preparedBoundaryLookbehind = 10 * time.Second
const preparedCandidateBatch = 16

type preparedChannelReader interface {
	ListChannels(context.Context) ([]store.Channel, error)
}

type preparedTimeline interface {
	ScheduledBroadcasts(context.Context, string, time.Time, time.Time) ([]playout.Broadcast, error)
	AudioTrackFor(context.Context, string) int
}

type preparedInputResolver interface {
	ResolveInput(context.Context, string, library.PathMap, func(string) bool) library.InputSource
}

type preparedLookup interface {
	Lookup(prepared.Request) (prepared.Specification, bool, error)
}

type preparedSource struct {
	policy  string
	request prepared.Request
}

// preparedRuntimeResolver is the composition adapter shared by the readiness planner and prepared
// origin. The planner side may resolve media-server paths and probe audio. The tune side reads only
// this warmed index plus Preparer.Lookup, so it cannot turn a viewer request into network or ffmpeg
// work.
type preparedRuntimeResolver struct {
	channels      preparedChannelReader
	timeline      preparedTimeline
	inputs        preparedInputResolver
	lookup        preparedLookup
	now           func() time.Time
	pathMap       func() library.PathMap
	policy        func() string
	globalBackend func() string
	rendition     func() prepared.RenditionContract

	mu      sync.RWMutex
	sources map[string]preparedSource // library item id → control-plane-resolved source
}

func newPreparedRuntimeResolver(
	channels preparedChannelReader,
	timeline preparedTimeline,
	inputs preparedInputResolver,
	lookup preparedLookup,
	now func() time.Time,
	pathMap func() library.PathMap,
	policy func() string,
	globalBackend func() string,
	rendition func() prepared.RenditionContract,
) *preparedRuntimeResolver {
	return &preparedRuntimeResolver{
		channels: channels, timeline: timeline, inputs: inputs, lookup: lookup, now: now,
		pathMap: pathMap, policy: policy, globalBackend: globalBackend, rendition: rendition,
		sources: make(map[string]preparedSource),
	}
}

// Candidates enumerates accepted internal-playout schedules, earliest need first. It resolves each
// distinct library item to a local file once per active source policy; HTTP-only media remains a
// live fallback because a finite reusable publication cannot depend on a remote stream staying put.
func (r *preparedRuntimeResolver) Candidates(
	ctx context.Context, from, to time.Time,
) ([]prepared.Candidate, error) {
	if r == nil || r.channels == nil || r.timeline == nil || r.inputs == nil || r.lookup == nil || !to.After(from) {
		return nil, nil
	}
	channels, err := r.channels.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	globalBackend := ""
	if r.globalBackend != nil {
		globalBackend = r.globalBackend()
	}
	needed := make(map[string]time.Time)
	var errs []error
	for _, channel := range channels {
		if !schedule.PlaysInternally(channel.Policy, globalBackend) {
			continue
		}
		broadcasts, timelineErr := r.timeline.ScheduledBroadcasts(ctx, channel.ID, from, to)
		if timelineErr != nil {
			errs = append(errs, timelineErr)
			continue
		}
		for _, broadcast := range broadcasts {
			if broadcast.Kind != schedule.SlotProgram || broadcast.LibraryItemID == "" {
				continue
			}
			at := broadcast.Start
			if at.Before(from) {
				at = from // every currently-airing item has equal first priority.
			}
			if prior, ok := needed[broadcast.LibraryItemID]; !ok || at.Before(prior) {
				needed[broadcast.LibraryItemID] = at
			}
		}
	}

	ids := make([]string, 0, len(needed))
	for id := range needed {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if needed[ids[i]].Equal(needed[ids[j]]) {
			return ids[i] < ids[j]
		}
		return needed[ids[i]].Before(needed[ids[j]])
	})
	candidates := make([]prepared.Candidate, 0, len(ids))
	for _, id := range ids {
		if len(candidates) >= preparedCandidateBatch {
			break
		}
		request, ok := r.resolveSource(ctx, id)
		if !ok {
			continue
		}
		_, ready, lookupErr := r.lookup.Lookup(request)
		if lookupErr != nil {
			errs = append(errs, lookupErr)
			continue
		}
		if ready {
			continue
		}
		candidates = append(candidates, prepared.Candidate{NeededAt: needed[id], Request: request})
	}
	return candidates, errors.Join(errs...)
}

func (r *preparedRuntimeResolver) resolveSource(
	ctx context.Context, libraryItemID string,
) (prepared.Request, bool) {
	policy := ""
	if r.policy != nil {
		policy = r.policy()
	}
	r.mu.RLock()
	warmed, ok := r.sources[libraryItemID]
	r.mu.RUnlock()
	if ok && warmed.policy == policy {
		return warmed.request, true
	}
	var pathMap library.PathMap
	if r.pathMap != nil {
		pathMap = r.pathMap()
	}
	input := r.inputs.ResolveInput(ctx, libraryItemID, pathMap, library.StatReadableFile)
	if input.URL == "" || input.Kind != library.InputFile {
		return prepared.Request{}, false
	}
	audioTrack := r.timeline.AudioTrackFor(ctx, input.URL)
	if r.rendition == nil {
		return prepared.Request{}, false
	}
	request := prepared.Request{
		Source:    prepared.Source{Path: input.URL, AudioTrack: audioTrack},
		Rendition: r.rendition(),
	}
	r.mu.Lock()
	r.sources[libraryItemID] = preparedSource{policy: policy, request: request}
	r.mu.Unlock()
	return request, true
}

// ResolvePrepared is the tune-time half. It intentionally cannot call resolveSource: an index miss
// is a prepared miss and Origin immediately uses live playout.
func (r *preparedRuntimeResolver) ResolvePrepared(
	ctx context.Context, request playout.TuneRequest,
) (playout.PreparedWindow, bool, error) {
	return r.resolvePrepared(ctx, request)
}

func (r *preparedRuntimeResolver) resolvePrepared(
	ctx context.Context, request playout.TuneRequest,
) (playout.PreparedWindow, bool, error) {
	if r == nil || r.timeline == nil || r.lookup == nil || r.now == nil || request.ChannelID == "" {
		return playout.PreparedWindow{}, false, nil
	}
	now := r.now()
	broadcasts, err := r.timeline.ScheduledBroadcasts(
		ctx, request.ChannelID, now.Add(-preparedBoundaryLookbehind), now.Add(time.Nanosecond),
	)
	if err != nil {
		return playout.PreparedWindow{}, false, err
	}
	current := -1
	for i := range broadcasts {
		if !broadcasts[i].Start.After(now) && broadcasts[i].Stop.After(now) {
			current = i
			break
		}
	}
	if current < 0 || broadcasts[current].Kind != schedule.SlotProgram {
		return playout.PreparedWindow{}, false, nil
	}
	currentAiring, ok, err := r.preparedAiring(broadcasts[current], now.Sub(broadcasts[current].Start))
	if err != nil || !ok {
		return playout.PreparedWindow{}, false, err
	}
	window := playout.PreparedWindow{Current: currentAiring}
	for _, broadcast := range broadcasts[:current] {
		airing, hit, lookupErr := r.preparedAiring(broadcast, 0)
		if lookupErr != nil {
			return playout.PreparedWindow{}, false, lookupErr
		}
		if hit {
			window.Previous = append(window.Previous, airing)
		}
	}
	return window, true, nil
}

func (r *preparedRuntimeResolver) preparedAiring(
	broadcast playout.Broadcast, offset time.Duration,
) (playout.PreparedAiring, bool, error) {
	if broadcast.LibraryItemID == "" || broadcast.Kind != schedule.SlotProgram {
		return playout.PreparedAiring{}, false, nil
	}
	policy := ""
	if r.policy != nil {
		policy = r.policy()
	}
	r.mu.RLock()
	source, warmed := r.sources[broadcast.LibraryItemID]
	r.mu.RUnlock()
	if !warmed || source.policy != policy {
		return playout.PreparedAiring{}, false, nil
	}
	specification, hit, err := r.lookup.Lookup(source.request)
	if err != nil || !hit {
		return playout.PreparedAiring{}, false, err
	}
	return playout.PreparedAiring{
		Specification: specification, StartedAt: broadcast.Start, Offset: offset,
	}, true, nil
}

func preparedSourcePolicy(tier, audioLanguage, pathMap string) string {
	return strings.Join([]string{tier, audioLanguage, pathMap}, "\x00")
}

// ScheduledBroadcasts is the metadata-free authoritative timeline used by preparation. Guide
// callers use BroadcastsBetween, which enriches these rows for display.
func (r *playoutResolver) ScheduledBroadcasts(
	ctx context.Context, channelID string, from, to time.Time,
) ([]playout.Broadcast, error) {
	return r.segmentedBroadcasts(ctx, channelID, from, to, playout.BroadcastsBetween)
}
