package playout

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"time"
)

// One encoder per channel, N viewers (§9.1; prior-art §6.3).
//
// The unit of work is a CHANNEL, not a viewer. A channel is a wall clock — everyone
// watching it at 20:15 sees the same frame — so encoding it once and fanning the bytes out
// is not an optimization, it is the only model that is even correct. Per-viewer encodes
// would drift apart, and three people watching one channel would cost three encoders.
//
// That inverts the usual VOD shape, where each viewer legitimately has their own position
// and their own transcode. viewra is a VOD transcoder, and its session manager EVICTED
// other sessions to make room when it hit its limit (prior-art, viewra §1) — behaviour
// which for playout means one person tuning in kills someone else's channel. Here the
// limit is an admission bound (`AtCapacity`) instead: we refuse the newcomer and keep
// faith with the people already watching.
//
// Three failure modes get explicit machinery below, because each is easy to write wrong
// and none of them fails loudly:
//
//  1. The same-key start race — two viewers arriving together must not start two encoders.
//  2. Grace-period teardown — a channel-surfing TV must not pay for a fresh encoder start.
//  3. The ABA problem inside that grace timer — see stopAfterGrace.

// ErrAtCapacity is returned when a new channel would exceed `playout.max_channels`.
//
// A distinct error because the API must render it as 503 with an actionable message, not as
// a generic failure: the operator's fix (raise the cap, or lower the quality tier so more
// channels fit) is only discoverable if we say which wall was hit.
var ErrAtCapacity = errors.New("playout: at channel capacity")

// viewerBuffer is how many chunks a single slow viewer may fall behind before it is
// dropped.
//
// Sized in CHUNKS, not bytes, because that is the unit the fan-out moves. Deliberately
// small: a viewer this far behind is not going to recover, and a bigger buffer only delays
// the drop while holding more memory per stalled TV.
const viewerBuffer = 8

// Session is one channel's encoder plus its connected viewers.
type Session struct {
	ChannelID string

	// cancel stops the encoder. The context IS the lifetime (process.go) — there is no
	// separate "stop the ffmpeg" path that could disagree with it.
	cancel context.CancelFunc
	proc   *Process
	log    *slog.Logger
	// grace is how long this session survives its last viewer. Copied from the manager
	// so onIdle does not need to reach back through a parent pointer (which would also
	// mean taking two locks in an order that has to stay consistent).
	grace time.Duration

	mu      sync.Mutex
	viewers map[int]chan []byte
	nextID  int
	// closed guards against a viewer attaching to a session that is already tearing
	// down. Without it, a viewer could join between "the grace timer fired" and "the map
	// entry was deleted" and then wait forever on a stream nobody is writing to.
	closed bool
}

// Manager owns the live sessions. One per process.
type Manager struct {
	// spawn starts an encoder. Injected so tests exercise the session lifecycle
	// (refcounting, grace, the race) without executing ffmpeg; the live test supplies
	// the real one.
	spawn Spawner

	log *slog.Logger

	// grace is how long a channel keeps encoding after its last viewer leaves.
	grace time.Duration
	// maxChannels is the admission bound (`playout.max_channels`).
	maxChannels int

	mu       sync.Mutex
	sessions map[string]*Session
}

// Spawner starts an encoder for a channel and returns the supervised process. The
// implementation builds args from the resolved Airing and the load-aware Profile.
type Spawner func(ctx context.Context, channelID string) (*Process, error)

// NewManager builds a session manager.
//
// There is deliberately NO resolver here. The manager owns the long-lived PARENT process (one
// `-c copy` ffmpeg per channel, reading an ffconcat playlist); resolving "what is airing now"
// happens in the /playout/program handler, once per program, because that is the request the
// concat demuxer makes. An earlier draft gave the manager a Resolver seam and nothing ever read
// it — dead surface whose only effect was a nil argument at the call site.
func NewManager(spawn Spawner, maxChannels int, grace time.Duration, log *slog.Logger) *Manager {
	if grace <= 0 {
		grace = DefaultGrace
	}
	return &Manager{
		spawn:       spawn,
		maxChannels: maxChannels, grace: grace, log: log,
		sessions: map[string]*Session{},
	}
}

// DefaultGrace is how long an encoder survives its last viewer.
//
// Long enough to absorb channel surfing and a client reconnecting after a network blip —
// both of which are common on a TV and both of which would otherwise pay the full encoder
// start cost (seek + ffmpeg init, seconds not milliseconds). Short enough that a genuinely
// abandoned channel stops burning a core promptly.
const DefaultGrace = 30 * time.Second

