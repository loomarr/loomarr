package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestApplicationOwnsHandlerAndLifecycle(t *testing.T) {
	lifecycle := newGenerationLifecycle(t.Context())
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	application := &Application{handler: handler, lifecycle: lifecycle}

	response := httptest.NewRecorder()
	application.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("handler status = %d, want %d", response.Code, http.StatusNoContent)
	}

	workerStopped := make(chan struct{})
	lifecycle.goRun(func(ctx context.Context) {
		<-ctx.Done()
		close(workerStopped)
	})

	var mu sync.Mutex
	var order []string
	firstErr := errors.New("first stop")
	lifecycle.addStop(func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, "first")
		return firstErr
	})
	lifecycle.addStop(func(context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, "second")
		return nil
	})

	if err := application.Shutdown(t.Context()); !errors.Is(err, firstErr) {
		t.Fatalf("Shutdown error = %v, want %v", err, firstErr)
	}
	select {
	case <-workerStopped:
	default:
		t.Fatal("Shutdown returned before the tracked worker stopped")
	}
	mu.Lock()
	gotOrder := append([]string(nil), order...)
	mu.Unlock()
	if want := []string{"second", "first"}; !reflect.DeepEqual(gotOrder, want) {
		t.Fatalf("stop order = %v, want %v", gotOrder, want)
	}
	if err := application.Shutdown(t.Context()); !errors.Is(err, firstErr) {
		t.Fatalf("second Shutdown error = %v, want stable %v", err, firstErr)
	}
}

// A worker that ignores cancellation must not hold the generation open past the shutdown deadline:
// a restart brings up its successor then, so this one's resources are stopped regardless, and
// every later call reports the same outcome.
func TestApplicationShutdownStopsResourcesAtTheDeadlineDespiteAStuckWorker(t *testing.T) {
	lifecycle := newGenerationLifecycle(t.Context())
	application := &Application{lifecycle: lifecycle}
	lifecycle.stopBudget = 50 * time.Millisecond
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	lifecycle.goRun(func(context.Context) { <-release })
	stopped := make(chan struct{})
	lifecycle.addStop(func(context.Context) error { close(stopped); return nil })

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if err := application.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v, want deadline exceeded", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("the stop never ran: a stuck worker held the generation open past its deadline")
	}
	if err := application.Shutdown(t.Context()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Shutdown error = %v, want the first outcome (deadline exceeded)", err)
	}
}

// A probe a component starts on first use (the watermark self-check, encoder detection) is the
// generation's work: shutdown waits for it before stopping resources, and a closing generation
// starts none.
func TestApplicationShutdownJoinsOnDemandWorkAndRefusesItAfterClosing(t *testing.T) {
	lifecycle := newGenerationLifecycle(t.Context())
	application := &Application{lifecycle: lifecycle}
	var mu sync.Mutex
	var events []string
	record := func(event string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	}
	if !lifecycle.goOnDemand(func(ctx context.Context) {
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond)
		record("probe returned")
	}) {
		t.Fatal("an open generation refused on-demand work")
	}
	lifecycle.addStop(func(context.Context) error { record("resource stopped"); return nil })

	if err := application.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	mu.Lock()
	got := append([]string(nil), events...)
	mu.Unlock()
	if want := []string{"probe returned", "resource stopped"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("shutdown order = %v, want %v", got, want)
	}
	if lifecycle.goOnDemand(func(context.Context) { t.Error("on-demand work ran after shutdown") }) {
		t.Fatal("a closed generation accepted on-demand work")
	}
}

func TestApplicationQuiesceStopsNetworkResourcesBeforeFinalizers(t *testing.T) {
	lifecycle := newGenerationLifecycle(t.Context())
	application := &Application{lifecycle: lifecycle}

	workerStopped := make(chan struct{})
	lifecycle.goRun(func(ctx context.Context) {
		<-ctx.Done()
		close(workerStopped)
	})

	var mu sync.Mutex
	var events []string
	record := func(event string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	}
	lifecycle.addQuiesce(func(context.Context) error { record("first-quiescer"); return nil })
	lifecycle.addQuiesce(func(context.Context) error { record("second-quiescer"); return nil })
	lifecycle.addStop(func(context.Context) error { record("finalizer"); return nil })

	if err := application.Quiesce(t.Context()); err != nil {
		t.Fatalf("Quiesce: %v", err)
	}
	select {
	case <-workerStopped:
	case <-time.After(time.Second):
		t.Fatal("generation worker did not observe quiesce cancellation")
	}
	mu.Lock()
	got := append([]string(nil), events...)
	mu.Unlock()
	if want := []string{"second-quiescer", "first-quiescer"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events after quiesce = %v, want %v", got, want)
	}

	if err := application.Quiesce(t.Context()); err != nil {
		t.Fatalf("second Quiesce: %v", err)
	}
	if err := application.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	mu.Lock()
	got = append([]string(nil), events...)
	mu.Unlock()
	if want := []string{"second-quiescer", "first-quiescer", "finalizer"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("events after shutdown = %v, want %v", got, want)
	}
}

