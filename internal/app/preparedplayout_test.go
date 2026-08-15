package app

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/prepared"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

type preparedChannels struct{ channels []store.Channel }

func (s preparedChannels) ListChannels(context.Context) ([]store.Channel, error) {
	return s.channels, nil
}

func (s preparedChannels) GetChannel(_ context.Context, id string) (store.Channel, error) {
	for _, channel := range s.channels {
		if channel.ID == id {
			return channel, nil
		}
	}
	return store.Channel{}, store.ErrNotFound
}

type preparedTimelineFake struct {
	broadcasts          map[string][]playout.Broadcast
	audioTrackByChannel map[string]int
	audioCalls          int
}

func (f *preparedTimelineFake) ScheduledBroadcasts(
	_ context.Context, channelID string, _, _ time.Time,
) ([]playout.Broadcast, error) {
	return f.broadcasts[channelID], nil
}

func (f *preparedTimelineFake) AudioTrackFor(_ context.Context, channelID, _ string) int {
	f.audioCalls++
	if track, ok := f.audioTrackByChannel[channelID]; ok {
		return track
	}
	return 2
}

type preparedInputsFake struct {
	sources map[string]library.InputSource
	calls   int
}

func (f *preparedInputsFake) ResolveInput(
	_ context.Context, itemID string, _ library.PathMap, _ func(string) bool,
) library.InputSource {
	f.calls++
	return f.sources[itemID]
}

type preparedLookupFake struct {
	hits map[prepared.Request]prepared.Specification
}

func (f preparedLookupFake) Lookup(request prepared.Request) (prepared.Specification, bool, error) {
	spec, ok := f.hits[request]
	return spec, ok, nil
}

func newPreparedRuntimeForTest(
	t *testing.T,
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
	t.Helper()
	preparedLibrary, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	readiness, err := prepared.OpenReadiness(preparedLibrary)
	if err != nil {
		t.Fatal(err)
	}
	return newPreparedRuntimeResolver(preparedRuntimeDependencies{
		Channels: channels, Timeline: timeline, Inputs: inputs, Lookup: lookup, Now: now,
		PathMap: pathMap, Policy: policy, GlobalBackend: globalBackend,
		Rendition: rendition, Readiness: readiness,
	})
}

func TestPreparedRuntimeCandidatesUseOnlyInternalLocalSchedule(t *testing.T) {
	now := time.Unix(10_000, 0)
	timeline := &preparedTimelineFake{broadcasts: map[string][]playout.Broadcast{
		"internal": {{Kind: schedule.SlotProgram, LibraryItemID: "shared", Start: now.Add(-time.Minute), Stop: now.Add(time.Hour)}},
		"tunarr":   {{Kind: schedule.SlotProgram, LibraryItemID: "remote", Start: now, Stop: now.Add(time.Hour)}},
	}}
	inputs := &preparedInputsFake{sources: map[string]library.InputSource{
		"shared": {URL: "/media/shared.mkv", Kind: library.InputFile},
		"remote": {URL: "http://media/remote", Kind: library.InputHTTP},
	}}
	r := newPreparedRuntimeForTest(t,
		preparedChannels{channels: []store.Channel{
			{Channel: schedule.Channel{ID: "internal"}},
			{Channel: schedule.Channel{ID: "tunarr"}, Policy: schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Playout: &schedule.PlayoutPolicy{Backend: "tunarr"}}}},
		}}, timeline, inputs, preparedLookupFake{}, func() time.Time { return now }, nil,
		func() string { return "policy" }, func() string { return "internal" },
		func() prepared.RenditionContract { return playout.CanonicalPreparedRendition(playout.TierBalanced) },
	)

	plan, err := r.Plan(t.Context(), now, now.Add(6*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].Request.Source.Path != "/media/shared.mkv" ||
		plan.Candidates[0].Request.Source.AudioTrack != 2 || !plan.Candidates[0].NeededAt.Equal(now) {
		t.Fatalf("candidates = %+v", plan.Candidates)
	}
	if inputs.calls != 1 || timeline.audioCalls != 1 {
		t.Fatalf("source resolution calls = input %d audio %d, want one each", inputs.calls, timeline.audioCalls)
	}

	// The warmed source policy makes the next planner pass a pure index read.
	if _, err := r.Plan(t.Context(), now, now.Add(6*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if inputs.calls != 1 || timeline.audioCalls != 1 {
		t.Fatal("unchanged source policy re-resolved or re-probed the library item")
	}
}

func TestPreparedRuntimeCandidatesKeepChannelAudioOverridesDistinct(t *testing.T) {
	now := time.Unix(15_000, 0)
	timeline := &preparedTimelineFake{
		broadcasts: map[string][]playout.Broadcast{
			"english":  {{Kind: schedule.SlotProgram, LibraryItemID: "shared", Start: now, Stop: now.Add(time.Hour)}},
			"japanese": {{Kind: schedule.SlotProgram, LibraryItemID: "shared", Start: now, Stop: now.Add(time.Hour)}},
		},
		audioTrackByChannel: map[string]int{"english": 1, "japanese": 2},
	}
	inputs := &preparedInputsFake{sources: map[string]library.InputSource{
		"shared": {URL: "/media/shared.mkv", Kind: library.InputFile},
	}}
	r := newPreparedRuntimeForTest(t,
		preparedChannels{channels: []store.Channel{
			{Channel: schedule.Channel{ID: "english"}, Policy: schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Playout: &schedule.PlayoutPolicy{AudioLanguage: "eng"}}}},
			{Channel: schedule.Channel{ID: "japanese"}, Policy: schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Playout: &schedule.PlayoutPolicy{AudioLanguage: "jpn"}}}},
		}}, timeline, inputs, preparedLookupFake{}, func() time.Time { return now }, nil,
		func() string { return "policy" }, func() string { return "internal" },
		func() prepared.RenditionContract { return playout.CanonicalPreparedRendition(playout.TierBalanced) },
	)

	plan, err := r.Plan(t.Context(), now, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Candidates) != 2 {
		t.Fatalf("candidate count = %d, want one publication per channel audio track", len(plan.Candidates))
	}
	tracks := map[int]bool{}
	for _, candidate := range plan.Candidates {
		tracks[candidate.Request.Source.AudioTrack] = true
	}
	if !tracks[1] || !tracks[2] {
		t.Fatalf("candidate audio tracks = %v, want tracks 1 and 2", tracks)
	}
}

