package backendtransition

import (
	"context"
	"fmt"
	"sync"
)

// Fleet prepares the durable local state of every active channel that inherits the global
// backend. Pinned and inactive channels remain the Fleet implementation's responsibility; the
// controller deliberately knows nothing about channel records.
type Fleet interface {
	PrepareInheritedBackend(ctx context.Context, target string) error
}

// Publisher owns the media-server tuner/listing pair. Prepare makes the target registration
// available without retiring the currently applied one; Refresh makes the target readable;
// RetireStale removes registrations other than target only after publication is durable.
//
// Every method must be idempotent. In particular Refresh is called even when Prepare reports no
// change: a prior process may have crashed after creating the registration but before refreshing
// it, and the durable state intentionally does not pretend that volatile phase completed.
type Publisher interface {
	Prepare(ctx context.Context, target string) (changed bool, err error)
	Refresh(ctx context.Context, target string) error
	RetireStale(ctx context.Context, target string) error
}

// Cutover performs the last local action immediately before publication, such as stopping live
// sessions owned by the previously applied backend. It is optional and must be idempotent: a
// process can crash after BeforePublish succeeds but before the published checkpoint is saved.
type Cutover interface {
	BeforePublish(ctx context.Context, from, to string) error
}

// Snapshot is one immutable runtime view of the durable publication checkpoint.
type Snapshot struct {
	Applied           string
	Prepared          string
	PublishedInternal bool
}

// Runtime exposes the checkpoint to hot request paths without a store read. Its zero value is
// safe. The controller publishes only states it loaded from durable storage or successfully saved;
// a failed checkpoint write never leaks an uncommitted transport decision into runtime behavior.
type Runtime struct {
	mu       sync.RWMutex
	snapshot Snapshot
}