// A worker cancelled at quiesce may still be unwinding: recording its outcome, closing a job. The
// resources it records into (River, the packager, the diagnostics recorder) are torn down only once
// every worker has returned, so its last writes land instead of meeting a closed resource.
func TestApplicationShutdownStopsResourcesOnlyAfterItsWorkersReturn(t *testing.T) {
	lifecycle := newGenerationLifecycle(t.Context())
	application := &Application{lifecycle: lifecycle}

	var mu sync.Mutex
	var events []string
	record := func(event string) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, event)
	}
	lifecycle.goRun(func(ctx context.Context) {
		<-ctx.Done()
		time.Sleep(50 * time.Millisecond)
		record("worker returned")
	})
	lifecycle.addStop(func(context.Context) error { record("resource stopped"); return nil })

	if err := application.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if want := []string{"worker returned", "resource stopped"}; !reflect.DeepEqual(events, want) {
		t.Fatalf("shutdown order = %v, want %v", events, want)
	}
}

func TestApplicationQuiesceCanBeWaitedAgainAfterCallerTimeout(t *testing.T) {
	lifecycle := newGenerationLifecycle(t.Context())
	application := &Application{lifecycle: lifecycle}
	entered := make(chan struct{})
	release := make(chan struct{})
	lifecycle.addQuiesce(func(context.Context) error {
		close(entered)
		<-release
		return nil
	})

	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	first := make(chan error, 1)
	go func() { first <- application.Quiesce(ctx) }()
	<-entered
	if err := <-first; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first Quiesce error = %v, want deadline exceeded", err)
	}
	close(release)
	if err := application.Quiesce(t.Context()); err != nil {
		t.Fatalf("second Quiesce: %v", err)
	}
}

func TestApplicationQuiesceRejectsLateLifecycleRegistration(t *testing.T) {
	lifecycle := newGenerationLifecycle(t.Context())
	application := &Application{lifecycle: lifecycle}
	if err := application.Quiesce(t.Context()); err != nil {
		t.Fatal(err)
	}

	for name, register := range map[string]func(){
		"worker":    func() { lifecycle.goRun(func(context.Context) {}) },
		"quiescer":  func() { lifecycle.addQuiesce(func(context.Context) error { return nil }) },
		"finalizer": func() { lifecycle.addStop(func(context.Context) error { return nil }) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("late lifecycle registration did not panic")
				}
			}()
			register()
		})
	}
}

func TestApplicationShutdownOwnsInteractiveOperations(t *testing.T) {
	lifecycle := newGenerationLifecycle(t.Context())
	application := &Application{lifecycle: lifecycle}
	started := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})

	if err := lifecycle.startInteractiveOperation(time.Minute, func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-release
		return ctx.Err()
	}, nil); err != nil {
		t.Fatalf("start interactive operation: %v", err)
	}
	<-started

	shutdown := make(chan error, 1)
	go func() { shutdown <- application.Shutdown(t.Context()) }()
	<-cancelled
	select {
	case err := <-shutdown:
		t.Fatalf("Shutdown returned before the operation stopped: %v", err)
	default:
	}
	close(release)
	if err := <-shutdown; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	if err := lifecycle.startInteractiveOperation(time.Minute, func(context.Context) error { return nil }, nil); !errors.Is(err, ErrApplicationQuiescing) {
		t.Fatalf("late start error = %v, want %v", err, ErrApplicationQuiescing)
	}
	if err := application.Shutdown(t.Context()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
}

func TestApplicationShutdownWaitsForBoundedInteractiveOperationCompletion(t *testing.T) {
	lifecycle := newGenerationLifecycle(t.Context())
	application := &Application{lifecycle: lifecycle}
	working := make(chan struct{})
	completing := make(chan struct{})
	release := make(chan struct{})

	if err := lifecycle.startInteractiveOperation(time.Minute, func(ctx context.Context) error {
		close(working)
		<-ctx.Done()
		return ctx.Err()
	}, func(ctx context.Context, runErr error) {
		if !errors.Is(runErr, context.Canceled) {
			t.Errorf("operation error = %v, want cancellation", runErr)
		}
		if err := ctx.Err(); err != nil {
			t.Errorf("completion context already cancelled: %v", err)
		}
		close(completing)
		<-release
	}); err != nil {
		t.Fatalf("start interactive operation: %v", err)
	}
	<-working

	shutdown := make(chan error, 1)
	go func() { shutdown <- application.Shutdown(t.Context()) }()
	<-completing
	select {
	case err := <-shutdown:
		t.Fatalf("Shutdown returned before terminal completion: %v", err)
	default:
	}
	close(release)
	if err := <-shutdown; err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}
