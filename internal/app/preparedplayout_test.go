package app

import (
	"context"
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

type preparedTimelineFake struct {
	broadcasts map[string][]playout.Broadcast
	audioCalls int
}

func (f *preparedTimelineFake) ScheduledBroadcasts(
	_ context.Context, channelID string, _, _ time.Time,
) ([]playout.Broadcast, error) {
	return f.broadcasts[channelID], nil
}

func (f *preparedTimelineFake) AudioTrackFor(context.Context, string) int {
	f.audioCalls++
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
	r := newPreparedRuntimeResolver(
		preparedChannels{channels: []store.Channel{
			{Channel: schedule.Channel{ID: "internal"}},
			{Channel: schedule.Channel{ID: "tunarr"}, Policy: schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Playout: &schedule.PlayoutPolicy{Backend: "tunarr"}}}},
		}}, timeline, inputs, preparedLookupFake{}, func() time.Time { return now }, nil,
		func() string { return "policy" }, func() string { return "internal" },
		func() prepared.RenditionContract { return playout.CanonicalPreparedRendition(playout.TierBalanced) },
	)

	candidates, err := r.Candidates(t.Context(), now, now.Add(6*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].Request.Source.Path != "/media/shared.mkv" ||
		candidates[0].Request.Source.AudioTrack != 2 || !candidates[0].NeededAt.Equal(now) {
		t.Fatalf("candidates = %+v", candidates)
	}
	if inputs.calls != 1 || timeline.audioCalls != 1 {
		t.Fatalf("source resolution calls = input %d audio %d, want one each", inputs.calls, timeline.audioCalls)
	}

	// The warmed source policy makes the next planner pass a pure index read.
	if _, err := r.Candidates(t.Context(), now, now.Add(6*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if inputs.calls != 1 || timeline.audioCalls != 1 {
		t.Fatal("unchanged source policy re-resolved or re-probed the library item")
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
	r := newPreparedRuntimeResolver(
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
	if _, err := r.Candidates(t.Context(), now.Add(-time.Minute), now.Add(time.Hour)); err != nil {
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
	r := newPreparedRuntimeResolver(
		preparedChannels{channels: []store.Channel{{Channel: schedule.Channel{ID: "ch"}}}}, timeline, inputs,
		preparedLookupFake{hits: map[prepared.Request]prepared.Specification{request: {SourceFingerprint: "hit", Rendition: rendition}}},
		func() time.Time { return now }, nil, func() string { return policy },
		func() string { return "internal" }, func() prepared.RenditionContract { return rendition },
	)
	if _, err := r.Candidates(t.Context(), now, now.Add(time.Hour)); err != nil {
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
