package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loomarr/loomarr/internal/app/playoutresolve"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/store"
)

var ErrApplicationQuiescing = errors.New("application generation is quiescing")

const interactiveOperationCompletionTimeout = 5 * time.Second

// generationStopBudget is the one budget the registered stops (River's Stop, the store close, the
// recorder flush) share. It is theirs alone: a join that used up the shutdown context would
// otherwise hand every stop a dead one, and they degrade to forced when given one. Shutdown waits
// for the stops, so a generation's teardown never overlaps the next generation or the cutover.
const generationStopBudget = 10 * time.Second

// generationStopGrace is how long past the budget a stop that ignores its context is waited for
// before it is reported, so one stuck stop cannot hold Shutdown open.
const generationStopGrace = time.Second

// ShutdownTail is what Application.Shutdown keeps for itself at the end of the deadline it is
// given: one late wait (a stopBudget) for workers and quiescers that outlived the join, then the stop
// phase, whose one hard stop is a stopBudget plus a stopGrace after it begins. A caller gives
// Shutdown a deadline of (the time the workers get) + ShutdownTail, so the whole teardown, stops
// included, ends by it. A deadline shorter than the tail is the workers' alone, and the tail then
// runs past it. cmd/loomarr's generationFinalizeBudget and the compose stop_grace_period derive
// from this constant; the arithmetic is written out once, at teardownBudget in cmd/loomarr.
const ShutdownTail = 2*generationStopBudget + generationStopGrace

type interactiveOperationLauncher func(
	time.Duration,
	func(context.Context) error,
	func(context.Context, error),
) error

// Application is one fully wired Loomarr generation. The process owns the listener, signals,
// and store; Application owns the handler and every generation-scoped worker built behind it.
type Application struct {
	handler         http.Handler
	log             *slog.Logger
	lifecycle       *generationLifecycle
	playoutResolver *playoutresolve.Resolver
	serverPublicURL func() string
}

