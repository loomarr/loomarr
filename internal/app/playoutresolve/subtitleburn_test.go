package playoutresolve

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/timeline"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/execfixture"
)

// burnPGS is a film's English PGS track as the Library lists it.
var burnPGS = librarystore.LibraryStream{Index: 3, Kind: "subtitle", Codec: "hdmv_pgs_subtitle", Language: "en"}

func TestBurnTrack_IsTheChannelLanguagesImageTrack(t *testing.T) {
	streams := []librarystore.LibraryStream{
		{Index: 2, Kind: "subtitle", Codec: "subrip", Language: "en"},
		{Index: 3, Kind: "subtitle", Codec: "hdmv_pgs_subtitle", Language: "fr", ExternalRef: "/media/film.fr.sup"},
		{Index: 4, Kind: "subtitle", Codec: "hdmv_pgs_subtitle", Language: "en"},
		{Index: 5, Kind: "subtitle", Codec: "dvd_subtitle", Language: "es"},
	}
	if got, ok := burnTrack(streams, "eng"); !ok || got.Index != 4 || got.Codec != "hdmv_pgs_subtitle" {
		t.Errorf("eng: %+v %v, want the English PGS stream, not the text one", got, ok)
	}
	if _, ok := burnTrack(streams, "spa"); ok {
		t.Error("spa: VOBSUB is not burnable yet")
	}
	if _, ok := burnTrack(streams, "fre"); ok {
		t.Error("fre: a track beside the file is not a stream of it")
	}
	if _, ok := burnTrack(streams, ""); ok {
		t.Error("no language burns nothing")
	}
}

// ownedWork is the generation's on-demand starter for tests: it runs work under a context the
// test cancels and waits for.
type ownedWork struct {
	ctx context.Context
	wg  sync.WaitGroup
	mu  sync.Mutex
	n   int
}

func (o *ownedWork) Go(run func(context.Context)) bool {
	o.mu.Lock()
	o.n++
	o.mu.Unlock()
	o.wg.Go(func() { run(o.ctx) })
	return true
}

func (o *ownedWork) starts() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.n
}

// newTestBurns is a burn-in whose preparer runs until the test ends.
func newTestBurns(t *testing.T, works bool, prepare playout.BurnInPreparer) (*SubtitleBurns, *ownedWork) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	owned := &ownedWork{ctx: ctx}
	s := NewSubtitleBurns(SubtitleBurnsConfig{Dir: t.TempDir(), Prepare: prepare, Decodes: true,
		Works: func(encode.Encoder) bool { return works }, Kernel: func() string { return "/k.cl" },
		Go: owned.Go, Log: testkit.Logger()})
	owned.Go(s.Run)
	t.Cleanup(func() {
		cancel()
		owned.wg.Wait()
	})
	return s, owned
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
	s, _ := newTestBurns(t, true, p.Prepare)
	if got := s.serve("/media/film.mkv", burnPGS); got != nil {
		t.Fatalf("served %+v before it was prepared", got)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() { _ = s.serve("/media/film.mkv", burnPGS) })
	}
	wg.Wait()
	waitIdle(t, s)
	got := s.serve("/media/film.mkv", burnPGS)
	if got == nil || got.Track.Width != 1920 || got.Kernel != "/k.cl" {
		t.Fatalf("after preparing: %+v", got)
	}
	if p.Runs() != 1 {
		t.Errorf("prepared %d times, want once", p.Runs())
	}
}

