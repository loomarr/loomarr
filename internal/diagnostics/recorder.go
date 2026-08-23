package diagnostics

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
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
	Now           func() time.Time
	OnFailure     func(error, int)
}

// Recorder accepts events without waiting for persistence and owns one batching worker.
type Recorder struct {
	sink Sink
	opts Options

	normal   chan Record
	priority chan Record
	stop     chan struct{}
	done     chan struct{}

	mu       sync.RWMutex
	closed   bool
	stopOnce sync.Once
	dropped  atomic.Uint64
}

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
	if r == nil || r.sink == nil {
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
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return opts
}

func newID(now time.Time) string {
	var random [12]byte
	if _, err := rand.Read(random[:]); err == nil {
		return "diag_" + hex.EncodeToString(random[:])
	}
	return fmt.Sprintf("diag_%d", now.UnixNano())
}