// Build constructs one application generation. If composition fails after starting any owned
// work, Build unwinds that partial generation before returning the construction error.
func Build(parent context.Context, st store.Store, log *slog.Logger, ov Overrides) (*Application, error) {
	lifecycle := newGenerationLifecycle(parent)
	var resolver *playoutresolve.Resolver
	handler, generationLog, serverPublicURL, err := buildHandler(lifecycle.ctx, records.Extend(st), log, ov, lifecycle, func(built *playoutresolve.Resolver) {
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
	lifecycle.startBuilt()
	return &Application{handler: handler, log: generationLog, lifecycle: lifecycle, playoutResolver: resolver, serverPublicURL: serverPublicURL}, nil
}

// ServerPublicURL returns the current validated address advertised to local unpaired clients.
func (a *Application) ServerPublicURL() string {
	if a == nil || a.serverPublicURL == nil {
		return ""
	}
	return a.serverPublicURL()
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

// Quiesce closes generation admission, cancels generation-owned work, and stops the narrow set of
// network resources whose active responses cannot finish without lifecycle intervention. It is
// safe to call repeatedly or concurrently and deliberately does not run ordinary finalizers.
func (a *Application) Quiesce(ctx context.Context) error {
	if a == nil || a.lifecycle == nil {
		return nil
	}
	return a.lifecycle.quiesce(ctx)
}

// Shutdown cancels generation-owned work, waits for tracked workers and quiescers, and runs explicit
// stops in reverse construction order, returning only once they have finished. The first call's
// context bounds the wait for workers and quiescers; one that outlives it is waited for one more
// bounded budget, shared by all of them (the stops must not begin beside it), and reported, and one
// stuck past that does not hold the stops back. The stops share one budget and one hard stop, so
// Shutdown outlasts ctx by at most ShutdownTail. It is safe to call repeatedly or concurrently; every
// call waits for, and reports, the same outcome.
func (a *Application) Shutdown(ctx context.Context) error {
	if a == nil || a.lifecycle == nil {
		return nil
	}
	return a.lifecycle.shutdown(ctx)
}

type generationLifecycle struct {
	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.Mutex
	closed      bool
	afterBuild  []func(context.Context) // workers held until composition finishes (goRunAfterBuild)
	quiescers   []func(context.Context) error
	stops       []func(context.Context) error
	wg          sync.WaitGroup
	running     atomic.Int64 // workers between track and untrack
	quiesceOnce sync.Once
	quiesced    chan struct{}
	quiesceErr  error
	stopOnce    sync.Once
	done        chan struct{}
	err         error
	// stopBudget is the one shared budget for the whole stop phase, whatever became of the shutdown
	// context. The wait for workers that outlived that context draws on a budget of the same size.
	stopBudget time.Duration
	// stopGrace is how long past the budget a stop is waited for to return before it is named.
	stopGrace time.Duration
}

func newGenerationLifecycle(parent context.Context) *generationLifecycle {
	ctx, cancel := context.WithCancel(parent)
	return &generationLifecycle{
		ctx: ctx, cancel: cancel, quiesced: make(chan struct{}), done: make(chan struct{}),
		stopBudget: generationStopBudget, stopGrace: generationStopGrace,
	}
}

// goRunAfterBuild registers a generation-owned worker that Build starts only once composition has
// finished (startBuilt). Work that reads components a later builder still configures (the channel
// engine's pods, the backend-transition checkpoint) must not start during construction: it would
// race those writes, or see them missing. A generation whose Build fails never starts it.
func (l *generationLifecycle) goRunAfterBuild(run func(context.Context)) {
	if run == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.afterBuild = append(l.afterBuild, run)
}

// startBuilt starts the workers held by goRunAfterBuild. Build calls it once, after composition.
func (l *generationLifecycle) startBuilt() {
	l.mu.Lock()
	runs := l.afterBuild
	l.afterBuild = nil
	l.mu.Unlock()
	for _, run := range runs {
		l.goRun(run)
	}
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
	l.track()
	l.mu.Unlock()
	go func() {
		defer l.untrack()
		run(l.ctx)
	}()
}

// goOnDemand starts generation-owned work that a component begins lazily, after construction: a
// probe the first tune or programme triggers. Shutdown joins it like any worker. Unlike goRun it
// refuses quietly once the generation is closing, reporting false, because a late request reaching
// the component then is ordinary, not a wiring fault.
func (l *generationLifecycle) goOnDemand(run func(context.Context)) bool {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return false
	}
	l.track()
	l.mu.Unlock()
	go func() {
		defer l.untrack()
		run(l.ctx)
	}()
	return true
}

// startInteractiveOperation accepts request-triggered work into this application generation.
// Callers cannot supply the request context: accepted work is intentionally independent of an
// HTTP disconnect and is instead bounded by the generation plus its operation-specific timeout.
func (l *generationLifecycle) startInteractiveOperation(
	timeout time.Duration,
	run func(context.Context) error,
	complete func(context.Context, error),
) error {
	if run == nil {
		return nil
	}
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return ErrApplicationQuiescing
	}
	l.track()
	l.mu.Unlock()
	go func() {
		defer l.untrack()
		ctx := l.ctx
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		runErr := run(ctx)
		if complete == nil {
			return
		}
		completionCtx, completionCancel := context.WithTimeout(
			context.WithoutCancel(l.ctx), interactiveOperationCompletionTimeout,
		)
		defer completionCancel()
		complete(completionCtx, runErr)
	}()
	return nil
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

// addQuiesce registers a network-facing resource that must stop before HTTP drain can wait for
// active responses. Reverse order preserves the dependency order established during construction.
func (l *generationLifecycle) addQuiesce(quiesce func(context.Context) error) {
	if quiesce == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		panic("app: register generation quiescer after shutdown")
	}
	l.quiescers = append(l.quiescers, quiesce)
}

func (l *generationLifecycle) quiesce(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	l.startQuiesce(ctx)
	if settled(ctx, l.quiesced) {
		return l.quiesceErr
	}
	return ctx.Err()
}

func (l *generationLifecycle) startQuiesce(ctx context.Context) {
	l.quiesceOnce.Do(func() {
		l.mu.Lock()
		l.closed = true
		quiescers := append([]func(context.Context) error(nil), l.quiescers...)
		l.mu.Unlock()
		l.cancel()
		go l.finishQuiesce(ctx, quiescers)
	})
}

func (l *generationLifecycle) finishQuiesce(ctx context.Context, quiescers []func(context.Context) error) {
	var errs []error
	for _, quiescer := range slices.Backward(quiescers) {
		if err := quiescer(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	l.quiesceErr = errors.Join(errs...)
	close(l.quiesced)
}

func (l *generationLifecycle) shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	l.stopOnce.Do(func() {
		l.startQuiesce(ctx)
		l.mu.Lock()
		stops := append([]func(context.Context) error(nil), l.stops...)
		l.mu.Unlock()
		go l.finish(ctx, stops)
	})

	// Shutdown is the barrier the next generation, the store close and the database cutover stand
	// behind, so it returns only once the stops have finished. Every phase of finish is bounded
	// (the join by ctx, the late-worker wait and the stops by their budgets), so this wait is too.
	<-l.done
	return l.err
}

func (l *generationLifecycle) finish(ctx context.Context, stops []func(context.Context) error) {
	var errs []error
	// Every worker runs on the generation's context, cancelled at quiesce, so each is unwinding now,
	// and the quiescers are closing the network resources. Both feed the resources below, so the
	// stops begin once they have returned, or at the deadline: a restart brings up its successor
	// then, and this one's resources must not outlive it.
	joinCtx := ctx
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) > l.tail() {
		// A caller that sized its deadline to include the tail (see ShutdownTail) is giving the
		// workers what is left once it is set aside, so the whole of Shutdown ends by its deadline.
		var cancel context.CancelFunc
		joinCtx, cancel = context.WithDeadline(ctx, deadline.Add(-l.tail()))
		defer cancel()
	}
	quiesced := l.quiescersSettled(joinCtx)
	joinErr := l.waitWorkers(joinCtx)
	if !quiesced || joinErr != nil {
		// The caller's deadline passed with a quiescer or a worker still running. A stop must not
		// begin beside either (River's Stop and the store close race a worker's last writes, and a
		// quiescer is closing a network resource), so they get one more bounded wait. It is one
		// budget for both, so the stops' start is late by at most stopBudget whichever is stuck.
		// Both are reported either way: one stuck past it is the only thing that starts the stops
		// beside it.
		lateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), l.stopBudget)
		if !quiesced {
			errs = append(errs, fmt.Errorf("quiesce unfinished at the shutdown deadline: %w", ctx.Err()))
			if l.quiescersSettled(lateCtx) {
				quiesced = true
			} else {
				errs = append(errs, fmt.Errorf("quiesce still unfinished %v later: %w", l.stopBudget, lateCtx.Err()))
			}
		}
		if joinErr != nil {
			if lateErr := l.waitWorkers(lateCtx); lateErr != nil {
				joinErr = errors.Join(joinErr, lateErr)
			}
		}
		cancel()
	}
	if quiesced {
		// Read only once the quiescers have all returned, which is what orders this read after the
		// write in finishQuiesce.
		errs = append(errs, l.quiesceErr)
	}
	errs = append(errs, joinErr)
	// The stops do not inherit what is left of ctx, which the join may have spent entirely: they
	// share one live budget of their own, so River's Stop and the store close are not forced by a
	// join that ran long. A slow stop spends the shared budget; the rest still run, on the spent
	// context, which they treat as a request to be quick. The phase has one hard stop, the budget
	// plus one grace, whatever the stops do.
	stopCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), l.stopBudget)
	defer cancel()
	hardStop := time.Now().Add(l.stopBudget + l.stopGrace)
	for _, stop := range slices.Backward(stops) {
		errs = append(errs, l.runStop(stopCtx, hardStop, stop))
	}
	l.err = errors.Join(errs...)
	close(l.done)
}

