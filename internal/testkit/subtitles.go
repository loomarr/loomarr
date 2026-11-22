package testkit

import (
	"context"
	"sync"
	"time"

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
// (subtitles.Budget). While Tight, every context it gives is born ended, as ResourceBudget's is
// when playout has no headroom.
type MediaBudget struct {
	mu     sync.Mutex
	tight  bool
	admits int
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
	wctx, cancel := context.WithCancel(ctx)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.tight {
		cancel()
		return wctx, func() {}
	}
	b.admits++
	return wctx, cancel
}
