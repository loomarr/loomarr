package playoutcertfixture

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"
)

// ProgrammeEvidence supplies private predeclared truth without depending on the
// certification package or observing the decoder's output.
type ProgrammeEvidence[T any] struct {
	ManifestSHA256 string
	Private        []string
	Value          T
	Error          error
	Calls          atomic.Int32
}

func (s *ProgrammeEvidence[T]) PrivateInputs() []string { return append([]string(nil), s.Private...) }

func (s *ProgrammeEvidence[T]) CohortManifestSHA256() string { return s.ManifestSHA256 }

func (s *ProgrammeEvidence[T]) Freeze(string, time.Time, time.Time) (T, error) {
	s.Calls.Add(1)
	return s.Value, s.Error
}

// SignalPair is a single predeclared decoded observation. The transport fixture
// selects it by its byte index; an absent component emits no signal of that kind.
type SignalPair[V, A any] struct {
	Video *V
	Audio *A
}

// SignalDecoder consumes a scripted byte stream. The real FFmpeg tests own
// actual media decoding; this double exercises observer and input lifecycles.
type SignalDecoder[V, A any] struct {
	Signals      []SignalPair[V, A]
	Failure      error
	ReturnAfter  int
	ReturnOnCall int
	// BatchSizes bounds read-ahead before each group of callbacks. Unspecified
	// batches read one byte. EmitInterval independently paces decoded output.
	BatchSizes   []int
	EmitInterval time.Duration
	BeforeEmit   func(int)
	Started      atomic.Int32
	Stopped      atomic.Int32
}

func (d *SignalDecoder[V, A]) DecodeSignals(ctx context.Context, input io.ReadCloser, video func(V), audio func(A)) error {
	call := int(d.Started.Add(1))
	observed := 0
	defer d.Stopped.Add(1)
	defer func() { _ = input.Close() }()
	for batch := 0; ; batch++ {
		size := 1
		if batch < len(d.BatchSizes) {
			size = d.BatchSizes[batch]
		}
		if size < 1 || size > 256 {
			return errors.New("invalid scripted batch size")
		}
		buf := make([]byte, size)
		n, err := io.ReadFull(input, buf)
		for _, index := range buf[:n] {
			if d.EmitInterval > 0 {
				timer := time.NewTimer(d.EmitInterval)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
			if d.Failure != nil {
				return d.Failure
			}
			if int(index) >= len(d.Signals) {
				return errors.New("invalid scripted signal index")
			}
			if d.BeforeEmit != nil {
				d.BeforeEmit(int(index))
			}
			s := d.Signals[index]
			if s.Video != nil {
				video(*s.Video)
			}
			if s.Audio != nil {
				audio(*s.Audio)
			}
			observed++
			if d.ReturnAfter > 0 && observed >= d.ReturnAfter && (d.ReturnOnCall == 0 || d.ReturnOnCall == call) {
				return nil
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// PacedBody emits one byte per interval and can remain open after its final
// byte. Closing it always interrupts a blocked read; it owns no goroutine.
type PacedBody struct {
	Bytes      []byte
	Interval   time.Duration
	Hold       bool
	Terminal   error
	CloseError error
	Closed     atomic.Int32
	ReadEnd    chan<- struct{}
	closed     chan struct{}
	once       sync.Once
}

func NewPacedBody(data []byte, interval time.Duration, hold bool) *PacedBody {
	return &PacedBody{Bytes: append([]byte(nil), data...), Interval: interval, Hold: hold, closed: make(chan struct{})}
}

func (b *PacedBody) Read(out []byte) (int, error) {
	if len(out) == 0 {
		return 0, nil
	}
	if len(b.Bytes) == 0 {
		select {
		case b.ReadEnd <- struct{}{}:
		default:
		}
		if b.Hold {
			<-b.closed
			return 0, io.ErrClosedPipe
		}
		if b.Terminal != nil {
			return 0, b.Terminal
		}
		return 0, io.EOF
	}
	timer := time.NewTimer(b.Interval)
	defer timer.Stop()
	select {
	case <-b.closed:
		return 0, io.ErrClosedPipe
	case <-timer.C:
	}
	out[0], b.Bytes = b.Bytes[0], b.Bytes[1:]
	return 1, nil
}

func (b *PacedBody) Close() error {
	b.once.Do(func() { b.Closed.Add(1); close(b.closed) })
	return b.CloseError
}