// quiescersSettled reports whether every quiescer has returned, waiting for them only until ctx ends.
func (l *generationLifecycle) quiescersSettled(ctx context.Context) bool {
	return settled(ctx, l.quiesced)
}

// tail is how long Shutdown can run past the workers' join: one late wait for a stuck worker or
// quiescer, then the stop phase up to its hard stop (the budget plus one grace).
func (l *generationLifecycle) tail() time.Duration { return 2*l.stopBudget + l.stopGrace }

// runStop runs one stop and waits for it to return, until hardStop. A stop that honours its
// context is waited for however slowly it unwinds, up to that instant; one still running then is
// named, so no number of stuck stops can hold Shutdown open past the phase's one hard stop, and the
// Shutdown caller is told the stop may still be running. A stop reached after hardStop is started
// and named without a wait.
func (l *generationLifecycle) runStop(stopCtx context.Context, hardStop time.Time, stop func(context.Context) error) error {
	result := make(chan error, 1)
	go func() { result <- stop(stopCtx) }()
	timer := time.NewTimer(time.Until(hardStop))
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-timer.C:
	}
	select {
	case err := <-result:
		return err
	default:
		return fmt.Errorf("stop still running at the end of the stop phase (%v budget plus %v grace): %w",
			l.stopBudget, l.stopGrace, context.DeadlineExceeded)
	}
}

// waitWorkers waits for the generation's workers until ctx ends. A worker still running then is
// reported, not waited for; workers that have returned are never reported, whatever became of ctx.
func (l *generationLifecycle) waitWorkers(ctx context.Context) error {
	returned := make(chan struct{})
	go func() {
		l.wg.Wait()
		close(returned)
	}()
	if settled(ctx, returned) || l.running.Load() == 0 {
		return nil
	}
	return fmt.Errorf("generation workers still running at the shutdown deadline: %w", ctx.Err())
}

// settled reports whether done is closed, waiting for it only until ctx ends. When both are ready
// it is done that wins: a deadline that passed as the work finished is not a failure, and a select
// over the two would pick one at random.
func settled(ctx context.Context, done <-chan struct{}) bool {
	select {
	case <-done:
		return true
	case <-ctx.Done():
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

// track and untrack bracket one worker's life. The WaitGroup is what the join waits on; the count
// lets the join tell, without waiting, that no worker is left when its deadline has already passed.
func (l *generationLifecycle) track() {
	l.wg.Add(1)
	l.running.Add(1)
}

func (l *generationLifecycle) untrack() {
	l.running.Add(-1)
	l.wg.Done()
}
