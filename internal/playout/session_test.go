package playout

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// A fake encoder: a pipe we can write "video" into, wrapped in the same Process the real
// spawner returns. No ffmpeg — the session's job is refcounting and fan-out, and executing
// a real encoder to test a refcount would make these tests slow and flaky for no coverage.
type fakeEncoder struct {
	w       *os.File
	stopped chan struct{}
	once    sync.Once
}

// The getter keys by channel id; when a test drives two targets on one channel it should use
// newFakeSpawnerByKey instead, which distinguishes them. Kept channel-only here because the
// overwhelming majority of tests use one target and reading `get("ch1")` stays terse.
func newFakeSpawner(t *testing.T) (Spawner, func(string) *fakeEncoder) {
	t.Helper()
	spawn, byKey := newFakeSpawnerByKey(t)
	get := func(channelID string) *fakeEncoder { return byKey(channelID, TargetMediaServer) }
	return spawn, get
}

// newFakeSpawnerByKey mirrors production's (channel, target) identity: two targets on one channel
// get two distinct fake encoders, so a test can prove they are separate sessions.
func newFakeSpawnerByKey(t *testing.T) (Spawner, func(string, Target) *fakeEncoder) {
	t.Helper()
	var mu sync.Mutex
	encoders := map[sessionKey]*fakeEncoder{}

	spawn := func(ctx context.Context, channelID string, target Target) (*Process, error) {
		pr, pw, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		fe := &fakeEncoder{w: pw, stopped: make(chan struct{})}
		mu.Lock()
		encoders[sessionKey{channel: channelID, target: target}] = fe
		mu.Unlock()

		// Mimic Start's contract: the context is the lifetime. Cancelling it must end the
		// stream, which for the fake means closing the write end so pump sees EOF.
		go func() {
			<-ctx.Done()
			fe.once.Do(func() { _ = pw.Close(); close(fe.stopped) })
		}()

		return &Process{Stdout: pr, cmd: &exec.Cmd{}}, nil
	}
	get := func(channelID string, target Target) *fakeEncoder {
		mu.Lock()
		defer mu.Unlock()
		return encoders[sessionKey{channel: channelID, target: target}]
	}
	return spawn, get
}

// countingSpawner records how many encoders were started, for the race test.
func countingSpawner(t *testing.T) (Spawner, func() int) {
	t.Helper()
	inner, _ := newFakeSpawner(t)
	var mu sync.Mutex
	n := 0
	return func(ctx context.Context, channelID string, target Target) (*Process, error) {
			mu.Lock()
			n++
			mu.Unlock()
			// A real spawn is slow (ffmpeg init + a seek). Sleeping widens the race window
			// so a lock-scope bug fails reliably instead of one run in a thousand.
			time.Sleep(20 * time.Millisecond)
			return inner(ctx, channelID, target)
		}, func() int {
			mu.Lock()
			defer mu.Unlock()
			return n
		}
}

func testManager(t *testing.T, spawn Spawner, maxChannels int, grace time.Duration) *Manager {
	t.Helper()
	m := NewManager(spawn, maxChannels, grace, nil)
	t.Cleanup(m.Stop)
	return m
}

// The core model: ONE encoder serves N viewers. Anything else is both wasteful and
// incorrect — separate encodes of one channel would drift apart.
func TestAttach_OneEncoderServesManyViewers(t *testing.T) {
	spawn, started := countingSpawner(t)
	m := testManager(t, spawn, 4, time.Minute)

	for i := 0; i < 3; i++ {
		if _, _, err := m.Attach(context.Background(), "ch1", TargetMediaServer); err != nil {
			t.Fatalf("viewer %d: %v", i, err)
		}
	}
	if n := started(); n != 1 {
		t.Errorf("started %d encoders for one channel, want 1", n)
	}
	if n := m.ActiveCount(); n != 1 {
		t.Errorf("ActiveCount = %d, want 1", n)
	}
}

