package playoutresolve

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/playout/subtitles"
)

// fakeSubtitleTracks lists each file's subtitle streams, whatever its stat.
type fakeSubtitleTracks map[string][]librarystore.LibraryStream

func (f fakeSubtitleTracks) LibrarySubtitleTracks(_ context.Context, path string, _, _ int64) ([]librarystore.LibraryStream, bool, error) {
	streams, ok := f[path]
	return streams, ok, nil
}

// fakeCues answers each input's cues, or Pending for an input it doesn't hold.
type fakeCues map[string][]subtitles.Cue

func (f fakeCues) Cues(src subtitles.Source) ([]subtitles.Cue, subtitles.State) {
	cues, ok := f[src.Input]
	if !ok {
		return nil, subtitles.Pending
	}
	return cues, subtitles.Ready
}

func TestSubtitles_PlacesCuesAcrossABreak(t *testing.T) {
	t.Parallel()
	// A film cut by a break: its first minute, a 30 s break, then its second minute.
	slots := []schedule.Slot{
		{Kind: schedule.SlotProgram, Title: "film", LibraryItemID: "film", Key: "film", DurationMs: 60_000},
		{Kind: schedule.SlotFiller, DurationMs: 30_000},
		{Kind: schedule.SlotProgram, Title: "film", LibraryItemID: "film", Key: "film", Segment: 1,
			SourceOffsetMs: 60_000, DurationMs: 60_000},
	}
	anchor := testPlayoutAnchor()
	r := stillResolver(t, slots, anchor, nil)
	film := r.libraryFile.(fakeLibraryFiles)["film"]
	beside := filepath.Join(filepath.Dir(film), "film.en.sdh.srt")
	if err := os.WriteFile(beside, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.SetSubtitles(fakeSubtitleTracks{film: {
		{Index: 2, Kind: "subtitle", Codec: "subrip", Language: "en", Title: "English"},
		{Index: 3, Kind: "subtitle", Codec: "hdmv_pgs_subtitle", Language: "fr"},
		{Index: 4, Kind: "subtitle", Codec: "subrip", Language: "en", HearingImpaired: true, ExternalRef: beside},
	}}, fakeCues{film: {
		{Start: 10 * time.Second, End: 12 * time.Second, Text: "one"},
		{Start: 59 * time.Second, End: 62 * time.Second, Text: "across the cut"},
		{Start: 70 * time.Second, End: 72 * time.Second, Text: "two"},
	}})

	window, err := r.Subtitles(context.Background(), "ch1", anchor, anchor.Add(150*time.Second), "en")
	if err != nil {
		t.Fatal(err)
	}
	keys := []string{}
	for _, track := range window.Tracks {
		keys = append(keys, track.Key)
	}
	if len(keys) != 2 || keys[0] != "en" || keys[1] != "en-sdh" {
		t.Fatalf("tracks = %v, want en and en-sdh (the image track not offered)", keys)
	}
	if src := window.Tracks[1].Source; src.Input != beside || src.Stream != -1 {
		t.Fatalf("the SDH track reads %+v, want the file beside the film", src)
	}
	at := func(seconds int) time.Time { return anchor.Add(time.Duration(seconds) * time.Second) }
	want := []subtitles.Timed{
		{Start: at(10), End: at(12), Text: "one"},
		// Cut by the break, and still on screen as the film resumes.
		{Start: at(59), End: at(60), Text: "across the cut"},
		{Start: at(90), End: at(92), Text: "across the cut"},
		{Start: at(100), End: at(102), Text: "two"},
	}
	if len(window.Cues) != len(want) || window.Pending {
		t.Fatalf("cues = %+v pending=%v, want %+v", window.Cues, window.Pending, want)
	}
	for i := range want {
		if got := window.Cues[i]; !got.Start.Equal(want[i].Start) || !got.End.Equal(want[i].End) || got.Text != want[i].Text {
			t.Errorf("cue %d = %+v, want %+v", i, got, want[i])
		}
	}

	// From inside the break: no tracks to list, and the film's cues once it resumes.
	window, err = r.Subtitles(context.Background(), "ch1", at(75), at(105), "en")
	if err != nil || len(window.Tracks) != 0 || len(window.Cues) != 2 {
		t.Fatalf("from the break: tracks=%v cues=%+v err=%v; want none listed and two cues", window.Tracks, window.Cues, err)
	}

	// The SDH track hasn't been read yet: no cues, and the player is told to ask again.
	window, err = r.Subtitles(context.Background(), "ch1", anchor, at(30), "en-sdh")
	if err != nil || len(window.Cues) != 0 || !window.Pending {
		t.Fatalf("unread track: cues=%+v pending=%v err=%v; want none, pending", window.Cues, window.Pending, err)
	}
}

func TestSubtitles_EmptyWithoutTracksOrChoice(t *testing.T) {
	t.Parallel()
	slots := []schedule.Slot{{Kind: schedule.SlotProgram, Title: "film", LibraryItemID: "film", DurationMs: 60_000}}
	anchor := testPlayoutAnchor()
	r := stillResolver(t, slots, anchor, nil)

	// Not attached: nothing, and no error.
	window, err := r.Subtitles(context.Background(), "ch1", anchor, anchor.Add(time.Minute), "en")
	if err != nil || len(window.Tracks) != 0 || len(window.Cues) != 0 {
		t.Fatalf("unattached: %+v %v", window, err)
	}
	film := r.libraryFile.(fakeLibraryFiles)["film"]
	r.SetSubtitles(fakeSubtitleTracks{film: {{Index: 2, Kind: "subtitle", Codec: "subrip", Language: "en"}}},
		fakeCues{film: {{Start: time.Second, End: 2 * time.Second, Text: "hi"}}})
	// No choice: the tracks are listed, and no cues are read.
	window, err = r.Subtitles(context.Background(), "ch1", anchor, anchor.Add(time.Minute), "")
	if err != nil || len(window.Tracks) != 1 || len(window.Cues) != 0 || window.Pending {
		t.Fatalf("no choice: %+v %v", window, err)
	}
}
