package app

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/prepared"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

const preparedCandidateBatch = 16

type preparedChannelReader interface {
	ListChannels(context.Context) ([]store.Channel, error)
	GetChannel(context.Context, string) (store.Channel, error)
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

// preparedRuntimeResolver is the composition adapter shared by the readiness planner and prepared
// origin. The planner side may resolve media-server paths and probe audio. The tune side reads only
// this warmed index plus Preparer.Lookup, so it cannot turn a viewer request into network or ffmpeg
// work.
type preparedRuntimeResolver struct {
	channels                preparedChannelReader
	timeline                preparedTimeline
	inputs                  preparedInputResolver
	lookup                  preparedLookup
	now                     func() time.Time
	pathMap                 func() library.PathMap
	policy                  func() string
	globalBackend           func() string
	transportBackend        func() string
	globalBackendContext    func(context.Context) (string, error)
	transportBackendContext func(context.Context) (string, error)
	rendition               func() prepared.RenditionContract
	readiness               *prepared.Readiness
}

type preparedRuntimeDependencies struct {
	Channels             preparedChannelReader
	Timeline             preparedTimeline
	Inputs               preparedInputResolver
	Lookup               preparedLookup
	Now                  func() time.Time
	PathMap              func() library.PathMap
	Policy               func() string
	GlobalBackend        func() string
	GlobalBackendContext func(context.Context) (string, error)
	// TransportBackend may temporarily differ from GlobalBackend while internal
	// transport is published for a prepared cutover. It is used only at tune time;
	// the background planner continues to follow the ordinarily applied backend.
	TransportBackend        func() string
	TransportBackendContext func(context.Context) (string, error)
	Rendition               func() prepared.RenditionContract
	Readiness               *prepared.Readiness
}

func newPreparedRuntimeResolver(deps preparedRuntimeDependencies) *preparedRuntimeResolver {
	return &preparedRuntimeResolver{
		channels: deps.Channels, timeline: deps.Timeline, inputs: deps.Inputs, lookup: deps.Lookup,
		now: deps.Now, pathMap: deps.PathMap, policy: deps.Policy,
		globalBackend: deps.GlobalBackend, transportBackend: deps.TransportBackend,
		globalBackendContext: deps.GlobalBackendContext, transportBackendContext: deps.TransportBackendContext,
		rendition: deps.Rendition, readiness: deps.Readiness,
	}
}

// Plan enumerates accepted internal-playout schedules, earliest need first. Existing durable
// bindings make the full schedule cheap to inspect and protect; at most one batch of absent
// bindings may resolve media-server paths or probe audio per pass.
func (r *preparedRuntimeResolver) Plan(
	ctx context.Context, from, to time.Time,
) (prepared.ReadinessPlan, error) {
	var plan prepared.ReadinessPlan
	if r == nil || r.channels == nil || r.timeline == nil || r.inputs == nil || r.lookup == nil ||
		r.readiness == nil || !to.After(from) {
		return plan, nil
	}
	channels, err := r.channels.ListChannels(ctx)
	if err != nil {
		return plan, err
	}
	globalBackend, err := r.resolveGlobalBackend(ctx, false)
	if err != nil {
		return plan, err
	}
	needed := make(map[prepared.BindingKey]time.Time)
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
			key := prepared.BindingKey{ChannelID: channel.ID, LibraryItemID: broadcast.LibraryItemID}
			if prior, ok := needed[key]; !ok || at.Before(prior) {
				needed[key] = at
			}
		}
	}

	keys := make([]prepared.BindingKey, 0, len(needed))
	channelReady := make(map[string]bool)
	for key := range needed {
		keys = append(keys, key)
		channelReady[key.ChannelID] = true
	}
	plan.Summary.Channels = len(channelReady)
	plan.Summary.ScheduledBindings = len(keys)
	sort.Slice(keys, func(i, j int) bool {
		if needed[keys[i]].Equal(needed[keys[j]]) {
			if keys[i].LibraryItemID == keys[j].LibraryItemID {
				return keys[i].ChannelID < keys[j].ChannelID
			}
			return keys[i].LibraryItemID < keys[j].LibraryItemID
		}
		return needed[keys[i]].Before(needed[keys[j]])
	})
	queued := make(map[prepared.Request]struct{})
	protected := make(map[prepared.Specification]struct{})
	resolvedBindings := make(map[prepared.BindingKey]prepared.Binding)
	policy := r.sourcePolicy()
	resolutionAttempts := 0
	for _, key := range keys {
		channelPolicy := channelPolicies[key.ChannelID]
		request, bound := r.readiness.Binding(key, policy, channelPolicy)
		if !bound {
			if resolutionAttempts >= preparedCandidateBatch {
				channelReady[key.ChannelID] = false
				continue
			}
			resolutionAttempts++
			var resolved bool
			request, resolved = r.resolveSource(ctx, key)
			if !resolved {
				channelReady[key.ChannelID] = false
				continue
			}
			resolvedBindings[key] = prepared.Binding{
				Policy: policy, ChannelPolicy: channelPolicy, Request: request,
			}
		}
		specification, ready, lookupErr := r.lookup.Lookup(request)
		if lookupErr != nil {
			errs = append(errs, lookupErr)
			channelReady[key.ChannelID] = false
			continue
		}
		if ready {
			plan.Summary.ReadyBindings++
			if _, exists := protected[specification]; !exists {
				protected[specification] = struct{}{}
				plan.Protected = append(plan.Protected, specification)
			}
			continue
		}
		channelReady[key.ChannelID] = false
		if len(plan.Candidates) >= preparedCandidateBatch {
			continue
		}
		if _, exists := queued[request]; exists {
			continue
		}
		queued[request] = struct{}{}
		plan.Candidates = append(plan.Candidates, prepared.Candidate{NeededAt: needed[key], Request: request})
	}
	if rememberErr := r.readiness.RememberBindings(resolvedBindings); rememberErr != nil {
		// Preparation must never depend on source selection that did not survive the restart boundary.
		plan.Candidates = nil
		errs = append(errs, rememberErr)
	}
	for _, ready := range channelReady {
		if ready {
			plan.Summary.ReadyChannels++
		}
	}
	plan.Summary.MissingBindings = plan.Summary.ScheduledBindings - plan.Summary.ReadyBindings
	plan.Summary.QueuedPublications = len(plan.Candidates)
	return plan, errors.Join(errs...)
}

