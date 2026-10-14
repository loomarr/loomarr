package diagnostics

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultCapacity      = 1024
	defaultBatchSize     = 64
	defaultFlushInterval = 250 * time.Millisecond
	defaultWriteTimeout  = 5 * time.Second
)

// Sink is the persistence role the recorder needs. SQLite and Postgres satisfy it through the
// shared store implementation; tests use an in-memory adapter.
type Sink interface {
	AppendDiagnosticEvents(context.Context, []Record) error
}

// Options controls process-local buffering. These are service-protection constants, not operator
// settings: retention controls durable evidence while this queue controls how much memory
// diagnostics may occupy during a slow or unavailable store.
type Options struct {
	Capacity      int
	BatchSize     int
	FlushInterval time.Duration
	WriteTimeout  time.Duration
	// MinLevel defaults to info. A future bounded verbose-capture controller may construct a
	// recorder at debug without changing every producer or making permanent debug retention the
	// default.
	MinLevel  Level
	Now       func() time.Time
	OnFailure func(error, int)
}

// Recorder accepts events without waiting for persistence and owns one batching worker.
type Recorder struct {
	sink Sink
	opts Options

	normal   chan Record
	priority chan Record
	stop     chan struct{}
	done     chan struct{}

	mu        sync.RWMutex
	closed    bool
	stopOnce  sync.Once
	dropped   atomic.Uint64
	captureMu sync.Mutex
	capture   VerboseCapture
}

// VerboseCapture is the current bounded exception to the recorder's default info threshold.
type VerboseCapture struct {
	Active    bool   `json:"active"`
	StartedAt int64  `json:"startedAt,omitempty"`
	EndsAt    int64  `json:"endsAt,omitempty"`
	Subsystem string `json:"subsystem,omitempty"`
	ChannelID string `json:"channelId,omitempty"`
}

var ErrInvalidVerboseCapture = errors.New("invalid verbose capture")

// New builds and starts a recorder. A nil Sink yields a no-op recorder so store-less boot and
// narrow unit tests do not need guards at every call site.
func New(sink Sink, opts Options) *Recorder {
	opts = withDefaults(opts)
	priorityCapacity := max(1, opts.Capacity/4)
	normalCapacity := max(1, opts.Capacity-priorityCapacity)
	r := &Recorder{
		sink: sink, opts: opts,
		normal: make(chan Record, normalCapacity), priority: make(chan Record, priorityCapacity),
		stop: make(chan struct{}), done: make(chan struct{}),
	}
	if sink == nil {
		close(r.done)
		return r
	}
	go r.run()
	return r
}

// Record normalizes and redacts one event, then attempts a non-blocking enqueue. Warn/error have a
// reserved queue so routine info/debug traffic is discarded first under saturation.
func (r *Recorder) Record(_ context.Context, event Event) {
	if r == nil || r.sink == nil || !r.acceptsEvent(event) {
		return
	}
	record := normalize(event, r.opts.Now())

	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		r.dropped.Add(1)
		return
	}
	queue := r.normal
	if record.Level == LevelWarn || record.Level == LevelError {
		queue = r.priority
	}
	select {
	case queue <- record:
	default:
		r.dropped.Add(1)
	}
}

// RecordDurable normalizes and writes one event synchronously. It is reserved for state
// checkpoints whose contract says they survived before the caller can advertise completion;
// ordinary diagnostic traffic must continue through Record's bounded non-blocking queue.
func (r *Recorder) RecordDurable(ctx context.Context, event Event) error {
	if r == nil || r.sink == nil {
		return errors.New("diagnostics persistence is unavailable")
	}
	if !r.acceptsEvent(event) {
		return fmt.Errorf("diagnostics severity %q is filtered", event.Level)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return errors.New("diagnostics recorder is closed")
	}
	record := normalize(event, r.opts.Now())
	writeCtx, cancel := context.WithTimeout(ctx, r.opts.WriteTimeout)
	err := r.sink.AppendDiagnosticEvents(writeCtx, []Record{record})
	cancel()
	if err != nil && r.opts.OnFailure != nil {
		r.opts.OnFailure(err, 1)
	}
	return err
}

// Accepts reports whether a severity is eligible for durable recording. Filtering happens before
// normalization and queue admission, so intentionally excluded debug evidence is not reported as
// dropped evidence.
func (r *Recorder) Accepts(level Level) bool {
	if r == nil || r.sink == nil {
		return false
	}
	level = normalizeLevel(level)
	if levelRank(level) >= levelRank(r.opts.MinLevel) {
		return true
	}
	return level == LevelDebug && r.VerboseCapture().Active
}

func (r *Recorder) acceptsEvent(event Event) bool {
	level := normalizeLevel(event.Level)
	if levelRank(level) >= levelRank(r.opts.MinLevel) {
		return true
	}
	if level != LevelDebug {
		return false
	}
	capture := r.VerboseCapture()
	return capture.Active && (capture.Subsystem == "" || capture.Subsystem == event.Subsystem) &&
		(capture.ChannelID == "" || capture.ChannelID == event.ChannelID)
}

