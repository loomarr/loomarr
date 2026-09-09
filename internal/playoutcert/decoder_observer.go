package playoutcert

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

type decoderSnapshot struct {
	frames                int64
	bytes, reads          int
	firstFrame, lastFrame time.Time
	lastRead              time.Time
	readErr, decoderErr   error
	decoderDone           bool
	capture               []byte
}

type decoderObserver struct {
	cancel context.CancelFunc
	input  *observedReadCloser

	mu                    sync.Mutex
	frames                int64
	firstFrame, lastFrame time.Time
	decoderErr            error
	decoderDone           bool
	notify                chan struct{}
	done                  chan struct{}
}

type observedReadCloser struct {
	source io.ReadCloser
	limit  int

	mu       sync.Mutex
	bytes    int
	reads    int
	lastRead time.Time
	readErr  error
	capture  []byte
	notify   chan struct{}
	once     sync.Once
}

func startDecoderObserver(ctx context.Context, decoder Decoder, input io.ReadCloser, captureLimit int) *decoderObserver {
	decodeCtx, cancel := context.WithCancel(ctx)
	reader := &observedReadCloser{source: input, limit: captureLimit, notify: make(chan struct{}, 1)}
	observer := &decoderObserver{cancel: cancel, input: reader, notify: make(chan struct{}, 1), done: make(chan struct{})}
	go func() {
		err := decoder.Decode(decodeCtx, reader, observer.recordFrames)
		observer.mu.Lock()
		observer.decoderErr = err
		observer.decoderDone = true
		observer.mu.Unlock()
		observer.signal()
		close(observer.done)
	}()
	return observer
}

func (r *observedReadCloser) Read(buffer []byte) (int, error) {
	n, err := r.source.Read(buffer)
	if n > 0 {
		r.mu.Lock()
		r.bytes += n
		r.reads++
		r.lastRead = time.Now()
		if len(r.capture) < r.limit {
			remaining := r.limit - len(r.capture)
			r.capture = append(r.capture, buffer[:min(n, remaining)]...)
		}
		r.mu.Unlock()
		r.signal()
	}
	if err != nil {
		r.mu.Lock()
		r.readErr = err
		r.mu.Unlock()
		r.signal()
	}
	return n, err
}

func (r *observedReadCloser) Close() error {
	var err error
	r.once.Do(func() { err = r.source.Close() })
	return err
}

func (r *observedReadCloser) signal() {
	select {
	case r.notify <- struct{}{}:
	default:
	}
}

func (o *decoderObserver) signal() {
	select {
	case o.notify <- struct{}{}:
	default:
	}
}

func (o *decoderObserver) recordFrames(frames int64) {
	if frames <= 0 {
		return
	}
	now := time.Now()
	o.mu.Lock()
	if frames > o.frames {
		o.frames = frames
		if o.firstFrame.IsZero() {
			o.firstFrame = now
		}
		o.lastFrame = now
	}
	o.mu.Unlock()
	o.signal()
}

func (o *decoderObserver) snapshot() decoderSnapshot {
	o.mu.Lock()
	snapshot := decoderSnapshot{frames: o.frames, firstFrame: o.firstFrame, lastFrame: o.lastFrame, decoderErr: o.decoderErr, decoderDone: o.decoderDone}
	o.mu.Unlock()
	o.input.mu.Lock()
	snapshot.bytes, snapshot.reads, snapshot.lastRead, snapshot.readErr = o.input.bytes, o.input.reads, o.input.lastRead, o.input.readErr
	snapshot.capture = append([]byte(nil), o.input.capture...)
	o.input.mu.Unlock()
	return snapshot
}

func (o *decoderObserver) wait(ctx context.Context, ready func(decoderSnapshot) bool) (decoderSnapshot, error) {
	for {
		snapshot := o.snapshot()
		if ready(snapshot) {
			return snapshot, nil
		}
		if snapshot.decoderDone {
			if snapshot.decoderErr != nil {
				return snapshot, snapshot.decoderErr
			}
			return snapshot, io.EOF
		}
		select {
		case <-ctx.Done():
			return snapshot, ctx.Err()
		case <-o.notify:
		case <-o.input.notify:
		}
	}
}

func (o *decoderObserver) close() error {
	o.cancel()
	closeErr := o.input.Close()
	<-o.done
	if closeErr != nil && !errors.Is(closeErr, context.Canceled) {
		return closeErr
	}
	return nil
}

type joinedReadCloser struct {
	io.Reader
	io.Closer
}