// THE RACE (prior-art §6.3). Two viewers tuning the same channel simultaneously must not
// each start an encoder — the loser's would be orphaned with no viewers and no map entry,
// i.e. a leaked ffmpeg burning a core until the process dies.
func TestAttach_SimultaneousViewersDoNotStartTwoEncoders(t *testing.T) {
	spawn, started := countingSpawner(t)
	m := testManager(t, spawn, 4, time.Minute)

	const viewers = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < viewers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start // release them all at once
			if _, _, err := m.Attach(context.Background(), "ch1", TargetMediaServer); err != nil {
				t.Errorf("attach: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if n := started(); n != 1 {
		t.Errorf("started %d encoders for %d simultaneous viewers, want 1 — the "+
			"find-or-create is not atomic", n, viewers)
	}
}

// Every viewer gets every byte. A viewer that receives a SUBSET of the stream has a corrupt
// one, not a delayed one.
func TestAttach_AllViewersReceiveTheSameBytes(t *testing.T) {
	spawn, encoder := newFakeSpawner(t)
	m := testManager(t, spawn, 4, time.Minute)

	a, _, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := encoder("ch1").w.Write([]byte("mpegts-payload")); err != nil {
		t.Fatal(err)
	}
	for name, ch := range map[string]<-chan []byte{"a": a, "b": b} {
		select {
		case got := <-ch:
			if string(got) != "mpegts-payload" {
				t.Errorf("viewer %s got %q", name, got)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("viewer %s received nothing", name)
		}
	}
}

// The V47 invariant: a channel's session identity is (channel, TARGET). A browser viewer and a
// tuner viewer of the SAME channel get SEPARATE encoders — because their copy plans differ (HEVC
// copied for the tuner, transcoded for the browser) — and tearing one down leaves the other live.
//
// This is the whole reason the black-frame bug existed: before V47 both shared one session, so
// whatever codec the tuner chose (HEVC) reached the browser, which cannot decode it.
func TestAttach_TargetForksTheSession(t *testing.T) {
	spawn, byKey := newFakeSpawnerByKey(t)
	m := testManager(t, spawn, 4, time.Minute)

	_, detachBrowser, err := m.Attach(context.Background(), "ch1", TargetBrowser)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatal(err)
	}

	// Two distinct encoders for one channel — the fake keys by (channel, target), so both being
	// non-nil and distinct proves two sessions were started, not one shared.
	browserEnc, tunerEnc := byKey("ch1", TargetBrowser), byKey("ch1", TargetMediaServer)
	if browserEnc == nil || tunerEnc == nil {
		t.Fatalf("expected an encoder per target, got browser=%v tuner=%v", browserEnc, tunerEnc)
	}
	if browserEnc == tunerEnc {
		t.Fatal("browser and tuner must be SEPARATE encoders, not one shared session")
	}
	if got := m.ActiveCount(); got != 2 {
		t.Fatalf("ActiveCount = %d, want 2 (one per target)", got)
	}

	// The two sessions are addressed independently.
	if m.session("ch1", TargetBrowser) == m.session("ch1", TargetMediaServer) {
		t.Fatal("session(ch1, browser) and session(ch1, mediaserver) must be different sessions")
	}

	// Tearing down the browser viewer leaves the tuner session live (immediate teardown: grace is a
	// minute here, but the browser session had exactly one viewer, so detaching arms grace, not a
	// stop — the tuner session is untouched regardless).
	detachBrowser()
	if m.session("ch1", TargetMediaServer) == nil {
		t.Fatal("detaching the browser viewer must not tear down the tuner session")
	}
}

// A stalled viewer must not freeze the channel for everyone else. Unlike the events bus
// (which drops the EVENT because the store is truth on reconnect), playout drops the
// VIEWER — there is no re-read for a byte stream, and blocking would punish the innocent.
//
// The fast viewer drains CONCURRENTLY with the writes, which is the only way to state this
// invariant honestly. An earlier version of this test wrote a burst first and read
// afterwards, then asserted "N writes ⇒ N chunks received" — false twice over: the OS pipe
// buffer coalesces small writes so one Read returns many of them, and a viewer nobody is
// draining is *correctly* dropped. It failed against correct code.
func TestBroadcast_SlowViewerIsDroppedNotBlocking(t *testing.T) {
	spawn, encoder := newFakeSpawner(t)
	m := testManager(t, spawn, 4, time.Minute)

	slow, _, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatal(err)
	}
	fast, _, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatal(err)
	}
	_ = slow // deliberately never drained — this is the stalled TV

	// The fast viewer keeps up throughout. If a stalled viewer could block the encoder,
	// this goroutine starves and the assertion below fires.
	received := make(chan int, 1)
	go func() {
		n := 0
		for range fast {
			n++
			received <- n
		}
		close(received)
	}()

	// Enough data to overrun the slow viewer's buffer many times over. 64 KiB per write so
	// each one is a distinct chunk rather than being coalesced with its neighbours.
	payload := make([]byte, 64*1024)
	w := encoder("ch1").w
	go func() {
		for i := 0; i < viewerBuffer*4; i++ {
			if _, err := w.Write(payload); err != nil {
				return // the pipe closed; the assertions below report the real problem
			}
		}
	}()

	// The property: the fast viewer keeps receiving well past the point at which the slow
	// one's buffer was exhausted.
	want := viewerBuffer * 2
	deadline := time.After(10 * time.Second)
	for {
		select {
		case n, ok := <-received:
			if !ok {
				t.Fatal("the fast viewer was closed; the slow one should have been dropped instead")
			}
			if n >= want {
				return // kept up past the slow viewer's capacity — invariant holds
			}
		case <-deadline:
			t.Fatalf("fast viewer starved before %d chunks — a slow viewer blocked the encoder", want)
		}
	}
}

