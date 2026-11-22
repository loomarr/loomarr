package subtitles

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/platform/langcode"
)

// The Auto-generated track (#4530, docs/design/playout.md "Transcription is on demand"): blocks
// of a media file's audio on a grid counted from the file's start, transcribed only while a
// viewer asks for them.
const (
	// LeadLength is the first block at a viewer's position: short, and always local, so the
	// first cue lands in seconds (#4529 measured a 30 s first block at 13–14 s to the first cue).
	LeadLength = 8 * time.Second
	// BlockLength is a grid block. The grid starts at the media file's start, so a rerun, a
	// repeat airing or a second viewer reuses every block already made.
	BlockLength = 30 * time.Second
	// blocksAhead is how many blocks past the one holding the window's end the queue runs.
	blocksAhead = 2
	// askedWithin is how long a request keeps its blocks wanted: one block's length. With no
	// request for that long the queue makes nothing more for the track.
	askedWithin = BlockLength
	// silentShare is the share of a cue inside detected silence at which it is dropped: Whisper
	// invents lines such as "Thank you." over silence and music.
	silentShare = 0.8
	// blockTimeout bounds one block's transcription on either engine.
	blockTimeout = 2 * time.Minute
	// failedRetry is how long a block whose transcription failed is left before it is tried again.
	failedRetry = time.Minute
	// idleTrack is how long a track nobody asks for keeps its blocks in memory. Its blocks stay on
	// disk.
	idleTrack = 10 * time.Minute
)

// AutoSuffix ends every Auto-generated track's choice key.
const AutoSuffix = "-auto"

// AutoKey is the Auto-generated track's choice key: "<language>-auto", "und-auto" until the
// language is known.
func AutoKey(language string) string { return KeyOf(language, false, false) + AutoSuffix }

// IsAutoKey reports whether a choice key names an Auto-generated track.
func IsAutoKey(key string) bool { return strings.HasSuffix(key, AutoSuffix) }

// Speech is one engine's transcription of a span: its cues relative to the span's start, and the
// language it heard ("" when it could not tell).
type Speech struct {
	Language string
	Cues     []Cue
}

// Transcriber turns a span of a file's audio into speech. An empty language asks it to detect
// the language; otherwise it transcribes in that language.
type Transcriber interface {
	Transcribe(ctx context.Context, file string, start, end time.Duration, language string) (Speech, error)
}

// Span is a stretch of time relative to a block's start.
type Span struct{ Start, End time.Duration }

// SilenceDetector reports the silent spans of [start,end) of a file's audio, relative to start:
// silencedetect at about -50 dB, or a voice-activity detector.
type SilenceDetector interface {
	Silence(ctx context.Context, file string, start, end time.Duration) ([]Span, error)
}

// Budget admits local transcription as filler media work (playout.ResourceBudget): the context
// is born ended when playout has no headroom, and ends when a live admission leaves none.
type Budget interface {
	MediaWorkContext(ctx context.Context, overdue bool) (context.Context, context.CancelFunc)
}

// Engines are what the Generator transcribes with.
type Engines struct {
	// Local is whisper-cli with the shipped multilingual tiny model. Every lead block runs on it,
	// and every block the hosted service does not take.
	Local Transcriber
	// Hosted is the hosted speech service while one is configured (asr.provider=hosted), else
	// nil. It is read once per block, so a settings change applies to the next block.
	Hosted func() Transcriber
	// Silence finds the silence a cue must not sit in; nil drops no cue.
	Silence SilenceDetector
	// Budget admits local work; without one, local work is refused.
	Budget Budget
}

// Media is the file an Auto-generated track transcribes, with its stat as the cache key.
type Media struct {
	Input            string
	Size, ModifiedNs int64
	// Duration is the file's length; zero when unknown, and then no block is clipped to it.
	Duration time.Duration
}

// Generator makes Auto-generated tracks' cues on demand. Cues records what a viewer asks for and
// answers from what exists; one worker (Run) transcribes the most urgent wanted block at a time,
// so concurrent viewers queue behind each other and a block is transcribed once. Blocks are kept
// in memory while their track is in use and as WebVTT under the Generator's directory.
type Generator struct {
	eng Engines
	dir string
	log *slog.Logger
	now func() time.Time
	// timeout bounds one block's transcription on each engine, and its silence measure.
	timeout time.Duration

	wake chan struct{}

	mu     sync.Mutex
	tracks map[string]*autoTrack
}

