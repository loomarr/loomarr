package playout

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/media"
)

// StreamClass is a source's cost class: what one stream of it costs this host to transcode into the
// channel output. Costs are measured per host by the capability probe on real content (#1512 G5).
type StreamClass string

const (
	// ClassCopy is a session that copies video. It costs no encoder and is always admitted.
	ClassCopy StreamClass = "copy"
	// ClassSDR is 8-bit SDR up to 1080p (56% of the household library). Also the class of a source
	// with no stream facts, which is the common case among unmeasured sources.
	ClassSDR StreamClass = "sdr_1080p"
	// ClassHEVC10 is 10-bit SDR up to 1080p: a heavier decode, the same filter graph.
	ClassHEVC10 StreamClass = "hevc10_1080p"
	// ClassHDR4K is anything above 1080p or HDR: a 4K decode, a downscale and (for HDR) the tone-map
	// stage. 4K SDR shares the class because its decode, not the tone-map, dominates.
	ClassHDR4K StreamClass = "uhd_hdr_tonemap"
	// ClassPremium4K is G10's premium 4K encode. Phase 0b has not measured it, so it is never
	// admitted on measured costs until the probe measures it.
	ClassPremium4K StreamClass = "premium_4k"
)

// TranscodeClasses are the classes the capability probe measures, cheapest first.
var TranscodeClasses = []StreamClass{ClassSDR, ClassHEVC10, ClassHDR4K}

// ClassOf is the cost class of a source.
func ClassOf(f MediaFormat) StreamClass {
	switch {
	case f.HDR() || f.Height > 1080 || f.Width > 1920:
		return ClassHDR4K
	case isTenBit(f.PixelFormat):
		return ClassHEVC10
	default:
		return ClassSDR
	}
}

func isTenBit(pixfmt string) bool {
	return len(pixfmt) > 4 && (pixfmt[len(pixfmt)-4:] == "10le" || pixfmt[len(pixfmt)-4:] == "10be")
}

// minStreamSpeed is the realtime multiple every admitted stream must keep (phase 0: 1.2x). A stream
// class measured at speed S alone therefore uses 1.2/S of the GPU's throughput.
const minStreamSpeed = 1.2

// capacityEpsilon absorbs float error when a whole number of streams exactly fills a limit.
const capacityEpsilon = 1e-9

// CostKey is one measured cell: a stream class encoded at one output height (a ladder rung).
type CostKey struct {
	Class  StreamClass
	Height int
	// Curve is the tone curve an HDR cell was measured with; empty for SDR classes, and Hable for
	// an HDR cell stored before curves were keyed.
	Curve ToneCurve
}

// HDR cost depends on the tone curve (playout.tone_curve, #1516): on Intel the libplacebo curves
// (bt2390, bt2446a, spline) take a CPU hop of ~0.25 cores per stream that the OpenCL ones (hable,
// mobius, reinhard, ~0.1) do not, so HDR costs are keyed by curve and capacity follows the
// operator's choice. An HDR cell stored without a curve was measured with Hable.

// ToneCurveSource reports the curve the live chain maps with right now.
type ToneCurveSource interface{ ToneCurve() ToneCurve }

// curve is the tone curve of an HDR cell ("" for other classes).
func (k CostKey) curve() ToneCurve {
	switch {
	case k.Class != ClassHDR4K:
		return ""
	case k.Curve == "":
		return ToneCurveHable
	default:
		return k.Curve
	}
}

// HDRKey is the cost key of class at height when HDR maps with curve.
func HDRKey(class StreamClass, height int, curve ToneCurve) CostKey {
	k := CostKey{Class: class, Height: height}
	if class == ClassHDR4K {
		k.Curve = curve
	}
	return k
}

// ClassCost is what one stream of a class costs, measured alone on this host.
type ClassCost struct {
	// Speed is the realtime multiple one stream reaches alone (ffmpeg speed=).
	Speed float64 `json:"speed"`
	// CPUCores is the host CPU one stream uses at 1x: process CPU time over media time.
	CPUCores float64 `json:"cpuCores"`
}

