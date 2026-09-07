package playoutcert

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/playoutcertfixture"
)

func TestSyntheticCloseJoinsOneOperationAndPreservesResult(t *testing.T) {
	terminal := errors.New("listener close failed")
	release := make(chan struct{})
	listener := playoutcertfixture.NewGatedListener(release, terminal)
	target := &SyntheticTarget{BaseURL: "http://127.0.0.1:9999", scope: "isolated", listener: listener}

	const callers = 8
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- target.Close(context.Background())
		}()
	}
	waitSignal(t, listener.Started, "listener Close")
	if got := listener.Calls.Load(); got != 1 {
		t.Fatalf("listener Close calls while blocked = %d, want 1", got)
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, terminal) {
			t.Fatalf("concurrent Close error = %v, want %v", err, terminal)
		}
	}
	if err := target.Close(context.Background()); !errors.Is(err, terminal) {
		t.Fatalf("repeated Close error = %v, want %v", err, terminal)
	}
	if got := listener.Calls.Load(); got != 1 {
		t.Fatalf("listener Close calls = %d, want 1", got)
	}
}

func TestSyntheticCloseCancellationBoundsOnlyCallerWait(t *testing.T) {
	terminal := errors.New("eventual disposal failed")
	release := make(chan struct{})
	listener := playoutcertfixture.NewGatedListener(release, terminal)
	target := &SyntheticTarget{BaseURL: "http://127.0.0.1:9999", scope: "isolated", listener: listener}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- target.Close(ctx) }()
	waitSignal(t, listener.Started, "listener Close")
	cancel()
	if err := waitError(t, result, "cancelled Close"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Close error = %v, want context canceled", err)
	}
	close(release)
	if err := target.Close(context.Background()); !errors.Is(err, terminal) {
		t.Fatalf("joined Close error = %v, want %v", err, terminal)
	}
}

func TestSyntheticShutdownCancellationDoesNotResetOrFabricateReceipt(t *testing.T) {
	release := make(chan struct{})
	listener := playoutcertfixture.NewGatedListener(release, nil)
	target := &SyntheticTarget{BaseURL: "http://127.0.0.1:9999", scope: "isolated", listener: listener}
	request := ShutdownRequest{BaseURL: target.BaseURL}
	if receipt, err := target.Shutdown(context.Background(), ShutdownRequest{BaseURL: "http://127.0.0.1:9998"}); err == nil || receipt != (ShutdownReceipt{}) {
		t.Fatalf("mismatched Shutdown = %+v, %v", receipt, err)
	}
	if got := listener.Calls.Load(); got != 0 {
		t.Fatalf("mismatched Shutdown started %d stops", got)
	}

	ctx, cancel := context.WithCancel(context.Background())
	type shutdownResult struct {
		receipt ShutdownReceipt
		err     error
	}
	result := make(chan shutdownResult, 1)
	go func() {
		receipt, err := target.Shutdown(ctx, request)
		result <- shutdownResult{receipt: receipt, err: err}
	}()
	waitSignal(t, listener.Started, "shutdown listener Close")
	cancel()
	got := waitValue(t, result, "cancelled Shutdown")
	if !errors.Is(got.err, context.Canceled) || got.receipt != (ShutdownReceipt{}) {
		t.Fatalf("cancelled Shutdown = %+v, %v", got.receipt, got.err)
	}
	close(release)
	receipt, err := target.Shutdown(context.Background(), request)
	if err != nil || receipt.Scope != target.scope || !receipt.ServingStopped || !receipt.ProcessesExited {
		t.Fatalf("joined Shutdown = %+v, %v", receipt, err)
	}
	alreadyCancelled, cancelAgain := context.WithCancel(context.Background())
	cancelAgain()
	completed, err := target.Shutdown(alreadyCancelled, request)
	if err != nil || completed != receipt {
		t.Fatalf("completed Shutdown with expired caller = %+v, %v", completed, err)
	}
	if got := listener.Calls.Load(); got != 1 {
		t.Fatalf("listener Close calls = %d, want 1", got)
	}
}

func TestSyntheticStoppedSamplingHonorsLifecycleAndJoinsDisposal(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	target, err := NewSyntheticTarget(ctx, SyntheticConfig{
		Channels: []Channel{{ID: "lifecycle-live", Roles: []string{"transcode_h264", "audio_aac"}}},
		FFmpeg:   ffmpeg, Capacity: 1, Grace: time.Second, ProgrammeDuration: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.SampleStopped(context.Background(), "pre-stop"); err == nil || !strings.Contains(err.Error(), "before stop") {
		t.Fatalf("pre-stop SampleStopped error = %v", err)
	}
	if _, err := target.Shutdown(ctx, ShutdownRequest{BaseURL: target.BaseURL}); err != nil {
		t.Fatal(err)
	}

	actualHandler := target.handler
	cancelRelease := make(chan struct{})
	cancelGate := playoutcertfixture.NewGatedHandler(actualHandler, cancelRelease)
	target.handler = cancelGate
	sampleCtx, sampleCancel := context.WithCancel(context.Background())
	cancelledSample := make(chan error, 1)
	go func() {
		_, sampleErr := target.SampleStopped(sampleCtx, "cancelled")
		cancelledSample <- sampleErr
	}()
	waitSignal(t, cancelGate.Started, "retained sample")
	sampleCancel()
	if err := waitError(t, cancelledSample, "cancelled SampleStopped"); err == nil {
		t.Fatal("cancelled SampleStopped succeeded")
	}
	close(cancelRelease)

	joinRelease := make(chan struct{})
	joinGate := playoutcertfixture.NewGatedHandler(actualHandler, joinRelease)
	target.handler = joinGate
	sampleResult := make(chan error, 1)
	go func() {
		_, sampleErr := target.SampleStopped(context.Background(), "final")
		sampleResult <- sampleErr
	}()
	waitSignal(t, joinGate.Started, "in-flight retained sample")
	closeResult := make(chan error, 1)
	go func() { closeResult <- target.Close(context.Background()) }()
	select {
	case err := <-closeResult:
		t.Fatalf("Close completed before retained sample: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(joinRelease)
	if err := waitError(t, sampleResult, "retained SampleStopped"); err != nil {
		t.Fatalf("retained SampleStopped error = %v", err)
	}
	if err := waitError(t, closeResult, "joined Close"); err != nil {
		t.Fatalf("Close error = %v", err)
	}
	if _, err := target.SampleStopped(context.Background(), "post-disposal"); err == nil || !strings.Contains(err.Error(), "after disposal") {
		t.Fatalf("post-disposal SampleStopped error = %v", err)
	}
}

func waitSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func waitError(t *testing.T, result <-chan error, name string) error {
	t.Helper()
	return waitValue(t, result, name)
}

func waitValue[T any](t *testing.T, result <-chan T, name string) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(2 * time.Second):
		var zero T
		t.Fatalf("timed out waiting for %s", name)
		return zero
	}
}
