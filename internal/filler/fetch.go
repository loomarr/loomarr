package filler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loomarr/loomarr/internal/sourceprovider"
)

// Auto-fetch (§10 V38b): a registered, enabled source is polled on a schedule and new items
// download without anyone asking.
//
// This supersedes §15's "there is no unattended crawler". ⚠ **The superseded rule's concern was
// legitimate — unattended fetching can fill a stranger's disk — so it survives as LIMITS rather
// than as a prohibition**, and every one of them fails toward doing less. The failure mode being
// designed against is "add a source, wake up to 8,000 files".
//
// Three properties §10 marks non-negotiable, each enforced here:
//
//  1. Only registered, ENABLED sources are polled. The Sources switch claims Loomarr "stops
//     scanning, searching and downloading" from a source that is off; auto-fetch honouring
//     anything less makes that copy false.
//  2. Everything fetched arrives HELD. Auto-fetch does not bypass the lifecycle — the clip is
//     still tagged, scored and gated by the confidence cap before it can play. The unattended
//     step is ACQUISITION, never ADMISSION.
//  3. A limit that stops the fetch is REPORTED. A crawler that quietly does nothing is
//     indistinguishable from one that is broken.

// FetchLimits bounds one auto-fetch pass. All are read per run (hot-apply), never captured.
type FetchLimits struct {
	// MaxPerRun caps items ONE source may pull per poll. ⚠ The bound that stops "add a source"
	// meaning "download a collection of thousands tonight".
	MaxPerRun func() int
	// MaxProviderPerRun caps the combined work of all Sources for one provider in one fetch
	// pass. It is an internal protection rather than another operator-facing setting.
	MaxProviderPerRun func() int
	// MaxCatalogClips stops auto-fetch once the catalog reaches this size.
	//
	// ⚠ Bounds the UNATTENDED path only. An admin queueing a clip or approving a pull is a
	// deliberate act that this must not block: a ceiling on what happens while nobody is looking
	// is not a ceiling on what someone chooses to do.
	MaxCatalogClips func() int
	// MinDuration and MaxDuration are the existing filler envelope. They apply to automatic
	// selection from a provider whose listing can supply an honest known duration before download,
	// such as YouTube's flat listing.
	MinDuration func() time.Duration
	MaxDuration func() time.Duration
	// Compilations holds compilation reels back from every scheduled source until automatic
	// splitting is certified (#1773). The zero gate takes them.
	Compilations CompilationGate
}

// FetchSource is one pollable source.
type FetchSource struct {
	ID string
	// Kind is the ID of the source's provider (sourceprovider.ID). A source whose provider is not
	// registered is inert: a pass skips it and leaves the row alone.
	Kind    string
	URI     string
	Enabled bool
	// Region is the source's country in the installation's preference order (#2035): 0 for home,
	// then each fallback country. A pass asks home sources first.
	Region int
	// NeverFetch is the resolved automatic-download opt-out. It stays separate from Every so
	// zero-value test and embedding sources remain immediately due without confusing that with
	// the store's explicit `fetch_every_seconds = 0` state.
	NeverFetch bool
	// Every is the resolved automatic-download interval. The store owns the
	// nil/0/N resolution so the planner cannot confuse inheritance with opting out. A zero value
	// here means no due delay; NeverFetch is the explicit off fact.
	Every time.Duration
	// LastCheckedAt is the last successful bounded check, whether or not it found a new item.
	LastCheckedAt time.Time
	// CheckFailureCount/CheckRetryAt retain provider backoff across restarts. CheckLeaseUntil
	// prevents a scheduled pass and Look for new clips from listing the same source together.
	CheckFailureCount int
	CheckRetryAt      time.Time
	CheckLeaseUntil   time.Time
	// MaxPerRun is this source's resolved per-run cap — its override, or the global default.
	// Zero means "use the global", which the caller has already applied.
	MaxPerRun int
	// ScanCheckpoint is durable progress through the provider's bounded listing. A provider that
	// does not resume leaves it empty.
	ScanCheckpoint sourceprovider.Checkpoint
}

// SourceCheckCompletion is committed atomically with lease release. ReplaceOutcomes begins a new
// sweep; otherwise Outcomes extend the summary retained by an in-progress cursor.
type SourceCheckCompletion struct {
	CheckedAt       time.Time
	Checkpoint      sourceprovider.Checkpoint
	Outcomes        sourceprovider.Outcomes
	ReplaceOutcomes bool
}

const SourceCheckLease = 30 * time.Minute

var (
	ErrSourceCheckInProgress = errors.New("filler: source check already in progress")
	ErrSourceCheckClaimLost  = errors.New("filler: source check claim lost")
	ErrFetchSourceNotFound   = errors.New("filler: source not found")
)

