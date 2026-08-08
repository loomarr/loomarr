package playout

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sort"
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
	// Target is the codec audience this session encodes for (§9.1 V47) — part of the session's
	// identity, not a viewer attribute. Two sessions can share a ChannelID and differ only here
	// (a browser one transcoding HEVC, a tuner one copying it). Reported on SessionStat so the
	// dashboard shows which audience an encoder serves.
	Target Target

	// cancel stops the encoder. The context IS the lifetime (process.go) — there is no
	// separate "stop the ffmpeg" path that could disagree with it. nil until the spawn completes
	// (see ready): a placeholder is in the map before its ffmpeg exists.
	cancel context.CancelFunc
	proc   *Process
	log    *slog.Logger

	// ready is closed once the encoder spawn resolves (success OR initErr). It lets the manager
	// insert a placeholder session under m.mu, release the lock, and spawn ffmpeg OUTSIDE it, while
	// concurrent same-key viewers wait here for the one spawn instead of starting a second encoder.
	// A nil ready means a fully-spawned session (the fake-spawner test path builds these directly).
	ready chan struct{}
	// initErr is the spawn failure, if any — read only after ready is closed. Set once by
	// spawnPlaceholder; never mutated afterwards, so no lock is needed for it.
	initErr error
	// grace is how long this session survives its last viewer. Copied from the manager
	// so onIdle does not need to reach back through a parent pointer (which would also
	// mean taking two locks in an order that has to stay consistent).
	grace time.Duration
	// onClosed tells the manager this session ended, so the dashboard learns a channel
	// stopped. Copied down for the same reason as `grace`: a parent pointer would mean
	// reaching back through the manager's lock from inside the session's.
	onClosed func()

	// startedAt is when the channel came on air — the dashboard's uptime, and the denominator
	// for judging whether a low speed is a cold start or a sustained problem.
	startedAt time.Time
	// encoder is the hardware/software choice the CURRENT program resolved (§9.1). The single
	// most actionable telemetry an operator has: it is the difference between four concurrent
	// streams and one.
	//
	// ⚠ Reported from OUTSIDE, by the per-program child, not captured at session start. The
	// session's own ffmpeg is the `-c copy` PARENT — it reads the ffconcat playlist and
	// remuxes, and never encodes anything. Its speed would measure remuxing throughput and its
	// "encoder" would be copy, so sourcing telemetry from it would put a confident,
	// meaningless number on the dashboard. Encoding happens in the per-program children the
	// concat demuxer requests, and Resolve is load-aware, so the answer can legitimately
	// change from one program to the next on the same channel.
	encoder Encoder

	mu      sync.Mutex
	viewers map[int]chan []byte
	nextID  int
	// last is the most recent progress sample from the CURRENT program's encoder (see
	// `encoder` above for why it is not the session's own process). The parser has always run
	// and every caller passed `nil` for its callback, so each sample was parsed and discarded —
	// the telemetry the dashboard needs was already being produced and had nowhere to go.
	//
	// Guarded by the same mutex as `viewers`: both are read together to build a snapshot,
	// and a second lock would be one more ordering to keep consistent for no gain.
	last Progress
	// closed guards against a viewer attaching to a session that is already tearing
	// down. Without it, a viewer could join between "the grace timer fired" and "the map
	// entry was deleted" and then wait forever on a stream nobody is writing to.
	closed bool
}

// SessionStat is a snapshot of one live encoder, for the dashboard (§12, V16).
//
// A VALUE, not a pointer into the live session: the caller reads it without holding a lock
// and cannot accidentally mutate an encoder's state while it runs.
type SessionStat struct {
	ChannelID string `json:"channelId"`
	// Target names the codec audience this encoder serves — "browser" or "mediaserver" (§9.1 V47).
	// A channel can have one row per target; the dashboard shows both so an operator seeing two
	// encoders on one channel knows one is a browser transcode and one is a tuner copy.
	Target   string  `json:"target"`
	Viewers  int     `json:"viewers"`
	Encoder  string  `json:"encoder" doc:"Resolved encoder: hardware vendor, or 'software'"`
	Hardware bool    `json:"hardware" doc:"Whether the resolved encoder is hardware-accelerated"`
	Speed    float64 `json:"speed" doc:"Realtime multiple from ffmpeg; sustained <1.0 means the channel will stutter"`
	// BufferedMS is how far ahead of realtime the encoder has produced output — out_time
	// minus wall-clock elapsed. Negative means it is BEHIND, which is the same condition a
	// sub-1.0 speed reports, seen as accumulated deficit rather than an instantaneous rate.
	BufferedMS int64 `json:"bufferedMs"`
	UptimeMS   int64 `json:"uptimeMs"`
}