// A track that fails is not retried at every airing, and is retried once its back-off has passed.
func TestSubtitleBurns_BacksOffAFailureThenRetries(t *testing.T) {
	var p testkit.BurnInPreparer
	p.SetFail(true)
	s, _ := newTestBurns(t, true, p.Prepare)
	s.Prepare("/media/film.mkv", burnPGS)
	waitIdle(t, s)
	s.Prepare("/media/film.mkv", burnPGS)
	if got := s.serve("/media/film.mkv", burnPGS); got != nil {
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
	if got := s.serve("/media/film.mkv", burnPGS); got != nil {
		t.Fatalf("served %+v before the retry ran", got)
	}
	waitIdle(t, s)
	if got := s.serve("/media/film.mkv", burnPGS); got == nil {
		t.Error("the retry after the back-off did not serve the track")
	}
	if p.Runs() != 2 {
		t.Errorf("prepared %d times, want the failure and one retry", p.Runs())
	}
}

// A decoder that refuses every distinct track holds at most burnFailedKeep back-offs, and a
// back-off that has passed is dropped.
func TestSubtitleBurns_BoundsTheFailures(t *testing.T) {
	var p testkit.BurnInPreparer
	p.SetFail(true)
	s := NewSubtitleBurns(SubtitleBurnsConfig{Dir: t.TempDir(), Prepare: p.Prepare, Log: testkit.Logger()})
	for i := range burnFailedKeep + 40 {
		s.do(t.Context(), s.job(fmt.Sprintf("/media/f%03d.mkv", i), burnPGS))
		s.mu.Lock()
		n := len(s.failed)
		s.mu.Unlock()
		if n > burnFailedKeep {
			t.Fatalf("%d back-offs held after %d failures, want at most %d", n, i+1, burnFailedKeep)
		}
	}
	s.mu.Lock()
	for dir := range s.failed {
		s.failed[dir] = time.Now().Add(-burnRetry - time.Minute)
	}
	s.mu.Unlock()
	s.enqueue(s.job("/media/f000.mkv", burnPGS))
	s.mu.Lock()
	_, kept := s.failed[s.trackDir("/media/f000.mkv", burnPGS.Index)]
	s.mu.Unlock()
	if kept {
		t.Error("a back-off that has passed was kept")
	}
}

// Run returns only once the preparation in flight has: Shutdown, joining Run, waits for it.
func TestSubtitleBurns_RunWaitsForARunningPreparation(t *testing.T) {
	var p testkit.BurnInPreparer
	p.SetHold(true)
	s := NewSubtitleBurns(SubtitleBurnsConfig{Dir: t.TempDir(), Prepare: p.Prepare, Log: testkit.Logger()})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Run(ctx)
	}()
	s.Prepare("/media/film.mkv", burnPGS)
	deadline := time.Now().Add(5 * time.Second)
	for p.Runs() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the preparation never started")
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	<-done
	if p.Finished() != 1 {
		t.Errorf("Run returned with %d of 1 preparations finished", p.Finished())
	}
}

// The wired decoder refuses until the image worker's adapter lands: the preparer copies the track
// out, the decoder refuses it once, nothing is left on disk, and it is not decoded again at every
// airing.
func TestSubtitleBurns_ARefusingDecoderAirsWithout(t *testing.T) {
	decoder := &testkit.SubtitleDecoder{Err: playout.ErrSubtitleDecoderUnavailable}
	ffmpeg := execfixture.POSIX(t, "ffmpeg", `printf 'PGtrack'`)
	s, _ := newTestBurns(t, true, playout.FFmpegBurnInWith(ffmpeg, nil, decoder))
	for range 3 {
		if got := s.serve("/media/film.mkv", burnPGS); got != nil {
			t.Fatalf("served %+v from a refused track", got)
		}
		waitIdle(t, s)
	}
	calls := decoder.Calls()
	if len(calls) != 1 || calls[0].Codec != "hdmv_pgs_subtitle" || calls[0].SHA256 == "" {
		t.Errorf("decoder asked for %+v, want the film's PGS track once", calls)
	}
	if entries, err := os.ReadDir(s.dir); err != nil || len(entries) != 0 {
		t.Errorf("left %v on disk (%v)", entries, err)
	}
}