// BudgetFacts are the inputs of one admission decision, re-read every time so settings, cgroup
// changes and a re-measurement apply without a restart.
type BudgetFacts struct {
	// Hardware is true when the encoder runs on a GPU; a software host has no GPU term.
	Hardware bool
	// SessionLimit is the encoder's concurrent-session limit (NVENC GeForce: 12); 0 = none known.
	SessionLimit int
	// CPUAllowance is the cores playout may use (see PlayoutCPUAllowance); 0 = no CPU term.
	CPUAllowance float64
	// CPUSource says where the host CPU total came from: cgroup-v2, cgroup-v1 or cpu-count.
	CPUSource string
	// OperatorCap is playout.max_channels: a positive value caps concurrent transcodes.
	OperatorCap int
	// Rungs are the output heights of the quality ladder, best first. A new session takes the first
	// rung that fits (dropping a rung before being refused) and keeps it for its lifetime.
	Rungs []int
	// FirstRung is the best rung a new session may take. A host with no measurement starts at the
	// bottom rung: an unmeasured box must not be treated as unlimited.
	FirstRung int
	// Costs are the probe's measurements. An empty table falls back to MeasuredCapacity.
	Costs map[CostKey]ClassCost
	// MeasuredCapacity is the legacy whole-stream budget used before per-class costs exist: each
	// transcode costs one of it, whatever its class. 0 means unmeasured, which never blocks playout.
	MeasuredCapacity int
	// ToneCurve is the curve HDR maps with now; it picks the HDR cells. Empty = Hable.
	ToneCurve ToneCurve
}

// ResourceBudget is playout's one admission ledger (#1512 G5, #1505): a session attach and a
// programme start consult the same leases, so "does another encode fit" has one answer. Capacity is
// min(GPU throughput at 1.2x per stream, CPU allowance over measured CPU per stream, encoder
// sessions), evaluated per stream class and rung. A copy costs no encoder and is always admitted,
// so a channel watched at two plans (baseline transcode + HEVC copy) costs one stream.
//
// Refusing is deliberate. Admitting an N+1th transcode that makes all N stutter is worse than
// declining it; and a live session is never evicted to make room (the bound viewra lacked).
//
// Prepared media is its lowest-priority client until phase 4 removes it: background leases come
// from the encode pool, whose capacity is derived from this ledger, and a foreground hardware lease
// takes a pool slot so preparation is preempted and host memory gated exactly as before.
type ResourceBudget struct {
	facts func() BudgetFacts
	pool  *media.EncodePool

	mu     sync.Mutex
	leases map[*Lease]struct{}

	log *slog.Logger
	// refined holds live corrections of measured CPU costs (ObserveCPU), under its own lock so a
	// facts read never nests inside mu.
	rmu     sync.Mutex
	refined map[CostKey]refinement

	// background is the work to interrupt when a live transcode is admitted (BackgroundContext),
	// under mu.
	background map[*byte]context.CancelCauseFunc
}

// ErrYielded is the cause of a background context that ended because live playback started.
var ErrYielded = errors.New("playout: background work yielded to live playback")

// idlePoll is how often WaitIdle rechecks the ledger. Resuming background work is not urgent.
const idlePoll = 100 * time.Millisecond

// BackgroundContext derives a context for low-priority work (the boot capacity probe) that ends with
// cause ErrYielded the moment a live transcode is reserved, inside Reserve, so the tune never waits
// on it. While a live transcode already runs, the context is born yielded.
func (b *ResourceBudget) BackgroundContext(ctx context.Context) (context.Context, context.CancelFunc) {
	bctx, cancel := context.WithCancelCause(ctx)
	key := new(byte)
	b.mu.Lock()
	if b.transcodingLocked() {
		b.mu.Unlock()
		cancel(ErrYielded)
		return bctx, func() {}
	}
	if b.background == nil {
		b.background = map[*byte]context.CancelCauseFunc{}
	}
	b.background[key] = cancel
	b.mu.Unlock()
	return bctx, func() {
		b.mu.Lock()
		delete(b.background, key)
		b.mu.Unlock()
		cancel(context.Canceled)
	}
}

