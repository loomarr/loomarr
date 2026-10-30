package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/encode"
	"github.com/loomarr/loomarr/internal/playout"
)

var burnTestTracks = []playout.Track{
	{Index: 0, Language: "eng", Codec: "subrip"},
	{Index: 1, Language: "eng", Codec: "hdmv_pgs_subtitle"},
	{Index: 2, Language: "spa", Codec: "dvd_subtitle"},
}

func TestBurnTrack_IsTheChannelLanguagesImageTrack(t *testing.T) {
	if got, ok := burnTrack(burnTestTracks, "eng"); !ok || got.Index != 1 {
		t.Errorf("eng: %+v %v, want the PGS track, not the text one", got, ok)
	}
	if _, ok := burnTrack(burnTestTracks, "spa"); ok {
		t.Error("spa: VOBSUB is not burnable yet")
	}
	if _, ok := burnTrack(burnTestTracks, ""); ok {
		t.Error("no language burns nothing")
	}
}

// fakePreparer writes a prepared track's index the way FFmpegBurnInWith does, counting its runs.
type fakePreparer struct {
	runs atomic.Int32
	fail atomic.Bool
	done chan string
}

func (f *fakePreparer) prepare(_ context.Context, _ string, _ int, dir string) (*playout.BurnIn, error) {
	f.runs.Add(1)
	defer func() { f.done <- dir }()
	if f.fail.Load() {
		return nil, errors.New("no such track")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	raw := `{"width":1920,"height":1080,"cues":[{"at":1000000000,"still":"c00001.nut"}]}`
	if err := os.WriteFile(filepath.Join(dir, "burnin.json"), []byte(raw), 0o644); err != nil {
		return nil, err
	}
	return &playout.BurnIn{Dir: dir, Width: 1920, Height: 1080}, nil
}

func newTestBurns(t *testing.T, works bool) (*subtitleBurns, *fakePreparer) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	f := &fakePreparer{done: make(chan string, 8)}
	s := newSubtitleBurns(ctx, t.TempDir(), f.prepare, func(encode.Encoder) bool { return works },
		func() string { return "/k.cl" }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	return s, f
}

func waitPrepared(t *testing.T, f *fakePreparer) {
	t.Helper()
	select {
	case <-f.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the track was never prepared")
	}
	// The worker clears its queued mark right after the preparer returns.
	time.Sleep(20 * time.Millisecond)
}

// The first airing has nothing ready and queues ONE preparation, however often it asks; once
// prepared, the next airing burns it in with the VAAPI kernel.
func TestSubtitleBurns_PreparesInTheBackgroundThenServes(t *testing.T) {
	s, f := newTestBurns(t, true)
	resolve := s.For("/media/film.mkv", burnTestTracks, "eng")
	if resolve == nil {
		t.Fatal("no resolver for a programme with an English PGS track")
	}
	if got := resolve(encode.EncoderNVENC); got != nil {
		t.Fatalf("served %+v before it was prepared", got)
	}
	_ = resolve(encode.EncoderNVENC)
	waitPrepared(t, f)
	got := resolve(encode.EncoderNVENC)
	if got == nil || got.Track.Width != 1920 || got.Kernel != "/k.cl" {
		t.Fatalf("after preparing: %+v", got)
	}
	if f.runs.Load() != 1 {
		t.Errorf("prepared %d times, want once", f.runs.Load())
	}
}

func TestSubtitleBurns_NothingWithoutTheOverlayOrATrack(t *testing.T) {
	s, f := newTestBurns(t, false)
	if s.For("/media/film.mkv", burnTestTracks, "fra") != nil {
		t.Error("a resolver for a language the programme lacks")
	}
	if got := s.For("/media/film.mkv", burnTestTracks, "eng")(encode.EncoderVAAPI); got != nil {
		t.Errorf("served %+v where the overlay failed its self-check", got)
	}
	if f.runs.Load() != 0 {
		t.Error("prepared a track no encoder can draw")
	}
}

// A track that fails is not retried at every airing.
func TestSubtitleBurns_BacksOffAFailure(t *testing.T) {
	s, f := newTestBurns(t, true)
	f.fail.Store(true)
	s.Prepare("/media/film.mkv", burnTestTracks, "eng")
	waitPrepared(t, f)
	s.Prepare("/media/film.mkv", burnTestTracks, "eng")
	_ = s.For("/media/film.mkv", burnTestTracks, "eng")(encode.EncoderNVENC)
	time.Sleep(50 * time.Millisecond)
	if f.runs.Load() != 1 {
		t.Errorf("prepared %d times, want the one failure", f.runs.Load())
	}
}

// A replaced file is a new track: its stills are named by size and modification time too.
func TestSubtitleBurns_TrackDirFollowsTheFile(t *testing.T) {
	s, _ := newTestBurns(t, true)
	file := filepath.Join(t.TempDir(), "film.mkv")
	if err := os.WriteFile(file, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := s.trackDir(file, 1)
	if s.trackDir(file, 1) != before || s.trackDir(file, 2) == before {
		t.Fatal("a track's directory is not stable, or two tracks share one")
	}
	if err := os.WriteFile(file, []byte("v2 longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if s.trackDir(file, 1) == before {
		t.Error("a replaced file kept its old stills")
	}
	if !playoutSafePath(before) {
		t.Errorf("%q would need escaping in a filter graph", before)
	}
}

func TestSubtitleBurns_EvictsTheLeastRecentlyAired(t *testing.T) {
	s, _ := newTestBurns(t, true)
	old := time.Now().Add(-time.Hour)
	for i := range burnKeep + 2 {
		dir := filepath.Join(s.dir, fmt.Sprintf("t%02d", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		at := old.Add(time.Duration(i) * time.Minute)
		_ = os.Chtimes(dir, at, at)
	}
	s.evict()
	for _, gone := range []string{"t00", "t01"} {
		if _, err := os.Stat(filepath.Join(s.dir, gone)); !os.IsNotExist(err) {
			t.Errorf("%s kept: the oldest go first", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(s.dir, "t02")); err != nil {
		t.Errorf("t02 evicted: %v", err)
	}
}

// playoutSafePath mirrors the graph builder's rule for paths it embeds without escaping.
func playoutSafePath(p string) bool {
	for _, r := range p {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '/' || r == '.' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