// A replaced file is a new track: its stills are named by size and modification time too.
func TestSubtitleBurns_TrackDirFollowsTheFile(t *testing.T) {
	var p testkit.BurnInPreparer
	s, _ := newTestBurns(t, true, p.Prepare)
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
	s, _ := newTestBurns(t, true, p.Prepare)
	input := func(i int) string { return fmt.Sprintf("/media/f%02d.mkv", i) }
	old := time.Now().Add(-time.Hour)
	for i := range burnKeep {
		dir := s.trackDir(input(i), burnPGS.Index)
		if _, err := seed.Prepare(t.Context(), input(i), burnPGS.Index, burnPGS.Codec, dir); err != nil {
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

	if s.serve(input(0), burnPGS) == nil {
		t.Fatal("a prepared track was not served")
	}
	s.Prepare("/media/new.mkv", burnPGS)
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
	if exists(s.trackDir(input(1), burnPGS.Index)) {
		t.Error("the least recently aired track was kept")
	}
	for name, dir := range map[string]string{
		"the served track": s.trackDir(input(0), burnPGS.Index), "the next oldest": s.trackDir(input(2), burnPGS.Index),
		"the new track": s.trackDir("/media/new.mkv", burnPGS.Index), "the build": building,
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
		t.Errorf("%d entries on disk, want %d tracks and the build", len(entries), burnKeep+1)
	}
}

// countedTracks is the Library's subtitle inventory, counting each file it is asked about.
type countedTracks struct {
	tracks fakeSubtitleTracks
	mu     sync.Mutex
	asked  []string
}

func (c *countedTracks) LibrarySubtitleTracks(ctx context.Context, path string, size, mod int64) ([]librarystore.LibraryStream, bool, error) {
	c.mu.Lock()
	c.asked = append(c.asked, path)
	c.mu.Unlock()
	return c.tracks.LibrarySubtitleTracks(ctx, path, size, mod)
}

func (c *countedTracks) reads() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.asked...)
}

// burnResolver is a channel that burns English image subtitles in, airing a film then a second
// programme, each with an English PGS stream in the Library's inventory. A probe fails the test.
func burnResolver(t *testing.T, works, decodes bool) (*Resolver, *SubtitleBurns, *testkit.BurnInPreparer, *ownedWork, *countedTracks) {
	t.Helper()
	slots := []schedule.Slot{
		{Kind: schedule.SlotProgram, Title: "film", LibraryItemID: "film", Key: "film", DurationMs: 60_000},
		{Kind: schedule.SlotProgram, Title: "next", LibraryItemID: "next", Key: "next", DurationMs: 60_000},
	}
	r := stillResolver(t, slots, testPlayoutAnchor(), nil)
	r.channels.(*stubChannels).set(func(c *channelstore.Channel) {
		c.Policy.Playout = &schedule.PlayoutPolicy{BurnSubtitles: "eng"}
	})
	r.probeTracks = func(context.Context, string) (playout.MediaTracks, error) {
		t.Error("burn-in probed a source; its tracks come from the Library")
		return playout.MediaTracks{}, errors.New("no probe")
	}
	files := r.libraryFile.(fakeLibraryFiles)
	inventory := &countedTracks{tracks: fakeSubtitleTracks{
		files["film"]: {{Index: 2, Kind: "subtitle", Codec: "subrip", Language: "en"}, burnPGS},
		files["next"]: {{Index: 5, Kind: "subtitle", Codec: "hdmv_pgs_subtitle", Language: "en"}},
	}}
	r.SetSubtitles(inventory, fakeCues{})
	p := &testkit.BurnInPreparer{}
	s, owned := newTestBurns(t, works, p.Prepare)
	s.decodes = decodes
	r.SetSubtitleBurns(s)
	return r, s, p, owned, inventory
}

