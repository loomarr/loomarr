package playoutresolve

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/testkit"
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

func newTestBurns(t *testing.T, works bool, prepare playout.BurnInPreparer) *SubtitleBurns {
	t.Helper()
	return NewSubtitleBurns(t.Context(), t.TempDir(), prepare, func(encode.Encoder) bool { return works },
		func() string { return "/k.cl" }, testkit.Logger())
}

// waitIdle waits until the background worker has finished every queued preparation. A request is
// marked queued before enqueue returns and unmarked after its preparation, so idle after a request
// means that request ran or was never queued.
func waitIdle(t *testing.T, s *SubtitleBurns) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		n := len(s.queued)
		s.mu.Unlock()
		if n == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d preparations still queued", n)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// The first airing has nothing ready and queues ONE preparation, however often and however
// concurrently its encoders ask; once prepared, the next airing burns it in with the VAAPI kernel.
func TestSubtitleBurns_PreparesInTheBackgroundThenServes(t *testing.T) {
	var p testkit.BurnInPreparer
	s := newTestBurns(t, true, p.Prepare)
	resolve := s.For("/media/film.mkv", burnTestTracks, "eng")
	if resolve == nil {
		t.Fatal("no resolver for a programme with an English PGS track")
	}
	if got := resolve(encode.EncoderNVENC); got != nil {
		t.Fatalf("served %+v before it was prepared", got)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { _ = resolve(encode.EncoderNVENC) })
	}
	wg.Wait()
	waitIdle(t, s)
	got := resolve(encode.EncoderVAAPI)
	if got == nil || got.Track.Width != 1920 || got.Kernel != "/k.cl" {
		t.Fatalf("after preparing: %+v", got)
	}
	if p.Runs() != 1 {
		t.Errorf("prepared %d times, want once", p.Runs())
	}
}

// An encoder whose GPU overlay failed its self-check gets no track and prepares none, and a
// programme without a burnable track in the channel's language gets no resolver.
func TestSubtitleBurns_NothingWithoutTheOverlayOrATrack(t *testing.T) {
	var p testkit.BurnInPreparer
	s := newTestBurns(t, false, p.Prepare)
	if s.For("/media/film.mkv", burnTestTracks, "fra") != nil {
		t.Error("a resolver for a language the programme lacks")
	}
	if s.For("/media/film.mkv", burnTestTracks, "spa") != nil {
		t.Error("a resolver for a VOBSUB track")
	}
	if s.For("/media/film.mkv", nil, "eng") != nil {
		t.Error("a resolver for a programme with no subtitle tracks")
	}
	if got := s.For("/media/film.mkv", burnTestTracks, "eng")(encode.EncoderVAAPI); got != nil {
		t.Errorf("served %+v where the overlay failed its self-check", got)
	}
	waitIdle(t, s)
	if p.Runs() != 0 {
		t.Error("prepared a track no encoder can draw")
	}
}

// A track that fails is not retried at every airing, and is retried once its back-off has passed.
func TestSubtitleBurns_BacksOffAFailureThenRetries(t *testing.T) {
	var p testkit.BurnInPreparer
	p.SetFail(true)
	s := newTestBurns(t, true, p.Prepare)
	s.Prepare("/media/film.mkv", burnTestTracks, "eng")
	waitIdle(t, s)
	s.Prepare("/media/film.mkv", burnTestTracks, "eng")
	resolve := s.For("/media/film.mkv", burnTestTracks, "eng")
	if got := resolve(encode.EncoderNVENC); got != nil {
		t.Fatalf("served %+v from a failed preparation", got)
	}
	waitIdle(t, s)
	if p.Runs() != 1 {
		t.Fatalf("prepared %d times inside the back-off, want the one failure", p.Runs())
	}

	s.mu.Lock()
	for dir := range s.failed {
		s.failed[dir] = time.Now().Add(-burnRetry - time.Minute)
	}
	s.mu.Unlock()
	p.SetFail(false)
	if got := resolve(encode.EncoderNVENC); got != nil {
		t.Fatalf("served %+v before the retry ran", got)
	}
	waitIdle(t, s)
	if got := resolve(encode.EncoderNVENC); got == nil {
		t.Error("the retry after the back-off did not serve the track")
	}
	if p.Runs() != 2 {
		t.Errorf("prepared %d times, want the failure and one retry", p.Runs())
	}
}