// And the stalled viewer is actually dropped, rather than left accumulating memory.
func TestBroadcast_StalledViewerIsClosed(t *testing.T) {
	spawn, encoder := newFakeSpawner(t)
	m := testManager(t, spawn, 4, time.Minute)

	stalled, _, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatal(err)
	}

	payload := make([]byte, 64*1024)
	w := encoder("ch1").w
	go func() {
		for i := 0; i < viewerBuffer*4; i++ {
			if _, err := w.Write(payload); err != nil {
				return
			}
		}
	}()

	// Read nothing for a while, then drain: the channel must be closed, not merely full.
	time.Sleep(200 * time.Millisecond)
	deadline := time.After(10 * time.Second)
	for {
		select {
		case _, ok := <-stalled:
			if !ok {
				return // closed, as intended
			}
		case <-deadline:
			t.Fatal("a viewer that stopped reading was never dropped")
		}
	}
}

// The admission bound. viewra EVICTED an existing session to make room (prior-art viewra
// §1); for playout that means one person tuning in kills someone else's channel. We refuse
// the newcomer instead and keep faith with whoever is already watching.
func TestAttach_AtCapacityRefusesRatherThanEvicting(t *testing.T) {
	spawn, _ := newFakeSpawner(t)
	m := testManager(t, spawn, 2, time.Minute)

	first, _, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = m.Attach(context.Background(), "ch2", TargetMediaServer); err != nil {
		t.Fatal(err)
	}

	_, _, err = m.Attach(context.Background(), "ch3", TargetMediaServer)
	if err == nil {
		t.Fatal("a third channel was admitted past a cap of 2")
	}
	if err != ErrAtCapacity {
		t.Errorf("err = %v, want ErrAtCapacity so the API can render an actionable 503", err)
	}

	// The existing channels are untouched — nothing was evicted.
	if n := m.ActiveCount(); n != 2 {
		t.Errorf("ActiveCount = %d, want 2", n)
	}
	select {
	case _, ok := <-first:
		if !ok {
			t.Error("an existing viewer was disconnected to make room — that is eviction")
		}
	default: // no bytes pending is fine; we only care that it is not closed
	}

	// A viewer already attached to an admitted channel is still fine, and attaching
	// ANOTHER viewer to an existing channel must not be refused — the cap is on channels
	// (encoders), not on people watching.
	if _, _, err := m.Attach(context.Background(), "ch1", TargetMediaServer); err != nil {
		t.Errorf("a second viewer on an admitted channel was refused: %v", err)
	}
}

