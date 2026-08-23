package testkit

import (
	"context"
	"sync"

	"github.com/loomarr/loomarr/internal/playout"
)

// Playout is the shared in-memory double for the API's playback seam. It records per-channel
// lifecycle stops; tests concerned with tuning can provide TuneResult/TuneErr without creating a
// private playback implementation.
type Playout struct {
	mu sync.Mutex

	TuneResult   playout.Presentation
	TuneErr      error
	AdmissionErr error
	stopped      []string
	stoppedAll   int
}

func (p *Playout) Tune(context.Context, playout.TuneRequest) (playout.Presentation, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.TuneResult, p.TuneErr
}

func (*Playout) OpenAsset(context.Context, string, playout.EncodePlan, string) (playout.Asset, bool, error) {
	return playout.Asset{}, false, nil
}

// AcquireAdmission satisfies the API's canonical lifecycle admission seam.
func (p *Playout) AcquireAdmission(ctx context.Context, _ string) (playout.Admission, error) {
	return playout.Admission{Context: ctx}, p.AdmissionErr
}

func (p *Playout) StopChannel(channelID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = append(p.stopped, channelID)
}

// StopAll records a process-wide lifecycle stop.
func (p *Playout) StopAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stoppedAll++
}

// StoppedChannels returns a snapshot of the lifecycle stops received so far.
func (p *Playout) StoppedChannels() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.stopped...)
}

// Stopped reports whether StopChannel was called for channelID.
func (p *Playout) Stopped(channelID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, stopped := range p.stopped {
		if stopped == channelID {
			return true
		}
	}
	return false
}

// StopAllCalls reports the number of process-wide lifecycle stops.
func (p *Playout) StopAllCalls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stoppedAll
}

// TunerRescanner is the shared in-memory double for operation-specific media-server channel-list
// refreshes. Err injects a best-effort poke failure.
type TunerRescanner struct {
	mu    sync.Mutex
	Err   error
	calls int
}

func (r *TunerRescanner) RescanTuner(context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.Err
}

// Calls returns the number of tuner re-scans requested.
func (r *TunerRescanner) Calls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}
