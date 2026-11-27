package api_test

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	guideapi "github.com/loomarr/loomarr/internal/api/guide"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/operator"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/storeadapter"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/playout/timeline"
)

// deferredPods lets a test wire the real pod previewer over the harness's own store, which exists
// only once the harness has started.
type deferredPods struct{ guideapi.PodPreviewer }

// #2147: the Guide resolves every row's breaks in one batch over the REAL assembler and store. The
// grid must not change: every block keeps its kind, start and stop, and every break lists the
// clips the per-row path (one PreviewAtMany per channel, what the Guide did before) assembles for
// it, in order, on channels whose filler selections differ.
func TestGuide_BatchedPodsMatchThePerRowPath(t *testing.T) {
	now := time.Now().Truncate(time.Minute)
	grid := map[string][]timeline.Broadcast{}
	for c, id := range []string{"eq-1", "eq-2", "eq-3", "eq-4"} {
		var bs []timeline.Broadcast
		at := now.Add(time.Duration(c) * 3 * time.Minute)
		for i := range 6 {
			programme := timeline.Broadcast{Kind: schedule.SlotProgram, Title: fmt.Sprintf("%s programme %d", id, i), Start: at, Stop: at.Add(22 * time.Minute)}
			brk := timeline.Broadcast{Kind: schedule.SlotFiller, Title: "Break", Start: programme.Stop, Stop: programme.Stop.Add(time.Duration(60+30*(i%3)) * time.Second)}
			bs = append(bs, programme, brk)
			at = brk.Stop
		}
		grid[id] = bs
	}
	grid["eq-4"] = grid["eq-4"][:1] // a row with no breaks

	pods := &deferredPods{}
	harness := newGuideHarness(t, guideHarnessConfig{Guide: &fakeXMLTVGuide{withPending: grid}, Pods: pods})
	st, ctx := harness.Store, context.Background()
	for i := range 30 {
		id := fmt.Sprintf("eq-spot-%02d", i)
		c := clips.Clip{
			Hash: id, Path: id + ".mp4", Name: "Spot " + id, NameSource: clips.NameFromCurated, Role: clips.Commercial,
			Era: 1970 + (i%3)*10, Audience: clips.General, DurationMs: int64(15_000 + (i%3)*15_000),
		}
		if i%7 == 0 {
			c.Role = clips.StationID
			c.DurationMs = 5_000
		}
		if err := st.UpsertClip(ctx, clipstore.Clip{Clip: c, UpdatedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	eras := []int{0, 1970, 1990, 1980}
	for i, id := range []string{"eq-1", "eq-2", "eq-3", "eq-4"} {
		ch := channelstore.Channel{Channel: schedule.Channel{ID: id, Name: id, Number: i + 1, Strategy: "sequential", Status: schedule.StatusLive}}
		if eras[i] != 0 {
			ch.Policy.Scope = schedule.ScopePolicy{Dates: schedule.EraDates(schedule.Range{From: eras[i], To: eras[i] + 9})}
		}
		if _, err := st.SaveChannel(ctx, ch); err != nil {
			t.Fatal(err)
		}
	}
	adapter := pod.NewAdapter(storeadapter.NewCatalog(st), nil, func() pod.Policy {
		return pod.Policy{PodMax: 4, Cooldown: time.Hour, BreakDurationMs: 120_000}
	}, slog.New(slog.DiscardHandler))
	previewer := operator.NewPodPreview(st, adapter)
	pods.PodPreviewer = previewer

	body := getGuide(t, harness.Server, "?from="+ms(now.UnixMilli())+"&to="+ms(now.Add(4*time.Hour).UnixMilli()))

	if len(body.Channels) != 4 {
		t.Fatalf("grid has %d rows, want 4", len(body.Channels))
	}
	distinct := map[string]bool{}
	for _, row := range body.Channels {
		want := grid[row.ChannelID]
		if len(row.Airings) != len(want) {
			t.Fatalf("%s: %d blocks, want %d", row.ChannelID, len(row.Airings), len(want))
		}
		var slots []pod.BreakSlot
		for _, b := range want {
			if b.Kind == schedule.SlotFiller {
				slots = append(slots, pod.BreakSlot{Start: b.Start, Length: b.BreakLength()})
			}
		}
		var perRow []pod.Pod
		if len(slots) > 0 {
			var err error
			if perRow, err = previewer.PreviewAtMany(ctx, row.ChannelID, slots); err != nil {
				t.Fatal(err)
			}
		}
		breaks := 0
		for i, a := range row.Airings {
			if a.Kind != string(want[i].Kind) || a.StartMs != want[i].Start.UnixMilli() || a.StopMs != want[i].Stop.UnixMilli() {
				t.Errorf("%s block %d = %s [%d, %d), want %s [%d, %d)", row.ChannelID, i,
					a.Kind, a.StartMs, a.StopMs, want[i].Kind, want[i].Start.UnixMilli(), want[i].Stop.UnixMilli())
			}
			if want[i].Kind != schedule.SlotFiller {
				if a.Pod != nil {
					t.Errorf("%s block %d: a programme carries a pod", row.ChannelID, i)
				}
				continue
			}
			expected := perRow[breaks]
			breaks++
			if a.Pod == nil {
				t.Fatalf("%s block %d: the break has no pod, the per-row path lists %d clips", row.ChannelID, i, len(expected.Entries))
			}
			var got, wantNames []string
			for _, e := range a.Pod.Entries {
				if e.Name == "" {
					t.Fatalf("%s block %d: an entry has no name to compare", row.ChannelID, i)
				}
				got = append(got, e.Role+":"+e.Name)
			}
			for _, e := range expected.Entries {
				wantNames = append(wantNames, string(e.Role)+":"+e.Name)
			}
			if fmt.Sprint(got) != fmt.Sprint(wantNames) || a.Pod.MatchLevel != string(expected.MatchLevel) || a.Pod.TotalMs != expected.TotalMs {
				t.Errorf("%s block %d pod = %s %v (%d ms), per-row path = %s %v (%d ms)", row.ChannelID, i,
					a.Pod.MatchLevel, got, a.Pod.TotalMs, expected.MatchLevel, wantNames, expected.TotalMs)
			}
			distinct[fmt.Sprint(got)] = true
		}
	}
	if len(distinct) < 4 {
		t.Errorf("only %d distinct pods across the grid: the fixture no longer tells the channels apart", len(distinct))
	}
}