// An unset cap must not block playout — see AtCapacity.
func TestAttach_UnconfiguredCapDoesNotBlock(t *testing.T) {
	spawn, _ := newFakeSpawner(t)
	m := testManager(t, spawn, 0, time.Minute)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		if _, _, err := m.Attach(context.Background(), id, TargetMediaServer); err != nil {
			t.Fatalf("channel %s refused with no cap configured: %v", id, err)
		}
	}
}

// When the encoder dies, viewers must be CLOSED, not left parked forever. A tuner handler
// blocked on a channel receive learns the stream ended only this way.
func TestSession_EncoderExitDisconnectsViewers(t *testing.T) {
	spawn, encoder := newFakeSpawner(t)
	m := testManager(t, spawn, 4, time.Minute)

	v, _, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatal(err)
	}
	_ = encoder("ch1").w.Close() // encoder exits → pump sees EOF

	select {
	case _, ok := <-v:
		if ok {
			t.Error("expected the viewer channel to be closed when the encoder exits")
		}
	case <-time.After(2 * time.Second):
		t.Error("viewer was left parked after the encoder exited")
	}
}

// Detach must be idempotent: a handler may reasonably call it from a defer AND on an error
// path, and a double-decrement would tear down a channel other people are watching.
func TestDetach_IsIdempotent(t *testing.T) {
	spawn, _ := newFakeSpawner(t)
	m := testManager(t, spawn, 4, time.Minute)

	if _, _, err := m.Attach(context.Background(), "ch1", TargetMediaServer); err != nil {
		t.Fatal(err)
	}
	_, detach, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatal(err)
	}

	detach()
	detach()
	detach()

	s := m.session("ch1", TargetMediaServer)
	if s == nil {
		t.Fatal("session gone")
	}
	if n := s.ViewerCount(); n != 1 {
		t.Errorf("ViewerCount = %d, want 1 — repeated detach double-decremented", n)
	}
}

// Shutdown must stop every encoder. A live encoder never exits on its own (process.go), so
// without this they outlive the process that started them.
func TestManagerStop_TearsDownEveryEncoder(t *testing.T) {
	spawn, encoder := newFakeSpawner(t)
	m := NewManager(spawn, 4, time.Minute, nil)

	for _, id := range []string{"ch1", "ch2"} {
		if _, _, err := m.Attach(context.Background(), id, TargetMediaServer); err != nil {
			t.Fatal(err)
		}
	}
	m.Stop()

	for _, id := range []string{"ch1", "ch2"} {
		select {
		case <-encoder(id).stopped:
		case <-time.After(2 * time.Second):
			t.Errorf("%s: encoder was not stopped by Manager.Stop", id)
		}
	}
	if n := m.ActiveCount(); n != 0 {
		t.Errorf("ActiveCount = %d after Stop, want 0", n)
	}
}

// --- Grace-period teardown (onIdle) ---------------------------------------------------
//
// These four cover the behaviour onIdle owes. They fail until it is implemented.

// The last viewer leaving must NOT stop the encoder immediately — that is what makes
// channel surfing cheap.
func TestOnIdle_EncoderSurvivesBrieflyAfterTheLastViewerLeaves(t *testing.T) {
	spawn, encoder := newFakeSpawner(t)
	m := testManager(t, spawn, 4, 10*time.Second)

	_, detach, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatal(err)
	}
	detach()

	select {
	case <-encoder("ch1").stopped:
		t.Fatal("the encoder stopped immediately; the grace period should keep it alive")
	case <-time.After(200 * time.Millisecond):
	}
}

