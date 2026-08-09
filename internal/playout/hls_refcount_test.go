package playout

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeAttacher stands in for the session Manager, counting how many times Attach is called so a
// test can assert the "N HLS viewers share ONE session" invariant.
type fakeAttacher struct {
	attaches atomic.Int32
	detaches atomic.Int32
	mu       sync.Mutex
	chans    []chan []byte
}

func (f *fakeAttacher) Attach(ctx context.Context, channelID string, target EncodePlan) (<-chan []byte, func(), error) {
	f.attaches.Add(1)
	ch := make(chan []byte)
	f.mu.Lock()
	f.chans = append(f.chans, ch)
	f.mu.Unlock()
	return ch, func() { f.detaches.Add(1) }, nil
}

// newTestHLSManager builds a manager whose ffmpeg spawn is faked: it writes a stub master
// playlist so awaitPlaylist succeeds, and returns a real-but-trivial process (a `true` command)
// so teardown's Wait has something to reap.
func newTestHLSManager(t *testing.T, att HLSAttacher) *HLSManager {
	t.Helper()
	m, err := NewHLSManager(att, "ffmpeg", t.TempDir(), 20*time.Millisecond, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	m.spawn = func(ctx context.Context, bin, dir string, _ EncodePlan, log *slog.Logger) (*hlsProcess, error) {
		// Write a stub playlist that REFERENCES A SEGMENT — awaitPlaylist waits for a `.ts` line
		// (the readiness signal), so a header-only stub would (correctly) never be considered ready.
		stub := "#EXTM3U\n#EXT-X-VERSION:6\n#EXTINF:4.0,\nseg-0.ts\n"
		if werr := os.WriteFile(filepath.Join(dir, hlsPlaylistName), []byte(stub), 0o644); werr != nil {
			return nil, werr
		}
		cmd := exec.CommandContext(ctx, "sleep", "3600")
		stdin, _ := cmd.StdinPipe()
		if serr := cmd.Start(); serr != nil {
			return nil, serr
		}
		return &hlsProcess{cmd: cmd, stdin: stdin}, nil
	}
	t.Cleanup(m.Stop)
	return m
}

// newTestHLSManagerWithPlaylist is newTestHLSManager with the stub playlist supplied by the
// caller, so a test can drive the REAL readiness predicate with a real playlist body.
func newTestHLSManagerWithPlaylist(t *testing.T, att HLSAttacher, stub string) *HLSManager {
	t.Helper()
	m, err := NewHLSManager(att, "ffmpeg", t.TempDir(), 20*time.Millisecond, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	// The negative case has to let the wait EXPIRE, so the production 45s would make this a
	// 45-second unit test. Short enough to be quick, long enough that the poll (100ms) runs
	// several times before giving up — a timeout below one poll interval would pass for the wrong
	// reason, never having read the file at all.
	m.readyTimeout = 600 * time.Millisecond
	m.spawn = func(ctx context.Context, bin, dir string, _ EncodePlan, log *slog.Logger) (*hlsProcess, error) {
		if werr := os.WriteFile(filepath.Join(dir, hlsPlaylistName), []byte(stub), 0o644); werr != nil {
			return nil, werr
		}
		cmd := exec.CommandContext(ctx, "sleep", "3600")
		stdin, _ := cmd.StdinPipe()
		if serr := cmd.Start(); serr != nil {
			return nil, serr
		}
		return &hlsProcess{cmd: cmd, stdin: stdin}, nil
	}
	t.Cleanup(m.Stop)
	return m
}

// READINESS MUST NOT NAME A CONTAINER (§9.1 V48).
//
// The HEVC plans emit fragmented MP4 — `seg-N.m4s` behind an `#EXT-X-MAP:URI="init.mp4"` — so
// their playlist contains no `.ts` anywhere. The readiness predicate matched the literal `.ts`,
// which meant awaitPlaylist could never see an fMP4 playlist become ready: every HEVC-capable
// client waited out the 45s timeout and received a 502, on the exact path V48 added to fix the
// HEVC black screen.
//
// The bodies below are what ffmpeg actually writes for each `-hls_segment_type`, so this fails
// against a container-specific predicate and passes only against a structural one.
func TestHLSManager_ReadyForBothSegmentContainers(t *testing.T) {
	const tsPlaylist = "#EXTM3U\n#EXT-X-VERSION:6\n#EXT-X-TARGETDURATION:4\n#EXTINF:4.000000,\nseg-0.ts\n"
	const fmp4Playlist = "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:4\n" +
		"#EXT-X-MAP:URI=\"init.mp4\"\n#EXTINF:4.000000,\nseg-0.m4s\n"

	for _, tc := range []struct {
		name string
		plan EncodePlan
		body string
	}{
		{"mpegts", PlanBaseline, tsPlaylist},
		{"fmp4", PlanHEVC10, fmp4Playlist},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestHLSManagerWithPlaylist(t, &fakeAttacher{}, tc.body)
			path, detach, err := m.Playlist("ch1", tc.plan)
			if err != nil {
				t.Fatalf("channel never became ready: %v — a client on this plan gets a 502", err)
			}
			defer detach()
			if path == "" {
				t.Error("ready but no playlist path")
			}
		})
	}
}

