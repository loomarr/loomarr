package playout

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"
)

func TestStartFailureNamesTheBlockSourceFault(t *testing.T) {
	dial := &url.Error{Op: "Get", URL: "http://x", Err: &net.OpError{Op: "dial", Err: errors.New("i/o timeout")}}
	cases := []struct {
		name      string
		open      error
		fallback  StartReason
		want      StartReason
		wantCause bool
	}{
		{"unreachable source outranks the symptom", dial, StartNoStream, StartProgramSourceUnreachable, true},
		{"endpoint refusal", errors.New("playout: block endpoint returned 404 Not Found"), StartNoStream, StartProgramSourceFailed, true},
		{"prepared miss is control flow", ErrPreparedUnavailable, StartNoStream, StartNoStream, false},
		{"cancellation is control flow", context.Canceled, StartEncoderExited, StartEncoderExited, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Process{}
			p.noteBlockOpen(tc.open)
			err := p.startFailure(tc.fallback, fmt.Errorf("produced no stream"))
			var start *StartError
			if !errors.As(err, &start) || start.Reason != tc.want {
				t.Fatalf("reason = %v, want %s", err, tc.want)
			}
			if tc.wantCause && !errors.Is(err, tc.open) {
				t.Fatalf("typed error dropped its cause: %v", err)
			}
		})
	}
}

func TestStartFailureClearsAfterABlockOpens(t *testing.T) {
	p := &Process{}
	p.noteBlockOpen(errors.New("boom"))
	p.noteBlockOpen(nil)
	var start *StartError
	if err := p.startFailure(StartNoStream, errors.New("late")); !errors.As(err, &start) || start.Reason != StartNoStream {
		t.Fatalf("a recovered source must not be blamed: %v", err)
	}
}

// A tune that never sees a segment must surface the recorded source fault, not a bare timeout.
func TestAwaitPlaylistReportsTheBlockSourceFault(t *testing.T) {
	source := &Process{}
	source.noteBlockOpen(&url.Error{Op: "Get", URL: "http://x", Err: &net.OpError{Op: "dial", Err: errors.New("i/o timeout")}})
	r := &hlsRemux{
		source: source, channelID: "ch", ctx: context.Background(),
		playlist: t.TempDir() + "/missing.m3u8", proc: &hlsProcess{},
	}
	err := r.awaitPlaylist(context.Background(), 150*time.Millisecond)
	var start *StartError
	if !errors.As(err, &start) || start.Reason != StartProgramSourceUnreachable {
		t.Fatalf("awaitPlaylist error = %v, want a program-source-unreachable StartError", err)
	}
}

// Once block-open retries for the first block are exhausted the tune fails at once; waiting out
// the whole first-segment deadline only makes the viewer stare at "Tuning in" for a known fault.
func TestAwaitPlaylistFailsEarlyOnceBlockOpenRetriesAreExhausted(t *testing.T) {
	refused := &url.Error{Op: "Get", URL: "http://x", Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}
	source := &Process{}
	done := make(chan struct{})
	detached := false
	r := &hlsRemux{
		// The fake encoder stays alive until teardown cancels it, so only the exhausted-retries
		// check (not the encoder-exited branch) can end the wait early.
		source: source, channelID: "ch", ctx: context.Background(), cancel: func() { close(done) },
		relay: newHLSRelay(), dir: t.TempDir(), sessDetach: func() { detached = true },
		playlist: t.TempDir() + "/missing.m3u8", proc: &hlsProcess{done: done},
	}
	for range blockOpenAttemptsBeforeFailingTune {
		source.noteBlockOpen(refused)
	}

	started := time.Now()
	err := r.awaitPlaylist(context.Background(), 30*time.Second)
	var start *StartError
	if !errors.As(err, &start) || start.Reason != StartProgramSourceUnreachable {
		t.Fatalf("awaitPlaylist error = %v, want a program-source-unreachable StartError", err)
	}
	if elapsed := time.Since(started); elapsed > 2*time.Second {
		t.Fatalf("awaitPlaylist took %s; it should fail as soon as retries are exhausted", elapsed)
	}
	if !detached {
		t.Fatal("the failed remux must be torn down so a retry starts a fresh session")
	}
}

func TestAwaitPlaylistKeepsWaitingWhileBlockOpenRetriesRemain(t *testing.T) {
	refused := &url.Error{Op: "Get", URL: "http://x", Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}
	source := &Process{}
	source.noteBlockOpen(refused)
	r := &hlsRemux{
		source: source, channelID: "ch", ctx: context.Background(),
		playlist: t.TempDir() + "/missing.m3u8", proc: &hlsProcess{},
	}
	started := time.Now()
	_ = r.awaitPlaylist(context.Background(), 400*time.Millisecond)
	if elapsed := time.Since(started); elapsed < 350*time.Millisecond {
		t.Fatalf("awaitPlaylist gave up after %s with retries still remaining", elapsed)
	}
}