// sessionKey identifies one live encoder. A channel does NOT have a single stream — it has one
// per codec AUDIENCE (§9.1 V47): a media-server tuner ingests HEVC, a browser needs h264, so the
// two get different copy/transcode plans and therefore different encoders. The key is the pair.
//
// A struct rather than a "channel|target" string: it is a comparable value Go maps accept
// directly, with no delimiter to escape and no parse to get wrong. For an h264 channel both
// targets' plans are `-c copy`, so the "second" session is a cheap remux, not a second encode —
// the split's cost is bounded by the copy plan, not the key (see §9.1).
type sessionKey struct {
	channel string
	target  Target
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

	// onChange fires after the live-session set changes — a channel starting or stopping.
	// The composition root uses it to publish an SSE `playout` frame so the dashboard learns
	// about a stream within a frame rather than at the next poll (§8: SSE is the latency
	// path, the GET endpoint is truth).
	//
	// Fired WITHOUT the manager lock held. Publishing takes the bus's lock, and calling out
	// to arbitrary code while holding m.mu is how a deadlock gets built between two packages
	// that never mention each other.
	onChange func()

	mu       sync.Mutex
	sessions map[sessionKey]*Session
}

// OnChange registers the session-set change hook. Called once during composition, before any
// viewer can attach.
func (m *Manager) OnChange(fn func()) { m.onChange = fn }

// notifyChange fires the hook if one is registered. Never called with m.mu held.
func (m *Manager) notifyChange() {
	if m.onChange != nil {
		m.onChange()
	}
}

// Spawner starts an encoder for a channel and returns the supervised process. The
// implementation builds args from the resolved Airing and the load-aware Profile.
//
// It takes the TARGET as well as the channel (§9.1 V47): the parent fetches its ffconcat playlist
// over HTTP, and that playlist URL must carry `?target=` so every program it requests plans its
// copy for the right codec audience. A browser session's parent and a tuner session's parent read
// DIFFERENT playlist URLs for the same channel.
type Spawner func(ctx context.Context, channelID string, target Target) (*Process, error)

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
		sessions: map[sessionKey]*Session{},
	}
}

// DefaultGrace is how long an encoder survives its last viewer.
//
// Long enough to absorb channel surfing and a client reconnecting after a network blip —
// both of which are common on a TV and both of which would otherwise pay the full encoder
// start cost (seek + ffmpeg init, seconds not milliseconds). Short enough that a genuinely
// abandoned channel stops burning a core promptly.
const DefaultGrace = 30 * time.Second

// Attach connects a viewer to a channel FOR A TARGET, starting the encoder if one is not already
// running for that (channel, target) pair (§9.1 V47). A tuner attaches as TargetMediaServer and a
// browser as TargetBrowser; they get separate sessions when the copy plan differs (HEVC) and
// separate-but-both-cheap `-c copy` sessions when it does not (h264).
//
// Returns a chunk channel and a detach func. The caller MUST call detach — it is what
// decrements the refcount, and a leaked viewer keeps a channel encoding forever.
func (m *Manager) Attach(ctx context.Context, channelID string, target Target) (<-chan []byte, func(), error) {
	key := sessionKey{channel: channelID, target: target}

	// ⚠ **The find-or-create is atomic, but the SPAWN is not held under m.mu.** The lock protects
	// only the map decision (reuse an existing session, or reserve a placeholder for a new one);
	// the ffmpeg spawn then runs OUTSIDE the lock. This is what lets four channels cold-start in
	// PARALLEL instead of queuing — the earlier design held m.mu across the whole spawn, so a
	// simultaneous four-channel tune-in serialized, each waiting on the previous encoder's init.
	//
	// The race the old lock prevented (two viewers of the SAME channel starting two encoders, one
	// orphaned) is still prevented: the placeholder is inserted atomically, so the second same-key
	// caller finds it and WAITS on the winner's spawn (session.ready) rather than starting its own.
	// Different keys never contend — each reserves its own placeholder and spawns concurrently.
	m.mu.Lock()
	if s := m.sessions[key]; s != nil {
		m.mu.Unlock()
		return m.joinExisting(ctx, key, s)
	}
	// Admission, not eviction. See ErrAtCapacity. The bound counts ENCODERS, which is what
	// consumes the box — so a channel watched as both a browser and a tuner legitimately counts
	// twice here, because it is two encoders. len(m.sessions) already reflects that. Checked here,
	// under the lock and against the placeholder count, so parallel starts cannot overshoot it.
	if AtCapacity(m.maxChannels, len(m.sessions)) {
		m.mu.Unlock()
		return nil, nil, ErrAtCapacity
	}
	// Reserve the slot with a not-yet-spawned placeholder, then spawn outside the lock.
	s := m.newPlaceholder(channelID, target)
	m.sessions[key] = s
	m.mu.Unlock()

	m.spawnPlaceholder(ctx, s)
	<-s.ready
	if s.initErr != nil {
		// Spawn failed: drop the placeholder so the next viewer starts fresh, and report it.
		m.mu.Lock()
		if m.sessions[key] == s {
			delete(m.sessions, key)
		}
		m.mu.Unlock()
		return nil, nil, s.initErr
	}
	m.notifyChange()
	ch, detach, _ := s.addViewer() // fresh, spawned session: cannot be closed yet
	return ch, detach, nil
}