// Attach connects a viewer to a channel, starting the encoder if it is not running.
//
// Returns a chunk channel and a detach func. The caller MUST call detach — it is what
// decrements the refcount, and a leaked viewer keeps a channel encoding forever.
func (m *Manager) Attach(ctx context.Context, channelID string) (<-chan []byte, func(), error) {
	// THE RACE (prior-art §6.3). This lock is held across the find-or-create, including
	// the spawn — deliberately, and it is the whole point. Two viewers tuning the same
	// channel in the same millisecond both find no session; if the lock were released
	// between the lookup and the insert, both would start an encoder and one would be
	// orphaned with no viewers and no map entry to ever find it again. Holding it means
	// the loser finds the winner's session.
	//
	// Cost of being wrong the other way (a brief serialization of channel starts) is a
	// few hundred ms of added latency on simultaneous first-tunes. Cost of the race is a
	// permanently leaked ffmpeg. Not a close call.
	m.mu.Lock()
	defer m.mu.Unlock()

	if s := m.sessions[channelID]; s != nil {
		ch, detach, ok := s.addViewer()
		if ok {
			return ch, detach, nil
		}
		// The session is mid-teardown. Drop it and start fresh below rather than
		// attaching to something that will never produce bytes.
		delete(m.sessions, channelID)
	}

	// Admission, not eviction. See ErrAtCapacity.
	if AtCapacity(m.maxChannels, len(m.sessions)) {
		return nil, nil, ErrAtCapacity
	}

	s, err := m.start(channelID)
	if err != nil {
		return nil, nil, err
	}
	m.sessions[channelID] = s

	ch, detach, _ := s.addViewer() // fresh session: cannot be closed
	return ch, detach, nil
}

// start launches an encoder for a channel. Caller holds m.mu.
func (m *Manager) start(channelID string) (*Session, error) {
	// context.Background, NOT the attaching viewer's request context. The session
	// outlives whoever started it — binding its lifetime to the first viewer's request
	// would kill the channel for everybody the moment that one person's TV disconnected.
	ctx, cancel := context.WithCancel(context.Background())

	proc, err := m.spawn(ctx, channelID)
	if err != nil {
		cancel()
		return nil, err
	}

	s := &Session{
		ChannelID: channelID,
		cancel:    cancel, proc: proc, log: m.log,
		grace:   m.grace,
		viewers: map[int]chan []byte{},
	}
	go s.pump()
	return s, nil
}

// pump reads the encoder's output and fans each chunk to every viewer.
//
// One reader, N writers. The chunk size is a plain read buffer rather than anything
// MPEG-TS-aware: this layer moves bytes and must not care where packet boundaries fall.
// Parsing the transport stream here would be a second muxer to keep correct, and the
// viewers' consumer (a media server) already handles arbitrary chunking.
func (s *Session) pump() {
	defer s.close()

	// 64 KiB: ~340 MPEG-TS packets, a few frames at playout bitrates. Large enough that
	// the per-chunk fan-out overhead is negligible, small enough that a viewer joining
	// mid-stream waits milliseconds for its first bytes.
	buf := make([]byte, 64*1024)
	for {
		n, err := s.proc.Stdout.Read(buf)
		if n > 0 {
			// Copy before broadcasting: buf is reused on the next iteration, and the
			// viewers hold their chunk in a buffered channel across it. Handing them the
			// shared slice would corrupt whatever they had not yet written — a data race
			// whose symptom is intermittently garbled video, which looks like an encoder
			// bug and is not.
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			s.broadcast(chunk)
		}
		if err != nil {
			if err != io.EOF && s.log != nil {
				s.log.Debug("playout: encoder read ended",
					"channel", s.ChannelID, "err", err, "ffmpeg", s.proc.LastError())
			}
			return
		}
	}
}

// broadcast sends a chunk to every viewer, dropping any that cannot keep up.
//
// The events bus (internal/events) has the same non-blocking shape with the OPPOSITE
// action, and the difference is worth stating. There, a full buffer drops the EVENT,
// because §8 makes SSE a latency optimization and the store is truth on reconnect. Here
// there is no re-read: a byte stream with a hole in the middle is corrupt, not stale. So
// the choice is between dropping the VIEWER and blocking the encoder — and blocking would
// let one stalled TV freeze the channel for everyone else watching it.
//
// Dropping the viewer is therefore the kind option: that one client reconnects (media
// servers do, promptly) and gets a clean stream from the current position, while nobody
// else notices.
func (s *Session) broadcast(chunk []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, ch := range s.viewers {
		select {
		case ch <- chunk:
		default:
			if s.log != nil {
				s.log.Debug("playout: dropping viewer that fell behind",
					"channel", s.ChannelID, "viewer", id)
			}
			delete(s.viewers, id)
			close(ch)
		}
	}
}

