package subtitles

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/media/bgexec"
)

// State is how far a track's cues are from being served.
type State int

const (
	// Pending: the track is queued or being read; ask again shortly.
	Pending State = iota
	// Ready: the cues are known (possibly none).
	Ready
	// Failed: the track could not be read. It is tried again after a while.
	Failed
)

const (
	// extractTimeout bounds one read. Demuxing a track reads the whole file, which on a large film
	// over a network share takes minutes.
	extractTimeout = 20 * time.Minute
	// retryAfter is how long a failed track is left before it is tried again.
	retryAfter = 10 * time.Minute
	// keptTracks is how many tracks' cues stay in memory. A film's track is ~100 KB of text.
	keptTracks = 48
	// queueDepth bounds the tracks waiting to be read; a full queue drops the request, which the
	// player's next ask makes again.
	queueDepth = 32
	// unusedFor is how long a kept file goes unread before it is removed: a replaced file's old
	// tracks, or a title no channel airs any more.
	unusedFor = 60 * 24 * time.Hour
)

// Reader reads one track as a WebVTT document.
type Reader func(ctx context.Context, src Source) ([]byte, error)

// Extractor reads text subtitle tracks once each and keeps their cues: in memory for the tracks
// in use, and as WebVTT files under its directory so a restart doesn't read them again. One worker
// reads one track at a time at background priority, so subtitles never compete with a channel's
// encode.
type Extractor struct {
	read Reader
	dir  string
	log  *slog.Logger
	now  func() time.Time

	queue chan Source

	mu     sync.Mutex
	kept   map[string]*keptTrack
	queued map[string]bool
}

type keptTrack struct {
	cues     []Cue
	failedAt time.Time
	used     time.Time
}

// NewExtractor reads tracks with ffmpeg at ffmpegPath, keeping them under dir ("" keeps them in
// memory only). Call Run to start its worker.
func NewExtractor(ffmpegPath, dir string, log *slog.Logger) *Extractor {
	return newExtractor(FFmpegReader(ffmpegPath), dir, log, time.Now)
}

func newExtractor(read Reader, dir string, log *slog.Logger, now func() time.Time) *Extractor {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Extractor{
		read: read, dir: dir, log: log, now: now,
		queue: make(chan Source, queueDepth), kept: map[string]*keptTrack{}, queued: map[string]bool{},
	}
}

// FFmpegReader reads a track as WebVTT with ffmpeg, which converts every text format it reads.
func FFmpegReader(ffmpegPath string) Reader {
	return func(ctx context.Context, src Source) ([]byte, error) {
		mapping := "0:s:0"
		if src.Stream >= 0 {
			mapping = fmt.Sprintf("0:%d", src.Stream)
		}
		out, err := bgexec.FFmpeg(ctx, ffmpegPath, "-nostdin", "-hide_banner", "-loglevel", "error",
			"-i", src.Input, "-map", mapping, "-c:s", "webvtt", "-f", "webvtt", "pipe:1").Output()
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) && len(exit.Stderr) > 0 {
				return nil, fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(exit.Stderr)))
			}
			return nil, fmt.Errorf("ffmpeg: %w", err)
		}
		return out, nil
	}
}

// Cues is a track's cues when they have been read. Otherwise it queues the track (once) and
// answers Pending, or Failed while a failed read waits to be tried again. It never blocks on a
// read.
func (e *Extractor) Cues(src Source) ([]Cue, State) {
	key := sourceKey(src)
	now := e.now()
	e.mu.Lock()
	if k, ok := e.kept[key]; ok {
		switch {
		case k.failedAt.IsZero():
			k.used = now
			e.mu.Unlock()
			return k.cues, Ready
		case now.Sub(k.failedAt) < retryAfter:
			e.mu.Unlock()
			return nil, Failed
		default:
			delete(e.kept, key)
		}
	}
	e.mu.Unlock()

	if cues, ok := e.fromDisk(key); ok {
		e.keep(key, &keptTrack{cues: cues, used: now})
		return cues, Ready
	}
	e.submit(key, src)
	return nil, Pending
}

