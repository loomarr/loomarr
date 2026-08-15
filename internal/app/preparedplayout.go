package app

import (
	"context"
	"errors"
	"math"
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
	AudioTrackFor(context.Context, string, string) int
}

type preparedInputResolver interface {
	ResolveInput(context.Context, string, library.PathMap, func(string) bool) library.InputSource
}

type preparedLookup interface {
	Lookup(prepared.Request) (prepared.Specification, bool, error)
}

type preparedSource struct {
	policy        string
	channelPolicy string
	request       prepared.Request
}

type preparedSourceKey struct {
	channelID     string
	libraryItemID string
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
	sources map[preparedSourceKey]preparedSource // channel + library item → control-plane-resolved source
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
		sources: make(map[preparedSourceKey]preparedSource),
	}
}

// Candidates enumerates accepted internal-playout schedules, earliest need first. Source resolution
// is channel-aware because two channels may select different audio tracks for the same item; equal
// requests are still collapsed to one reusable publication. HTTP-only media remains a live fallback
// because a finite reusable publication cannot depend on a remote stream staying put.
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
	needed := make(map[preparedSourceKey]time.Time)
	channelPolicies := make(map[string]string)
	var errs []error
	for _, channel := range channels {
		if !schedule.PlaysInternally(channel.Policy, globalBackend) {
			continue
		}
		channelPolicies[channel.ID] = schedule.ResolveAudioLanguage(channel.Policy, "")
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
			key := preparedSourceKey{channelID: channel.ID, libraryItemID: broadcast.LibraryItemID}
			if prior, ok := needed[key]; !ok || at.Before(prior) {
				needed[key] = at
			}
		}
	}

	keys := make([]preparedSourceKey, 0, len(needed))
	for key := range needed {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if needed[keys[i]].Equal(needed[keys[j]]) {
			if keys[i].libraryItemID == keys[j].libraryItemID {
				return keys[i].channelID < keys[j].channelID
			}
			return keys[i].libraryItemID < keys[j].libraryItemID
		}
		return needed[keys[i]].Before(needed[keys[j]])
	})
	candidates := make([]prepared.Candidate, 0, len(keys))
	queued := make(map[prepared.Request]struct{})
	for _, key := range keys {
		if len(candidates) >= preparedCandidateBatch {
			break
		}
		request, ok := r.resolveSource(ctx, key, channelPolicies[key.channelID])
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
		if _, exists := queued[request]; exists {
			continue
		}
		queued[request] = struct{}{}
		candidates = append(candidates, prepared.Candidate{NeededAt: needed[key], Request: request})
	}
	return candidates, errors.Join(errs...)
}

func (r *preparedRuntimeResolver) resolveSource(
	ctx context.Context, key preparedSourceKey, channelPolicy string,
) (prepared.Request, bool) {
	policy := ""
	if r.policy != nil {
		policy = r.policy()
	}
	r.mu.RLock()
	warmed, ok := r.sources[key]
	r.mu.RUnlock()
	if ok && warmed.policy == policy && warmed.channelPolicy == channelPolicy {
		return warmed.request, true
	}
	var pathMap library.PathMap
	if r.pathMap != nil {
		pathMap = r.pathMap()
	}
	input := r.inputs.ResolveInput(ctx, key.libraryItemID, pathMap, library.StatReadableFile)
	if input.URL == "" || input.Kind != library.InputFile {
		return prepared.Request{}, false
	}
	audioTrack := r.timeline.AudioTrackFor(ctx, key.channelID, input.URL)
	if r.rendition == nil {
		return prepared.Request{}, false
	}
	request := prepared.Request{
		Source:    prepared.Source{Path: input.URL, AudioTrack: audioTrack},
		Rendition: r.rendition(),
	}
	r.mu.Lock()
	r.sources[key] = preparedSource{policy: policy, channelPolicy: channelPolicy, request: request}
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
	currentAiring, ok, err := r.preparedAiring(request.ChannelID, broadcasts[current], now.Sub(broadcasts[current].Start))
	if err != nil || !ok {
		return playout.PreparedWindow{}, false, err
	}
	window := playout.PreparedWindow{Current: currentAiring}
	for _, broadcast := range broadcasts[:current] {
		airing, hit, lookupErr := r.preparedAiring(request.ChannelID, broadcast, 0)
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
	channelID string, broadcast playout.Broadcast, offset time.Duration,
) (playout.PreparedAiring, bool, error) {
	if broadcast.LibraryItemID == "" || broadcast.Kind != schedule.SlotProgram {
		return playout.PreparedAiring{}, false, nil
	}
	policy := ""
	if r.policy != nil {
		policy = r.policy()
	}
	r.mu.RLock()
	source, warmed := r.sources[preparedSourceKey{channelID: channelID, libraryItemID: broadcast.LibraryItemID}]
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

func preparedBudgetBytes(gib int) int64 {
	if gib <= 0 {
		return 0
	}
	if int64(gib) > math.MaxInt64>>30 {
		return math.MaxInt64
	}
	return int64(gib) << 30
}

// ScheduledBroadcasts is the metadata-free authoritative timeline used by preparation. Guide
// callers use BroadcastsBetween, which enriches these rows for display.
func (r *playoutResolver) ScheduledBroadcasts(
	ctx context.Context, channelID string, from, to time.Time,
) ([]playout.Broadcast, error) {
	return r.segmentedBroadcasts(ctx, channelID, from, to, playout.BroadcastsBetween)
}