// After the grace period with nobody watching, it must actually stop. Otherwise an
// abandoned channel burns a core forever.
func TestOnIdle_EncoderStopsAfterTheGracePeriod(t *testing.T) {
	spawn, encoder := newFakeSpawner(t)
	m := testManager(t, spawn, 4, 50*time.Millisecond)

	_, detach, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatal(err)
	}
	detach()

	select {
	case <-encoder("ch1").stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("the encoder never stopped after the grace period expired")
	}
}

// A viewer returning inside the grace window must ABORT the teardown. This is the bug in
// the naive `time.AfterFunc(grace, close)`: the timer fires and kills a channel someone is
// now watching.
func TestOnIdle_ReconnectInsideGraceAbortsTeardown(t *testing.T) {
	spawn, encoder := newFakeSpawner(t)
	m := testManager(t, spawn, 4, 300*time.Millisecond)

	_, detach, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatal(err)
	}
	detach()

	time.Sleep(50 * time.Millisecond) // still inside the grace window
	v, _, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatalf("reattach inside the grace window: %v", err)
	}

	// Well past the original deadline: the returning viewer must still be connected.
	time.Sleep(500 * time.Millisecond)
	select {
	case _, ok := <-v:
		if !ok {
			t.Fatal("the grace timer tore down a session that a viewer had rejoined")
		}
	default:
	}
	select {
	case <-encoder("ch1").stopped:
		t.Fatal("the encoder was stopped despite a viewer reconnecting inside the grace window")
	default:
	}
}

// The ABA problem. Viewers leave and rejoin repeatedly; each idle period arms a timer, and
// an early timer must not tear down a LATER session (or a later live viewer) just because
// it holds the same channel key.
func TestOnIdle_StaleTimerDoesNotKillALaterViewer(t *testing.T) {
	spawn, encoder := newFakeSpawner(t)
	m := testManager(t, spawn, 4, 200*time.Millisecond)

	// Three quick join/leave cycles, each arming a grace timer.
	for i := 0; i < 3; i++ {
		_, detach, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
		if err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
		time.Sleep(30 * time.Millisecond)
		detach()
		time.Sleep(30 * time.Millisecond)
	}

	// A viewer settles in for the long haul.
	v, _, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatal(err)
	}

	// Let every earlier timer's deadline pass.
	time.Sleep(600 * time.Millisecond)

	select {
	case _, ok := <-v:
		if !ok {
			t.Fatal("a stale grace timer disconnected a live viewer")
		}
	default:
	}
	select {
	case <-encoder("ch1").stopped:
		t.Fatal("a stale grace timer stopped an encoder that has a viewer")
	default:
	}
}

// A session torn down by the grace period must be removed from the manager, so the next
// viewer starts a fresh encoder rather than attaching to a dead one and waiting forever.
func TestOnIdle_TornDownSessionIsReplacedOnNextAttach(t *testing.T) {
	spawn, encoder := newFakeSpawner(t)
	m := testManager(t, spawn, 4, 50*time.Millisecond)

	_, detach, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatal(err)
	}
	detach()
	select {
	case <-encoder("ch1").stopped:
	case <-time.After(3 * time.Second):
		t.Fatal("encoder never stopped")
	}

	// A new viewer must get a working stream.
	v, _, err := m.Attach(context.Background(), "ch1", TargetMediaServer)
	if err != nil {
		t.Fatalf("attach after teardown: %v", err)
	}
	if _, err := encoder("ch1").w.Write([]byte("fresh")); err != nil {
		t.Fatal(err)
	}
	select {
	case got, ok := <-v:
		if !ok {
			t.Fatal("attached to a dead session")
		}
		if string(got) != "fresh" {
			t.Errorf("got %q, want %q", got, "fresh")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no bytes from the replacement encoder")
	}
}