// NextAutomaticCheck returns the earliest time this source may be checked. Disabled, opted-out,
// and local sources have no automatic check to promise.
func (s FetchSource) NextAutomaticCheck(now time.Time) (time.Time, bool) {
	if !s.Enabled || s.NeverFetch || s.URI == "" || s.Kind == "folder" || s.Kind == "library" {
		return time.Time{}, false
	}
	due := now
	if !s.LastCheckedAt.IsZero() {
		due = s.LastCheckedAt.Add(s.Every)
	}
	if s.CheckRetryAt.After(due) {
		due = s.CheckRetryAt
	}
	if s.CheckLeaseUntil.After(due) {
		due = s.CheckLeaseUntil
	}
	if due.Before(now) {
		due = now
	}
	return due, true
}

// SourceCheckRetryDelay is the bounded provider-listing retry ladder. Attempts after the third
// remain hourly until a successful check clears the durable failure count.
func SourceCheckRetryDelay(failures int) time.Duration {
	switch {
	case failures <= 1:
		return time.Minute
	case failures == 2:
		return 5 * time.Minute
	case failures == 3:
		return 15 * time.Minute
	default:
		return time.Hour
	}
}

// FetchStore is the slice of the store the fetch job needs.
type FetchStore interface {
	ListFetchSources(ctx context.Context) ([]FetchSource, error)
	// ListAcquisitionRemoteStates is the typed, durable authority for remote-item identity.
	// Paths cannot prove which provider/source produced a filename.
	ListAcquisitionRemoteStates(ctx context.Context) (map[string]ExistingRemoteState, error)
	// CatalogPaths returns every clip path, including HELD ones.
	//
	// ⚠ Held clips must be included or the job re-downloads what is already sitting in the
	// review queue: a held clip is not in the catalog by the ListClips default, but it is very
	// much already on disk. That is the bug this comment exists to prevent.
	CatalogPaths(ctx context.Context) ([]string, error)
	ClaimCheck(ctx context.Context, id string, observedLastCheck, now, leaseUntil time.Time) (bool, error)
	CompleteCheck(ctx context.Context, id string, leaseUntil time.Time, completion SourceCheckCompletion) error
	FailCheck(ctx context.Context, id string, leaseUntil, retryAt time.Time) error
	// MarkFetched stamps a source that successfully queued at least one item.
	//
	// ⚠ Without this the Sources tab reads "never fetched" forever while auto-fetch runs behind
	// it — the row would describe a source nobody had touched, on an install downloading from it
	// every six hours. `MarkFillerSourceFetched` shipped with V33 and had no production caller
	// until now, which is exactly how it stayed easy to forget.
	MarkFetched(ctx context.Context, id string, at time.Time) error
}

// FetchIngestor hands URLs to the ordinary ingest path.
//
// ⚠ The SAME path a queued search result or an approved pull uses. A second downloader is the
// shape §10 rejects by name, and it is how one route would quietly stop honouring the lifecycle.
type FetchIngestor interface {
	IngestSource(ctx context.Context, sourceID, sourceKind string, urls []string) (string, error)
}

// IdentifiedFetchIngestor is the V66 production seam: scheduled selection retains provider item
// identity into the durable manifest. The URL-only interface remains for narrow older embeddings.
type IdentifiedFetchIngestor interface {
	IngestSourceItems(ctx context.Context, sourceID, sourceKind string, items []sourceprovider.Item) (string, error)
}

// Fetcher polls registered sources.
type Fetcher struct {
	store FetchStore
	// providers lists each source; Kind names the provider that does.
	providers *sourceprovider.Registry
	ingest    FetchIngestor
	limits    FetchLimits
	log       *slog.Logger
	now       func() time.Time
	gaps      func(context.Context) (CoverageGaps, error)
	open      OpenSearcher
	// inertLogged holds the provider IDs a pass has already reported as not fetched, so a source
	// that stays due is reported once rather than every minute.
	inertLogged sync.Map
	// metUntil is when a met pool target is next read again, in Unix milliseconds. A met pass
	// claims no source, so every source stays due and each minute's wake would otherwise re-read
	// every channel's coverage and the household's airings.
	metUntil atomic.Int64
}

// TargetRecheck is how long an unattended pass that found the pool target met waits before reading
// it again.
const TargetRecheck = 15 * time.Minute

// WithCoverageGaps steers each pass toward what the live channels' breaks are short of (#749,
// CoverageGapsFrom). Optional: without it a pass ranks by quality and diversity alone, as it
// always has.
func (f *Fetcher) WithCoverageGaps(gaps func(context.Context) (CoverageGaps, error)) *Fetcher {
	f.gaps = gaps
	return f
}