func (r *preparedRuntimeResolver) resolveSource(
	ctx context.Context, key prepared.BindingKey,
) (prepared.Request, bool) {
	if r.rendition == nil {
		return prepared.Request{}, false
	}
	var pathMap library.PathMap
	if r.pathMap != nil {
		pathMap = r.pathMap()
	}
	input := r.inputs.ResolveInput(ctx, key.LibraryItemID, pathMap, library.StatReadableFile)
	if input.URL == "" || input.Kind != library.InputFile {
		return prepared.Request{}, false
	}
	audioTrack := r.timeline.AudioTrackFor(ctx, key.ChannelID, input.URL)
	request := prepared.Request{
		Source:    prepared.Source{Path: input.URL, AudioTrack: audioTrack},
		Rendition: r.rendition(),
	}
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
	if r == nil || r.channels == nil || r.timeline == nil || r.lookup == nil || r.readiness == nil ||
		r.now == nil || request.ChannelID == "" {
		return playout.PreparedWindow{}, false, nil
	}
	channel, err := r.channels.GetChannel(ctx, request.ChannelID)
	if err != nil {
		return playout.PreparedWindow{}, false, err
	}
	globalBackend, err := r.resolveGlobalBackend(ctx, true)
	if err != nil {
		return playout.PreparedWindow{}, false, err
	}
	if !schedule.PlaysInternally(channel.Policy, globalBackend) {
		return playout.PreparedWindow{}, false, nil
	}
	channelPolicy := schedule.ResolveAudioLanguage(channel.Policy, "")
	now := r.now()
	broadcasts, err := r.timeline.ScheduledBroadcasts(
		ctx, request.ChannelID, now.Add(-playout.DVRHorizon), now.Add(time.Nanosecond),
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
	currentAiring, ok, err := r.preparedAiring(
		request.ChannelID, channelPolicy, broadcasts[current], now.Sub(broadcasts[current].Start),
	)
	if err != nil || !ok {
		return playout.PreparedWindow{}, false, err
	}
	window := playout.PreparedWindow{Current: currentAiring}
	for _, broadcast := range broadcasts[:current] {
		airing, hit, lookupErr := r.preparedAiring(request.ChannelID, channelPolicy, broadcast, 0)
		if lookupErr != nil {
			return playout.PreparedWindow{}, false, lookupErr
		}
		if hit {
			window.Previous = append(window.Previous, airing)
		}
	}
	return window, true, nil
}

func (r *preparedRuntimeResolver) resolveGlobalBackend(ctx context.Context, transport bool) (string, error) {
	if transport && r.transportBackendContext != nil {
		return r.transportBackendContext(ctx)
	}
	if !transport && r.globalBackendContext != nil {
		return r.globalBackendContext(ctx)
	}
	if transport && r.transportBackend != nil {
		return r.transportBackend(), nil
	}
	if r.globalBackend != nil {
		return r.globalBackend(), nil
	}
	return "", nil
}

func (r *preparedRuntimeResolver) preparedAiring(
	channelID, channelPolicy string, broadcast playout.Broadcast, offset time.Duration,
) (playout.PreparedAiring, bool, error) {
	if broadcast.LibraryItemID == "" || broadcast.Kind != schedule.SlotProgram {
		return playout.PreparedAiring{}, false, nil
	}
	request, warmed := r.readiness.Binding(prepared.BindingKey{
		ChannelID: channelID, LibraryItemID: broadcast.LibraryItemID,
	}, r.sourcePolicy(), channelPolicy)
	if !warmed {
		return playout.PreparedAiring{}, false, nil
	}
	specification, hit, err := r.lookup.Lookup(request)
	if err != nil || !hit {
		return playout.PreparedAiring{}, false, err
	}
	return playout.PreparedAiring{
		Specification: specification, StartedAt: broadcast.Start, Offset: offset,
	}, true, nil
}

func (r *preparedRuntimeResolver) sourcePolicy() string {
	if r.policy == nil {
		return ""
	}
	return r.policy()
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