// StartVerboseCapture temporarily admits scoped debug evidence through the same bounded queue,
// redaction, and retention path as ordinary diagnostics. A later start replaces the prior window.
func (r *Recorder) StartVerboseCapture(duration time.Duration, subsystem, channelID string) (VerboseCapture, error) {
	if r == nil || r.sink == nil {
		return VerboseCapture{}, errors.New("diagnostics recorder unavailable")
	}
	if duration < time.Minute || duration > 15*time.Minute {
		return VerboseCapture{}, fmt.Errorf("%w: duration must be between 1 and 15 minutes", ErrInvalidVerboseCapture)
	}
	subsystem, channelID = strings.TrimSpace(subsystem), strings.TrimSpace(channelID)
	if len(subsystem) > 128 || len(channelID) > 128 {
		return VerboseCapture{}, fmt.Errorf("%w: scope cannot exceed 128 bytes", ErrInvalidVerboseCapture)
	}
	now := r.opts.Now()
	capture := VerboseCapture{
		Active: true, StartedAt: now.UnixMilli(), EndsAt: now.Add(duration).UnixMilli(),
		Subsystem: subsystem, ChannelID: channelID,
	}
	r.captureMu.Lock()
	r.capture = capture
	r.captureMu.Unlock()
	return capture, nil
}

// StopVerboseCapture returns the default durable threshold immediately.
func (r *Recorder) StopVerboseCapture() VerboseCapture {
	if r == nil {
		return VerboseCapture{}
	}
	r.captureMu.Lock()
	r.capture = VerboseCapture{}
	r.captureMu.Unlock()
	return VerboseCapture{}
}

// VerboseCapture reports live state and lazily expires elapsed windows.
func (r *Recorder) VerboseCapture() VerboseCapture {
	if r == nil {
		return VerboseCapture{}
	}
	r.captureMu.Lock()
	defer r.captureMu.Unlock()
	if r.capture.Active && r.opts.Now().UnixMilli() >= r.capture.EndsAt {
		r.capture = VerboseCapture{}
	}
	return r.capture
}

// Dropped returns how many records this process refused because its bounded queue was full or the
// recorder was already closing.
func (r *Recorder) Dropped() uint64 {
	if r == nil {
		return 0
	}
	return r.dropped.Load()
}

// Close stops admission, drains accepted records, and waits for a bounded flush. It is idempotent.
func (r *Recorder) Close(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.stopOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		close(r.stop)
		r.mu.Unlock()
	})
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Recorder) run() {
	defer close(r.done)
	ticker := time.NewTicker(r.opts.FlushInterval)
	defer ticker.Stop()
	batch := make([]Record, 0, r.opts.BatchSize)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), r.opts.WriteTimeout)
		err := r.sink.AppendDiagnosticEvents(ctx, batch)
		cancel()
		if err != nil && r.opts.OnFailure != nil {
			r.opts.OnFailure(err, len(batch))
		}
		batch = batch[:0]
	}
	appendRecord := func(record Record) {
		batch = append(batch, record)
		if len(batch) >= r.opts.BatchSize {
			flush()
		}
	}

	for {
		// Give reserved warn/error evidence first refusal on each loop without starving normal
		// traffic when no priority record is ready.
		select {
		case record := <-r.priority:
			appendRecord(record)
			continue
		default:
		}

		select {
		case record := <-r.priority:
			appendRecord(record)
		case record := <-r.normal:
			appendRecord(record)
		case <-ticker.C:
			flush()
		case <-r.stop:
			for {
				select {
				case record := <-r.priority:
					appendRecord(record)
					continue
				default:
				}
				select {
				case record := <-r.normal:
					appendRecord(record)
				default:
					flush()
					return
				}
			}
		}
	}
}

func withDefaults(opts Options) Options {
	if opts.Capacity <= 0 {
		opts.Capacity = defaultCapacity
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultBatchSize
	}
	if opts.FlushInterval <= 0 {
		opts.FlushInterval = defaultFlushInterval
	}
	if opts.WriteTimeout <= 0 {
		opts.WriteTimeout = defaultWriteTimeout
	}
	if opts.MinLevel == "" {
		opts.MinLevel = LevelInfo
	} else {
		opts.MinLevel = normalizeLevel(opts.MinLevel)
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return opts
}

func levelRank(level Level) int {
	switch level {
	case LevelDebug:
		return 0
	case LevelInfo:
		return 1
	case LevelWarn:
		return 2
	case LevelError:
		return 3
	default:
		return 1
	}
}

func newID(now time.Time) string {
	var random [12]byte
	if _, err := rand.Read(random[:]); err == nil {
		return "diag_" + hex.EncodeToString(random[:])
	}
	return fmt.Sprintf("diag_%d", now.UnixNano())
}
