package app

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/playout"
)

func TestWallCues_PlacesSourceCuesOnTheWallClock(t *testing.T) {
	at := time.Date(2026, 10, 30, 20, 0, 0, 0, time.UTC)
	// The programme is 10 min in at `at`; this part of it began 2 min earlier (source 8:00).
	offset, partStart := 10*time.Minute, 8*time.Minute
	stop := at.Add(5 * time.Minute)
	cues := []playout.SubtitleCue{
		{Start: 7 * time.Minute, End: 7*time.Minute + 2*time.Second, Text: "earlier part"},
		{Start: 9*time.Minute + 58*time.Second, End: 10*time.Minute + 3*time.Second, Text: "on screen at lo"},
		{Start: 11 * time.Minute, End: 11*time.Minute + 2*time.Second, Text: "in window"},
		{Start: 14*time.Minute + 59*time.Second, End: 15*time.Minute + 4*time.Second, Text: "cut at stop"},
		{Start: 20 * time.Minute, End: 20*time.Minute + time.Second, Text: "after hi"},
	}
	got := wallCues(cues, offset, at, partStart, at, stop, stop)
	want := []playout.SubtitleWallCue{
		{Start: at.Add(-2 * time.Second), End: at.Add(3 * time.Second), Text: "on screen at lo"},
		{Start: at.Add(time.Minute), End: at.Add(time.Minute + 2*time.Second), Text: "in window"},
		{Start: at.Add(4*time.Minute + 59*time.Second), End: stop, Text: "cut at stop"},
	}
	if len(got) != len(want) {
		t.Fatalf("cues = %+v, want %+v", got, want)
	}
	for i := range want {
		if !got[i].Start.Equal(want[i].Start) || !got[i].End.Equal(want[i].End) || got[i].Text != want[i].Text {
			t.Errorf("cue %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSubtitleCache_SharesOneExtractionAndRetriesAFailure(t *testing.T) {
	var c subtitleCache
	var calls atomic.Int32
	fail := true
	extract := func(_ context.Context, _ string, _ int, from, to time.Duration) ([]playout.SubtitleCue, error) {
		calls.Add(1)
		if fail {
			return nil, errors.New("ffmpeg went away")
		}
		return []playout.SubtitleCue{{Start: from, End: to, Text: "x"}}, nil
	}
	if _, err := c.chunk(context.Background(), "in", 0, 1, extract); err == nil {
		t.Fatal("a failed extraction succeeded")
	}
	fail = false
	for range 3 {
		cues, err := c.chunk(context.Background(), "in", 0, 1, extract)
		if err != nil || len(cues) != 1 || cues[0].Start != playout.SubtitleChunk {
			t.Fatalf("cues = %+v, err = %v", cues, err)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("extractions = %d, want the failure plus one", calls.Load())
	}
}

func TestSubtitleCache_OffersOnlyTextTracks(t *testing.T) {
	var c subtitleCache
	probe := func(context.Context, string) (playout.MediaTracks, error) {
		return playout.MediaTracks{Subtitles: []playout.Track{
			{Index: 0, Language: "eng", Codec: "hdmv_pgs_subtitle"},
			{Index: 1, Language: "eng", Codec: "subrip"},
		}}, nil
	}
	tracks, err := c.tracks(context.Background(), "in", probe)
	if err != nil || len(tracks) != 1 || tracks[0].Index != 1 {
		t.Fatalf("tracks = %+v, err = %v", tracks, err)
	}
}