// WaitIdle blocks until no live transcode holds a lease, or ctx ends.
func (b *ResourceBudget) WaitIdle(ctx context.Context) error {
	tick := time.NewTicker(idlePoll)
	defer tick.Stop()
	for b.PlaybackNeedsHeadroom() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
	return nil
}

// yieldLocked interrupts all background work: a live transcode was just admitted.
func (b *ResourceBudget) yieldLocked() {
	for key, cancel := range b.background {
		cancel(ErrYielded)
		delete(b.background, key)
	}
}

func (b *ResourceBudget) transcodingLocked() bool {
	for l := range b.leases {
		if l.class != ClassCopy {
			return true
		}
	}
	return false
}

// refinement is a live CPU correction of one measured cell. It applies only while the probe's
// measurement is still base; a re-measurement starts over from the new number.
type refinement struct {
	base  ClassCost
	cores float64
}

// Live refinement bounds (#1512): a programme must cover refineMinMedia to count, each moves the
// estimate refineWeight of the way, and the result stays within refineBound of the measurement.
const (
	refineMinMedia = 20 * time.Second
	refineWeight   = 0.25
	refineBound    = 2.0
)

// WithLog logs live cost refinements. Call before the budget is shared.
func (b *ResourceBudget) WithLog(log *slog.Logger) *ResourceBudget {
	b.log = log
	return b
}

// currentFacts is the facts with live CPU refinements applied.
func (b *ResourceBudget) currentFacts() BudgetFacts {
	f := b.facts()
	b.rmu.Lock()
	defer b.rmu.Unlock()
	if len(b.refined) == 0 || len(f.Costs) == 0 {
		return f
	}
	costs := make(map[CostKey]ClassCost, len(f.Costs))
	for k, c := range f.Costs {
		if r, ok := b.refined[k]; ok && r.base == c {
			c.CPUCores = r.cores
		}
		costs[k] = c
	}
	f.Costs = costs
	return f
}

// ObserveCPU refines the CPU cost of class at this lease's rung from one finished live programme:
// cpu is the encoder's CPU time over media seconds of output. Speed is not refined: a live encode
// is paced to real time, so its speed says nothing about spare throughput.
func (l *Lease) ObserveCPU(class StreamClass, cpu, media time.Duration) {
	if l == nil || class == ClassCopy || media < refineMinMedia || cpu <= 0 {
		return
	}
	b := l.b
	facts := b.facts()
	height := facts.rungHeight(l.rung)
	key, ok := facts.cellKey(class, height)
	base := facts.Costs[key]
	if !ok || key.Height != height || key.curve() != HDRKey(class, height, facts.toneCurve()).curve() || base.CPUCores <= 0 {
		return // only a measured cell is refined; a fallback cell borrows another height's or curve's cost
	}
	perSecond := cpu.Seconds() / media.Seconds()
	if !facts.Hardware {
		// The cell is rung 0's cost: a programme on a cheaper software rung counts at its rung-0 worth.
		perSecond /= rungCostsOf(class)[l.SoftwareRung()]
	}
	observed := min(max(perSecond, base.CPUCores/refineBound), base.CPUCores*refineBound)
	b.rmu.Lock()
	if b.refined == nil {
		b.refined = map[CostKey]refinement{}
	}
	r, ok := b.refined[key]
	if !ok || r.base != base {
		r = refinement{base: base, cores: base.CPUCores}
	}
	before := r.cores
	r.cores += refineWeight * (observed - r.cores)
	b.refined[key] = r
	b.rmu.Unlock()
	if b.log != nil {
		b.log.Info("playout: live programme refined a stream class's CPU cost",
			"class", class, "height", key.Height, "measured_cores", base.CPUCores,
			"observed_cores", cpu.Seconds()/media.Seconds(), "from", before, "to", r.cores)
	}
}

