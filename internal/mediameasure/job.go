package mediameasure

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/inventory"
)

// DefaultTimeout bounds one source's whole analysis (keyframe index plus decode pass).
const DefaultTimeout = 45 * time.Minute

const queueDepth = 1024

// SourceRef names one source revision to measure. Path is the file Loomarr reads directly (through
// library.path_map and the path cache); the media server is never consulted.
type SourceRef struct {
	ID       inventory.SourceID
	Revision string
	Path     string
	// Facts are the stream facts when the caller already has them (the first-play fast path); the
	// job probes them itself when empty.
	Facts inventory.SourceFacts
}

// Sink is where measurements land: the inventory service for stream facts and the analysis store
// for the rest.
type Sink interface {
	RecordMeasurement(context.Context, inventory.Measurement) error
	RecordInventoryAnalysis(context.Context, inventory.Analysis) error
	InventoryAnalysis(context.Context, inventory.SourceID) (inventory.Analysis, bool, error)
}

// Deps are the Measurer's collaborators.
type Deps struct {
	Sink  Sink
	Tools Tools
	// Facts probes a file's stream facts with Loomarr's own ffprobe.
	Facts func(ctx context.Context, path string) (inventory.SourceFacts, error)
	// Revision returns the file's current revision string, in the same form inventory stores it, so
	// a file replaced while it is being measured is noticed.
	Revision func(path string) (string, error)
	Now      func() time.Time
	Timeout  time.Duration
	// Sampling bounds the reads; the zero value means DefaultSampling.
	Sampling Sampling
	Log      *slog.Logger
}

// Measurer measures sources in the background, one at a time, and measures stream facts
// synchronously for a source that is about to air for the first time.
type Measurer struct {
	deps    Deps
	queue   chan SourceRef
	pending sync.WaitGroup
	mu      sync.Mutex
	queued  map[string]bool
}

// New returns an idle Measurer; call Run to start its worker.
func New(deps Deps) *Measurer {
	if deps.Timeout <= 0 {
		deps.Timeout = DefaultTimeout
	}
	if deps.Sampling == (Sampling{}) {
		deps.Sampling = DefaultSampling()
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	return &Measurer{deps: deps, queue: make(chan SourceRef, queueDepth), queued: map[string]bool{}}
}

// MeasureFacts is the first-play fast path: it probes stream facts now and records them against
// the source revision, so the tune that asked (and every later one) builds its command from stored
// facts. It does no keyframe or decode work; Submit follows for that.
func (m *Measurer) MeasureFacts(ctx context.Context, ref SourceRef) (inventory.SourceFacts, error) {
	facts, err := m.deps.Facts(ctx, ref.Path)
	if err != nil {
		return inventory.SourceFacts{}, fmt.Errorf("measure stream facts: %w", err)
	}
	audio := inventory.CoverageEmpty
	for _, s := range facts.Streams {
		if s.Kind == inventory.StreamAudio {
			audio = inventory.CoveragePresent
			break
		}
	}
	err = m.deps.Sink.RecordMeasurement(ctx, inventory.Measurement{
		SourceID: ref.ID, Revision: ref.Revision,
		Observation: inventory.Observation[inventory.SourceFacts]{
			SchemaVersion: 1, ObservedAt: m.deps.Now(),
			Coverage: map[string]inventory.Coverage{"streams": inventory.CoveragePresent, "audioStreams": audio},
			Facts:    facts,
		},
	})
	if err != nil && !errors.Is(err, inventory.ErrSourceRevisionGone) {
		m.log("stream facts measured but not stored", err)
	}
	return facts, nil
}

// Submit queues a source revision for analysis. It never blocks: a full queue drops the request
// (the source is measured the next time it is submitted), and a revision already queued is not
// queued twice.
func (m *Measurer) Submit(ref SourceRef) {
	key := string(ref.ID) + "@" + ref.Revision
	m.mu.Lock()
	if m.queued[key] {
		m.mu.Unlock()
		return
	}
	m.queued[key] = true
	m.mu.Unlock()
	m.pending.Add(1)
	select {
	case m.queue <- ref:
	default:
		m.finish(key)
		m.log("measurement queue full, source will be measured on a later play", nil)
	}
}

func (m *Measurer) finish(key string) {
	m.mu.Lock()
	delete(m.queued, key)
	m.mu.Unlock()
	m.pending.Done()
}

// Drain blocks until everything submitted so far has been processed. Tests use it.
func (m *Measurer) Drain() { m.pending.Wait() }

// Run processes the queue until ctx ends. It is the only worker, so at most one source (and one
// external tool) is being measured at any time.
func (m *Measurer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case ref := <-m.queue:
			if err := m.analyse(ctx, ref); err != nil && ctx.Err() == nil {
				m.log("source analysis failed", err, "source", string(ref.ID))
			}
			m.finish(string(ref.ID) + "@" + ref.Revision)
		}
	}
}

