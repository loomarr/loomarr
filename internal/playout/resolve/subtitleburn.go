package resolve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/platform/langcode"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/timeline"
)

// SubtitleBurns is the image-subtitle burn-in (#2088 step 8): which of a programme's tracks a
// channel burns in, and the tracks prepared on disk for the packager (playout/burnin.go).
//
// Preparing a track reads its whole source once, so it never runs on the tune path: a programme
// whose track is not ready airs without it while one background worker prepares it, and the next
// programme's track is queued as the current one starts, so a channel's following airing has it.
// A programme's tracks come from the Library's inventory, never a probe.
type SubtitleBurns struct {
	dir     string
	prepare playout.BurnInPreparer
	decodes bool
	// works reports whether an encoder's GPU overlay passed its self-check (the bug's gate).
	works   func(encode.Encoder) bool
	kernel  func() string
	goOwned func(func(context.Context)) bool
	log     *slog.Logger

	jobs   chan burnJob
	mu     sync.Mutex
	queued map[string]bool
	// failed holds when each refused track failed, for its back-off; at most burnFailedKeep.
	failed map[string]time.Time
}

// SubtitleBurnsConfig wires SubtitleBurns.
type SubtitleBurnsConfig struct {
	// Dir holds the prepared tracks.
	Dir     string
	Prepare playout.BurnInPreparer
	// Decodes reports whether this build has an image subtitle decoder
	// (playout.SubtitleDecoderAvailable). Without one nothing is looked up or queued.
	Decodes bool
	// Works is the encoder's GPU overlay gate and Kernel the VAAPI blend kernel's path.
	Works  func(encode.Encoder) bool
	Kernel func() string
	// Go starts per-item work the application generation owns, so Shutdown joins it; it reports
	// false once the generation is closing.
	Go  func(func(context.Context)) bool
	Log *slog.Logger
}

type burnJob struct {
	input  string
	stream int
	codec  string
	dir    string
}

const (
	// burnKeep is how many prepared tracks stay on disk (tens of MB each for a feature); the least
	// recently aired goes first.
	burnKeep = 48
	// burnRetry is how long a track that failed to prepare is left alone.
	burnRetry = time.Hour
	// burnFailedKeep bounds the failures held for their back-off; a full set starts again.
	burnFailedKeep = 256
	// burnQueue bounds the waiting preparations; a full queue drops the request (the next airing
	// asks again).
	burnQueue    = 16
	burnPrepTime = 20 * time.Minute
	// burnLookupTime bounds one inventory read of a programme's tracks.
	burnLookupTime = 5 * time.Second
)

// NewSubtitleBurns is the burn-in; its owner runs Run, the background preparer.
func NewSubtitleBurns(cfg SubtitleBurnsConfig) *SubtitleBurns {
	return &SubtitleBurns{dir: cfg.Dir, prepare: cfg.Prepare, decodes: cfg.Decodes, works: cfg.Works,
		kernel: cfg.Kernel, goOwned: cfg.Go, log: cfg.Log,
		jobs: make(chan burnJob, burnQueue), queued: map[string]bool{}, failed: map[string]time.Time{}}
}

// burnTrack is the stream a channel burns in: the first image subtitle track inside the file
// (not one beside it) in its language, however the file and the channel spell it.
func burnTrack(streams []librarystore.LibraryStream, language string) (librarystore.LibraryStream, bool) {
	want := langcode.Normalize(language)
	if want == "" {
		return librarystore.LibraryStream{}, false
	}
	for _, s := range streams {
		if s.ExternalRef == "" && playout.IsBurnableSubtitle(s.Codec) && langcode.Normalize(s.Language) == want {
			return s, true
		}
	}
	return librarystore.LibraryStream{}, false
}

// trackID names a source by its path, size and modification time, so a replaced file is new.
func trackID(input string) string {
	if fi, err := os.Stat(input); err == nil {
		return fmt.Sprintf("%s\x00%d\x00%d", input, fi.Size(), fi.ModTime().UnixNano())
	}
	return input
}