// NewResourceBudget creates the ledger. facts is called on every decision and must be cheap.
func NewResourceBudget(facts func() BudgetFacts) *ResourceBudget {
	if facts == nil {
		facts = func() BudgetFacts { return BudgetFacts{} }
	}
	return &ResourceBudget{facts: facts, leases: map[*Lease]struct{}{}}
}

// WithEncodePool attaches the hardware encode pool (preemption of preparation and the host-memory
// gate). Call before the budget is shared.
func (b *ResourceBudget) WithEncodePool(pool *media.EncodePool) *ResourceBudget {
	b.pool = pool
	return b
}

// EncodePool returns the attached pool; background clients lease from it.
func (b *ResourceBudget) EncodePool() *media.EncodePool { return b.pool }

// AdmitRequest asks for one stream.
type AdmitRequest struct {
	Class StreamClass
	// NoRungDrop admits only at the first rung. Real demand sets it while idle warm work is still
	// reclaimable: a viewer beats a speculative pre-warm before it gives up picture quality.
	NoRungDrop bool
}

// Lease is one admitted stream. Its rung is fixed at admission.
type Lease struct {
	b        *ResourceBudget
	rung     int
	software SoftwareRung // the software ladder rung (#1517) the current item encodes at
	class    StreamClass
	demand   demand
	release  func() // the pool slot of a hardware transcode, nil otherwise
	released bool
}

// demand is what a lease holds of each limit.
type demand struct {
	gpu, cpu float64
	session  bool
}

// Rung is the ladder index the stream was admitted at.
func (l *Lease) Rung() int { return l.rung }

// SoftwareRung is the software ladder rung (#1517) the lease's current class was admitted at: on a
// measured software host the best rung whose CPU cost fits, else the unmeasured start rung. A GPU
// encoder ignores it; a software fallback inherits it. Nil-safe.
func (l *Lease) SoftwareRung() SoftwareRung {
	if l == nil {
		return RungFull
	}
	l.b.mu.Lock()
	defer l.b.mu.Unlock()
	return l.software
}

// StepSoftware re-prices the lease for a live ladder step (RungMonitorConfig.Reprice): a step down
// always applies and releases the CPU it no longer needs; a step up applies only if the rung's
// demand fits the ledger beside the other leases, else the lease and the encoder stay.
func (l *Lease) StepSoftware(to SoftwareRung) bool {
	b := l.b
	facts := b.currentFacts()
	b.mu.Lock()
	defer b.mu.Unlock()
	if l.released || l.class == ClassCopy {
		return false
	}
	d, ok := facts.demandAt(l.class, facts.rungHeight(l.rung), to)
	if !ok || (to < l.software && !b.fitsLocked(facts, d, l)) {
		return false
	}
	l.software, l.demand = to, d
	return true
}

// NewRungMonitor is the live ladder monitor for this lease's current item, bound to the lease: it
// starts on the admitted rung, with the class's rung costs, and every step re-prices the lease.
func (l *Lease) NewRungMonitor(cfg RungMonitorConfig) *RungMonitor {
	if cfg.Costs[RungFull] == 0 {
		cfg.Costs = rungCostsOf(l.Class())
	}
	if cfg.CPUAllowance == 0 {
		cfg.CPUAllowance = l.b.currentFacts().CPUAllowance // the ledger vetoes what others hold
	}
	cfg.Reprice = l.StepSoftware
	return NewRungMonitor(l.SoftwareRung(), cfg)
}

// Class is the lease's current stream class; a nil lease is a copy.
func (l *Lease) Class() StreamClass {
	if l == nil {
		return ClassCopy
	}
	l.b.mu.Lock()
	defer l.b.mu.Unlock()
	return l.class
}

// Transcoding reports whether the lease currently holds an encoder.
func (l *Lease) Transcoding() bool { return l.Class() != ClassCopy }