func (e *Extractor) submit(key string, src Source) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.queued[key] {
		return
	}
	select {
	case e.queue <- src:
		e.queued[key] = true
	default:
		e.log.Debug("subtitles: read queue full; the track is read on a later ask", "input", src.Input)
	}
}

// Run reads queued tracks until ctx ends. It is the only worker. It first removes the kept files
// nothing has read for a while.
func (e *Extractor) Run(ctx context.Context) {
	e.prune()
	for {
		select {
		case <-ctx.Done():
			return
		case src := <-e.queue:
			e.extract(ctx, src)
		}
	}
}

func (e *Extractor) extract(ctx context.Context, src Source) {
	key := sourceKey(src)
	defer func() {
		e.mu.Lock()
		delete(e.queued, key)
		e.mu.Unlock()
	}()
	readCtx, cancel := context.WithTimeout(ctx, extractTimeout)
	defer cancel()
	doc, err := e.read(readCtx, src)
	var cues []Cue
	if err == nil {
		cues, err = ParseWebVTT(doc)
	}
	if err != nil {
		if ctx.Err() != nil {
			return // shutting down: not the track's fault
		}
		e.log.Warn("subtitles: track not read", "input", src.Input, "stream", src.Stream, "err", err)
		e.keep(key, &keptTrack{failedAt: e.now()})
		return
	}
	e.toDisk(key, doc)
	e.keep(key, &keptTrack{cues: cues, used: e.now()})
}

// keep stores a track, forgetting the least recently used one past keptTracks.
func (e *Extractor) keep(key string, k *keptTrack) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.kept[key] = k
	for len(e.kept) > keptTracks {
		oldest := ""
		for name, candidate := range e.kept {
			if oldest == "" || candidate.used.Before(e.kept[oldest].used) {
				oldest = name
			}
		}
		delete(e.kept, oldest)
	}
}

func (e *Extractor) fromDisk(key string) ([]Cue, bool) {
	if e.dir == "" {
		return nil, false
	}
	path := filepath.Join(e.dir, key+".vtt")
	doc, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	cues, err := ParseWebVTT(doc)
	if err != nil {
		return nil, false
	}
	// A read marks the file used, so prune keeps it.
	now := e.now()
	_ = os.Chtimes(path, now, now)
	return cues, true
}

// prune removes kept files unread for unusedFor.
func (e *Extractor) prune() {
	if e.dir == "" {
		return
	}
	entries, err := os.ReadDir(e.dir)
	if err != nil {
		return
	}
	cutoff := e.now().Add(-unusedFor)
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || entry.IsDir() || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(e.dir, entry.Name())); err != nil {
			e.log.Debug("subtitles: unused track not removed", "file", entry.Name(), "err", err)
		}
	}
}

// toDisk writes a track's WebVTT atomically, so a reader never sees half a file. A failed write
// only costs a read after the next restart.
func (e *Extractor) toDisk(key string, doc []byte) {
	if e.dir == "" {
		return
	}
	if err := os.MkdirAll(e.dir, 0o750); err != nil {
		e.log.Debug("subtitles: cache directory not made", "dir", e.dir, "err", err)
		return
	}
	tmp, err := os.CreateTemp(e.dir, key+".*.tmp")
	if err != nil {
		e.log.Debug("subtitles: track not cached", "err", err)
		return
	}
	_, werr := tmp.Write(doc)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), filepath.Join(e.dir, key+".vtt")) != nil {
		_ = os.Remove(tmp.Name())
		e.log.Debug("subtitles: track not cached", "write", werr, "close", cerr)
	}
}

// sourceKey names a track's revision: its file, stream and the file's stat.
func sourceKey(src Source) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%d\x00%d\x00%d", src.Input, src.Stream, src.Size, src.ModifiedNs))
	return hex.EncodeToString(sum[:16])
}