func (m *Measurer) analyse(ctx context.Context, ref SourceRef) error {
	if existing, ok, err := m.deps.Sink.InventoryAnalysis(ctx, ref.ID); err == nil && ok && existing.Revision == ref.Revision {
		return nil
	}
	if err := m.unchanged(ref); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, m.deps.Timeout)
	defer cancel()

	facts := ref.Facts
	if len(facts.Streams) == 0 || facts.DurationMillis <= 0 {
		var err error
		if facts, err = m.deps.Facts(ctx, ref.Path); err != nil {
			return fmt.Errorf("probe stream facts: %w", err)
		}
	}
	hasVideo, hasAudio := false, false
	for _, s := range facts.Streams {
		hasVideo = hasVideo || s.Kind == inventory.StreamVideo
		hasAudio = hasAudio || s.Kind == inventory.StreamAudio
	}
	if facts.DurationMillis <= 0 || (!hasVideo && !hasAudio) {
		return errors.New("source has no measurable duration or streams")
	}

	var keyframes []inventory.Keyframe
	if hasVideo {
		var err error
		if keyframes, err = m.deps.Tools.Keyframes(ctx, ref.Path); err != nil {
			return err
		}
	}
	info, err := os.Stat(ref.Path)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	var lufs, peak *float64
	if hasAudio {
		if lufs, peak, err = m.deps.Tools.SampleLoudness(ctx, ref.Path, facts.DurationMillis, info.Size(), m.deps.Sampling); err != nil {
			return err
		}
	}
	// Chapters are free metadata; only a file without them pays for targeted windows.
	breaks, err := m.deps.Tools.ChapterBreaks(ctx, ref.Path, facts.DurationMillis, keyframes)
	if err != nil {
		return err
	}
	if len(breaks) == 0 && hasAudio && hasVideo {
		if breaks, err = m.deps.Tools.TargetedBreaks(ctx, ref.Path, facts.DurationMillis, info.Size(), keyframes, m.deps.Sampling); err != nil {
			return err
		}
	}
	// The file may have been replaced while it was read; a measurement of the old bytes must not
	// be filed under the new revision, or under any revision.
	if err := m.unchanged(ref); err != nil {
		return err
	}
	analysis := inventory.Analysis{
		SourceID: ref.ID, Revision: ref.Revision, Keyframes: keyframes, AnalyzedAt: m.deps.Now(),
		IntegratedLUFS: lufs, TruePeakDBTP: peak, Breaks: breaks,
	}
	if err := m.deps.Sink.RecordInventoryAnalysis(ctx, analysis); err != nil && !errors.Is(err, inventory.ErrSourceRevisionGone) {
		return fmt.Errorf("store analysis: %w", err)
	}
	return nil
}

var errChanged = errors.New("source changed during measurement")

func (m *Measurer) unchanged(ref SourceRef) error {
	if m.deps.Revision == nil {
		return nil
	}
	current, err := m.deps.Revision(ref.Path)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}
	if current != ref.Revision {
		return errChanged
	}
	return nil
}

func (m *Measurer) log(msg string, err error, args ...any) {
	if m.deps.Log == nil {
		return
	}
	if err != nil {
		args = append(args, "err", err)
	}
	m.deps.Log.Debug("mediameasure: "+msg, args...)
}
