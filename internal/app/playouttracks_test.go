package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/playout"
)

// The Watch pickers' tracks (#2088 decision 6): those of the programme at the player's playhead,
// not only the one airing now, with the two audio tracks the encode maps, picked exactly as the
// encoder picks them (AudioTracksFor), so the player's labels name what each rendition carries.
func TestTracks_DescribesThePlayheadAndItsTwoAudioRenditions(t *testing.T) {
	t.Parallel()
	pod := filler.Pod{Entries: []filler.PodEntry{
		{Path: clipAt("bumper.mp4"), Name: "bumper", DurationMs: 4000},
		{Path: clipAt("1994/toys.mp4"), Name: "toys", DurationMs: 8000},
	}}
	r := fillerResolver(t, t.TempDir(), pod)
	r.audioLanguage = func() string { return "eng" }
	var probed string
	r.probeTracks = func(_ context.Context, input string) (playout.MediaTracks, error) {
		probed = input
		return playout.MediaTracks{Audio: []playout.Track{
			{Index: 0, Language: "spa"}, {Index: 1, Language: "eng"}, {Index: 2, Language: "eng", Title: "Commentary"},
		}}, nil
	}

	tracks, pick, err := r.Tracks(context.Background(), "ch1", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(probed, clipAt("bumper.mp4")) || len(tracks.Audio) != 3 {
		t.Fatalf("now: probed %q, %d audio tracks; want the bumper airing now", probed, len(tracks.Audio))
	}
	// The default is the channel's English; the alternate the Spanish, never the commentary.
	if pick != (playout.AudioPick{Default: 1, Alternate: 0}) {
		t.Fatalf("pick %+v, want default 1 (eng) and alternate 0 (spa)", pick)
	}

	// A player five seconds into the break is watching the second clip.
	if _, _, err := r.Tracks(context.Background(), "ch1", testPlayoutAnchor().Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(probed, clipAt("1994/toys.mp4")) {
		t.Fatalf("at the playhead: probed %q, want the clip airing then", probed)
	}

	// A channel whose audio preference is cleared keeps ffmpeg's first track, with no alternate.
	r.audioLanguage = func() string { return "" }
	if _, pick, _ := r.Tracks(context.Background(), "ch1", time.Time{}); pick != (playout.AudioPick{}) {
		t.Fatalf("cleared preference: pick %+v, want the first track twice", pick)
	}
}