type autoTrack struct {
	key   string
	media Media
	// asked is the latest request and asks counts them. refusedAt is the count when the budget
	// last refused this track's local work: until a later request no block starts on the local
	// engine, while grid blocks still go to a configured hosted service.
	asked           time.Time
	asks, refusedAt int
	// language is detected on the first block with speech and kept.
	language string
	blocks   map[int]*autoBlock
	leads    []*autoBlock
}

// autoBlock is one grid block (index ≥ 0) or a lead block (index -1).
type autoBlock struct {
	index      int
	start, end time.Duration
	// wanted is the latest request that needs the block; rank is its distance in blocks from
	// that request's position, so the nearest is made first.
	wanted time.Time
	rank   int

	probed  bool // the disk has been asked for this block
	running bool
	// refusedAt is the track's request count when the budget refused this block's local
	// fallback; the block waits for a later request.
	refusedAt int
	ready     bool
	failedAt  time.Time
	cues      []Cue
}

// NewGenerator transcribes with eng, keeping blocks under dir ("" keeps them in memory only).
// Call Run to start its worker.
func NewGenerator(eng Engines, dir string, log *slog.Logger) *Generator {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Generator{
		eng: eng, dir: dir, log: log, now: time.Now, timeout: blockTimeout,
		wake: make(chan struct{}, 1), tracks: map[string]*autoTrack{},
	}
}

