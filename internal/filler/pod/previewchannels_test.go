package pod_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/pod"
)

// #2147: the Guide assembles every row's breaks in ONE batch. The batch must not change a single
// pod: each channel's pods are what PreviewAtMany returns for that channel alone, with its own
// selection, its own stored plan and the same household history, while the catalog and the
// history are read once for every channel. Channels assemble concurrently, so run it under -race.
func TestPreviewChannelsMatchesEachChannelOnItsOwn(t *testing.T) {
	start := time.Date(2026, 11, 3, 20, 0, 0, 0, time.UTC)
	var cat []pod.Clip
	for i := range 24 {
		id := fmt.Sprintf("spot-%02d", i)
		cat = append(cat, pod.Clip{
			Hash: id, Path: id + ".mp4", Role: pod.Commercial, Era: 1970 + (i%3)*10,
			Audience: pod.General, DurationMs: int64(15_000 + (i%3)*15_000),
			Category: fmt.Sprintf("cat-%d", i%4), Tags: []string{fmt.Sprintf("cat-%d", i%4)},
		})
	}
	cat = append(cat,
		pod.Clip{Hash: "ident-a", Path: "ident-a.mp4", Role: pod.StationID, Audience: pod.General, DurationMs: 5_000},
		pod.Clip{Hash: "ident-b", Path: "ident-b.mp4", Role: pod.Bumper, Era: 1980, Audience: pod.General, DurationMs: 4_000},
	)
	// Airings on several channels, before, inside and after the window: each break cuts its own
	// snapshot from the one history.
	history := []pod.Airing{
		aired("spot-00", start.Add(-2*time.Hour)), aired("spot-03", start.Add(-20*time.Minute)),
		aired("spot-07", start.Add(40*time.Minute)), aired("spot-01", start.Add(90*time.Minute)),
		aired("ident-a", start.Add(-5*time.Minute)),
	}
	policy := func() pod.Policy { return pod.Policy{PodMax: 4, Cooldown: time.Hour, BreakDurationMs: 120_000} }
	clock := func() time.Time { return start.Add(-time.Hour) }
	seventies := []pod.EraRange{{From: 1970, To: 1979}}
	spacing := 10 * time.Minute

	breaksOf := func(seed int64, n int, offset time.Duration) []pod.Break {
		out := make([]pod.Break, n)
		for i := range out {
			out[i] = pod.Break{
				Seed: seed + int64(i), Start: start.Add(offset + time.Duration(i)*25*time.Minute),
				Length: time.Duration(60+30*(i%3)) * time.Second,
			}
		}
		return out
	}
	chans := []pod.ChannelBreaks{
		{ChannelID: "ch-1", Breaks: breaksOf(100, 6, 0)},
		{ChannelID: "ch-2", Selection: pod.Selection{EraWindows: seventies}, Breaks: breaksOf(200, 5, 7*time.Minute)},
		{ChannelID: "ch-3"}, // a row with no breaks
		{ChannelID: "ch-4", Selection: pod.Selection{Categories: []string{"cat-1", "cat-2"}, Spacing: &spacing}, Breaks: breaksOf(300, 4, 3*time.Minute)},
		{ChannelID: "ch-5", Selection: pod.Selection{Excluded: []string{"spot-02", "spot-05"}}, Breaks: breaksOf(400, 6, 11*time.Minute)},
	}

	// ch-1's first two breaks are planned ahead, so both paths must read them rather than assemble.
	stored := newMemoryBreaks()
	newAdapter := func(reads *int, exposures *stubExposures) *pod.Adapter {
		return pod.NewAdapter(stubCatalog{clips: cat, reads: reads}, exposures, policy, discardLogger()).
			WithBreaks(stored).WithClock(clock)
	}
	planned := make([]pod.StoredBreak, 2)
	for i := range planned {
		b := chans[0].Breaks[i]
		planned[i] = pod.StoredBreak{ChannelID: "ch-1", Start: b.Start, Pod: pod.Pod{Entries: []pod.Entry{
			{Hash: fmt.Sprintf("planned-%d", i), Role: pod.Commercial, DurationMs: 30_000},
		}}}
	}
	if err := stored.SaveBreaks(context.Background(), planned); err != nil {
		t.Fatal(err)
	}

	reads := 0
	exposures := &stubExposures{items: history}
	got, err := newAdapter(&reads, exposures).PreviewChannels(context.Background(), chans)
	if err != nil {
		t.Fatal(err)
	}
	if reads != 1 || exposures.reads != 1 {
		t.Fatalf("catalog reads = %d, history reads = %d for %d channels, want one each", reads, exposures.reads, len(chans))
	}
	if len(got) != len(chans) {
		t.Fatalf("got pods for %d channels, want %d", len(got), len(chans))
	}

	for i, c := range chans {
		want, err := newAdapter(nil, &stubExposures{items: history}).PreviewAtMany(context.Background(), c.ChannelID, c.Selection, c.Breaks)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got[i], want) {
			t.Errorf("%s: batch pods differ from the channel on its own\n batch: %+v\n alone: %+v", c.ChannelID, got[i], want)
		}
	}
	if got[2] != nil {
		t.Errorf("a channel with no breaks got %d pods, want none", len(got[2]))
	}
	if got[0][0].Entries[0].Hash != "planned-0" || got[0][1].Entries[0].Hash != "planned-1" {
		t.Errorf("ch-1's planned breaks were assembled again: %+v", got[0][:2])
	}
	for _, i := range []int{0, 1, 3, 4} {
		last := got[i][len(got[i])-1]
		if len(last.Entries) == 0 || last.MatchLevel == pod.MatchBumperCard {
			t.Fatalf("%s: last break assembled to %+v, want real commercials to compare", chans[i].ChannelID, last)
		}
	}
	if reflect.DeepEqual(got[0][2], got[1][0]) {
		t.Error("two channels with different selections and seeds got the same pod: the batch shared one channel's answer")
	}
}

// A batch with no breaks anywhere reads nothing, as a single channel with none does.
func TestPreviewChannelsWithNoBreaksReadsNothing(t *testing.T) {
	reads := 0
	exposures := &stubExposures{}
	adapter := pod.NewAdapter(stubCatalog{reads: &reads}, exposures, nil, discardLogger())
	got, err := adapter.PreviewChannels(context.Background(), []pod.ChannelBreaks{{ChannelID: "ch-1"}, {ChannelID: "ch-2"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != nil || got[1] != nil {
		t.Fatalf("pods = %+v, want two empty rows", got)
	}
	if reads != 0 || exposures.reads != 0 {
		t.Errorf("catalog reads = %d, history reads = %d, want none", reads, exposures.reads)
	}
}