// WithOpenSearch lets a pass the sources left short search the whole provider for nearby stations
// (#2059 decisions 10 and 11). Optional: without it a pass searches only inside its sources.
func (f *Fetcher) WithOpenSearch(open OpenSearcher) *Fetcher {
	f.open = open
	return f
}

// NewFetcher builds the automatic-download worker. Source policy, including whether any source
// should run at all, arrives through ListFetchSources on every pass so global and per-source edits
// hot-apply through one authority.
func NewFetcher(store FetchStore, providers *sourceprovider.Registry, ingest FetchIngestor, limits FetchLimits, log *slog.Logger) *Fetcher {
	return &Fetcher{
		store: store, providers: providers, ingest: ingest, limits: limits, log: log, now: time.Now,
	}
}

// WithClock injects the clock, so a test can assert WHAT was stamped rather than merely that
// something was.
func (f *Fetcher) WithClock(now func() time.Time) *Fetcher {
	f.now = now
	return f
}

// FetchResult reports what one pass did — and, when it did nothing, WHY.
type FetchResult struct {
	SourcesPolled int
	Queued        int
	// Skipped counts items already in the catalog.
	Skipped int
	// Outcomes is the closed explanation of the items considered by this pass.
	Outcomes sourceprovider.Outcomes
	// MaxPerCheck is the selected source's effective cap on a manual run.
	MaxPerCheck int
	// StoppedBy names the limit that ended the pass early ("catalog", "provider", "target" for a
	// met pool target, or ""). ⚠ Reported
	// rather than logged-and-forgotten: an operator whose catalog stopped growing must be able
	// to see which ceiling stopped it (§10).
	StoppedBy string
}

// FetchStatus is the current answer to “why will auto-fetch do no work?”. It is recomputed from the
// same live catalog limit Run uses, so the UI does not infer health from a
// stale last-run log line.
type FetchStatus struct {
	Enabled      bool
	StoppedBy    string
	CatalogClips int
	MaxCatalog   int
}

func (f *Fetcher) Status(ctx context.Context) (FetchStatus, error) {
	status := FetchStatus{}
	sources, err := f.store.ListFetchSources(ctx)
	if err != nil {
		return status, fmt.Errorf("list sources: %w", err)
	}
	for _, source := range sources {
		if _, fetched := f.providers.Lister(sourceprovider.ID(source.Kind)); fetched && source.Enabled && !source.NeverFetch && source.URI != "" {
			status.Enabled = true
			break
		}
	}
	paths, err := f.store.CatalogPaths(ctx)
	if err != nil {
		return status, fmt.Errorf("read catalog: %w", err)
	}
	status.CatalogClips = len(paths)
	status.MaxCatalog = f.limits.MaxCatalogClips()
	if status.Enabled && status.MaxCatalog > 0 && status.CatalogClips >= status.MaxCatalog {
		status.StoppedBy = "catalog"
	}
	return status, nil
}

// Run polls only enabled sources whose effective interval has elapsed.
func (f *Fetcher) Run(ctx context.Context) (FetchResult, error) {
	return f.run(ctx, "", true)
}

// RunSource performs one deliberate, bounded pass for a single registered source. The operator's
// click is not an unattended schedule, so it ignores the global/per-source timing opt-outs while
// retaining the source's enabled switch and every disk/catalog/per-run safety ceiling.
func (f *Fetcher) RunSource(ctx context.Context, sourceID string) (FetchResult, error) {
	return f.run(ctx, sourceID, false)
}

// run is one fetch pass: the due sources in region order, each checked by checkSource, then open
// search for whatever the sources left short.
func (f *Fetcher) run(ctx context.Context, sourceID string, scheduled bool) (FetchResult, error) {
	var res FetchResult
	srcs, err := f.store.ListFetchSources(ctx)
	if err != nil {
		return res, fmt.Errorf("list sources: %w", err)
	}
	p := &fetchPass{f: f, scheduled: scheduled, res: &res, providerQueued: map[string]int{}}
	p.globalPerRun, p.providerLimit = f.passLimits()
	if sourceID != "" {
		if res.MaxPerCheck, err = p.manualCap(srcs, sourceID); err != nil {
			return res, err
		}
	}

	// Resolve the due set before reading catalog or filesystem capacity. The scheduler wakes once
	// a minute, while an ordinary source is due every six hours; an idle wake must remain a cheap
	// source-state read rather than walking the filler directory 359 extra times between checks.
	now := f.now()
	due, err := f.dueSources(srcs, sourceID, scheduled, now)
	if err != nil || len(due) == 0 {
		return res, err
	}
	if scheduled && now.UnixMilli() < f.metUntil.Load() {
		res.StoppedBy = "target"
		return res, nil
	}
	// Home sources first, then each fallback country's in preference order (#2035): a per-provider
	// limit reached partway through the pass is then spent on the home country.
	slices.SortStableFunc(due, func(a, b FetchSource) int { return a.Region - b.Region })

	if stop, err := p.prepare(ctx, now); err != nil || stop {
		return res, err
	}
	for _, src := range due {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}
		done, err := p.checkSource(ctx, src)
		if err != nil {
			return res, err
		}
		if done {
			break
		}
	}
	if err := p.searchOpen(ctx); err != nil {
		return res, err
	}
	if res.Queued > 0 {
		f.log.Info("filler auto-fetch", "sources", res.SourcesPolled, "queued", res.Queued, "skipped", res.Skipped)
	}
	if res.StoppedBy == "provider" && f.log != nil {
		f.log.Info("filler auto-fetch paused at the provider pass limit",
			"max_per_provider", p.providerLimit,
			"note", "remaining due sources will be considered on the next scheduler pass")
	}
	return res, nil
}