// A programme on a burning channel serves its own track once prepared, and queues the NEXT
// airing's as it starts, so the following programme has it ready; neither is probed.
func TestBurnSubtitlesFor_ServesTheProgrammeAndQueuesTheNextAiring(t *testing.T) {
	r, s, p, owned, inventory := burnResolver(t, true, true)
	film := r.libraryFile.(fakeLibraryFiles)["film"]
	at := testPlayoutAnchor().Add(30 * time.Second)
	airing := timeline.Airing{Kind: schedule.SlotProgram, Remaining: 30 * time.Second}
	resolve := r.BurnSubtitlesFor(t.Context(), "ch1", airing, at, film)
	if resolve == nil {
		t.Fatal("no resolver for a programme on a burning channel")
	}
	if got := resolve(encode.EncoderNVENC); got != nil {
		t.Fatalf("served %+v before it was prepared", got)
	}
	if owned.starts() != 2 {
		t.Errorf("%d owned starts, want the worker and the next airing's lookup", owned.starts())
	}
	deadline := time.Now().Add(5 * time.Second)
	for p.Finished() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("%d of 2 tracks prepared, want the film's and the next airing's", p.Finished())
		}
		time.Sleep(2 * time.Millisecond)
	}
	waitIdle(t, s)
	next := r.libraryFile.(fakeLibraryFiles)["next"]
	if _, ok := playout.LoadBurnIn(s.trackDir(next, 5)); !ok {
		t.Error("the next airing's track was not prepared")
	}
	if got := resolve(encode.EncoderNVENC); got == nil || got.Track == nil {
		t.Error("the prepared film track was not served")
	}
	reads := inventory.reads()
	if len(reads) == 0 || reads[0] != film {
		t.Errorf("inventory read %v, want the film first", reads)
	}
}

// Filler and a source not read directly (a URL) are never looked up or prepared, though the next
// airing still is; an encoder whose overlay failed, or a build with no decoder, reads nothing.
func TestBurnSubtitlesFor_ExcludesWhatCannotBurn(t *testing.T) {
	at := testPlayoutAnchor().Add(30 * time.Second)
	program := timeline.Airing{Kind: schedule.SlotProgram, Remaining: 30 * time.Second}

	for name, c := range map[string]struct {
		airing timeline.Airing
		input  string
	}{
		"filler":   {timeline.Airing{Kind: schedule.SlotFiller, Source: "clip", Remaining: 30 * time.Second}, "/media/clip.mp4"},
		"a URL":    {program, "http://emby.local/stream.mkv"},
		"relative": {program, "film.mkv"},
	} {
		r, s, p, owned, inventory := burnResolver(t, true, true)
		resolve := r.BurnSubtitlesFor(t.Context(), "ch1", c.airing, at, c.input)
		if resolve == nil {
			t.Fatalf("%s: no resolver; the next airing would never be queued", name)
		}
		if got := resolve(encode.EncoderNVENC); got != nil {
			t.Errorf("%s: served %+v", name, got)
		}
		deadline := time.Now().Add(5 * time.Second)
		for p.Finished() < 1 {
			if time.Now().After(deadline) {
				t.Fatalf("%s: the next airing was not prepared", name)
			}
			time.Sleep(2 * time.Millisecond)
		}
		waitIdle(t, s)
		for _, path := range inventory.reads() {
			if path == c.input {
				t.Errorf("%s: looked up %s", name, path)
			}
		}
		if p.Runs() != 1 || owned.starts() != 2 {
			t.Errorf("%s: %d preparations and %d owned starts, want only the next airing's", name, p.Runs(), owned.starts())
		}
	}

	film := func(r *Resolver) string { return r.libraryFile.(fakeLibraryFiles)["film"] }
	r, _, p, owned, inventory := burnResolver(t, false, true)
	if got := r.BurnSubtitlesFor(t.Context(), "ch1", program, at, film(r))(encode.EncoderVAAPI); got != nil {
		t.Errorf("served %+v where the overlay failed its self-check", got)
	}
	if n := len(inventory.reads()); n != 0 || owned.starts() != 1 || p.Runs() != 0 {
		t.Errorf("a failed overlay read the inventory %d times, started %d, prepared %d", n, owned.starts()-1, p.Runs())
	}
	r, _, _, _, inventory = burnResolver(t, true, false)
	if r.BurnSubtitlesFor(t.Context(), "ch1", program, at, film(r)) != nil || len(inventory.reads()) != 0 {
		t.Error("a build with no decoder resolved or read the inventory")
	}
}

// playoutSafePath mirrors the graph builder's rule for paths it embeds without escaping.
func playoutSafePath(p string) bool {
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '/', r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}