// Capacity is how many 1080p SDR streams fit an idle host at the top rung (0 = unmeasured).
func (b *ResourceBudget) Capacity() int {
	f := b.currentFacts()
	return f.capacity(ClassSDR, f.rungHeight(0))
}

// Admit admits one stream at the best rung that fits, or returns ErrAtCapacity. It is Reserve then
// Hold; callers that must reserve under their own lock call the two separately.
func (b *ResourceBudget) Admit(ctx context.Context, req AdmitRequest) (*Lease, error) {
	l, err := b.Reserve(req)
	if err != nil {
		return nil, err
	}
	if !l.Hold(ctx) {
		l.Release()
		return nil, ErrAtCapacity
	}
	return l, nil
}

// Reserve books one stream in the ledger at the best rung that fits (a new session drops a rung
// before it is refused), or returns ErrAtCapacity. It never blocks.
func (b *ResourceBudget) Reserve(req AdmitRequest) (*Lease, error) {
	facts := b.currentFacts()
	b.mu.Lock()
	defer b.mu.Unlock()
	first := min(max(facts.FirstRung, 0), max(len(facts.Rungs)-1, 0))
	last := max(len(facts.Rungs), 1)
	if req.NoRungDrop {
		last = first + 1
	}
	for rung := first; rung < last; rung++ {
		d, sw, ok := b.pickLocked(facts, req.Class, rung, nil, req.NoRungDrop)
		if !ok {
			continue
		}
		l := &Lease{b: b, rung: rung, software: sw, class: req.Class, demand: d}
		b.leases[l] = struct{}{}
		if req.Class != ClassCopy {
			b.yieldLocked()
		}
		return l, nil
	}
	return nil, ErrAtCapacity
}

// Hold takes a reserved hardware transcode's encode-pool slot: preparation is preempted (waiting
// briefly for it to drain) and the host-memory gate applies. False means the host has no memory
// for another encode; the caller releases the lease.
func (l *Lease) Hold(ctx context.Context) bool { return l.takePoolSlot(ctx, l.b.currentFacts()) }

// Reclass moves a lease to another class at its pinned rung (a programme boundary). Starting to
// transcode must fit, like a new session. A lease that already transcodes always moves: refusing it
// would stop a watched channel mid-lineup, and the overcommit shows in the next admission instead.
func (l *Lease) Reclass(ctx context.Context, class StreamClass) bool {
	return l.reclass(ctx, class, false)
}

// ReclassItem is Reclass at an item boundary: the software rung is re-picked for the ledger as it is
// now even when the class is unchanged, so an item starts on the best rung that fits today.
func (l *Lease) ReclassItem(ctx context.Context, class StreamClass) bool {
	return l.reclass(ctx, class, true)
}

func (l *Lease) reclass(ctx context.Context, class StreamClass, repick bool) bool {
	b := l.b
	facts := b.currentFacts()
	b.mu.Lock()
	if l.released {
		b.mu.Unlock()
		return false
	}
	if l.class == class && !repick {
		b.mu.Unlock()
		return true
	}
	d, sw, ok := b.pickLocked(facts, class, l.rung, l, false)
	starting := l.class == ClassCopy && class != ClassCopy
	if !ok {
		if starting {
			b.mu.Unlock()
			return false
		}
		// Already transcoding: it moves anyway, on the cheapest rung.
		rungs := facts.softwareRungs(class, false)
		sw = rungs[len(rungs)-1]
		d, _ = facts.demandAt(class, facts.rungHeight(l.rung), sw)
	}
	if l.class == class {
		l.demand, l.software = d, sw
		b.mu.Unlock()
		return true
	}
	l.class, l.demand, l.software = class, d, sw
	if starting {
		b.yieldLocked()
	}
	release := l.release
	if class == ClassCopy {
		l.release = nil
	}
	b.mu.Unlock()
	if class == ClassCopy && release != nil {
		release()
	}
	if starting && !l.takePoolSlot(ctx, facts) {
		b.mu.Lock()
		l.class, l.demand = ClassCopy, demand{}
		b.mu.Unlock()
		return false
	}
	return true
}