// fetchPass is one pass's shared state: its limits, what the pool is short of, and what the pass
// has queued so far.
type fetchPass struct {
	f         *Fetcher
	scheduled bool
	res       *FetchResult

	globalPerRun, providerLimit int
	providerQueued              map[string]int

	// inPass is the acquisition state the pass started from, plus what it has queued since.
	inPass    map[string]ExistingRemoteState
	gaps      CoverageGaps
	targeted  bool
	gapReason string
}

// passLimits are a pass's default per-source cap and its per-provider ceiling.
func (f *Fetcher) passLimits() (globalPerRun, providerLimit int) {
	globalPerRun = max(f.limits.MaxPerRun(), 1)
	providerLimit = 50
	if f.limits.MaxProviderPerRun != nil && f.limits.MaxProviderPerRun() > 0 {
		providerLimit = f.limits.MaxProviderPerRun()
	}
	return globalPerRun, providerLimit
}

// manualCap is a manual run's cap for its one source: the source's own, or the global one, held
// to the provider ceiling.
func (p *fetchPass) manualCap(srcs []FetchSource, sourceID string) (int, error) {
	i := slices.IndexFunc(srcs, func(src FetchSource) bool { return src.ID == sourceID })
	if i < 0 {
		return p.globalPerRun, ErrFetchSourceNotFound
	}
	limit := p.globalPerRun
	if srcs[i].MaxPerRun > 0 {
		limit = srcs[i].MaxPerRun
	}
	return min(limit, p.providerLimit), nil
}

// dueSources are the sources a pass checks: every enabled source with somewhere to fetch FROM
// whose provider lists it and whose automatic check is due, or for a manual run just the one
// named, due or not. A named source that is switched off is refused.
//
// A source whose provider has no Lister is never due: a scanned provider (folder, library) is read
// where it is, and an unregistered provider is inert until it is registered again. Dropping both
// here keeps a wake whose only sources are inert as cheap as an idle one.
//
// NextAutomaticCheck carries the source's NeverFetch opt-out, which is distinct from Enabled: a
// source can be on (searched, its clips counted) while opting out of UNATTENDED fetching.
func (f *Fetcher) dueSources(srcs []FetchSource, sourceID string, scheduled bool, now time.Time) ([]FetchSource, error) {
	due := make([]FetchSource, 0, len(srcs))
	for _, src := range srcs {
		if sourceID != "" && src.ID != sourceID {
			continue
		}
		if !src.Enabled || src.URI == "" {
			if sourceID != "" && !src.Enabled {
				return nil, ErrSourceDisabled
			}
			continue
		}
		if _, fetched := f.providers.Lister(sourceprovider.ID(src.Kind)); !fetched {
			if _, registered := f.providers.Get(sourceprovider.ID(src.Kind)); !registered {
				f.reportInert(src)
			}
			continue
		}
		if scheduled {
			if next, automatic := src.NextAutomaticCheck(now); !automatic || next.After(now) {
				continue
			}
		}
		due = append(due, src)
	}
	return due, nil
}