// addViewer registers a viewer. Reports false if the session is already tearing down.
func (s *Session) addViewer() (<-chan []byte, func(), bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, nil, false
	}
	id := s.nextID
	s.nextID++
	ch := make(chan []byte, viewerBuffer)
	s.viewers[id] = ch

	var once sync.Once
	detach := func() { once.Do(func() { s.removeViewer(id) }) }
	return ch, detach, true
}

// removeViewer drops a viewer and arms the grace timer if it was the last one.
func (s *Session) removeViewer(id int) {
	s.mu.Lock()
	ch, ok := s.viewers[id]
	if ok {
		delete(s.viewers, id)
		close(ch)
	}
	last := len(s.viewers) == 0 && !s.closed
	s.mu.Unlock()

	if last {
		s.onIdle()
	}
}

// onIdle is called when the last viewer leaves. It does NOT tear down immediately — see
// DefaultGrace: a channel-surfing TV that comes back in two seconds should find the encoder
// still running rather than pay the start cost again.
//
// The obvious `time.AfterFunc(s.grace, s.close)` is wrong:
//
//	t=0   last viewer leaves      → timer armed for t=30s
//	t=2s  a viewer attaches       → the session is live again, 1 viewer
//	t=30s the timer fires         → closes a session that someone is watching
//
// The fix is to make FIRING CONDITIONAL rather than to cancel the timer. Cancelling looks
// tidier but is harder to be sure of: `timer.Stop()` returning false means the callback may
// already be running, so a re-attach would still need this same idle check to be safe — the
// cancellation would be an optimization layered on top of the real guard, not a replacement
// for it. A stray wakeup every grace period on an idle channel costs nothing measurable, so
// the simpler correct thing wins.
//
// Re-checking `len(s.viewers) == 0` at fire time also solves the ABA case for free: several
// join/leave cycles arm several timers, and an early one firing while a LATER viewer is
// watching sees a non-empty map and does nothing. No generation counter needed — the viewer
// map is already the authoritative answer to "is anyone watching?", and a second source of
// truth could only disagree with it.
func (s *Session) onIdle() {
	time.AfterFunc(s.grace, func() {
		// Read the state under the lock, then act OUTSIDE it: s.close() takes s.mu itself,
		// and Go mutexes are not reentrant, so closing while holding it would deadlock the
		// timer goroutine — and with it the channel's teardown, permanently.
		s.mu.Lock()
		idle := len(s.viewers) == 0 && !s.closed
		s.mu.Unlock()

		if !idle {
			// Someone is watching (or pump already tore the session down when the encoder
			// died). Either way this timer has nothing to do.
			return
		}
		if s.log != nil {
			s.log.Debug("playout: stopping idle channel",
				"channel", s.ChannelID, "grace", s.grace)
		}
		s.close()
	})
}

// ViewerCount is for the API's telemetry and for tests.
func (s *Session) ViewerCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.viewers)
}

// close tears the session down and disconnects every remaining viewer.
//
// Closing the viewer channels is what unblocks their handlers: a tuner handler is parked
// on a channel receive, and a closed channel is how it learns the stream ended and can
// return instead of hanging until the client gives up.
func (s *Session) close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	for id, ch := range s.viewers {
		delete(s.viewers, id)
		close(ch)
	}
	s.mu.Unlock()

	s.cancel() // kills the process group (process.go)
}

// Stop tears down the session immediately, regardless of viewers. For shutdown and for an
// operator stopping a channel.
func (s *Session) Stop() { s.close() }

// Stop tears down every session. Called on shutdown — a live encoder never exits on its
// own, so without this they outlive the process that started them.
func (m *Manager) Stop() {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for id, s := range m.sessions {
		sessions = append(sessions, s)
		delete(m.sessions, id)
	}
	m.mu.Unlock()

	for _, s := range sessions {
		s.Stop()
	}
}

// session returns a live session by channel id, or nil. For tests and telemetry.
func (m *Manager) session(channelID string) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[channelID]
}

// ActiveCount reports how many channels are encoding. This is the `active` input to
// Resolve's load-aware quality decision, and what the operator sees as concurrent load.
func (m *Manager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}