// takePoolSlot holds a hardware pool slot for a transcode: it preempts preparation and applies the
// host-memory gate. The pool never refuses on count, because its capacity follows this ledger.
func (l *Lease) takePoolSlot(ctx context.Context, facts BudgetFacts) bool {
	if l.b.pool == nil || !facts.Hardware || l.class == ClassCopy {
		return true
	}
	release, ok := l.b.pool.AcquireForeground(ctx)
	if !ok {
		return false
	}
	l.b.mu.Lock()
	l.release = release
	l.b.mu.Unlock()
	return true
}

// Release returns the lease's capacity. Idempotent and nil-safe.
func (l *Lease) Release() {
	if l == nil {
		return
	}
	b := l.b
	b.mu.Lock()
	if l.released {
		b.mu.Unlock()
		return
	}
	l.released = true
	delete(b.leases, l)
	release := l.release
	l.release = nil
	b.mu.Unlock()
	if release != nil {
		release()
	}
}

// PlaybackNeedsHeadroom reports whether any live transcode is running, so capped background work
// (filler, preparation) yields to playback (filler.PlaybackHeadroom, #1514).
func (b *ResourceBudget) PlaybackNeedsHeadroom() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.transcodingLocked()
}

// PlaybackBusy is filler.PlaybackHeadroom (#1514): background media work waits while a live
// transcode holds a lease. A copy holds no encoder and costs almost no CPU, so it does not count.
func (b *ResourceBudget) PlaybackBusy() (bool, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if n := b.useLocked(nil).Transcodes; n > 0 {
		return true, fmt.Sprintf("live playout is transcoding %d stream(s)", n)
	}
	return false, ""
}

// BackgroundSlots is the encode pool's capacity: the whole SDR streams that fit an empty ledger.
// The pool keeps one of them as its live reserve and admits no preparation while playback runs.
func (b *ResourceBudget) BackgroundSlots() int {
	facts := b.currentFacts()
	if !facts.Hardware {
		return 0 // software preparation would compete with playback for the CPU
	}
	return facts.capacity(ClassSDR, facts.rungHeight(0))
}

func (b *ResourceBudget) fitsLocked(facts BudgetFacts, d demand, except *Lease) bool {
	if !d.session {
		return true // a copy costs no encoder
	}
	use := b.useLocked(except)
	if facts.OperatorCap > 0 && use.Transcodes+1 > facts.OperatorCap {
		return false
	}
	if facts.SessionLimit > 0 && use.Transcodes+1 > facts.SessionLimit {
		return false
	}
	if len(facts.Costs) == 0 {
		// Legacy whole-stream units: d.gpu is 1/MeasuredCapacity (0 when unmeasured).
		return use.GPUShare+d.gpu <= 1+capacityEpsilon
	}
	if facts.Hardware && use.GPUShare+d.gpu > 1+capacityEpsilon {
		return false
	}
	return facts.CPUAllowance <= 0 || use.CPUCores+d.cpu <= facts.CPUAllowance+capacityEpsilon
}

// BudgetUse is what the live leases hold.
type BudgetUse struct {
	Sessions   int     `json:"sessions" doc:"Admitted sessions, copies included"`
	Transcodes int     `json:"transcodes" doc:"Sessions holding an encoder"`
	GPUShare   float64 `json:"gpuShare" doc:"Fraction of measured GPU throughput in use (1 = full)"`
	CPUCores   float64 `json:"cpuCores" doc:"Measured CPU cores the admitted streams use"`
}

func (b *ResourceBudget) useLocked(except *Lease) BudgetUse {
	var u BudgetUse
	for l := range b.leases {
		if l == except {
			continue
		}
		u.Sessions++
		if l.demand.session {
			u.Transcodes++
		}
		u.GPUShare += l.demand.gpu
		u.CPUCores += l.demand.cpu
	}
	return u
}

func (f BudgetFacts) rungHeight(rung int) int {
	if rung < len(f.Rungs) {
		return f.Rungs[rung]
	}
	return 0
}