// prepare reads what the pass selects against: the catalog, each remote item's acquisition state
// and the pool's coverage gaps. stop reports a pass that ends here because its target is met or
// its catalog is full; res.StoppedBy says which.
func (p *fetchPass) prepare(ctx context.Context, now time.Time) (stop bool, err error) {
	f := p.f
	have, err := f.store.CatalogPaths(ctx)
	if err != nil {
		return false, fmt.Errorf("read catalog: %w", err)
	}
	// The catalog is one input to selection; source-local history and acquisition intent also
	// determine whether a discovered item is eligible to be offered again.
	existing, err := f.store.ListAcquisitionRemoteStates(ctx)
	if err != nil {
		return false, fmt.Errorf("read acquisition remote states: %w", err)
	}
	// Treat the store result as a snapshot: in-pass queue state must not mutate adapter-owned
	// fixture or store state across retry attempts.
	p.inPass = make(map[string]ExistingRemoteState, len(existing))
	for key, state := range existing {
		p.inPass[key] = state
	}
	// Channel coverage gaps steer which items this pass takes first (#749). Read once per pass
	// that has due work, never on an idle wake. A failed read degrades to the unsteered pass:
	// steering is an optimisation, and the source still deserves its bounded refresh.
	if f.gaps != nil {
		gaps, gapsErr := f.gaps(ctx)
		if gapsErr != nil {
			f.log.Warn("filler auto-fetch: coverage gaps unavailable; selecting without them", "err", gapsErr)
		} else {
			p.gaps, p.targeted = gaps, true
		}
	}
	// An unattended pass fetches toward the pool target and stops when it is met (#2035). The
	// operator's click is not held to it: asking a source for more is a deliberate act.
	if p.scheduled && p.targeted && p.gaps.Met() {
		f.metUntil.Store(now.Add(TargetRecheck).UnixMilli())
		p.res.StoppedBy = "target"
		return true, nil
	}
	if len(p.gaps.Eras) > 0 {
		p.gapReason = " Items from " + formatEraWindows(p.gaps.Eras) + " go first: channels there cannot fill a break from their own era."
	}
	if len(p.gaps.Roles) > 0 {
		p.gapReason += " Bumpers and station IDs go first: no break has one to open or close on."
	}
	if max := f.limits.MaxCatalogClips(); max > 0 && len(have) >= max {
		p.res.StoppedBy = "catalog"
		f.logStop("catalog", len(have), max)
		return true, nil
	}
	return false, nil
}

// sourceBudget is how many items one source may take this pass.
type sourceBudget struct {
	// take caps what is selected. list sizes the listing: it is take before the pool's shortfall
	// cut it, because a short over-list of a source whose newest items are all held would report
	// nothing new while the pool stays short.
	take, list int
	// providerBound is a take cut short by the provider ceiling.
	providerBound bool
}

// budget is src's share of the pass: its own cap or the global one, held to what its provider has
// left and, on an unattended pass, to the pool's shortfall. ok is false once that shortfall is
// queued, which ends the pass.
func (p *fetchPass) budget(src FetchSource) (b sourceBudget, ok bool) {
	// The source's own override, or the global. A busy collection and a small playlist want
	// different numbers, which one figure served badly.
	b.take = src.MaxPerRun
	if b.take < 1 {
		b.take = p.globalPerRun
	}
	if remaining := p.providerLimit - p.providerQueued[src.Kind]; b.take > remaining {
		b.take, b.providerBound = remaining, true
	}
	b.list = b.take
	if p.scheduled && p.targeted && p.gaps.Short > 0 && len(p.gaps.Roles) == 0 {
		if left := p.gaps.Short - p.res.Queued; left < b.take {
			if left < 1 {
				return b, false
			}
			b.take = left
		}
	}
	return b, true
}

// checkSource is one due source's bounded check: claim it, then list, select and queue. done
// reports that the pool's shortfall is already queued, which ends the pass.
func (p *fetchPass) checkSource(ctx context.Context, src FetchSource) (done bool, err error) {
	lister, _ := p.f.providers.Lister(sourceprovider.ID(src.Kind))
	if p.providerQueued[src.Kind] >= p.providerLimit {
		p.res.StoppedBy = "provider"
		return false, nil
	}
	b, ok := p.budget(src)
	if !ok {
		p.res.StoppedBy = "target"
		return true, nil
	}
	checkAt := p.f.now()
	c := &sourceCheck{
		pass: p, src: src, leaseUntil: checkAt.Add(SourceCheckLease),
		checkpoint: src.ScanCheckpoint, outcomes: sourceprovider.Outcomes{}, replaceOutcomes: true,
	}
	claimed, err := p.f.store.ClaimCheck(ctx, src.ID, src.LastCheckedAt, checkAt, c.leaseUntil)
	if err != nil {
		return false, fmt.Errorf("claim source %q: %w", src.ID, err)
	}
	if !claimed {
		if !p.scheduled {
			return false, fmt.Errorf("%w: %s", ErrSourceCheckInProgress, src.ID)
		}
		return false, nil
	}
	p.res.SourcesPolled++
	return false, c.run(ctx, lister, b)
}

// reportInert says once per provider that its sources are skipped because the provider is not
// registered here. The rows stay as they are.
func (f *Fetcher) reportInert(src FetchSource) {
	if _, reported := f.inertLogged.LoadOrStore(src.Kind, true); reported || f.log == nil {
		return
	}
	f.log.Info("filler auto-fetch: skipping sources whose provider is not registered or not fetched",
		"provider", src.Kind, "source", src.ID,
		"note", "the sources are kept and are fetched again once the provider is registered")
}