// A HEADER-ONLY playlist must still NOT be ready — the stall the readiness gate exists to prevent.
//
// This is the other half of the predicate, and the reason it matches `#EXTINF` rather than
// anything looser: an fMP4 playlist carries `#EXT-X-MAP` with the init segment BEFORE any media
// segment exists, so a predicate keyed on that tag would call this ready and hand hls.js a
// playlist with nothing to fetch.
func TestHLSManager_HeaderOnlyPlaylistIsNotReady(t *testing.T) {
	const headerOnly = "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:4\n#EXT-X-MAP:URI=\"init.mp4\"\n"

	m := newTestHLSManagerWithPlaylist(t, &fakeAttacher{}, headerOnly)
	if _, detach, err := m.Playlist("ch1", PlanHEVC10); err == nil {
		if detach != nil {
			detach()
		}
		t.Fatal("a header-only playlist was reported ready — hls.js would parse it, find no media and stall")
	}
}

// The core invariant: three browser viewers of one channel share ONE remux and therefore ONE
// session Attach — three tabs cost one encoder, exactly like three TVs (§9.1).
func TestHLSManager_ViewersShareOneRemux(t *testing.T) {
	att := &fakeAttacher{}
	m := newTestHLSManager(t, att)

	var detaches []func()
	for i := 0; i < 3; i++ {
		_, d, err := m.Playlist("ch1", PlanBaseline)
		if err != nil {
			t.Fatalf("viewer %d: %v", i, err)
		}
		detaches = append(detaches, d)
	}

	if got := att.attaches.Load(); got != 1 {
		t.Fatalf("3 viewers caused %d session attaches, want 1 (shared encoder)", got)
	}
	for _, d := range detaches {
		d()
	}
}

// After the last viewer leaves and the grace window elapses, the remux tears down and releases
// its single session refcount — so an abandoned channel stops encoding.
func TestHLSManager_TeardownReleasesSessionAfterGrace(t *testing.T) {
	att := &fakeAttacher{}
	m := newTestHLSManager(t, att)

	_, detach, err := m.Playlist("ch1", PlanBaseline)
	if err != nil {
		t.Fatal(err)
	}
	detach()

	// Grace is 20ms in the test manager; give it room to fire.
	deadline := time.Now().Add(2 * time.Second)
	for att.detaches.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if att.detaches.Load() != 1 {
		t.Fatalf("session was not detached after the last viewer + grace (detaches=%d)", att.detaches.Load())
	}
}

// A viewer arriving within the grace window keeps the remux alive — no fresh Attach, no
// interruption for the people (re)joining.
func TestHLSManager_RejoinWithinGraceKeepsOneAttach(t *testing.T) {
	att := &fakeAttacher{}
	m := newTestHLSManager(t, att)

	_, d1, err := m.Playlist("ch1", PlanBaseline)
	if err != nil {
		t.Fatal(err)
	}
	d1() // last viewer leaves; grace timer arms

	// Re-join immediately, well within the 20ms grace.
	_, d2, err := m.Playlist("ch1", PlanBaseline)
	if err != nil {
		t.Fatal(err)
	}
	defer d2()

	if got := att.attaches.Load(); got != 1 {
		t.Fatalf("a rejoin within grace caused %d attaches, want 1", got)
	}
}
