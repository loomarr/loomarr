package filler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode"
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
	// YouTube selection, whose flat listing can supply an honest known duration before download.
	MinDuration func() time.Duration
	MaxDuration func() time.Duration
}

// FetchSource is one pollable source.
type FetchSource struct {
	ID      string
	Kind    string // "archive" | "youtube"
	URI     string
	Enabled bool
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
	// ScanCheckpoint is durable progress through one bounded newest-first YouTube sweep.
	// Archive ignores it because its collection enumeration keeps the existing tolerant path.
	ScanCheckpoint SourceScanCheckpoint
}

// SourceScanCheckpoint resumes a bounded provider listing by stable item identity. Cursor is the
// last item examined in the in-progress sweep; PendingWatermark is the newest item seen when that
// sweep began; Watermark is the newest item committed by the prior completed sweep.
type SourceScanCheckpoint struct {
	Cursor           string
	Watermark        string
	PendingWatermark string
}

// SourceOutcome is a durable, user-explainable result of considering one discovered source item.
// Keep this set closed with migration 00117; extractor-specific errors belong in diagnostics.
type SourceOutcome string

const (
	SourceOutcomeQueued             SourceOutcome = "queued"
	SourceOutcomeAlreadyKnown       SourceOutcome = "already_known"
	SourceOutcomeTooShort           SourceOutcome = "too_short"
	SourceOutcomeTooLong            SourceOutcome = "too_long"
	SourceOutcomeLive               SourceOutcome = "live"
	SourceOutcomeUpcoming           SourceOutcome = "upcoming"
	SourceOutcomePrivate            SourceOutcome = "private"
	SourceOutcomeUnavailable        SourceOutcome = "unavailable"
	SourceOutcomeMetadataIncomplete SourceOutcome = "metadata_incomplete"
)

// SourceCheckSummary aggregates item outcomes for the current or most recently completed sweep.
type SourceCheckSummary map[SourceOutcome]int

// SourceCheckCompletion is committed atomically with lease release. ReplaceOutcomes begins a new
// sweep; otherwise Outcomes extend the summary retained by an in-progress cursor.
type SourceCheckCompletion struct {
	CheckedAt       time.Time
	Checkpoint      SourceScanCheckpoint
	Outcomes        SourceCheckSummary
	ReplaceOutcomes bool
}

const SourceCheckLease = 30 * time.Minute

// YouTubeInitialLookback bounds one newest-first sweep. A cursor can spread that sweep over
// several per-source checks, but registration never means walking an unbounded channel history.
const YouTubeInitialLookback = 100

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

// SourceEnumerator lists what one registered source currently offers without downloading media.
// The whole FetchSource crosses this seam because Kind is authoritative: Archive and YouTube are
// peer lanes, and inferring a provider from an enumerated item's URL would let listing disagree
// with the registered source policy.
type SourceEnumerator interface {
	Enumerate(ctx context.Context, source FetchSource, limit int) ([]DiscoveredRef, int, error)
}

// DiscoveredRef is one item a source offers — an id stable enough to dedupe on, and the URL to
// hand the ingest path.
type DiscoveredRef struct {
	ID      string
	URL     string
	Title   string
	License string
	// ObservedYear is provider metadata used only to rank acquisition. It is not grounded clip era.
	ObservedYear  int
	PublishedAt   string
	DurationMS    int
	DurationKnown bool
	Height        int
	Availability  string
	LiveStatus    string
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
	IngestSourceItems(ctx context.Context, sourceID, sourceKind string, items []DiscoveredRef) (string, error)
}

// Fetcher polls registered sources.
type Fetcher struct {
	store  FetchStore
	enum   SourceEnumerator
	ingest FetchIngestor
	limits FetchLimits
	log    *slog.Logger
	now    func() time.Time
}