// sourceCheck is one claimed source's check: its lease, its scan checkpoint and the outcomes it
// records when it completes.
type sourceCheck struct {
	pass            *fetchPass
	src             FetchSource
	leaseUntil      time.Time
	checkpoint      sourceprovider.Checkpoint
	outcomes        sourceprovider.Outcomes
	replaceOutcomes bool
}

// run lists the source through its provider, selects from the listing and queues the selection.
func (c *sourceCheck) run(ctx context.Context, lister sourceprovider.Lister, b sourceBudget) error {
	p, src := c.pass, c.src
	listing, err := lister.List(ctx,
		sourceprovider.Source{ID: src.ID, URI: src.URI, Checkpoint: src.ScanCheckpoint}, c.listRequest(b))
	if err != nil {
		return c.fail(ctx, "check", "list", "filler auto-fetch: source could not be listed", err)
	}
	c.checkpoint, c.replaceOutcomes = listing.Checkpoint, listing.ReplaceOutcomes
	for outcome, count := range listing.Outcomes {
		c.outcomes[outcome] += count
	}
	p.res.Skipped += listing.Outcomes[sourceprovider.OutcomeAlreadyKnown]

	// The scheduled path uses the same deterministic selector as explicit pulls. There are no
	// hard semantic constraints here—the remote listing cannot certify them—but declared rights,
	// representation quality, era-observation diversity, and stable identity now choose the
	// bounded prefix instead of whichever item the provider returned first.
	selection, err := PlanAcquisitionFor(AcquisitionIntent{
		Count:         b.take,
		CatalogReason: "Bounded scheduled refresh of a registered source." + p.gapReason,
	}, c.candidates(listing.Items), p.inPass, p.gaps)
	if err != nil {
		if completeErr := c.complete(ctx); completeErr != nil {
			return completeErr
		}
		return c.report("select", "filler auto-fetch: source candidates could not be selected", err)
	}
	for _, decision := range selection.Rejected {
		switch decision.Disposition {
		case CandidateAlreadyCatalogued, CandidateAlreadyQueued, CandidatePreviouslyDeclined:
			p.res.Skipped++
			c.outcomes[sourceprovider.OutcomeAlreadyKnown]++
		}
	}
	if len(selection.Selected) == 0 {
		return c.complete(ctx)
	}
	return c.queue(ctx, selection.Selected, b)
}

// listRequest is this check's terms for its provider: the budget, the station searches of a short
// pass, the clip envelope, and the two rules the provider cannot know — the rejection every
// provider is held to, and what the pass already has.
func (c *sourceCheck) listRequest(b sourceBudget) sourceprovider.ListRequest {
	p, f, src := c.pass, c.pass.f, c.src
	req := sourceprovider.ListRequest{
		Take: b.take, List: b.list, Now: f.now(),
		Reject: func(item sourceprovider.Item) (sourceprovider.Outcome, bool) {
			return f.sharedRejection(src, item)
		},
		Known: func(item sourceprovider.Item) bool {
			return p.inPass[c.identity(item).Key()] != ""
		},
	}
	if p.scheduled && p.targeted {
		req.Stations = p.gaps.Stations
	}
	if f.limits.MinDuration != nil {
		req.MinDuration = f.limits.MinDuration()
	}
	if f.limits.MaxDuration != nil {
		req.MaxDuration = f.limits.MaxDuration()
	}
	return req
}

func (c *sourceCheck) identity(item sourceprovider.Item) RemoteIdentity {
	return RemoteIdentity{Provider: c.src.Kind, SourceID: c.src.ID, RemoteID: item.ID}
}

// candidates are the items the provider offered, as the selector takes them.
func (c *sourceCheck) candidates(items []sourceprovider.Item) []AcquisitionCandidate {
	candidates := make([]AcquisitionCandidate, 0, len(items))
	for _, item := range items {
		candidates = append(candidates, AcquisitionCandidate{
			Identity: c.identity(item),
			URL:      item.URL, Title: item.Title, License: item.License,
			ObservedYear: item.ObservedYear, PublishedAt: item.PublishedAt,
			DurationMS: item.DurationMS, Height: item.Height,
			StationSearch: item.Station,
		})
	}
	return candidates
}