// The wired decoder refuses until the image worker's adapter lands: the preparer gives up before
// ffmpeg, leaves nothing on disk, and the track is not decoded again at every airing.
func TestSubtitleBurns_ARefusingDecoderAirsWithout(t *testing.T) {
	decoder := &testkit.SubtitleDecoder{Err: playout.ErrSubtitleDecoderUnavailable}
	s := newTestBurns(t, true, playout.FFmpegBurnInWith(filepath.Join(t.TempDir(), "no-ffmpeg"), nil, decoder))
	resolve := s.For("/media/film.mkv", burnTestTracks, "eng")
	for range 3 {
		if got := resolve(encode.EncoderNVENC); got != nil {
			t.Fatalf("served %+v from a refused track", got)
		}
		waitIdle(t, s)
	}
	calls := decoder.Calls()
	if len(calls) != 1 || calls[0].Index != 1 || calls[0].Input != "/media/film.mkv" {
		t.Errorf("decoder asked for %+v, want track 1 of the film once", calls)
	}
	if entries, err := os.ReadDir(s.dir); err != nil || len(entries) != 0 {
		t.Errorf("left %v on disk (%v)", entries, err)
	}
}

// A replaced file is a new track: its stills are named by size and modification time too.
func TestSubtitleBurns_TrackDirFollowsTheFile(t *testing.T) {
	var p testkit.BurnInPreparer
	s := newTestBurns(t, true, p.Prepare)
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

// A preparation keeps the burnKeep most recently aired tracks: serving a track marks it aired, so
// the oldest unserved one goes, and a build in progress is not a track.
func TestSubtitleBurns_KeepsTheMostRecentlyAired(t *testing.T) {
	var seed, p testkit.BurnInPreparer
	s := newTestBurns(t, true, p.Prepare)
	input := func(i int) string { return fmt.Sprintf("/media/f%02d.mkv", i) }
	old := time.Now().Add(-time.Hour)
	for i := range burnKeep {
		dir := s.trackDir(input(i), 1)
		if _, err := seed.Prepare(t.Context(), input(i), 1, dir); err != nil {
			t.Fatal(err)
		}
		at := old.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(dir, at, at); err != nil {
			t.Fatal(err)
		}
	}
	building := filepath.Join(s.dir, "x.building")
	if err := os.MkdirAll(building, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(building, old.Add(-time.Hour), old.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	if s.For(input(0), burnTestTracks, "eng")(encode.EncoderNVENC) == nil {
		t.Fatal("a prepared track was not served")
	}
	s.Prepare("/media/new.mkv", burnTestTracks, "eng")
	waitIdle(t, s)
	if p.Runs() != 1 {
		t.Fatalf("prepared %d times, want the new track once", p.Runs())
	}

	exists := func(dir string) bool {
		_, err := os.Stat(dir)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		return err == nil
	}
	if exists(s.trackDir(input(1), 1)) {
		t.Error("the least recently aired track was kept")
	}
	for name, dir := range map[string]string{
		"the served track": s.trackDir(input(0), 1), "the next oldest": s.trackDir(input(2), 1),
		"the new track": s.trackDir("/media/new.mkv", 1), "the build": building,
	} {
		if !exists(dir) {
			t.Errorf("%s was evicted", name)
		}
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != burnKeep+1 {
		t.Errorf("%d entries on disk, want %d tracks and the build", len(entries), burnKeep)
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