// cost is a class's measured cost at an output height. An unmeasured height takes the nearest
// measured taller output, which costs at least as much; never a cheaper guess.
func (f BudgetFacts) cost(class StreamClass, height int) (ClassCost, bool) {
	k, ok := f.cellKey(class, height)
	return f.Costs[k], ok
}

// cellKey finds the measured cell for class at height: that height, else the nearest taller one.
// An HDR stream takes the cell of the live tone curve; a curve this host has not measured yet
// borrows Hable's until the probe measures it, rather than refusing HDR.
func (f BudgetFacts) cellKey(class StreamClass, height int) (CostKey, bool) {
	curves := []ToneCurve{""}
	if class == ClassHDR4K {
		curves = []ToneCurve{f.toneCurve(), DefaultToneCurve}
	}
	for _, curve := range curves {
		best, found := CostKey{}, false
		for k := range f.Costs {
			if k.Class != class || k.Height < height || k.curve() != curve {
				continue
			}
			if !found || k.Height < best.Height {
				best, found = k, true
			}
		}
		if found {
			return best, true
		}
	}
	return CostKey{}, false
}

func (f BudgetFacts) toneCurve() ToneCurve {
	if f.ToneCurve == "" {
		return DefaultToneCurve
	}
	return f.ToneCurve
}

// demandOf is what one stream of class at height holds at full quality. ok is false when the class
// cannot be admitted on this host's measurements (never measured, or a GPU measured below realtime).
func (f BudgetFacts) demandOf(class StreamClass, height int) (demand, bool) {
	return f.demandAt(class, height, RungFull)
}

// demandAt is demandOf on a software ladder rung. A software host never refuses a class for its
// measured speed (#1517): the probe measured rung 0, and each rung holds its share of that CPU
// (RungCosts), so a class too slow at full quality is admitted further down the ladder.
func (f BudgetFacts) demandAt(class StreamClass, height int, sw SoftwareRung) (demand, bool) {
	if class == ClassCopy {
		return demand{}, true
	}
	if len(f.Costs) == 0 {
		if f.MeasuredCapacity <= 0 {
			return demand{session: true}, true // unmeasured: never block playout
		}
		return demand{gpu: 1 / float64(f.MeasuredCapacity), session: true}, true
	}
	c, ok := f.cost(class, height)
	if !ok {
		return demand{}, false
	}
	if !f.Hardware {
		return demand{cpu: c.CPUCores * rungCostsOf(class)[sw], session: true}, true
	}
	if c.Speed < minStreamSpeed {
		return demand{}, false
	}
	return demand{gpu: minStreamSpeed / c.Speed, cpu: c.CPUCores, session: true}, true
}

// softwareRungs are the software ladder rungs admission tries for class, best first. Only a
// measured software host chooses by cost: full, light, then keyframes-only, the rung that always
// fits one stream (rung 2's gain is source-dependent, so it is never promised in advance). Anywhere
// else the rung is StartRung's unmeasured one, which only a software fallback encodes at. noDrop
// keeps full quality, like the output ladder's NoRungDrop.
func (f BudgetFacts) softwareRungs(class StreamClass, noDrop bool) []SoftwareRung {
	switch {
	case f.Hardware || len(f.Costs) == 0 || class == ClassCopy:
		if class == ClassHDR4K {
			return []SoftwareRung{RungKeyframes}
		}
		return []SoftwareRung{RungFull}
	case noDrop:
		return []SoftwareRung{RungFull}
	}
	return []SoftwareRung{RungFull, RungLight, RungKeyframes}
}

// pickLocked is the best software rung at output rung whose demand fits the ledger (except the
// lease being moved). On a software host only keyframes-only not fitting refuses.
func (b *ResourceBudget) pickLocked(
	facts BudgetFacts, class StreamClass, rung int, except *Lease, noDrop bool,
) (demand, SoftwareRung, bool) {
	for _, sw := range facts.softwareRungs(class, noDrop) {
		if d, ok := facts.demandAt(class, facts.rungHeight(rung), sw); ok && b.fitsLocked(facts, d, except) {
			return d, sw, true
		}
	}
	return demand{}, RungFull, false
}