// joinExisting attaches to a session another caller created. If it is mid-spawn (a same-key viewer
// that arrived a moment after the winner), we wait on its readiness rather than starting a second
// encoder; if it turns out to have failed or is tearing down, we retry the whole find-or-create so
// the caller still gets a working session.
func (m *Manager) joinExisting(ctx context.Context, key sessionKey, s *Session) (<-chan []byte, func(), error) {
	<-s.ready
	if s.initErr != nil {
		// The winner's spawn failed; it will remove itself. Retry from the top.
		return m.Attach(ctx, key.channel, key.target)
	}
	ch, detach, ok := s.addViewer()
	if ok {
		return ch, detach, nil
	}
	// Mid-teardown: drop it and start fresh rather than joining something dying.
	m.mu.Lock()
	if m.sessions[key] == s {
		delete(m.sessions, key)
	}
	m.mu.Unlock()
	return m.Attach(ctx, key.channel, key.target)
}

// newPlaceholder builds a Session whose encoder is not spawned yet. `ready` gates every viewer until
// spawnPlaceholder resolves it (success or initErr), so concurrent same-key callers share one spawn.
func (m *Manager) newPlaceholder(channelID string, target Target) *Session {
	return &Session{
		ChannelID: channelID,
		Target:    target,
		log:       m.log,
		grace:     m.grace,
		onClosed:  m.notifyChange,
		startedAt: time.Now(),
		viewers:   map[int]chan []byte{},
		ready:     make(chan struct{}),
	}
}

