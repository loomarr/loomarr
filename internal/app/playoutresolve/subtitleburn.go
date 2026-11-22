package playoutresolve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/timeline"
)

// SubtitleBurns is the image-subtitle burn-in (#2088 step 8): which of a programme's tracks a
// channel burns in, and the tracks prepared on disk for the packager (playout/burnin.go).
//
// Preparing a track reads its whole source once, so it never runs on the tune path: a programme
// whose track is not ready airs without it while one background worker prepares it, and the next
// programme's track is queued as the current one starts, so a channel's following airing has it.
type SubtitleBurns struct {
	dir     string
	prepare playout.BurnInPreparer
	// works reports whether an encoder's GPU overlay passed its self-check (the bug's gate).
	works  func(encode.Encoder) bool
	kernel func() string
	log    *slog.Logger

	jobs   chan burnJob
	mu     sync.Mutex
	queued map[string]bool
	failed map[string]time.Time
	// tracks holds each probed source's subtitle tracks by its identity (trackID), so an airing
	// probes its programme once.
	tracks map[string][]playout.Track
}

type burnJob struct {
	input string
	track int
	dir   string
}

const (
	// burnKeep is how many prepared tracks stay on disk (tens of MB each for a feature); the least
	// recently aired goes first.
	burnKeep = 48
	// burnRetry is how long a track that failed to prepare is left alone.
	burnRetry = time.Hour
	// burnQueue bounds the waiting preparations; a full queue drops the request (the next airing
	// asks again).
	burnQueue    = 16
	burnPrepTime = 20 * time.Minute
	// burnProbeKeep bounds the probed-tracks memo; a full memo starts again.
	burnProbeKeep = 512
)

// NewSubtitleBurns starts the background preparer, which stops with ctx. Prepared tracks live in
// dir; works is the encoder's GPU overlay gate and kernel the VAAPI blend kernel's path.
func NewSubtitleBurns(ctx context.Context, dir string, prepare playout.BurnInPreparer,
	works func(encode.Encoder) bool, kernel func() string, log *slog.Logger,
) *SubtitleBurns {
	s := &SubtitleBurns{dir: dir, prepare: prepare, works: works, kernel: kernel, log: log,
		jobs: make(chan burnJob, burnQueue), queued: map[string]bool{}, failed: map[string]time.Time{},
		tracks: map[string][]playout.Track{}}
	go s.run(ctx)
	return s
}

// SetSubtitleBurns attaches the burn-in and the prober of a programme's tracks. Until it is set,
// nothing is burned in.
func (r *Resolver) SetSubtitleBurns(burns *SubtitleBurns) { r.burns = burns }

// burnTrack is the image track a channel burns in: the first burnable track in its language.
func burnTrack(tracks []playout.Track, language string) (playout.Track, bool) {
	if language == "" {
		return playout.Track{}, false
	}
	for _, t := range tracks {
		if t.Language == language && playout.IsBurnableSubtitle(t.Codec) {
			return t, true
		}
	}
	return playout.Track{}, false
}

// trackID names a source by its path, size and modification time, so a replaced file is new.
func trackID(input string) string {
	if fi, err := os.Stat(input); err == nil {
		return fmt.Sprintf("%s\x00%d\x00%d", input, fi.Size(), fi.ModTime().UnixNano())
	}
	return input
}

// trackDir names a track's prepared stills by its source's identity and track.
func (s *SubtitleBurns) trackDir(input string, track int) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%d", trackID(input), track))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:12]))
}

// For is the packager's resolver for a programme with this track, or nil when it has none to burn.
func (s *SubtitleBurns) For(input string, tracks []playout.Track, language string) playout.SubtitlesFor {
	t, ok := burnTrack(tracks, language)
	if !ok {
		return nil
	}
	job := burnJob{input: input, track: t.Index, dir: s.trackDir(input, t.Index)}
	return func(enc encode.Encoder) *playout.ItemSubtitles {
		if !s.works(enc) {
			return nil
		}
		b, ok := playout.LoadBurnIn(job.dir)
		if !ok {
			s.enqueue(job)
			return nil
		}
		now := time.Now()
		_ = os.Chtimes(job.dir, now, now) // most recently aired: evicted last
		return &playout.ItemSubtitles{Track: b, Kernel: s.kernel()}
	}
}

// Prepare queues a programme's track ahead of its airing.
func (s *SubtitleBurns) Prepare(input string, tracks []playout.Track, language string) {
	if t, ok := burnTrack(tracks, language); ok {
		dir := s.trackDir(input, t.Index)
		if _, ready := playout.LoadBurnIn(dir); !ready {
			s.enqueue(burnJob{input: input, track: t.Index, dir: dir})
		}
	}
}

func (s *SubtitleBurns) enqueue(job burnJob) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.queued[job.dir] || time.Since(s.failed[job.dir]) < burnRetry {
		return
	}
	select {
	case s.jobs <- job:
		s.queued[job.dir] = true
	default:
		s.log.Debug("subtitle burn-in: queue full; the track is asked for again at its next airing", "track", job.track)
	}
}

func (s *SubtitleBurns) run(ctx context.Context) {
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
	b, err := s.prepare(ctx, job.input, job.track, job.dir)
	if err != nil {
		s.mu.Lock()
		s.failed[job.dir] = time.Now()
		s.mu.Unlock()
		s.log.Warn("subtitle burn-in: track not prepared; its programmes air without it",
			"track", job.track, "retry_in", burnRetry, "err", err)
		return
	}
	s.log.Info("subtitle burn-in: track prepared", "track", job.track, "cues", len(b.Cues),
		"took_ms", time.Since(start).Milliseconds())
	s.evict()
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

// BurnSubtitlesFor is a programme item's image subtitles: the channel's burn-in language, the
// programme's tracks (one probe per source), and the prepared track. It queues the NEXT airing's
// track in the background, so a channel's following programme is ready when it starts. nil when
// the channel burns none in or the programme has no such track. Only a source read directly (a
// local file) is prepared: preparing reads all of it.
func (r *Resolver) BurnSubtitlesFor(ctx context.Context, channelID string, airing timeline.Airing, at time.Time, input string) playout.SubtitlesFor {
	if r.burns == nil || r.channels == nil || r.probeTracks == nil {
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
	go r.prepareBurnAt(context.WithoutCancel(ctx), channelID, at.Add(airing.Remaining), language)
	if airing.Kind != schedule.SlotProgram || airing.Source != "" || !filepath.IsAbs(input) {
		return nil
	}
	tracks, ok := r.burnTracks(ctx, input)
	if !ok {
		return nil
	}
	return r.burns.For(input, tracks, language)
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
	if input == "" {
		return
	}
	if tracks, ok := r.burnTracks(ctx, input); ok {
		r.burns.Prepare(input, tracks, language)
	}
}

// burnTracks is a local source's subtitle tracks, probed once per source identity.
func (r *Resolver) burnTracks(ctx context.Context, input string) ([]playout.Track, bool) {
	s := r.burns
	id := trackID(input)
	s.mu.Lock()
	tracks, ok := s.tracks[id]
	s.mu.Unlock()
	if ok {
		return tracks, true
	}
	all, err := r.probeTracks(ctx, input)
	if err != nil {
		s.log.Debug("subtitle burn-in: tracks not probed", "err", err)
		return nil, false
	}
	s.mu.Lock()
	if len(s.tracks) >= burnProbeKeep {
		s.tracks = map[string][]playout.Track{}
	}
	s.tracks[id] = all.Subtitles
	s.mu.Unlock()
	return all.Subtitles, true
}
