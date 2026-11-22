package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout"
)

type roomStore struct{ channels []channelstore.Channel }

func (s roomStore) ListChannels(context.Context) ([]channelstore.Channel, error) {
	return s.channels, nil
}
func (roomStore) FillerAirings(context.Context) ([]clips.Airing, error) { return nil, nil }

type roomPods struct {
	slots map[string][]pod.BreakSlot // what each channel was asked to assemble
	pods  map[string][]pod.Pod
	err   error
}

func (roomPods) Standing(context.Context, []clips.Airing, time.Duration, time.Time) (pod.Standing, error) {
	return pod.Standing{}, nil
}

func (p roomPods) PreviewAtMany(_ context.Context, channelID string, slots []pod.BreakSlot) ([]pod.Pod, error) {
	p.slots[channelID] = slots
	return p.pods[channelID], p.err
}

type roomTimeline struct {
	byChannel map[string][]playout.Broadcast
	err       error
	from, to  time.Time
}

func (tl *roomTimeline) BroadcastsWithPending(_ context.Context, channelID string, from, to time.Time) ([]playout.Broadcast, error) {
	tl.from, tl.to = from, to
	return tl.byChannel[channelID], tl.err
}

func roomChannel(id string, status schedule.ChannelStatus) channelstore.Channel {
	return channelstore.Channel{Channel: schedule.Channel{ID: id, Status: status}}
}

func podOf(hashes ...string) pod.Pod {
	var p pod.Pod
	for _, h := range hashes {
		p.Entries = append(p.Entries, pod.Entry{Hash: h})
	}
	return p
}

// Every clip in a break from just before now to the end of the guide window is held back from
// eviction, on every channel whose breaks are airing: the break on air now, the ones the guide
// shows, and the ones a viewer can tune into. A paused channel airs nothing, so its breaks protect
// nothing.
func TestClipsInBreaksAreThoseOfEveryLiveChannelsGuideWindow(t *testing.T) {
	now := time.Date(2026, 11, 14, 20, 0, 0, 0, time.UTC)
	timeline := &roomTimeline{byChannel: map[string][]playout.Broadcast{
		"ch-a": {
			{Kind: schedule.SlotProgram, Start: now, Stop: now.Add(20 * time.Minute)},
			{Kind: schedule.SlotFiller, Start: now.Add(20 * time.Minute), Stop: now.Add(22 * time.Minute)},
		},
		"ch-b":      {{Kind: schedule.SlotFiller, Start: now.Add(time.Hour), Stop: now.Add(time.Hour + 2*time.Minute)}},
		"ch-paused": {{Kind: schedule.SlotFiller, Start: now, Stop: now.Add(2 * time.Minute)}},
	}}
	pods := roomPods{slots: map[string][]pod.BreakSlot{}, pods: map[string][]pod.Pod{
		"ch-a":      {podOf("ad-1", "ad-2", "")},
		"ch-b":      {podOf("ad-2", "ad-3")},
		"ch-paused": {podOf("ad-9")},
	}}
	st := roomStore{channels: []channelstore.Channel{
		roomChannel("ch-a", schedule.StatusLive), roomChannel("ch-b", schedule.StatusDrifted), roomChannel("ch-paused", schedule.StatusPaused),
	}}

	got, err := clipsInBreaks(context.Background(), st, pods, timeline, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !got["ad-1"] || !got["ad-2"] || !got["ad-3"] {
		t.Fatalf("clips in breaks = %v, want ad-1, ad-2 and ad-3 (no empty hash, nothing from the paused channel)", got)
	}
	if want := now.Add(-airingBreakGrace); !timeline.from.Equal(want) || !timeline.to.Equal(now.Add(guideWindow)) {
		t.Fatalf("window = %v to %v, want from %v (the break on air) for the guide's window", timeline.from, timeline.to, want)
	}
	if len(pods.slots["ch-a"]) != 1 || pods.slots["ch-a"][0].Length != 2*time.Minute || len(pods.slots["ch-paused"]) != 0 {
		t.Fatalf("slots = %v, want one 2 minute break on ch-a only (filler slots, at their scheduled length)", pods.slots)
	}
}

// A channel whose timeline or breaks cannot be read fails the whole answer: a clip is only evictable
// when every channel has said it is not in a coming break.
func TestClipsInBreaksFailsWhenAChannelCannotBeRead(t *testing.T) {
	now := time.Date(2026, 11, 14, 20, 0, 0, 0, time.UTC)
	st := roomStore{channels: []channelstore.Channel{roomChannel("ch-a", schedule.StatusLive)}}
	broadcasts := map[string][]playout.Broadcast{"ch-a": {{Kind: schedule.SlotFiller, Start: now, Stop: now.Add(time.Minute)}}}

	_, err := clipsInBreaks(context.Background(), st, roomPods{slots: map[string][]pod.BreakSlot{}},
		&roomTimeline{byChannel: broadcasts, err: errors.New("timeline down")}, now)
	if err == nil {
		t.Error("a timeline that failed was read as no breaks")
	}
	_, err = clipsInBreaks(context.Background(), st,
		roomPods{slots: map[string][]pod.BreakSlot{}, err: errors.New("catalog down")}, &roomTimeline{byChannel: broadcasts}, now)
	if err == nil {
		t.Error("breaks that failed to assemble were read as empty")
	}
}
