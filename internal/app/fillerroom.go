package app

import (
	"context"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/upkeep"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/playout/timeline"
)

// guideWindow is how far ahead a break's clips are protected from eviction: the window the guide
// opens on. A break inside it is one a viewer can be shown or tune into.
const guideWindow = 4 * time.Hour

// airingBreakGrace reaches back past now so a break already on air, whose clips are being resolved
// one at a time, is inside the window.
const airingBreakGrace = 15 * time.Minute

// fillerRoomStore is what the room reads from the store.
type fillerRoomStore interface {
	ListChannels(ctx context.Context) ([]channelstore.Channel, error)
	FillerAirings(ctx context.Context) ([]clips.Airing, error)
}

// fillerRoomPods is the channel pod previewer (operator.PodPreview).
type fillerRoomPods interface {
	Standing(ctx context.Context, airings []clips.Airing, spacing time.Duration, now time.Time) (pod.Standing, error)
	PreviewAtMany(ctx context.Context, channelID string, slots []pod.BreakSlot) ([]pod.Pod, error)
}

// fillerRoomTimeline is the guide's timeline walk (playoutresolve.Resolver).
type fillerRoomTimeline interface {
	BroadcastsWithPending(ctx context.Context, channelID string, from, to time.Time) ([]timeline.Broadcast, error)
}

// fillerRoom wires what a full budget evicts by (#3855, decision 13 of #3838): the governor's
// shortfall, how the live channels stand toward each clip, and the clips in breaks inside the guide
// window. A nil timeline (playout off) leaves InBreak unset, so no clip on air is evicted: without
// it a clip in a coming break cannot be told apart from one that is not.
func fillerRoom(
	governor *storagegovernor.Governor, clipDir string, st fillerRoomStore, pods fillerRoomPods,
	timeline fillerRoomTimeline, spacing func() time.Duration, now func() time.Time,
) upkeep.Room {
	if governor == nil {
		return upkeep.Room{}
	}
	room := upkeep.Room{
		Shortfall: func(ctx context.Context) int64 {
			return governor.LibraryShortfall(ctx, clipDir, storagegovernor.DomainFiller)
		},
		Standing: func(ctx context.Context) (upkeep.Standing, error) {
			airings, err := st.FillerAirings(ctx)
			if err != nil {
				return upkeep.Standing{}, err
			}
			standing, err := pods.Standing(ctx, airings, spacing(), now())
			return upkeep.Standing{Idle: standing.Idle, Surplus: standing.Surplus}, err
		},
	}
	if timeline == nil {
		return room
	}
	room.InBreak = func(ctx context.Context) (map[string]bool, error) {
		return clipsInBreaks(ctx, st, pods, timeline, now())
	}
	return room
}

// clipsInBreaks is every clip in a break of a channel's timeline from now into the guide window,
// assembled as the guide assembles it (PreviewAtMany over the timeline's filler slots). A channel
// whose timeline or breaks cannot be read fails the whole answer: a clip is only evictable when
// every channel has said it is not in a coming break.
func clipsInBreaks(ctx context.Context, st fillerRoomStore, pods fillerRoomPods, timeline fillerRoomTimeline, now time.Time) (map[string]bool, error) {
	chans, err := st.ListChannels(ctx)
	if err != nil {
		return nil, err
	}
	in := map[string]bool{}
	for _, ch := range chans {
		if !ch.Status.Reconcilable() {
			continue
		}
		broadcasts, err := timeline.BroadcastsWithPending(ctx, ch.ID, now.Add(-airingBreakGrace), now.Add(guideWindow))
		if err != nil {
			return nil, fmt.Errorf("read channel %s's timeline: %w", ch.ID, err)
		}
		var slots []pod.BreakSlot
		for _, b := range broadcasts {
			if b.Kind == schedule.SlotFiller {
				slots = append(slots, pod.BreakSlot{Start: b.Start, Length: b.BreakLength()})
			}
		}
		if len(slots) == 0 {
			continue
		}
		pods, err := pods.PreviewAtMany(ctx, ch.ID, slots)
		if err != nil {
			return nil, fmt.Errorf("assemble channel %s's breaks: %w", ch.ID, err)
		}
		for _, p := range pods {
			for _, e := range p.Entries {
				if e.Hash != "" {
					in[e.Hash] = true
				}
			}
		}
	}
	return in, nil
}