// Cues is the Auto-generated track's cues over [from,to) of m's own clock, the language detected
// so far ("" until a block with speech exists) and whether every block in the window exists:
// Pending while one is still to be made, when the player asks again. A lead block's cues stand in
// for a grid block still being made. Failed means no engine is configured. Cues never blocks on a
// transcription; it records the request, which keeps the window and the blocks ahead of it wanted.
func (g *Generator) Cues(m Media, from, to time.Duration) ([]Cue, string, State) {
	if g.eng.Local == nil && g.hosted() == nil {
		return nil, "", Failed
	}
	key := mediaKey(m)
	now := g.now()

	g.mu.Lock()
	duration := m.Duration
	known, ok := g.tracks[key]
	if ok && duration == 0 {
		duration = known.media.Duration
	}
	from = max(from, 0)
	if duration > 0 {
		to = min(to, duration)
	}
	// An empty window, or one past the file's end, wants nothing.
	if to <= from {
		language := ""
		if ok {
			language = known.language
		}
		g.mu.Unlock()
		return []Cue{}, language, Ready
	}
	t := g.trackLocked(key, m, now)
	first, last := blockIndex(from), blockIndex(to-1)
	ahead := last + blocksAhead
	if duration > 0 {
		ahead = min(ahead, blockIndex(duration-1))
	}
	var probe []*autoBlock
	for i := first; i <= ahead; i++ {
		if b := t.blockLocked(i); !b.probed && !b.ready && !b.running {
			probe = append(probe, b)
		}
	}
	g.mu.Unlock()

	for _, b := range probe {
		cues, language, ok := g.fromDisk(key, b.index)
		g.mu.Lock()
		b.probed = true
		if ok && !b.ready {
			b.ready, b.cues = true, cues
			if t.language == "" && language != "" {
				t.language = language
			}
		}
		g.mu.Unlock()
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	t.asked = now
	t.asks++
	// A block is missing when a concurrent request learned the file's length and forgot the
	// blocks past its end: there is nothing to want or show there.
	for i := first; i <= ahead; i++ {
		b := t.blocks[i]
		if b == nil {
			continue
		}
		if now.Sub(b.wanted) > askedWithin || i-first < b.rank {
			b.rank = i - first
		}
		b.wanted = now
	}
	if b := t.blocks[first]; b != nil && !b.ready {
		t.leadAtLocked(from).wanted = now
	}
	g.signal()

	cues := []Cue{}
	state := Ready
	for i := first; i <= last; i++ {
		b := t.blocks[i]
		if b == nil {
			continue
		}
		span := Span{Start: max(b.start, from), End: min(b.end, to)}
		if b.ready {
			cues = append(cues, overlapping(b.cues, span)...)
			continue
		}
		state = Pending
		for _, lead := range t.leads {
			if lead.ready {
				cues = append(cues, startingIn(lead.cues, span)...)
			}
		}
	}
	return cues, t.language, state
}

// trackLocked is m's track, made on the first request, after forgetting tracks idle too long. A
// track made while the file's length was unknown takes it from the first request that knows it.
func (g *Generator) trackLocked(key string, m Media, now time.Time) *autoTrack {
	for k, t := range g.tracks {
		if k != key && now.Sub(t.asked) > idleTrack {
			delete(g.tracks, k)
		}
	}
	t, ok := g.tracks[key]
	if !ok {
		t = &autoTrack{key: key, media: m, blocks: map[int]*autoBlock{}}
		g.tracks[key] = t
	} else if t.media.Duration == 0 && m.Duration > 0 {
		t.learnDurationLocked(m.Duration)
	}
	return t
}

// learnDurationLocked clips the blocks still to be made to the file's end, and forgets those that
// start past it.
func (t *autoTrack) learnDurationLocked(d time.Duration) {
	t.media.Duration = d
	for i, b := range t.blocks {
		switch {
		case b.running || b.ready:
		case b.start >= d:
			delete(t.blocks, i)
		default:
			b.end = min(b.end, d)
		}
	}
	for _, lead := range t.leads {
		if !lead.running && !lead.ready {
			lead.end = min(lead.end, d)
		}
	}
}

func (t *autoTrack) blockLocked(i int) *autoBlock {
	b, ok := t.blocks[i]
	if !ok {
		start := time.Duration(i) * BlockLength
		end := start + BlockLength
		if t.media.Duration > 0 {
			end = min(end, t.media.Duration)
		}
		b = &autoBlock{index: i, start: start, end: end}
		t.blocks[i] = b
	}
	return b
}

// leadAtLocked is the lead block covering pos, made when none does. It first forgets the leads
// far from pos that are not running; the filter runs over every lead before one is returned, so
// each lead is kept exactly once.
func (t *autoTrack) leadAtLocked(pos time.Duration) *autoBlock {
	var found *autoBlock
	kept := t.leads[:0]
	for _, lead := range t.leads {
		covers := lead.start <= pos && pos < lead.end
		if covers && found == nil {
			found = lead
		}
		if covers || lead.running || pos-lead.end < askedWithin && lead.start-pos < askedWithin {
			kept = append(kept, lead)
		}
	}
	clear(t.leads[len(kept):])
	t.leads = kept
	if found != nil {
		return found
	}
	end := pos + LeadLength
	if t.media.Duration > 0 {
		end = min(end, t.media.Duration)
	}
	lead := &autoBlock{index: -1, start: pos, end: end, probed: true}
	t.leads = append(t.leads, lead)
	return lead
}

func (g *Generator) signal() {
	select {
	case g.wake <- struct{}{}:
	default:
	}
}

func (g *Generator) hosted() Transcriber {
	if g.eng.Hosted == nil {
		return nil
	}
	return g.eng.Hosted()
}

// Run transcribes wanted blocks until ctx ends. It is the only worker. It first removes the kept
// files nothing has read for a while.
func (g *Generator) Run(ctx context.Context) {
	pruneDir(g.dir, g.now().Add(-unusedFor), g.log)
	for ctx.Err() == nil {
		if g.step(ctx) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-g.wake:
		}
	}
}

// job is one block to transcribe.
type job struct {
	track    *autoTrack
	block    *autoBlock
	language string
}

// step transcribes the most urgent wanted block, if any, and reports whether it ran one.
func (g *Generator) step(ctx context.Context) bool {
	j, ok := g.next()
	if !ok {
		return false
	}
	g.run(ctx, j)
	return true
}