// NewFetcher builds the automatic-download worker. Source policy, including whether any source
// should run at all, arrives through ListFetchSources on every pass so global and per-source edits
// hot-apply through one authority.
func NewFetcher(store FetchStore, enumerator SourceEnumerator, ingest FetchIngestor, limits FetchLimits, log *slog.Logger) *Fetcher {
	return &Fetcher{
		store: store, enum: enumerator, ingest: ingest, limits: limits, log: log, now: time.Now,
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
	Outcomes SourceCheckSummary
	// MaxPerCheck is the selected source's effective cap on a manual run.
	MaxPerCheck int
	// StoppedBy names the limit that ended the pass early ("catalog", "provider", or ""). ⚠ Reported
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
		if source.Enabled && !source.NeverFetch && source.URI != "" && source.Kind != "folder" && source.Kind != "library" {
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

func (f *Fetcher) run(ctx context.Context, sourceID string, scheduled bool) (FetchResult, error) {
	var res FetchResult
	srcs, err := f.store.ListFetchSources(ctx)
	if err != nil {
		return res, fmt.Errorf("list sources: %w", err)
	}
	globalPerRun := f.limits.MaxPerRun()
	if globalPerRun < 1 {
		globalPerRun = 1
	}
	providerLimit := 50
	if f.limits.MaxProviderPerRun != nil && f.limits.MaxProviderPerRun() > 0 {
		providerLimit = f.limits.MaxProviderPerRun()
	}
	if sourceID != "" {
		res.MaxPerCheck = globalPerRun
		found := false
		for _, src := range srcs {
			if src.ID != sourceID {
				continue
			}
			found = true
			if src.MaxPerRun > 0 {
				res.MaxPerCheck = src.MaxPerRun
			}
			break
		}
		if !found {
			return res, ErrFetchSourceNotFound
		}
		if res.MaxPerCheck > providerLimit {
			res.MaxPerCheck = providerLimit
		}
	}

	// Resolve the due set before reading catalog or filesystem capacity. The scheduler wakes once
	// a minute, while an ordinary source is due every six hours; an idle wake must remain a cheap
	// source-state read rather than walking the filler directory 359 extra times between checks.
	now := f.now()
	due := make([]FetchSource, 0, len(srcs))
	for _, src := range srcs {
		if sourceID != "" && src.ID != sourceID {
			continue
		}
		if !src.Enabled || src.URI == "" || src.Kind == "folder" || src.Kind == "library" {
			if sourceID != "" && src.ID == sourceID && !src.Enabled {
				return res, ErrSourceDisabled
			}
			continue
		}
		if scheduled {
			next, automatic := src.NextAutomaticCheck(now)
			if !automatic || next.After(now) {
				continue
			}
		}
		due = append(due, src)
	}
	if len(due) == 0 {
		return res, nil
	}

	have, err := f.store.CatalogPaths(ctx)
	if err != nil {
		return res, fmt.Errorf("read catalog: %w", err)
	}
	// The catalog is one input to selection; source-local history and acquisition intent also
	// determine whether a discovered item is eligible to be offered again.
	existing, err := f.store.ListAcquisitionRemoteStates(ctx)
	if err != nil {
		return res, fmt.Errorf("read acquisition remote states: %w", err)
	}
	if existing == nil {
		existing = map[string]ExistingRemoteState{}
	}
	// Treat the store result as a snapshot: in-pass queue state must not mutate adapter-owned
	// fixture or store state across retry attempts.
	inPass := make(map[string]ExistingRemoteState, len(existing))
	for key, state := range existing {
		inPass[key] = state
	}

	if max := f.limits.MaxCatalogClips(); max > 0 && len(have) >= max {
		res.StoppedBy = "catalog"
		f.logStop("catalog", len(have), max)
		return res, nil
	}
	providerQueued := map[string]int{}
	for _, src := range due {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}
		if providerQueued[src.Kind] >= providerLimit {
			res.StoppedBy = "provider"
			continue
		}
		// Property 1: only enabled sources, and only ones with somewhere to fetch FROM. The
		// config-backed folder/library rows have no URI and are scanned, not fetched.
		//
		// NeverFetch is the source's effective automatic opt-out, distinct from Enabled. A
		// source can be on — searched, its clips counted, its switch showing "on" — while opting
		// out of UNATTENDED fetching. Collapsing the two would make "stop auto-downloading from
		// this one" require switching it off entirely, which also stops search.
		// This source's cap: its own override, or the global. A busy collection and a small
		// playlist want different numbers, which one figure served badly.
		perRun := src.MaxPerRun
		if perRun < 1 {
			perRun = globalPerRun
		}
		remaining := providerLimit - providerQueued[src.Kind]
		providerBound := perRun > remaining
		if providerBound {
			perRun = remaining
		}
		checkAt := f.now()
		leaseUntil := checkAt.Add(SourceCheckLease)
		claimed, claimErr := f.store.ClaimCheck(ctx, src.ID, src.LastCheckedAt, checkAt, leaseUntil)
		if claimErr != nil {
			return res, fmt.Errorf("claim source %q: %w", src.ID, claimErr)
		}
		if !claimed {
			if !scheduled {
				return res, fmt.Errorf("%w: %s", ErrSourceCheckInProgress, src.ID)
			}
			continue
		}
		res.SourcesPolled++

		// Over-list so that a page full of already-held items still yields new ones. Without
		// this a source whose first N items are all catalogued would report "nothing new"
		// forever while the rest of the collection sat unfetched.
		enumerationLimit := perRun * 4
		if src.Kind == "youtube" {
			enumerationLimit = YouTubeInitialLookback
		}
		items, _, derr := f.enum.Enumerate(ctx, src, enumerationLimit)
		if derr != nil {
			retryAt := f.now().Add(SourceCheckRetryDelay(src.CheckFailureCount + 1))
			if failErr := f.store.FailCheck(ctx, src.ID, leaseUntil, retryAt); failErr != nil {
				return res, fmt.Errorf("record source %q check failure: %w", src.ID, failErr)
			}
			if !scheduled {
				return res, fmt.Errorf("list source %q: %w", src.ID, derr)
			}
			f.log.Warn("filler auto-fetch: source could not be listed", "source", src.ID, "err", derr)
			continue
		}
		outcomes := SourceCheckSummary{}
		checkpoint := src.ScanCheckpoint
		replaceOutcomes := true
		completeCheck := func() error {
			if completeErr := f.store.CompleteCheck(ctx, src.ID, leaseUntil, SourceCheckCompletion{
				CheckedAt: f.now(), Checkpoint: checkpoint,
				Outcomes: outcomes, ReplaceOutcomes: replaceOutcomes,
			}); completeErr != nil {
				return fmt.Errorf("complete source %q check: %w", src.ID, completeErr)
			}
			if len(outcomes) > 0 {
				if res.Outcomes == nil {
					res.Outcomes = SourceCheckSummary{}
				}
				for outcome, count := range outcomes {
					res.Outcomes[outcome] += count
				}
			}
			return nil
		}

		// The scheduled path uses the same deterministic selector as explicit pulls. There are no
		// hard semantic constraints here—the remote listing cannot certify them—but declared rights,
		// representation quality, era-observation diversity, and stable identity now choose the
		// bounded prefix instead of whichever item the provider returned first.
		candidates := make([]AcquisitionCandidate, 0, len(items))
		preferredCampaigns := map[string]AcquisitionCandidate{}
		if src.Kind == "youtube" {
			// The cursor can split one provider listing across several checks. Choose the best
			// explicit duration variant from the whole bounded listing before applying that cursor,
			// otherwise a :15 cut in one check and a :30 cut in the next both reach the catalog.
			for _, item := range items {
				if _, rejected := f.automaticRejection(src, item); rejected {
					continue
				}
				campaign := sourceCampaignKey(item.Title)
				if campaign == "" {
					continue
				}
				candidate := AcquisitionCandidate{
					Identity: RemoteIdentity{Provider: src.Kind, SourceID: src.ID, RemoteID: item.ID},
					URL:      item.URL, Title: item.Title, License: item.License, ObservedYear: item.ObservedYear,
					PublishedAt: item.PublishedAt, DurationMS: item.DurationMS, Height: item.Height,
				}
				current, found := preferredCampaigns[campaign]
				if !found || sourceVariantBetter(candidate, current) {
					preferredCampaigns[campaign] = candidate
				}
			}
		}
		consider := items
		if src.Kind == "youtube" {
			start := 0
			replaceOutcomes = checkpoint.Cursor == ""
			if checkpoint.Cursor != "" {
				found := false
				for i, item := range items {
					if item.ID == checkpoint.Cursor {
						start, found = i+1, true
						break
					}
				}
				if !found {
					// Provider lists can shift or remove an item. Restart the bounded sweep and rely
					// on exact durable identity dedupe; guessing an offset could skip unseen work.
					checkpoint.Cursor = ""
					checkpoint.PendingWatermark = ""
					replaceOutcomes = true
				}
			}
			if start < len(items) && checkpoint.PendingWatermark == "" {
				checkpoint.PendingWatermark = items[0].ID
			}
			consider = items[start:]
		}
		lastExamined := ""
		reachedYouTubeCap := false
		for i, item := range consider {
			if src.Kind == "youtube" && checkpoint.Watermark != "" && item.ID == checkpoint.Watermark {
				break
			}
			if item.ID != "" {
				lastExamined = item.ID
			}
			if outcome, rejected := f.automaticRejection(src, item); rejected {
				outcomes[outcome]++
				continue
			}
			if campaign := sourceCampaignKey(item.Title); campaign != "" {
				if preferred := preferredCampaigns[campaign]; preferred.Identity.RemoteID != item.ID {
					res.Skipped++
					outcomes[SourceOutcomeAlreadyKnown]++
					continue
				}
			}
			identity := RemoteIdentity{Provider: src.Kind, SourceID: src.ID, RemoteID: item.ID}
			if src.Kind == "youtube" && inPass[identity.Key()] != "" {
				res.Skipped++
				outcomes[SourceOutcomeAlreadyKnown]++
				continue
			}
			candidate := AcquisitionCandidate{
				Identity: identity,
				URL:      item.URL, Title: item.Title, License: item.License,
				ObservedYear: item.ObservedYear, PublishedAt: item.PublishedAt,
				DurationMS: item.DurationMS, Height: item.Height,
			}
			candidates = append(candidates, candidate)
			if src.Kind == "youtube" && len(candidates) == perRun {
				reachedYouTubeCap = i < len(consider)-1
				break
			}
		}
		if src.Kind == "youtube" {
			if reachedYouTubeCap {
				checkpoint.Cursor = lastExamined
			} else {
				if checkpoint.PendingWatermark != "" {
					checkpoint.Watermark = checkpoint.PendingWatermark
				}
				checkpoint.Cursor = ""
				checkpoint.PendingWatermark = ""
			}
		}
		selection, serr := PlanAcquisition(AcquisitionIntent{
			Count:         perRun,
			CatalogReason: "Bounded scheduled refresh of a registered source.",
		}, candidates, inPass)
		if serr != nil {
			if completeErr := completeCheck(); completeErr != nil {
				return res, completeErr
			}
			if !scheduled {
				return res, fmt.Errorf("select source %q: %w", src.ID, serr)
			}
			f.log.Warn("filler auto-fetch: source candidates could not be selected", "source", src.ID, "err", serr)
			continue
		}
		var urls []string
		selectedItems := make([]DiscoveredRef, 0, len(selection.Selected))
		for _, decision := range selection.Rejected {
			switch decision.Disposition {
			case CandidateAlreadyCatalogued, CandidateAlreadyQueued, CandidatePreviouslyDeclined:
				res.Skipped++
				outcomes[SourceOutcomeAlreadyKnown]++
			}
		}
		for _, decision := range selection.Selected {
			urls = append(urls, decision.Candidate.URL)
			selectedItems = append(selectedItems, DiscoveredRef{
				ID: decision.Candidate.Identity.RemoteID, URL: decision.Candidate.URL,
				Title: decision.Candidate.Title, License: decision.Candidate.License,
				ObservedYear: decision.Candidate.ObservedYear, PublishedAt: decision.Candidate.PublishedAt,
				DurationMS: decision.Candidate.DurationMS, Height: decision.Candidate.Height,
			})
		}
		if len(urls) == 0 {
			if completeErr := completeCheck(); completeErr != nil {
				return res, completeErr
			}
			continue
		}
		var ierr error
		if identified, ok := f.ingest.(IdentifiedFetchIngestor); ok {
			_, ierr = identified.IngestSourceItems(ctx, src.ID, src.Kind, selectedItems)
		} else {
			_, ierr = f.ingest.IngestSource(ctx, src.ID, src.Kind, urls)
		}
		if ierr != nil {
			retryAt := f.now().Add(SourceCheckRetryDelay(src.CheckFailureCount + 1))
			if failErr := f.store.FailCheck(ctx, src.ID, leaseUntil, retryAt); failErr != nil {
				return res, fmt.Errorf("record source %q queue failure: %w", src.ID, failErr)
			}
			if !scheduled {
				return res, fmt.Errorf("queue source %q: %w", src.ID, ierr)
			}
			f.log.Warn("filler auto-fetch: queueing failed", "source", src.ID, "err", ierr)
			continue
		}
		outcomes[SourceOutcomeQueued] += len(urls)
		// Only a successful queue becomes in-pass state. A failed queue must leave an exact
		// identity eligible for a healthy retry later in this pass.
		for _, decision := range selection.Selected {
			inPass[decision.Candidate.Identity.Key()] = RemoteQueued
		}
		res.Queued += len(urls)
		providerQueued[src.Kind] += len(urls)
		// ⚠ Stamped only AFTER a successful queue, and only when something was actually queued —
		// a source whose whole page was already catalogued `continue`s above without touching
		// this. "Last fetched" must mean "last brought something in", or a source that has been
		// polled fruitlessly for a week reads as freshly productive.
		//
		// A failure here is logged, not fatal: the clips ARE queued, and losing the whole pass
		// over a cosmetic timestamp would be the worse trade.
		if merr := f.store.MarkFetched(ctx, src.ID, f.now()); merr != nil {
			f.log.Warn("filler auto-fetch: could not stamp the source", "source", src.ID, "err", merr)
		}
		if completeErr := completeCheck(); completeErr != nil {
			return res, completeErr
		}
		if providerBound && providerQueued[src.Kind] >= providerLimit && checkpoint.Cursor != "" {
			res.StoppedBy = "provider"
		}
	}

	if res.Queued > 0 {
		f.log.Info("filler auto-fetch", "sources", res.SourcesPolled, "queued", res.Queued, "skipped", res.Skipped)
	}
	if res.StoppedBy == "provider" && f.log != nil {
		f.log.Info("filler auto-fetch paused at the provider pass limit",
			"max_per_provider", providerLimit,
			"note", "remaining due sources will be considered on the next scheduler pass")
	}
	return res, nil
}

func sourceCampaignKey(title string) string {
	fields := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(title)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(fields) == 0 {
		return ""
	}
	durationAt := -1
	for index, field := range fields {
		if !sourceDurationToken(field) {
			continue
		}
		beforeTime := index > 0 && sourceTimeWord(fields[index-1])
		afterTime := index+1 < len(fields) && sourceTimeWord(fields[index+1])
		beforeLanguage := index > 0 && sourceLanguageWord(fields[index-1]) && index == len(fields)-1
		afterLanguage := index+1 < len(fields) && sourceLanguageWord(fields[index+1]) && index+2 == len(fields)
		if beforeTime || afterTime || beforeLanguage || afterLanguage {
			durationAt = index
			break
		}
	}
	if durationAt < 0 {
		return ""
	}
	key := make([]string, 0, len(fields)-1)
	for index, field := range fields {
		if index == durationAt || sourceTimeWord(field) || field == "spot" {
			continue
		}
		key = append(key, field)
	}
	return strings.Join(key, " ")
}

func sourceDurationToken(value string) bool {
	switch value {
	case "5", "10", "15", "20", "30", "45", "60", "90", "120":
		return true
	default:
		return false
	}
}

func sourceTimeWord(value string) bool {
	switch value {
	case "s", "sec", "secs", "second", "seconds":
		return true
	default:
		return false
	}
}

func sourceLanguageWord(value string) bool {
	switch value {
	case "english", "spanish", "french", "german", "italian", "portuguese":
		return true
	default:
		return false
	}
}

func sourceVariantBetter(candidate, current AcquisitionCandidate) bool {
	if candidate.DurationMS != current.DurationMS {
		return candidate.DurationMS > current.DurationMS
	}
	if candidate.Height != current.Height {
		return candidate.Height > current.Height
	}
	return candidate.Identity.Key() < current.Identity.Key()
}

func (f *Fetcher) automaticRejection(source FetchSource, item DiscoveredRef) (SourceOutcome, bool) {
	if source.Kind != "youtube" {
		return "", false
	}
	if item.ID == "" || item.URL == "" {
		return SourceOutcomeMetadataIncomplete, true
	}
	switch item.Availability {
	case "private":
		return SourceOutcomePrivate, true
	case "needs_auth", "premium_only", "subscriber_only", "unavailable":
		return SourceOutcomeUnavailable, true
	}
	switch item.LiveStatus {
	case "is_live", "post_live":
		return SourceOutcomeLive, true
	case "is_upcoming":
		return SourceOutcomeUpcoming, true
	}
	if !item.DurationKnown {
		return SourceOutcomeMetadataIncomplete, true
	}
	if f.limits.MinDuration != nil && time.Duration(item.DurationMS)*time.Millisecond < f.limits.MinDuration() {
		return SourceOutcomeTooShort, true
	}
	if f.limits.MaxDuration != nil {
		max := f.limits.MaxDuration()
		if max > 0 && time.Duration(item.DurationMS)*time.Millisecond > max {
			return SourceOutcomeTooLong, true
		}
	}
	return "", false
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