func TestPreparedRuntimePlanBoundsColdResolutionButStillProtectsDurableSchedule(t *testing.T) {
	now := time.Unix(17_000, 0)
	channels := make([]store.Channel, 100)
	broadcasts := make(map[string][]playout.Broadcast, len(channels))
	sources := make(map[string]library.InputSource, len(channels))
	for i := range channels {
		channelID := fmt.Sprintf("ch-%03d", i)
		itemID := fmt.Sprintf("item-%03d", i)
		channels[i] = store.Channel{Channel: schedule.Channel{ID: channelID}}
		broadcasts[channelID] = []playout.Broadcast{{
			Kind: schedule.SlotProgram, LibraryItemID: itemID,
			Start: now.Add(time.Duration(i) * time.Second), Stop: now.Add(time.Hour),
		}}
		sources[itemID] = library.InputSource{URL: "/media/" + itemID + ".mkv", Kind: library.InputFile}
	}
	r := newPreparedRuntimeForTest(
		t, preparedChannels{channels: channels}, &preparedTimelineFake{broadcasts: broadcasts},
		&preparedInputsFake{sources: sources}, preparedLookupFake{}, func() time.Time { return now }, nil,
		func() string { return "policy" }, func() string { return "internal" },
		func() prepared.RenditionContract { return playout.CanonicalPreparedRendition(playout.TierBalanced) },
	)
	lastRequest := prepared.Request{
		Source:    prepared.Source{Path: "/media/item-099.mkv", AudioTrack: 2},
		Rendition: playout.CanonicalPreparedRendition(playout.TierBalanced),
	}
	if err := r.readiness.RememberBinding(
		prepared.BindingKey{ChannelID: "ch-099", LibraryItemID: "item-099"},
		prepared.Binding{Policy: "policy", Request: lastRequest},
	); err != nil {
		t.Fatal(err)
	}
	r.lookup = preparedLookupFake{hits: map[prepared.Request]prepared.Specification{
		lastRequest: {SourceFingerprint: "durable", Rendition: lastRequest.Rendition},
	}}

	plan, err := r.Plan(t.Context(), now, now.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	inputs := r.inputs.(*preparedInputsFake)
	if inputs.calls != preparedCandidateBatch || len(plan.Candidates) != preparedCandidateBatch {
		t.Fatalf("cold work = %d resolutions, %d candidates; want bounded %d", inputs.calls, len(plan.Candidates), preparedCandidateBatch)
	}
	if len(plan.Protected) != 1 || plan.Protected[0].SourceFingerprint != "durable" {
		t.Fatalf("protected = %+v, want durable publication beyond cold batch", plan.Protected)
	}
	wantSummary := prepared.ReadinessSummary{
		Channels: 100, ReadyChannels: 1,
		ScheduledBindings: 100, ReadyBindings: 1, MissingBindings: 99,
		QueuedPublications: preparedCandidateBatch,
	}
	if plan.Summary != wantSummary {
		t.Fatalf("summary = %+v, want %+v", plan.Summary, wantSummary)
	}
}

func TestPreparedRuntimeTuneIsLookupOnlyAndCarriesPreviousAiring(t *testing.T) {
	now := time.Unix(20_000, 0)
	previous := playout.Broadcast{Kind: schedule.SlotProgram, LibraryItemID: "previous", Start: now.Add(-time.Minute), Stop: now.Add(-time.Second)}
	current := playout.Broadcast{Kind: schedule.SlotProgram, LibraryItemID: "current", Start: now.Add(-time.Second), Stop: now.Add(time.Hour)}
	timeline := &preparedTimelineFake{broadcasts: map[string][]playout.Broadcast{"ch": {previous, current}}}
	inputs := &preparedInputsFake{sources: map[string]library.InputSource{
		"previous": {URL: "/media/previous.mkv", Kind: library.InputFile},
		"current":  {URL: "/media/current.mkv", Kind: library.InputFile},
	}}
	rendition := playout.CanonicalPreparedRendition(playout.TierBalanced)
	previousRequest := prepared.Request{Source: prepared.Source{Path: "/media/previous.mkv", AudioTrack: 2}, Rendition: rendition}
	currentRequest := prepared.Request{Source: prepared.Source{Path: "/media/current.mkv", AudioTrack: 2}, Rendition: rendition}
	lookup := preparedLookupFake{hits: map[prepared.Request]prepared.Specification{
		previousRequest: {SourceFingerprint: "previous", Rendition: rendition},
		currentRequest:  {SourceFingerprint: "current", Rendition: rendition},
	}}
	r := newPreparedRuntimeForTest(t,
		preparedChannels{channels: []store.Channel{{Channel: schedule.Channel{ID: "ch"}}}},
		timeline, inputs, lookup, func() time.Time { return now }, nil,
		func() string { return "policy" }, func() string { return "internal" },
		func() prepared.RenditionContract { return rendition },
	)

	// No control-plane pass yet: tune misses without touching the media server or audio prober.
	if _, ok, err := r.ResolvePrepared(t.Context(), playout.TuneRequest{ChannelID: "ch"}); err != nil || ok {
		t.Fatalf("cold ResolvePrepared = ok %v err %v, want clean miss", ok, err)
	}
	if inputs.calls != 0 || timeline.audioCalls != 0 {
		t.Fatal("cold tune performed control-plane source work")
	}
	if _, err := r.Plan(t.Context(), now.Add(-time.Minute), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	inputCalls, audioCalls := inputs.calls, timeline.audioCalls

	window, ok, err := r.ResolvePrepared(t.Context(), playout.TuneRequest{ChannelID: "ch"})
	if err != nil || !ok {
		t.Fatalf("warmed ResolvePrepared = ok %v err %v", ok, err)
	}
	if window.Current.Specification.SourceFingerprint != "current" || window.Current.Offset != time.Second ||
		len(window.Previous) != 1 || window.Previous[0].Specification.SourceFingerprint != "previous" {
		t.Fatalf("window = %+v", window)
	}
	if inputs.calls != inputCalls || timeline.audioCalls != audioCalls {
		t.Fatal("warmed tune re-resolved a source or ran an audio probe")
	}
}

func TestPreparedRuntimePolicyChangeMakesTuneMissUntilPlannerRewarms(t *testing.T) {
	now := time.Unix(30_000, 0)
	policy := "balanced"
	timeline := &preparedTimelineFake{broadcasts: map[string][]playout.Broadcast{"ch": {{
		Kind: schedule.SlotProgram, LibraryItemID: "item", Start: now.Add(-time.Minute), Stop: now.Add(time.Hour),
	}}}}
	inputs := &preparedInputsFake{sources: map[string]library.InputSource{
		"item": {URL: "/media/item.mkv", Kind: library.InputFile},
	}}
	rendition := playout.CanonicalPreparedRendition(playout.TierBalanced)
	request := prepared.Request{Source: prepared.Source{Path: "/media/item.mkv", AudioTrack: 2}, Rendition: rendition}
	r := newPreparedRuntimeForTest(t,
		preparedChannels{channels: []store.Channel{{Channel: schedule.Channel{ID: "ch"}}}}, timeline, inputs,
		preparedLookupFake{hits: map[prepared.Request]prepared.Specification{request: {SourceFingerprint: "hit", Rendition: rendition}}},
		func() time.Time { return now }, nil, func() string { return policy },
		func() string { return "internal" }, func() prepared.RenditionContract { return rendition },
	)
	if _, err := r.Plan(t.Context(), now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	policy = "quality"
	if _, ok, err := r.ResolvePrepared(t.Context(), playout.TuneRequest{ChannelID: "ch"}); err != nil || ok {
		t.Fatalf("ResolvePrepared after policy change = ok %v err %v, want lookup-only miss", ok, err)
	}
	if inputs.calls != 1 {
		t.Fatal("tune tried to rewarm the changed policy")
	}
}

func TestPreparedRuntimeTuneUsesDurableReadinessBeforeTheFirstPlannerPass(t *testing.T) {
	now := time.Unix(40_000, 0)
	preparedLibrary, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	readiness, err := prepared.OpenReadiness(preparedLibrary)
	if err != nil {
		t.Fatal(err)
	}
	rendition := playout.CanonicalPreparedRendition(playout.TierBalanced)
	request := prepared.Request{
		Source: prepared.Source{Path: "/media/item.mkv", AudioTrack: 2}, Rendition: rendition,
	}
	key := prepared.BindingKey{ChannelID: "ch", LibraryItemID: "item"}
	if err := readiness.RememberBinding(key, prepared.Binding{Policy: "policy", Request: request}); err != nil {
		t.Fatal(err)
	}
	reopened, err := prepared.OpenReadiness(preparedLibrary)
	if err != nil {
		t.Fatal(err)
	}
	timeline := &preparedTimelineFake{broadcasts: map[string][]playout.Broadcast{"ch": {{
		Kind: schedule.SlotProgram, LibraryItemID: "item", Start: now.Add(-time.Minute), Stop: now.Add(time.Hour),
	}}}}
	inputs := &preparedInputsFake{}
	r := newPreparedRuntimeResolver(preparedRuntimeDependencies{
		Channels: preparedChannels{channels: []store.Channel{{Channel: schedule.Channel{ID: "ch"}}}},
		Timeline: timeline, Inputs: inputs,
		Lookup: preparedLookupFake{hits: map[prepared.Request]prepared.Specification{
			request: {SourceFingerprint: "ready", Rendition: rendition},
		}},
		Now: func() time.Time { return now }, Policy: func() string { return "policy" },
		GlobalBackend: func() string { return "internal" }, Rendition: func() prepared.RenditionContract { return rendition },
		Readiness: reopened,
	})

	window, ok, err := r.ResolvePrepared(t.Context(), playout.TuneRequest{ChannelID: "ch"})
	if err != nil || !ok || window.Current.Specification.SourceFingerprint != "ready" {
		t.Fatalf("ResolvePrepared after restart = (%+v, %v, %v), want durable hit", window, ok, err)
	}
	if inputs.calls != 0 || timeline.audioCalls != 0 {
		t.Fatal("cold-start tune performed media-server or audio-probe work")
	}
}

func TestPreparedRuntimeChannelAudioPolicyChangeMakesTuneMissImmediately(t *testing.T) {
	now := time.Unix(50_000, 0)
	channels := preparedChannels{channels: []store.Channel{{
		Channel: schedule.Channel{ID: "ch"},
		Policy:  schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Playout: &schedule.PlayoutPolicy{AudioLanguage: "eng"}}},
	}}}
	timeline := &preparedTimelineFake{broadcasts: map[string][]playout.Broadcast{"ch": {{
		Kind: schedule.SlotProgram, LibraryItemID: "item", Start: now.Add(-time.Minute), Stop: now.Add(time.Hour),
	}}}}
	inputs := &preparedInputsFake{sources: map[string]library.InputSource{
		"item": {URL: "/media/item.mkv", Kind: library.InputFile},
	}}
	rendition := playout.CanonicalPreparedRendition(playout.TierBalanced)
	request := prepared.Request{Source: prepared.Source{Path: "/media/item.mkv", AudioTrack: 2}, Rendition: rendition}
	r := newPreparedRuntimeForTest(
		t, channels, timeline, inputs,
		preparedLookupFake{hits: map[prepared.Request]prepared.Specification{
			request: {SourceFingerprint: "ready", Rendition: rendition},
		}},
		func() time.Time { return now }, nil, func() string { return "policy" },
		func() string { return "internal" }, func() prepared.RenditionContract { return rendition },
	)
	if _, err := r.Plan(t.Context(), now, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	channels.channels[0].Policy.Playout.AudioLanguage = "jpn"
	if _, ok, err := r.ResolvePrepared(t.Context(), playout.TuneRequest{ChannelID: "ch"}); err != nil || ok {
		t.Fatalf("ResolvePrepared after channel audio change = ok %v err %v, want immediate miss", ok, err)
	}
	if inputs.calls != 1 || timeline.audioCalls != 1 {
		t.Fatal("tune tried to re-resolve the changed channel policy")
	}
}

func TestPreparedBudgetBytesUsesGiBAndSaturates(t *testing.T) {
	if got := preparedBudgetBytes(512); got != int64(512)<<30 {
		t.Fatalf("preparedBudgetBytes(512) = %d", got)
	}
	if got := preparedBudgetBytes(0); got != 0 {
		t.Fatalf("preparedBudgetBytes(0) = %d, want disabled", got)
	}
	if got := preparedBudgetBytes(int(^uint(0) >> 1)); got <= 0 {
		t.Fatalf("preparedBudgetBytes(max int) overflowed to %d", got)
	}
}
