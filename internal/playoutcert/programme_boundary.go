package playoutcert

import (
	"context"
	"errors"
	"io"
	"sync"

	"github.com/loomarr/loomarr/internal/playout"
)

type syntheticBoundaryEvent struct {
	sourceID uint64
	identity playout.AiringIdentity
}

type syntheticBoundaryWitness struct {
	mu           sync.Mutex
	nextSourceID uint64
	subscribers  map[string]map[*syntheticBoundarySubscription]struct{}
}

func newSyntheticBoundaryWitness() *syntheticBoundaryWitness {
	return &syntheticBoundaryWitness{subscribers: make(map[string]map[*syntheticBoundarySubscription]struct{})}
}

func (w *syntheticBoundaryWitness) nextSource() uint64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.nextSourceID++
	return w.nextSourceID
}

func (w *syntheticBoundaryWitness) Subscribe(channelID string) (ProgrammeBoundarySubscription, error) {
	if channelID == "" {
		return nil, errors.New("programme boundary witness requires a channel")
	}
	s := &syntheticBoundarySubscription{owner: w, channelID: channelID, events: make(chan syntheticBoundaryEvent, 16)}
	w.mu.Lock()
	if w.subscribers[channelID] == nil {
		w.subscribers[channelID] = make(map[*syntheticBoundarySubscription]struct{})
	}
	w.subscribers[channelID][s] = struct{}{}
	w.mu.Unlock()
	return s, nil
}

func (w *syntheticBoundaryWitness) publish(channelID string, event syntheticBoundaryEvent) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for subscriber := range w.subscribers[channelID] {
		select {
		case subscriber.events <- event:
		default:
		}
	}
}

type syntheticBoundarySubscription struct {
	owner     *syntheticBoundaryWitness
	channelID string
	events    chan syntheticBoundaryEvent
	once      sync.Once
	sourceID  uint64
	initial   playout.AiringIdentity
}

func (s *syntheticBoundarySubscription) WaitInitial(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case event := <-s.events:
		if !validSyntheticAiring(event.identity) {
			return errors.New("programme boundary witness emitted invalid initial identity")
		}
		s.sourceID = event.sourceID
		s.initial = event.identity
		return nil
	}
}

func (s *syntheticBoundarySubscription) WaitTransition(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-s.events:
			if event.sourceID == s.sourceID && successiveSyntheticAiring(s.initial, event.identity) {
				return nil
			}
		}
	}
}

func (s *syntheticBoundarySubscription) Close() {
	s.once.Do(func() {
		s.owner.mu.Lock()
		delete(s.owner.subscribers[s.channelID], s)
		if len(s.owner.subscribers[s.channelID]) == 0 {
			delete(s.owner.subscribers, s.channelID)
		}
		s.owner.mu.Unlock()
	})
}

func sameSyntheticAiring(a, b playout.AiringIdentity) bool {
	return a.StartedAt.Equal(b.StartedAt) && a.EndsAt.Equal(b.EndsAt) && a.Kind == b.Kind && a.ContentID == b.ContentID && a.ScheduleBlockID == b.ScheduleBlockID
}

func validSyntheticAiring(identity playout.AiringIdentity) bool {
	return !identity.StartedAt.IsZero() && identity.EndsAt.After(identity.StartedAt) && identity.Kind != "" && identity.ContentID != "" && identity.ScheduleBlockID != ""
}

func successiveSyntheticAiring(previous, next playout.AiringIdentity) bool {
	// A stream can legitimately advance more than one finite block before the
	// reader reaches the next observable block. It must nevertheless advance
	// strictly forward from this parent's initial epoch; duplicate, stale, and
	// backwards identities are never a transition.
	return validSyntheticAiring(previous) && validSyntheticAiring(next) && !sameSyntheticAiring(previous, next) && next.StartedAt.After(previous.StartedAt) && next.EndsAt.After(previous.EndsAt)
}

type witnessedBlockContent struct {
	source  io.ReadCloser
	onFirst func()
	once    sync.Once
}

func (c *witnessedBlockContent) Read(buffer []byte) (int, error) {
	n, err := c.source.Read(buffer)
	if n > 0 {
		c.once.Do(c.onFirst)
	}
	return n, err
}

func (c *witnessedBlockContent) Close() error { return c.source.Close() }
