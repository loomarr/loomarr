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

// A programme offset into a movie in parts airs the part it falls in, from where in that part it
// is; the last part airs to the end of the slot. Parts whose lengths are not all read air as the
// first alone.
func TestTheFileAProgrammeAirsAtAnOffset(t *testing.T) {
	parts := []librarystore.LibraryPlayingPart{{Path: "a", DurationMs: 60_000}, {Path: "b", DurationMs: 30_000}, {Path: "c", DurationMs: 90_000}}
	for _, c := range []struct {
		offset time.Duration
		want   programmeInput
	}{
		{0, programmeInput{Path: "a", End: time.Minute}},
		{59 * time.Second, programmeInput{Path: "a", End: time.Minute}},
		{time.Minute, programmeInput{Path: "b", Start: time.Minute, End: 90 * time.Second}},
		{100 * time.Second, programmeInput{Path: "c", Start: 90 * time.Second}},
		{time.Hour, programmeInput{Path: "c", Start: 90 * time.Second}},
	} {
		if got := inputAt(filesOf(parts), c.offset); got != c.want {
			t.Errorf("at %v: %+v, want %+v", c.offset, got, c.want)
		}
	}
	if got := inputAt(filesOf(parts[:1]), time.Hour); got != (programmeInput{Path: "a"}) {
		t.Errorf("a whole file = %+v, want it to the end of its slot", got)
	}
	if got := inputAt(filesOf(nil), 0); got != (programmeInput{}) {
		t.Errorf("no file = %+v, want nothing", got)
	}
	unread := []librarystore.LibraryPlayingPart{{Path: "a", DurationMs: 60_000}, {Path: "b"}}
	if got := inputAt(filesOf(unread), 2*time.Minute); got != (programmeInput{Path: "a"}) {
		t.Errorf("parts not all read = %+v, want the first alone", got)
	}
}

// partsResolver airs one 3-minute movie in two parts, a minute then two, each a file on disk.
func partsResolver(t *testing.T, now time.Time) (r *Resolver, one, two string) {
	t.Helper()
	slots := []schedule.Slot{{Kind: schedule.SlotProgram, Title: "film", LibraryItemID: "film", Key: "film", DurationMs: 180_000}}
	r = stillResolver(t, slots, now, nil)
	dir := t.TempDir()
	one, two = filepath.Join(dir, "film cd1.mkv"), filepath.Join(dir, "film cd2.mkv")
	for _, p := range []string{one, two} {
		if err := os.WriteFile(p, []byte("matroska"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r.libraryFile = fakeLibraryParts{"film": {{Path: one, DurationMs: 60_000}, {Path: two, DurationMs: 120_000}}}
	return r, one, two
}

// A movie in parts airs a part at a time: the encode stops where a part does, and the packager,
// asking again there, seeks the next part from its start. The airing's offset stays the
// programme's, which its history and its guide block are kept by.
func TestAMovieInPartsAirsEachPartInTurn(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	anchor := testPlayoutAnchor()
	for _, c := range []struct {
		into                    time.Duration
		input                   string
		seek, offset, remaining time.Duration
	}{
		{30 * time.Second, "one", 30 * time.Second, 30 * time.Second, 30 * time.Second},
		{time.Minute, "two", 0, time.Minute, 2 * time.Minute},
		{100 * time.Second, "two", 40 * time.Second, 100 * time.Second, 80 * time.Second},
	} {
		r, one, two := partsResolver(t, anchor.Add(c.into))
		airing, input, err := r.AiringAt(ctx, "ch1", anchor.Add(c.into))
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{"one": one, "two": two}[c.input]
		if input != want || airing.Seek() != c.seek || airing.Offset != c.offset || airing.Remaining != c.remaining {
			t.Errorf("%v in: %s from %v (offset %v, %v left); want %s from %v (offset %v, %v left)", c.into,
				filepath.Base(input), airing.Seek(), airing.Offset, airing.Remaining, filepath.Base(want), c.seek, c.offset, c.remaining)
		}

		still, ok, err := r.StillAiring(ctx, "ch1")
		if err != nil || !ok {
			t.Fatalf("still: ok=%v err=%v", ok, err)
		}
		source, ok, err := still.Source(ctx)
		if err != nil || !ok || source.Input != want || still.Offset-source.PartStart != c.seek {
			t.Errorf("%v in: still from %s at %v, %v, %v; want %s at %v", c.into, filepath.Base(source.Input),
				still.Offset-source.PartStart, ok, err, filepath.Base(want), c.seek)
		}
	}

	// A part gone takes the whole movie off air, not just that part.
	r, _, two := partsResolver(t, anchor)
	if err := os.Remove(two); err != nil {
		t.Fatal(err)
	}
	if airing, input, err := r.AiringAt(ctx, "ch1", anchor); err != nil || input != "" || airing.Playable() {
		t.Errorf("with a part gone: %+v %q %v; want nothing airing", airing, input, err)
	}
}

// Each part of a movie in parts has its own subtitles on its own clock: a cue past the end of a
// part's picture never airs, and the next part's cues start where it does.
func TestSubtitlesFollowAMovieInParts(t *testing.T) {
	t.Parallel()
	anchor := testPlayoutAnchor()
	r, one, two := partsResolver(t, anchor)
	en := []librarystore.LibraryStream{{Index: 2, Kind: "subtitle", Codec: "subrip", Language: "en"}}
	r.SetSubtitles(fakeSubtitleTracks{one: en, two: en}, fakeCues{
		one: {{Start: 10 * time.Second, End: 12 * time.Second, Text: "one"}, {Start: 59 * time.Second, End: 61 * time.Second, Text: "end of one"}},
		two: {{Start: 5 * time.Second, End: 7 * time.Second, Text: "two"}},
	})
	window, err := r.Subtitles(context.Background(), "ch1", anchor, anchor.Add(2*time.Minute), "en")
	if err != nil {
		t.Fatal(err)
	}
	at := func(seconds int) time.Time { return anchor.Add(time.Duration(seconds) * time.Second) }
	want := []subtitles.Timed{
		{Start: at(10), End: at(12), Text: "one"},
		{Start: at(59), End: at(60), Text: "end of one"},
		{Start: at(65), End: at(67), Text: "two"},
	}
	if len(window.Cues) != len(want) || len(window.Tracks) != 1 {
		t.Fatalf("tracks=%d cues=%+v, want one track and %+v", len(window.Tracks), window.Cues, want)
	}
	for i := range want {
		if got := window.Cues[i]; !got.Start.Equal(want[i].Start) || !got.End.Equal(want[i].End) || got.Text != want[i].Text {
			t.Errorf("cue %d = %+v, want %+v", i, got, want[i])
		}
	}
}