// queue hands the selection to ingest and, once it is queued, records it: in-pass state, the
// pass's counts and the source's last-fetched stamp.
func (c *sourceCheck) queue(ctx context.Context, selected []AcquisitionDecision, b sourceBudget) error {
	p, f, src := c.pass, c.pass.f, c.src
	urls := make([]string, 0, len(selected))
	items := make([]sourceprovider.Item, 0, len(selected))
	for _, decision := range selected {
		urls = append(urls, decision.Candidate.URL)
		items = append(items, sourceprovider.Item{
			ID: decision.Candidate.Identity.RemoteID, URL: decision.Candidate.URL,
			Title: decision.Candidate.Title, License: decision.Candidate.License,
			ObservedYear: decision.Candidate.ObservedYear, PublishedAt: decision.Candidate.PublishedAt,
			DurationMS: decision.Candidate.DurationMS, Height: decision.Candidate.Height,
			Gap: decision.Gap,
		})
	}
	var err error
	if identified, ok := f.ingest.(IdentifiedFetchIngestor); ok {
		_, err = identified.IngestSourceItems(ctx, src.ID, src.Kind, items)
	} else {
		_, err = f.ingest.IngestSource(ctx, src.ID, src.Kind, urls)
	}
	if err != nil {
		return c.fail(ctx, "queue", "queue", "filler auto-fetch: queueing failed", err)
	}
	c.outcomes[sourceprovider.OutcomeQueued] += len(urls)
	// Only a successful queue becomes in-pass state. A failed queue must leave an exact
	// identity eligible for a healthy retry later in this pass.
	for _, decision := range selected {
		p.inPass[decision.Candidate.Identity.Key()] = RemoteQueued
	}
	p.res.Queued += len(urls)
	p.providerQueued[src.Kind] += len(urls)
	// ⚠ Stamped only AFTER a successful queue, and only when something was actually queued —
	// a source whose whole page was already catalogued completes in run without reaching
	// this. "Last fetched" must mean "last brought something in", or a source that has been
	// polled fruitlessly for a week reads as freshly productive.
	//
	// A failure here is logged, not fatal: the clips ARE queued, and losing the whole pass
	// over a cosmetic timestamp would be the worse trade.
	if err := f.store.MarkFetched(ctx, src.ID, f.now()); err != nil {
		f.log.Warn("filler auto-fetch: could not stamp the source", "source", src.ID, "err", err)
	}
	if err := c.complete(ctx); err != nil {
		return err
	}
	if b.providerBound && p.providerQueued[src.Kind] >= p.providerLimit && c.checkpoint.Cursor != "" {
		p.res.StoppedBy = "provider"
	}
	return nil
}

// complete ends the check: the store records its checkpoint and outcomes and releases the lease,
// and the outcomes join the pass's.
func (c *sourceCheck) complete(ctx context.Context) error {
	f, res := c.pass.f, c.pass.res
	if err := f.store.CompleteCheck(ctx, c.src.ID, c.leaseUntil, SourceCheckCompletion{
		CheckedAt: f.now(), Checkpoint: c.checkpoint,
		Outcomes: c.outcomes, ReplaceOutcomes: c.replaceOutcomes,
	}); err != nil {
		return fmt.Errorf("complete source %q check: %w", c.src.ID, err)
	}
	if len(c.outcomes) > 0 && res.Outcomes == nil {
		res.Outcomes = sourceprovider.Outcomes{}
	}
	for outcome, count := range c.outcomes {
		res.Outcomes[outcome] += count
	}
	return nil
}

// fail records a failed check, so the source retries after its backoff, then reports cause.
// stage names the failure in the record's error; verb names the step in the reported one.
func (c *sourceCheck) fail(ctx context.Context, stage, verb, warning string, cause error) error {
	f := c.pass.f
	retryAt := f.now().Add(SourceCheckRetryDelay(c.src.CheckFailureCount + 1))
	if err := f.store.FailCheck(ctx, c.src.ID, c.leaseUntil, retryAt); err != nil {
		return fmt.Errorf("record source %q %s failure: %w", c.src.ID, stage, err)
	}
	return c.report(verb, warning, cause)
}

// report is a failure's effect on the pass: a manual run fails with it, while a scheduled pass
// logs it and moves on to its next source.
func (c *sourceCheck) report(verb, warning string, cause error) error {
	if !c.pass.scheduled {
		return fmt.Errorf("%s source %q: %w", verb, c.src.ID, cause)
	}
	c.pass.f.log.Warn(warning, "source", c.src.ID, "err", cause)
	return nil
}

// searchOpen comes after the sources (#2059 decision 10): only an unattended pass the sources left
// short asks the whole provider, and only for nearby stations in the eras it is short of.
func (p *fetchPass) searchOpen(ctx context.Context) error {
	gaps := p.gaps
	if !p.scheduled || !p.targeted || p.f.open == nil || p.res.StoppedBy == "target" || len(gaps.Stations) == 0 || len(gaps.Eras) == 0 {
		return nil
	}
	count := p.globalPerRun
	if gaps.Short > 0 {
		count = min(count, gaps.Short-p.res.Queued)
	}
	provider := string(p.f.open.Provider())
	count = min(count, p.providerLimit-p.providerQueued[provider])
	if count <= 0 {
		return nil
	}
	queued, err := p.f.openSearch(ctx, gaps, p.inPass, count)
	if err != nil {
		return err
	}
	p.res.Queued += queued
	p.providerQueued[provider] += queued
	return nil
}

