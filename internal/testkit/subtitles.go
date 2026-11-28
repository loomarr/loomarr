package testkit

import (
	"context"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/playout/admission"
	"github.com/loomarr/loomarr/internal/playout/subtitles"
)

// SpeechCall is one transcription a SpeechEngine was asked for.
type SpeechCall struct {
	File       string
	Start, End time.Duration
	Language   string
}

// SpeechEngine is the shared double for subtitles.Transcriber. Respond answers each call (nil
// answers no speech); Hold, when set, makes every call wait until it is closed or the call's
// context ends, so a test can catch a transcription in flight.
type SpeechEngine struct {
	Respond func(SpeechCall) (subtitles.Speech, error)
	Hold    chan struct{}

	mu    sync.Mutex
	calls []SpeechCall
}

func (s *SpeechEngine) Transcribe(ctx context.Context, file string, start, end time.Duration, language string) (subtitles.Speech, error) {
	call := SpeechCall{File: file, Start: start, End: end, Language: language}
	s.mu.Lock()
	s.calls = append(s.calls, call)
	respond, hold := s.Respond, s.Hold
	s.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return subtitles.Speech{}, ctx.Err()
		}
	}
	if respond == nil {
		return subtitles.Speech{}, nil
	}
	return respond(call)
}

// Calls is every transcription asked for, in order.
func (s *SpeechEngine) Calls() []SpeechCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]SpeechCall(nil), s.calls...)
}

// SilenceSpans is the shared double for subtitles.SilenceDetector: it reports Spans for every
// block, or Err.
type SilenceSpans struct {
	Spans []subtitles.Span
	Err   error
}

func (s SilenceSpans) Silence(context.Context, string, time.Duration, time.Duration) ([]subtitles.Span, error) {
	return s.Spans, s.Err
}

// MediaBudget is the shared double for the playout budget's filler-priority admission
// (subtitles.Budget). While Tight, every context it gives is born ended with cause
// playout.ErrYielded, as ResourceBudget's is when playout has no headroom; Yield takes the
// headroom back from work already admitted.
type MediaBudget struct {
	mu     sync.Mutex
	tight  bool
	admits int
	live   map[*byte]context.CancelCauseFunc
}

// Yield leaves playout tight and ends every admitted context with cause playout.ErrYielded, as a
// live admission that leaves no room does.
func (b *MediaBudget) Yield() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.tight = true
	for key, cancel := range b.live {
		cancel(admission.ErrYielded)
		delete(b.live, key)
	}
}

// SetTight sets whether playout has headroom.
func (b *MediaBudget) SetTight(tight bool) {
	b.mu.Lock()
	b.tight = tight
	b.mu.Unlock()
}

// Admitted is how many contexts were given while playout had headroom.
func (b *MediaBudget) Admitted() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.admits
}

func (b *MediaBudget) MediaWorkContext(ctx context.Context, _ bool) (context.Context, context.CancelFunc) {
	wctx, cancel := context.WithCancelCause(ctx)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.tight {
		cancel(admission.ErrYielded)
		return wctx, func() {}
	}
	b.admits++
	key := new(byte)
	if b.live == nil {
		b.live = map[*byte]context.CancelCauseFunc{}
	}
	b.live[key] = cancel
	return wctx, func() {
		b.mu.Lock()
		delete(b.live, key)
		b.mu.Unlock()
		cancel(context.Canceled)
	}
}
