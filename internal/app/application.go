package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/store"
)

// Application is one fully wired Loomarr generation. The process owns the listener, signals,
// and store; Application owns the handler and every generation-scoped worker built behind it.
type Application struct {
	handler         http.Handler
	log             *slog.Logger
	lifecycle       *generationLifecycle
	playoutResolver *playoutResolver
}

// Build constructs one application generation. If composition fails after starting any owned
// work, Build unwinds that partial generation before returning the construction error.
func Build(parent context.Context, st store.Store, log *slog.Logger, ov Overrides) (*Application, error) {
	lifecycle := newGenerationLifecycle(parent)
	var resolver *playoutResolver
	handler, generationLog, err := buildHandler(lifecycle.ctx, st, log, ov, lifecycle, func(built *playoutResolver) {
		resolver = built
	})
	if err != nil {
		if ov.Startup != nil {
			ov.Startup.Complete(diagnostics.StartupCheckHTTP, diagnostics.StartupFailed,
				"application HTTP assembly failed", "/settings/system/diagnostics", "")
			ov.Startup.CompletePending(diagnostics.StartupSkipped, "application assembly stopped")
		}
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return nil, errors.Join(err, lifecycle.shutdown(shutdownCtx))
	}
	return &Application{handler: handler, log: generationLog, lifecycle: lifecycle, playoutResolver: resolver}, nil
}

// Logger returns the generation's logger. Once a store-backed generation is built, this is the
// redacted stdout-plus-diagnostics logger; store-less builds retain the supplied stdout logger.
func (a *Application) Logger() *slog.Logger {
	if a == nil {
		return nil
	}
	return a.log
}

// Handler returns the generation's immutable HTTP entry point.
func (a *Application) Handler() http.Handler {
	if a == nil {
		return nil
	}
	return a.handler
}

// Shutdown cancels generation-owned work, runs explicit stops in reverse construction order,
// and waits for tracked workers. It is safe to call repeatedly or concurrently. A caller whose
// context expires may call again with a fresh context to continue waiting for the same shutdown.
func (a *Application) Shutdown(ctx context.Context) error {
	if a == nil || a.lifecycle == nil {
		return nil
	}
	return a.lifecycle.shutdown(ctx)
}

type generationLifecycle struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu       sync.Mutex
	closed   bool
	stops    []func(context.Context) error
	wg       sync.WaitGroup
	stopOnce sync.Once
	done     chan struct{}
	err      error
}

func newGenerationLifecycle(parent context.Context) *generationLifecycle {
	ctx, cancel := context.WithCancel(parent)
	return &generationLifecycle{ctx: ctx, cancel: cancel, done: make(chan struct{})}
}

// goRun starts one generation-owned worker. Construction is single-threaded, but the closed
// guard makes an accidental late start fail immediately instead of escaping Shutdown's wait.
func (l *generationLifecycle) goRun(run func(context.Context)) {
	if run == nil {
		return
	}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		panic("app: start generation worker after shutdown")
	}
	l.wg.Add(1)
	l.mu.Unlock()
	go func() {
		defer l.wg.Done()
		run(l.ctx)
	}()
}

// addStop registers teardown for a resource that is not represented by a worker function.
// Reverse order preserves the dependency order established during construction.
func (l *generationLifecycle) addStop(stop func(context.Context) error) {
	if stop == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		panic("app: register generation stop after shutdown")
	}
	l.stops = append(l.stops, stop)
}

func (l *generationLifecycle) shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	l.stopOnce.Do(func() {
		l.mu.Lock()
		l.closed = true
		stops := append([]func(context.Context) error(nil), l.stops...)
		l.mu.Unlock()
		l.cancel()
		go l.finish(ctx, stops)
	})

	select {
	case <-l.done:
		return l.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (l *generationLifecycle) finish(ctx context.Context, stops []func(context.Context) error) {
	var stopErrs []error
	for i := len(stops) - 1; i >= 0; i-- {
		if err := stops[i](ctx); err != nil {
			stopErrs = append(stopErrs, err)
		}
	}
	l.wg.Wait()
	l.err = errors.Join(stopErrs...)
	close(l.done)
}
