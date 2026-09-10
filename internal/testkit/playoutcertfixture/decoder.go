package playoutcertfixture

import (
	"context"
	"errors"
	"io"
	"sync"
)

// Decoder is a shared streaming decoder double. It consumes its input until
// cancellation and reports one cumulative decoded-frame advance per read.
type Decoder struct {
	Fail                bool
	FrameLimit          int64
	FailAfterFrameLimit bool
	FailAfter           <-chan struct{}
	CallCompleted       chan<- int
	FirstFrameGate      <-chan struct{}
	FirstFrameGateCall  int
	FirstFrameWaiting   chan<- int
	FirstFrameFailCall  int

	mu      sync.Mutex
	active  int
	started int
	stopped int
}

func (d *Decoder) Decode(ctx context.Context, input io.ReadCloser, reportFrames func(int64)) error {
	d.mu.Lock()
	d.active++
	d.started++
	call := d.started
	d.mu.Unlock()
	defer func() {
		_ = input.Close()
		d.mu.Lock()
		d.active--
		d.stopped++
		d.mu.Unlock()
		if d.CallCompleted != nil {
			select {
			case d.CallCompleted <- call:
			default:
			}
		}
	}()
	if d.Fail {
		return errors.New("controlled decode failure")
	}
	buffer := make([]byte, 188)
	var frames int64
	var observed int64
	for {
		n, err := input.Read(buffer)
		if n > 0 {
			if d.FailAfter != nil {
				select {
				case <-d.FailAfter:
					return errors.New("controlled post-event decode failure")
				default:
				}
			}
			observed += int64(n)
			available := observed / 188
			for frames < available && (d.FrameLimit == 0 || frames < d.FrameLimit) {
				if frames == 0 && d.FirstFrameGate != nil && (d.FirstFrameGateCall == 0 || d.FirstFrameGateCall == call) {
					if d.FirstFrameWaiting != nil {
						select {
						case d.FirstFrameWaiting <- call:
						default:
						}
					}
					select {
					case <-d.FirstFrameGate:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				if frames == 0 && d.FirstFrameFailCall == call {
					return errors.New("controlled first-frame failure")
				}
				frames++
				reportFrames(frames)
			}
			if d.FrameLimit > 0 && available > d.FrameLimit && d.FailAfterFrameLimit {
				return errors.New("controlled post-frame decode failure")
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

func (d *Decoder) Counts() (active, started, stopped int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.active, d.started, d.stopped
}