// spawnPlaceholder runs the ffmpeg spawn for a reserved placeholder OUTSIDE m.mu, then closes
// s.ready. On failure it records s.initErr; Attach/joinExisting remove the placeholder from the map.
func (m *Manager) spawnPlaceholder(ctx context.Context, s *Session) {
	// context.Background, NOT the attaching viewer's request context. The session outlives whoever
	// started it — binding its lifetime to the first viewer's request would kill the channel for
	// everybody the moment that one person's TV disconnected. ctx is accepted only for symmetry with
	// the spawner signature; the viewer's cancellation must not propagate here.
	_ = ctx
	sctx, cancel := context.WithCancel(context.Background())

	if m.log != nil {
		m.log.Info("playout: session.start spawning encoder", "channel", s.ChannelID, "target", s.Target.String())
	}
	proc, err := m.spawn(sctx, s.ChannelID, s.Target)
	if err != nil {
		cancel()
		if m.log != nil {
			m.log.Warn("playout: session.start spawn failed", "channel", s.ChannelID, "target", s.Target.String(), "err", err)
		}
		s.initErr = err
		close(s.ready)
		return
	}
	s.cancel = cancel
	s.proc = proc
	go s.pump()
	close(s.ready)
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
	var total int64
	for {
		n, err := s.proc.Stdout.Read(buf)
		if n > 0 {
			if total == 0 && s.log != nil {
				s.mu.Lock()
				vc := len(s.viewers)
				s.mu.Unlock()
				s.log.Info("playout: session first bytes from parent", "channel", s.ChannelID, "n", n, "viewers", vc)
			}
			total += int64(n)
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

	// Outside the lock, and after the cancel, so a subscriber that immediately re-reads the
	// telemetry sees the session already gone rather than mid-teardown.
	if s.onClosed != nil {
		s.onClosed()
	}
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

// session returns a live session by (channel, target), or nil. For tests and telemetry.
func (m *Manager) session(channelID string, target Target) *Session {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sessions[sessionKey{channel: channelID, target: target}]
}

// ActiveCount reports how many channels are encoding. This is the `active` input to
// Resolve's load-aware quality decision, and what the operator sees as concurrent load.
func (m *Manager) ActiveCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

// Capacity reports the admission bound (`playout.max_channels`) — the denominator in the
// dashboard's "2 / 4" load line.
func (m *Manager) Capacity() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.maxChannels
}

// Stats snapshots every live encoder for the dashboard (§12, V16).
//
// Sorted by channel id so a polling or re-rendering caller sees a stable order — an
// unsorted map walk would reshuffle the rows on every read, which reads as flicker rather
// than as data changing.
//
// Takes the manager lock, then each session's, in that order — the same order Attach uses,
// so the two cannot deadlock against each other.
func (m *Manager) Stats(now time.Time) []SessionStat {
	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()

	out := make([]SessionStat, 0, len(sessions))
	for _, s := range sessions {
		// ⚠ Skip CLOSED sessions. Teardown is lazy: close() marks the session and disconnects
		// its viewers, but the map entry survives until the next Attach on that channel
		// deletes it. An unfiltered snapshot would keep reporting a dead encoder as live —
		// indefinitely, on a channel nobody tunes again.
		if st, ok := s.statIfLive(now); ok {
			out = append(out, st)
		}
	}
	// By channel, then target — a channel can now have two rows (browser + tuner), and a stable
	// tiebreak keeps them from reshuffling between polls.
	sort.Slice(out, func(i, j int) bool {
		if out[i].ChannelID != out[j].ChannelID {
			return out[i].ChannelID < out[j].ChannelID
		}
		return out[i].Target < out[j].Target
	})
	return out
}

// statIfLive snapshots one session, reporting false if it has already been torn down.
//
// Liveness is read under the SAME lock as the rest of the snapshot: checking `closed`
// separately would leave a window where a session closes between the check and the read, and
// the row would describe an encoder that no longer exists.
func (s *Session) statIfLive(now time.Time) (SessionStat, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return SessionStat{}, false
	}

	uptime := now.Sub(s.startedAt)
	// How far ahead of realtime the encoder has produced output. `out_time` is how much
	// media exists; uptime is how much wall-clock has passed. The difference is the cushion
	// absorbing a slow moment before a viewer sees a stall — and it goes NEGATIVE when the
	// encoder is losing, which is the same condition a sub-1.0 speed reports, expressed as
	// accumulated deficit rather than an instantaneous rate.
	buffered := s.last.OutTimeMS - uptime.Milliseconds()

	return SessionStat{
		ChannelID:  s.ChannelID,
		Target:     s.Target.String(),
		Viewers:    len(s.viewers),
		Encoder:    string(s.encoder),
		Hardware:   s.encoder != "" && s.encoder != EncoderSoftware,
		Speed:      s.last.Speed,
		BufferedMS: buffered,
		UptimeMS:   uptime.Milliseconds(),
	}, true
}

// ReportProgram records telemetry for the program currently encoding on a (channel, target) (V16).
//
// The TARGET is part of the address (§9.1 V47): a channel can have two live sessions (a browser
// one and a tuner one), and a per-program child belongs to exactly one — the one whose parent
// requested it, which is why the child carries its target through the program URL. Reporting to the
// wrong session would put a browser transcode's speed on the tuner's dashboard row.
//
// Called by the per-program encode path, which is where the real encoder runs — see the
// `encoder` field for why the session's own process is the wrong source. A report for a channel
// with no live session is dropped: the child is bound to its request, so it can briefly outlive
// a session that was just torn down, and resurrecting a dead session to hold its telemetry
// would be worse than losing a sample.
//
// Progress samples are a LATENCY signal, never load-bearing (§8) — the same discipline the SSE
// bus documents. Dropping one costs a stale number for a second, nothing more.
func (m *Manager) ReportProgram(channelID string, target Target, enc Encoder, p Progress) {
	s := m.session(channelID, target)
	if s == nil {
		return
	}
	s.mu.Lock()
	s.encoder = enc
	s.last = p
	s.mu.Unlock()
}