// Snapshot returns a copy that cannot change underneath the caller.
func (r *Runtime) Snapshot() Snapshot {
	if r == nil {
		return Snapshot{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.snapshot
}

func (r *Runtime) publish(state State) {
	r.mu.Lock()
	r.snapshot = Snapshot{
		Applied:           state.Applied(),
		Prepared:          state.Prepared(),
		PublishedInternal: state.PublishedInternal(),
	}
	r.mu.Unlock()
}

// Controller owns the complete prepare -> publish -> retire transition. Apply calls are
// serialized within this process and always reload the durable checkpoint, so a new Controller
// resumes exactly where a crashed one stopped. Store writes are not CAS; cross-process leadership
// remains the composition layer's responsibility, matching the State persistence contract.
type Controller struct {
	mu sync.Mutex

	store     stateStore
	fleet     Fleet
	publisher Publisher
	cutover   Cutover
	runtime   Runtime
}

// NewController constructs a transition controller. Fleet and Publisher are required for a real
// transition; Cutover is optional. Dependencies are accepted rather than constructed so the same
// ordering is exercised by production adapters and tests.
func NewController(st stateStore, fleet Fleet, publisher Publisher, cutover Cutover) *Controller {
	return &Controller{store: st, fleet: fleet, publisher: publisher, cutover: cutover}
}

// Runtime returns the concurrency-safe live checkpoint owned by this controller.
func (c *Controller) Runtime() *Runtime {
	if c == nil {
		return nil
	}
	return &c.runtime
}

// Initialize loads (or durably initializes) the publication checkpoint and exposes its runtime
// snapshot without performing fleet convergence or media-server I/O. Composition calls this before
// request-serving so transport gates never infer a backend from Runtime's zero value. Desired
// validation is intentionally delegated to Load, the single State validation path.
func (c *Controller) Initialize(ctx context.Context, desired string) error {
	if c == nil || c.store == nil {
		return fmt.Errorf("backend transition: store is unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.load(ctx, desired)
	return err
}

// Apply converges desired through five retryable phases:
//
//  1. prepare inherited channel state;
//  2. durably mark the target prepared (which opens internal transport when applicable);
//  3. prepare and refresh the target publisher registration;
//  4. perform the optional cutover and durably publish the target;
//  5. retire stale registrations.
//
// Failures through phase 4 preserve the previously applied backend. A phase-5 failure returns an
// error but deliberately leaves the new backend applied; the next Apply repairs steady state and
// retries retirement. Reversing desired while another target is merely prepared first cancels that
// checkpoint durably, then repairs the still-applied backend.
func (c *Controller) Apply(ctx context.Context, desired string) error {
	return c.applyResolved(ctx, func() string { return desired })
}

// ApplyCurrent resolves desired only after entering the controller's serialization boundary.
// Composition uses it for settings-driven work so a maintenance retry that waited behind a newer
// save cannot later publish the stale value it captured before waiting.
func (c *Controller) ApplyCurrent(ctx context.Context, desired func() string) error {
	if desired == nil {
		return fmt.Errorf("backend transition: desired resolver is unavailable")
	}
	return c.applyResolved(ctx, desired)
}

func (c *Controller) applyResolved(ctx context.Context, resolveDesired func() string) error {
	if c == nil || c.store == nil {
		return fmt.Errorf("backend transition: store is unavailable")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	desired := resolveDesired()

	state, err := c.load(ctx, desired)
	if err != nil {
		return err
	}

	if state.Prepared() != "" && state.Prepared() != desired {
		cancelled, cerr := state.CancelPrepared()
		if cerr != nil {
			return fmt.Errorf("cancel prepared backend %q: %w", state.Prepared(), cerr)
		}
		if err := c.save(ctx, cancelled); err != nil {
			return fmt.Errorf("cancel prepared backend %q: %w", state.Prepared(), err)
		}
		state = cancelled
	}

	// Steady state still runs the publisher repair path. URLs and credentials can change while
	// the selected backend does not, and a failed post-publication retirement must retry here.
	if state.Applied() == desired && state.Prepared() == "" {
		return c.repairPublished(ctx, desired)
	}

	if state.Prepared() != desired {
		if c.fleet == nil {
			return fmt.Errorf("prepare inherited fleet for %q: fleet is unavailable", desired)
		}
		if err := c.fleet.PrepareInheritedBackend(ctx, desired); err != nil {
			return fmt.Errorf("prepare inherited fleet for %q: %w", desired, err)
		}
		prepared, perr := state.MarkPrepared(desired)
		if perr != nil {
			return fmt.Errorf("mark backend %q prepared: %w", desired, perr)
		}
		if err := c.save(ctx, prepared); err != nil {
			return fmt.Errorf("save prepared backend %q: %w", desired, err)
		}
		state = prepared
	}

	if c.publisher == nil {
		return fmt.Errorf("prepare publisher for %q: publisher is unavailable", desired)
	}
	// changed is useful to the concrete publisher for its own result, but cannot gate Refresh:
	// after a crash, the registration exists (changed=false) while refresh may never have run.
	if _, err := c.publisher.Prepare(ctx, desired); err != nil {
		return fmt.Errorf("prepare publisher for %q: %w", desired, err)
	}
	if err := c.publisher.Refresh(ctx, desired); err != nil {
		return fmt.Errorf("refresh publisher for %q: %w", desired, err)
	}

	from := state.Applied()
	if c.cutover != nil && from != desired {
		if err := c.cutover.BeforePublish(ctx, from, desired); err != nil {
			return fmt.Errorf("cut over publisher from %q to %q: %w", from, desired, err)
		}
	}

	published, perr := state.PublishPrepared()
	if perr != nil {
		return fmt.Errorf("publish prepared backend %q: %w", desired, perr)
	}
	if err := c.save(ctx, published); err != nil {
		return fmt.Errorf("save published backend %q: %w", desired, err)
	}

	if err := c.publisher.RetireStale(ctx, desired); err != nil {
		return fmt.Errorf("retire stale publishers after %q publication: %w", desired, err)
	}
	return nil
}

func (c *Controller) repairPublished(ctx context.Context, target string) error {
	if c.publisher == nil {
		return fmt.Errorf("repair publisher for %q: publisher is unavailable", target)
	}
	if _, err := c.publisher.Prepare(ctx, target); err != nil {
		return fmt.Errorf("repair publisher for %q: %w", target, err)
	}
	// Always refresh for restart safety. If a prior repair created the target and refresh then
	// failed, its next Prepare reports unchanged; skipping here would make the failure permanent.
	if err := c.publisher.Refresh(ctx, target); err != nil {
		return fmt.Errorf("refresh published backend %q: %w", target, err)
	}
	if err := c.publisher.RetireStale(ctx, target); err != nil {
		return fmt.Errorf("retire stale publishers for %q: %w", target, err)
	}
	return nil
}

func (c *Controller) save(ctx context.Context, state State) error {
	if err := Save(ctx, c.store, state); err != nil {
		return err
	}
	c.runtime.publish(state)
	return nil
}

func (c *Controller) load(ctx context.Context, desired string) (State, error) {
	state, err := Load(ctx, c.store, desired)
	if err != nil {
		return State{}, err
	}
	// Load either read this exact state or initialized and saved it. Publishing it is therefore
	// safe even when this is the first operation made by a freshly restarted process.
	c.runtime.publish(state)
	return state, nil
}
