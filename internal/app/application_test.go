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

func TestApplicationShutdownCanBeWaitedAgainAfterCallerTimeout(t *testing.T) {
	lifecycle := newGenerationLifecycle(t.Context())
	application := &Application{lifecycle: lifecycle}
	release := make(chan struct{})
	lifecycle.goRun(func(context.Context) { <-release })

	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()
	if err := application.Shutdown(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Shutdown error = %v, want deadline exceeded", err)
	}
	close(release)
	if err := application.Shutdown(t.Context()); err != nil {
		t.Fatalf("second Shutdown: %v", err)
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