// trackDir names a track's prepared stills by its source's identity and stream.
func (s *SubtitleBurns) trackDir(input string, stream int) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%d", trackID(input), stream))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:12]))
}

func (s *SubtitleBurns) job(input string, t librarystore.LibraryStream) burnJob {
	return burnJob{input: input, stream: t.Index, codec: t.Codec, dir: s.trackDir(input, t.Index)}
}

// serve is stream t of input prepared for burning in, or nil, having queued it, when it is not
// ready yet.
func (s *SubtitleBurns) serve(input string, t librarystore.LibraryStream) *playout.ItemSubtitles {
	job := s.job(input, t)
	b, ok := playout.LoadBurnIn(job.dir)
	if !ok {
		s.enqueue(job)
		return nil
	}
	now := time.Now()
	_ = os.Chtimes(job.dir, now, now) // most recently aired: evicted last
	return &playout.ItemSubtitles{Track: b, Kernel: s.kernel()}
}

// Prepare queues stream t of input ahead of its airing.
func (s *SubtitleBurns) Prepare(input string, t librarystore.LibraryStream) {
	job := s.job(input, t)
	if _, ready := playout.LoadBurnIn(job.dir); !ready {
		s.enqueue(job)
	}
}

func (s *SubtitleBurns) enqueue(job burnJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if at, ok := s.failed[job.dir]; ok {
		if time.Since(at) < burnRetry {
			return
		}
		delete(s.failed, job.dir)
	}
	if s.queued[job.dir] {
		return
	}
	select {
	case s.jobs <- job:
		s.queued[job.dir] = true
	default:
		s.log.Debug("subtitle burn-in: queue full; the track is asked for again at its next airing", "stream", job.stream)
	}
}

// Run prepares queued tracks one at a time until ctx ends; a preparation in flight is cancelled
// with it, and Run returns once it has.
func (s *SubtitleBurns) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-s.jobs:
			s.do(ctx, job)
		}
	}
}

func (s *SubtitleBurns) do(ctx context.Context, job burnJob) {
	defer func() {
		s.mu.Lock()
		delete(s.queued, job.dir)
		s.mu.Unlock()
	}()
	if _, ready := playout.LoadBurnIn(job.dir); ready {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, burnPrepTime)
	defer cancel()
	start := time.Now()
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		s.log.Warn("subtitle burn-in: no directory for prepared tracks", "dir", s.dir, "err", err)
		return
	}
	b, err := s.prepare(ctx, job.input, job.stream, job.codec, job.dir)
	if errors.Is(err, playout.ErrSubtitleDecoderUnavailable) {
		// The image worker is bound moments after the playout build; a track asked for before then
		// is asked for again by its next programme rather than held back for burnRetry.
		s.log.Info("subtitle burn-in: no decoder yet; the track is prepared on its next request", "stream", job.stream)
		return
	}
	if err != nil {
		s.fail(job.dir, time.Now())
		s.log.Warn("subtitle burn-in: track not prepared; its programmes air without it",
			"stream", job.stream, "retry_in", burnRetry, "err", err)
		return
	}
	s.log.Info("subtitle burn-in: track prepared", "stream", job.stream, "cues", len(b.Cues),
		"took_ms", time.Since(start).Milliseconds())
	s.evict()
}

// fail starts dir's back-off, dropping back-offs that have passed; a set still full of live ones
// starts again, so a decoder refusing every distinct track holds at most burnFailedKeep.
func (s *SubtitleBurns) fail(dir string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.failed) >= burnFailedKeep {
		for d, at := range s.failed {
			if now.Sub(at) >= burnRetry {
				delete(s.failed, d)
			}
		}
		if len(s.failed) >= burnFailedKeep {
			clear(s.failed)
		}
	}
	s.failed[dir] = now
}