// next picks the most urgent block and marks it running: a lead block first, then the wanted
// grid block nearest its request's position. A track nobody asked for within one block's length
// is passed over. Once the budget has refused a track's local work, until its next request only
// grid blocks are offered, and only while a hosted service is configured to take them.
func (g *Generator) next() (job, bool) {
	now := g.now()
	hostedOn := g.hosted() != nil
	g.mu.Lock()
	defer g.mu.Unlock()
	var best job
	bestRank := 0
	consider := func(t *autoTrack, b *autoBlock, rank int) {
		if b.ready || b.running || !b.probed || now.Sub(b.wanted) > askedWithin ||
			(!b.failedAt.IsZero() && now.Sub(b.failedAt) < failedRetry) {
			return
		}
		better := best.block == nil || rank < bestRank ||
			(rank == bestRank && (t.key < best.track.key || (t.key == best.track.key && b.start < best.block.start)))
		if better {
			best, bestRank = job{track: t, block: b}, rank
		}
	}
	for _, t := range g.tracks {
		refused := t.refusedAt == t.asks
		if now.Sub(t.asked) > askedWithin || refused && !hostedOn {
			continue
		}
		for _, lead := range t.leads {
			if grid, ok := t.blocks[blockIndex(lead.start)]; refused || ok && grid.ready {
				continue
			}
			consider(t, lead, -1)
		}
		for _, b := range t.blocks {
			if b.refusedAt != t.asks {
				consider(t, b, b.rank)
			}
		}
	}
	if best.block == nil {
		return job{}, false
	}
	best.block.running = true
	best.language = best.track.language
	return best, true
}

// errNoLocal is a block the hosted service did not take, with no local engine to fall back to.
var errNoLocal = errors.New("no local transcriber is configured")

func (g *Generator) run(ctx context.Context, j job) {
	t, b := j.track, j.block
	speech, refused, err := g.transcribe(ctx, j)
	var cues []Cue
	if err == nil && !refused {
		cues = g.placed(ctx, t.media.Input, b, speech.Cues)
	}

	g.mu.Lock()
	b.running = false
	switch {
	case ctx.Err() != nil: // shutting down: not the block's fault
		g.mu.Unlock()
		return
	case refused:
		t.refusedAt, b.refusedAt = t.asks, t.asks
		g.mu.Unlock()
		g.log.Debug("subtitles: no headroom for local transcription; the block waits for the next request",
			"input", t.media.Input, "start", b.start)
		return
	case err != nil:
		b.failedAt = g.now()
		g.mu.Unlock()
		g.log.Warn("subtitles: block not transcribed", "input", t.media.Input, "start", b.start, "err", err)
		return
	}
	b.ready, b.cues = true, cues
	if t.language == "" && len(cues) > 0 {
		t.language = languageCode(speech.Language)
	}
	language := t.language
	g.mu.Unlock()
	if b.index >= 0 {
		g.toDisk(t.key, b.index, cues, language)
	}
}

// transcribe runs a block on the hosted service when one is configured and takes it, and
// otherwise on the local engine under the budget. A hosted failure (a busy service's 429
// included) falls back for this block only and is never retried. A hosted call that runs out its
// time is a failure of the block, which then waits failedRetry, rather than a local fallback: a
// hung service would otherwise hold the single worker for both engines' time on every block. A
// lead block is always local. Each engine has its own timeout. refused reports that the budget
// had no headroom, or took it back mid-block.
func (g *Generator) transcribe(ctx context.Context, j job) (speech Speech, refused bool, err error) {
	b, file := j.block, j.track.media.Input
	if b.index >= 0 {
		if hosted := g.hosted(); hosted != nil {
			hctx, cancel := context.WithTimeout(ctx, g.timeout)
			speech, err := hosted.Transcribe(hctx, file, b.start, b.end, j.language)
			timedOut := hctx.Err() != nil
			cancel()
			if err == nil {
				return speech, false, nil
			}
			if ctx.Err() != nil {
				return Speech{}, false, err
			}
			if timedOut {
				return Speech{}, false, fmt.Errorf("the hosted speech service did not answer within %v: %w", g.timeout, err)
			}
			g.log.Info("subtitles: the hosted speech service did not take a block; it is transcribed locally",
				"input", file, "start", b.start, "err", err)
		}
	}
	if g.eng.Local == nil {
		return Speech{}, false, errNoLocal
	}
	if g.eng.Budget == nil {
		return Speech{}, true, nil
	}
	lctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()
	wctx, release := g.eng.Budget.MediaWorkContext(lctx, false)
	defer release()
	// The budget's refusal is its context ending while the one it was given is still live; an
	// ended lctx is this block's timeout or shutdown, never the budget's.
	yielded := func() bool { return wctx.Err() != nil && lctx.Err() == nil }
	if yielded() {
		return Speech{}, true, nil
	}
	speech, err = g.eng.Local.Transcribe(wctx, file, b.start, b.end, j.language)
	if err != nil && yielded() {
		return Speech{}, true, nil
	}
	return speech, false, err
}