// sharedRejection is the rejection every provider's items are held to: a compilation reel waits
// for certified splitting (#1773). What a provider's own metadata rules out is the provider's.
func (f *Fetcher) sharedRejection(source FetchSource, item sourceprovider.Item) (sourceprovider.Outcome, bool) {
	if f.limits.Compilations.Defers(source.Kind, int64(item.DurationMS)) {
		return sourceprovider.OutcomeTooLong, true
	}
	return "", false
}

// openSearch searches the whole provider for up to OpenSearchesPerPass of the pass's call signs and
// queues at most count of the hits its listing gate admits. A failed search or gate costs only the
// open search; only a context error ends the pass.
func (f *Fetcher) openSearch(ctx context.Context, gaps CoverageGaps, inPass map[string]ExistingRemoteState, count int) (int, error) {
	gate, err := f.open.Gate(ctx)
	if err != nil {
		f.log.Warn("filler auto-fetch: open search skipped; its listing gate is unavailable", "err", err)
		return 0, nil
	}
	n := min(OpenSearchesPerPass, len(gaps.Stations))
	start := int(f.now().Unix()/3600) % len(gaps.Stations)
	seen := map[string]bool{}
	refused := map[string]int{}
	var candidates []AcquisitionCandidate
	for i := range n {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		sign := gaps.Stations[(start+i)%len(gaps.Stations)]
		refs, err := f.open.OpenSearch(ctx, sign, count*4)
		if err != nil {
			f.log.Warn("filler auto-fetch: open search failed", "call_sign", sign, "err", err)
			continue
		}
		for _, ref := range refs {
			if ref.ID == "" || seen[ref.ID] {
				continue
			}
			seen[ref.ID] = true
			callSign, why := gate.Admit(ref, gaps.Eras)
			if why != "" {
				refused[why]++
				continue
			}
			candidates = append(candidates, AcquisitionCandidate{
				Identity: RemoteIdentity{Provider: string(f.open.Provider()), SourceID: OpenSearchSourceID, RemoteID: ref.ID},
				URL:      ref.URL, Title: ref.Title, License: ref.License,
				ObservedYear: ref.ObservedYear, PublishedAt: ref.PublishedAt,
				DurationMS: ref.DurationMS, Height: ref.Height,
				StationSearch: callSign,
			})
		}
	}
	if len(candidates) == 0 {
		f.log.Info("filler auto-fetch: open search admitted nothing", "searched", n, "refused", refused)
		return 0, nil
	}
	selection, err := PlanAcquisitionFor(AcquisitionIntent{
		Count:         count,
		CatalogReason: "Open search for a nearby station: the registered sources left the pool short.",
	}, candidates, inPass, gaps)
	if err != nil {
		f.log.Warn("filler auto-fetch: open-search hits could not be selected", "err", err)
		return 0, nil
	}
	items := make([]sourceprovider.Item, 0, len(selection.Selected))
	urls := make([]string, 0, len(selection.Selected))
	for _, decision := range selection.Selected {
		c := decision.Candidate
		urls = append(urls, c.URL)
		items = append(items, sourceprovider.Item{ID: c.Identity.RemoteID, URL: c.URL, Title: c.Title,
			License: c.License, ObservedYear: c.ObservedYear, PublishedAt: c.PublishedAt,
			DurationMS: c.DurationMS, Height: c.Height, Gap: decision.Gap})
	}
	if len(items) == 0 {
		return 0, nil
	}
	if identified, ok := f.ingest.(IdentifiedFetchIngestor); ok {
		_, err = identified.IngestSourceItems(ctx, OpenSearchSourceID, string(f.open.Provider()), items)
	} else {
		_, err = f.ingest.IngestSource(ctx, OpenSearchSourceID, string(f.open.Provider()), urls)
	}
	if err != nil {
		f.log.Warn("filler auto-fetch: queueing open-search hits failed", "err", err)
		return 0, nil
	}
	for _, decision := range selection.Selected {
		inPass[decision.Candidate.Identity.Key()] = RemoteQueued
	}
	f.log.Info("filler auto-fetch: open search", "searched", n, "queued", len(items), "refused", refused)
	return len(items), nil
}

// logStop reports a limit that ended a pass. ⚠ At Info, not Debug: this is the answer to "why
// did my catalog stop growing", and an operator who cannot find it concludes the feature is
// broken (§10).
func (f *Fetcher) logStop(which string, have, max int) {
	if f.log == nil {
		return
	}
	f.log.Info("filler auto-fetch paused at its limit",
		"limit", which, "have", have, "max", max,
		"note", "raise filler.fetch.max_catalog_clips to continue, or queue clips by hand")
}