// evict keeps the burnKeep most recently aired tracks.
func (s *SubtitleBurns) evict() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	type track struct {
		path string
		at   time.Time
	}
	var tracks []track
	for _, e := range entries {
		if !e.IsDir() || strings.HasSuffix(e.Name(), ".building") {
			continue
		}
		if fi, err := e.Info(); err == nil {
			tracks = append(tracks, track{filepath.Join(s.dir, e.Name()), fi.ModTime()})
		}
	}
	if len(tracks) <= burnKeep {
		return
	}
	sort.Slice(tracks, func(i, j int) bool { return tracks[i].at.After(tracks[j].at) })
	for _, t := range tracks[burnKeep:] {
		_ = os.RemoveAll(t.path)
	}
}

// BurnSubtitlesFor is a programme item's image subtitles: nil when this build has no decoder or
// the channel burns none in, and otherwise the packager's resolver. The resolver asks the
// encoder's GPU overlay gate first, so a host that cannot draw the track reads nothing; past it, it
// queues the NEXT airing's track in the background (so a channel's following programme is ready
// when it starts) and serves this programme's track from the Library's inventory. Only a source
// read directly (a local file) is prepared: preparing reads all of it.
func (r *Resolver) BurnSubtitlesFor(ctx context.Context, channelID string, airing timeline.Airing, at time.Time, input string) playout.SubtitlesFor {
	s := r.burns
	if s == nil || !s.decodes || s.goOwned == nil || r.channels == nil || r.librarySubtitles == nil {
		return nil
	}
	ch, err := r.channels.GetChannel(ctx, channelID)
	if err != nil {
		return nil
	}
	language := schedule.BurnSubtitlesLanguage(ch.Policy)
	if language == "" {
		return nil
	}
	next := at.Add(airing.Remaining)
	local := airing.Kind == schedule.SlotProgram && airing.Source == "" && filepath.IsAbs(input)
	lookup := context.WithoutCancel(ctx)
	return func(enc encode.Encoder) *playout.ItemSubtitles {
		if !s.works(enc) {
			return nil
		}
		s.goOwned(func(ctx context.Context) { r.prepareBurnAt(ctx, channelID, next, language) })
		if !local {
			return nil
		}
		ctx, cancel := context.WithTimeout(lookup, burnLookupTime)
		defer cancel()
		t, ok := r.burnStream(ctx, input, language)
		if !ok {
			return nil
		}
		return s.serve(input, t)
	}
}

// prepareBurnAt queues the track of the programme airing at `at`, read from the accepted lineup
// without AiringAt's side effects.
func (r *Resolver) prepareBurnAt(ctx context.Context, channelID string, at time.Time, language string) {
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	slots, epoch, err := r.acceptedCycle(ctx, channelID, at)
	if err != nil {
		return
	}
	airing := timeline.AiringAt(slots, epoch, at)
	if airing.Kind != schedule.SlotProgram || airing.Source != "" || airing.LibraryItemID == "" {
		return
	}
	input := r.programmeInput(ctx, airing.LibraryItemID, airing.Offset).Path
	if !filepath.IsAbs(input) {
		return
	}
	if t, ok := r.burnStream(ctx, input, language); ok {
		r.burns.Prepare(input, t)
	}
}

// burnStream is the stream of a local source a channel burns in, as the Library's inventory
// lists the file's subtitle tracks; none when the inventory does not describe the file as it is.
func (r *Resolver) burnStream(ctx context.Context, input, language string) (librarystore.LibraryStream, bool) {
	info, err := os.Stat(input)
	if err != nil {
		return librarystore.LibraryStream{}, false
	}
	streams, ok, err := r.librarySubtitles.LibrarySubtitleTracks(ctx, input, info.Size(), info.ModTime().UnixNano())
	if err != nil || !ok {
		if err != nil {
			r.burns.log.Debug("subtitle burn-in: tracks unavailable", "err", err)
		}
		return librarystore.LibraryStream{}, false
	}
	return burnTrack(streams, language)
}