// placed puts a block's cues on the media's clock, clipped to the block, without those that sit
// mostly in silence. A silence measure that fails drops nothing.
func (g *Generator) placed(ctx context.Context, file string, b *autoBlock, cues []Cue) []Cue {
	var silence []Span
	if g.eng.Silence != nil {
		sctx, cancel := context.WithTimeout(ctx, g.timeout)
		spans, err := g.eng.Silence.Silence(sctx, file, b.start, b.end)
		cancel()
		if err != nil {
			g.log.Debug("subtitles: silence not measured; no cue is dropped", "input", file, "start", b.start, "err", err)
		}
		silence = spans
	}
	length := b.end - b.start
	out := []Cue{}
	for _, c := range cues {
		start, end := max(c.Start, 0), min(c.End, length)
		if end <= start || strings.TrimSpace(c.Text) == "" || mostlySilent(start, end, silence) {
			continue
		}
		out = append(out, Cue{Start: b.start + start, End: b.start + end, Text: c.Text})
	}
	return out
}

// mostlySilent reports whether at least silentShare of [start,end) lies inside silence.
func mostlySilent(start, end time.Duration, silence []Span) bool {
	var inside time.Duration
	for _, s := range silence {
		if lo, hi := max(start, s.Start), min(end, s.End); hi > lo {
			inside += hi - lo
		}
	}
	return float64(inside) >= silentShare*float64(end-start)
}

// languageCode is a reported language as a key's language: whisper answers a code, an
// OpenAI-compatible service a name ("english") or a region tag, and langcode maps each onto one
// code. Anything it cannot place (an engine's "auto", an unknown name) is "", which leaves the
// language to be detected on a later block.
func languageCode(language string) string {
	if !langcode.Valid(language) {
		return ""
	}
	return langcode.Normalize(language)
}

func blockIndex(at time.Duration) int { return int(max(at, 0) / BlockLength) }

// overlapping is the cues that show during span.
func overlapping(cues []Cue, span Span) []Cue {
	var out []Cue
	for _, c := range cues {
		if c.End > span.Start && c.Start < span.End {
			out = append(out, c)
		}
	}
	return out
}

// startingIn is the cues that begin in span, so a lead cue is counted under one block only.
func startingIn(cues []Cue, span Span) []Cue {
	var out []Cue
	for _, c := range cues {
		if c.Start >= span.Start && c.Start < span.End {
			out = append(out, c)
		}
	}
	return out
}

// mediaKey names a file's revision for its Auto-generated blocks: its path and stat.
func mediaKey(m Media) string {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00auto\x00%d\x00%d", m.Input, m.Size, m.ModifiedNs))
	return hex.EncodeToString(sum[:16])
}

func (g *Generator) blockPath(key string, index int) string {
	return filepath.Join(g.dir, key+"-"+strconv.Itoa(index)+".vtt")
}

func (g *Generator) fromDisk(key string, index int) ([]Cue, string, bool) {
	if g.dir == "" {
		return nil, "", false
	}
	path := g.blockPath(key, index)
	doc, err := os.ReadFile(path)
	if err != nil {
		return nil, "", false
	}
	cues, err := ParseWebVTT(doc)
	if err != nil {
		return nil, "", false
	}
	// A read marks the file used, so prune keeps it.
	now := g.now()
	_ = os.Chtimes(path, now, now)
	return cues, headerLanguage(doc), true
}

func (g *Generator) toDisk(key string, index int, cues []Cue, language string) {
	if g.dir != "" {
		writeAtomic(g.dir, key+"-"+strconv.Itoa(index), FormatWebVTT(cues, language), g.log)
	}
}