// rungCostsOf is the rung cost table for a stream class (RungCostsFor by class).
func rungCostsOf(class StreamClass) RungCosts {
	if class == ClassHDR4K {
		return heavyRungCosts
	}
	return sdrRungCosts
}

// capacity is how many streams of class at height fit an empty ledger.
func (f BudgetFacts) capacity(class StreamClass, height int) int {
	d, ok := f.demandOf(class, height)
	if !ok {
		return 0
	}
	n := math.MaxInt
	if d.gpu > 0 {
		n = min(n, int(1/d.gpu+capacityEpsilon))
	}
	if d.cpu > 0 && f.CPUAllowance > 0 {
		n = min(n, int(f.CPUAllowance/d.cpu+capacityEpsilon))
	}
	if f.SessionLimit > 0 {
		n = min(n, f.SessionLimit)
	}
	if f.OperatorCap > 0 {
		n = min(n, f.OperatorCap)
	}
	if n == math.MaxInt {
		return 0 // unbounded (unmeasured)
	}
	return n
}

// ClassBudget is one class's measured cost and capacity at the top rung.
type ClassBudget struct {
	Measured bool    `json:"measured" doc:"Whether this host measured the class"`
	Speed    float64 `json:"speed" doc:"Measured realtime multiple of one stream alone"`
	CPUCores float64 `json:"cpuCores" doc:"Measured CPU cores one stream uses at 1x"`
	Units    float64 `json:"units" doc:"Cost relative to one 1080p SDR stream"`
	Capacity int     `json:"capacity" doc:"Streams of this class that fit an idle host (0 = none or unbounded)"`
}

// BudgetSnapshot is the ledger as GET /v1/playout/status reports it.
type BudgetSnapshot struct {
	Hardware     bool                        `json:"hardware"`
	CPUAllowance float64                     `json:"cpuAllowance" doc:"Cores playout may use"`
	CPUSource    string                      `json:"cpuSource,omitempty" doc:"cgroup-v2 | cgroup-v1 | cpu-count"`
	SessionLimit int                         `json:"sessionLimit" doc:"Encoder session limit (0 = none known)"`
	OperatorCap  int                         `json:"operatorCap" doc:"playout.max_channels (0 = none)"`
	Classes      map[StreamClass]ClassBudget `json:"classes"`
	InUse        BudgetUse                   `json:"inUse"`
}

// Snapshot reports the capacity terms, per-class costs and what is in use.
func (b *ResourceBudget) Snapshot() BudgetSnapshot {
	f := b.currentFacts()
	b.mu.Lock()
	use := b.useLocked(nil)
	b.mu.Unlock()
	s := BudgetSnapshot{
		Hardware: f.Hardware, CPUAllowance: f.CPUAllowance, CPUSource: f.CPUSource,
		SessionLimit: f.SessionLimit, OperatorCap: f.OperatorCap,
		Classes: map[StreamClass]ClassBudget{}, InUse: use,
	}
	top := f.rungHeight(0)
	base := float64(f.capacity(ClassSDR, top))
	for _, class := range append(TranscodeClasses, ClassPremium4K) {
		c, measured := f.cost(class, top)
		cb := ClassBudget{Measured: measured, Speed: c.Speed, CPUCores: c.CPUCores, Capacity: f.capacity(class, top)}
		if cb.Capacity > 0 && base > 0 {
			cb.Units = math.Round(base/float64(cb.Capacity)*100) / 100
		}
		s.Classes[class] = cb
	}
	return s
}

// Ceiling is how many top-rung 1080p SDR streams the measured costs admit on an idle host (0 =
// unbounded or unmeasured): the count the operator cap and VRAM shading are applied to.
func (f BudgetFacts) Ceiling() int { return f.capacity(ClassSDR, f.rungHeight(0)) }
