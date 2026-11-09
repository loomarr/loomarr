// Package operator runs what a filler operator asks for: downloading clips into the drop folder,
// splitting a compilation, rewinding a clip's ladder, syncing the catalog, and reporting the
// readiness of the whole filler pipeline. The HTTP surface and the composition root adapt it; it
// owns the rules, the durable operation records and the progress it announces.
//
// Anything the person has to wait for runs in the background, launched through the generation's
// launcher so a restart owns it, and reports through an Events sink the composition root builds
// the wire frames for.
package operator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/fetch"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/source"
	"github.com/loomarr/loomarr/internal/filler/split"
	"github.com/loomarr/loomarr/internal/filler/storeadapter"
	"github.com/loomarr/loomarr/internal/platform/storagegovernor"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// ErrIngestUnavailable reports that this install cannot run the ingest tooling. It is NOT
// a configuration error — no setting can assert that a binary executes — which is why the
// API renders it as a distinct problem type rather than feature_not_configured.
//
// The single image always ships the tooling (§16), so this means a DEGRADED install: a custom
// build without the vendored binaries, or a path that is missing or not executable.
var ErrIngestUnavailable = errors.New("ingest tooling not present in this image")

// ErrSplitUnavailable reports that compilation splitting cannot run because the
// filler drop-folder (filler.dir) is unset — clip paths are relative to it, so
// without it there is nothing to cut. Unlike ErrIngestUnavailable this IS a
// configuration remedy: Settings → Filler opens it (§10, V34).
var ErrSplitUnavailable = errors.New("splitting needs the filler drop-folder (filler.dir)")

// Launcher starts one request-launched operation under the generation that owns it: the work runs
// to completion or until that generation shuts down, whatever happens to the request.
type Launcher func(timeout time.Duration, run func(context.Context) error, done func(context.Context, error)) error

// Events receives the progress of the operations the operator starts.
type Events interface {
	// IngestProgress reports one ingest-progress frame. Empty counts sources that yielded
	// nothing without erroring.
	IngestProgress(jobID, status string, res fetch.Result, errMsg string)
	// SplitProgress reports one split-detection frame. The terminal "success" frame carries the
	// proposal id the review UI reads back. clipHash is a HASH and is named like one.
	SplitProgress(jobID, clipHash, status, proposalID string, segments int, errMsg string)
}

// SourceRegistry is the acquisition-side source slice. Readiness is deliberately absent:
// the operator registers and fetches sources but cannot publish a Clip by itself.
type SourceRegistry interface {
	ListFillerProviders(context.Context) ([]records.FillerProvider, error)
	ListFillerSources(context.Context) ([]records.FillerSource, error)
	UpsertFillerSource(context.Context, records.FillerSource) error
}

// AcquisitionWriter persists the durable record of a background download.
type AcquisitionWriter interface {
	UpsertAcquisitionRun(context.Context, acquisition.AcquisitionRun) error
}

// OperationWriter persists the durable record of an interactive operation.
type OperationWriter interface {
	UpsertInteractiveOperation(context.Context, store.InteractiveOperation) error
}

// ReadinessStore is the set of projections the readiness report composes.
type ReadinessStore interface {
	PipelineOverview(context.Context, time.Time) (pipelinestate.PipelineOverview, error)
	ListAcquisitionRuns(context.Context, int, time.Time) ([]acquisition.AcquisitionRun, error)
	AcquisitionRepairSummary(context.Context) (acquisition.RepairSummary, error)
	CountReadySplitProposals(context.Context) (int, error)
}

// Fetcher prepares a download plan. It is nil unless the running image carries the ingest tooling.
type Fetcher interface {
	Prepare(context.Context, []fetch.Source, storagegovernor.Mode) (fetch.AcquisitionPlan, error)
}

// Config is what an Operator needs. Every field is optional: a missing seam leaves its operations
// answering "not configured" (or ErrIngestUnavailable / ErrSplitUnavailable) rather than failing.
type Config struct {
	Syncer *filler.Syncer
	// Fetcher is nil unless the running image carries the ingest tooling (the single image
	// — §16). nil is the normal state on loomarr:latest, not a misconfiguration.
	Fetcher Fetcher
	// AfterIngest closes the watch-folder → catalog → pipeline loop before a successful ingest
	// event is published (§10 V56). Optional only in narrow unit tests.
	AfterIngest func(context.Context) error
	Events      Events
	Log         *slog.Logger
	NewID       func() string
	Timeout     time.Duration
	Start       Launcher
	Operations  OperationWriter
	// Sources registers what an operator added, so the Sources tab can show where clips
	// came from. Narrow interface, not the whole store: the operator has no other reason
	// to reach persistence.
	Sources SourceRegistry
	// Providers suggests and verifies the sources an operator is about to register.
	Providers *source.Registry
	// Acquisitions is the reconnect truth for background downloads. nil is allowed only in
	// narrow tests; production always supplies the store before any job can be accepted.
	Acquisitions AcquisitionWriter
	Readiness    ReadinessStore
	Pool         func(context.Context) (pod.PoolReport, error)
	Now          func() time.Time
	// Splitter / SplitClips back compilation splitting (§10, V34). nil Splitter ⇒
	// no drop-folder configured ⇒ Split/ConfirmSplit answer ErrSplitUnavailable.
	Splitter   *filler.Splitter
	SplitClips storeadapter.Split
	// Pipeline is the ingest pipeline (§10 V51b), so the operator-triggered paths reach the same
	// machinery the cron driver does: a confirmed split enrols its segments, and an ingest nudges
	// the runner instead of leaving a fresh download until the next tick. nil on an install with
	// no drop-folder, where there is nothing to ingest.
	Pipeline *filler.Pipeline
	// AutoFetch supplies the live limit status rendered by /v1/filler/watch. It is the same
	// Fetcher the scheduler runs, so reporting and enforcement cannot drift.
	AutoFetch        *acquisition.Fetcher
	Storage          *storagegovernor.Governor
	StoragePath      string
	StorageAutomatic func() bool
	StorageCleanup   *fetch.AcquisitionCleaner
}

// Operator runs the operator's filler operations.
type Operator struct{ c Config }

// New returns the operator over c.
func New(c Config) *Operator { return &Operator{c: c} }

func (a *Operator) clock() func() time.Time {
	if a.c.Now != nil {
		return a.c.Now
	}
	return time.Now
}

// SuggestSources asks a provider for sources matching query.
func (a *Operator) SuggestSources(ctx context.Context, provider, query string, limit int) ([]source.Suggestion, error) {
	finder, ok := a.c.Providers.Finder(source.ID(provider))
	if !ok {
		return nil, fmt.Errorf("source suggestions are not implemented for %s", provider)
	}
	return finder.Suggest(ctx, query, limit)
}

// ResolveSource resolves what an operator typed to a source of provider.
func (a *Operator) ResolveSource(ctx context.Context, provider, input string) (source.Suggestion, error) {
	finder, ok := a.c.Providers.Finder(source.ID(provider))
	if !ok {
		return source.Suggestion{}, fmt.Errorf("source resolution is not implemented for %s", provider)
	}
	return finder.Resolve(ctx, input)
}

// Readiness composes existing authoritative projections and delegates prioritisation to the
// filler domain. It is intentionally available before the HTTP route so API wiring cannot tempt
// the client to rebuild the decision from lower-level endpoints.
func (a *Operator) Readiness(ctx context.Context) (filler.Readiness, error) {
	if a.c.Readiness == nil || a.c.Pool == nil {
		return filler.Readiness{}, errors.New("filler readiness is not configured")
	}
	now := time.Now().UTC()
	if a.c.Now != nil {
		now = a.c.Now().UTC()
	}
	fetch, err := a.FetchStatus(ctx)
	if err != nil {
		return filler.Readiness{}, err
	}
	pipeline, err := a.c.Readiness.PipelineOverview(ctx, now)
	if err != nil {
		return filler.Readiness{}, err
	}
	pool, err := a.c.Pool(ctx)
	if err != nil {
		return filler.Readiness{}, err
	}
	runs, err := a.c.Readiness.ListAcquisitionRuns(ctx, 20, now)
	if err != nil {
		return filler.Readiness{}, err
	}
	repairs, err := a.c.Readiness.AcquisitionRepairSummary(ctx)
	if err != nil {
		return filler.Readiness{}, err
	}
	storage := filler.StorageStatus{}
	if a.c.Storage != nil {
		decision := a.c.Storage.Snapshot(ctx, a.c.StoragePath)
		snapshot := decision.Snapshot
		storage = filler.StorageStatus{
			TotalBytes: snapshot.TotalBytes, FreeBytes: snapshot.FreeBytes,
			ManagedBytes: snapshot.ManagedBytes, ReservedBytes: snapshot.ReservedBytes,
			FilesystemReservedBytes: snapshot.FilesystemReservedBytes,
			SoftBudgetBytes:         snapshot.SoftBudgetBytes, HardReserveBytes: snapshot.HardReserveBytes,
			AvailableBytes: snapshot.AvailableBytes, State: string(storagegovernor.State(snapshot)),
			PausedBy: string(snapshot.Reason),
		}
		if a.c.StorageAutomatic != nil {
			storage.Automatic = a.c.StorageAutomatic()
		}
	}
	// Incoming's needs-help total, from the same store count Incoming and the watch header use.
	needsYou, err := a.c.Readiness.CountReadySplitProposals(ctx)
	if err != nil {
		return filler.Readiness{}, err
	}
	return filler.ProjectReadiness(filler.ReadinessInput{
		Fetch: fetch, Storage: storage, Pipeline: pipeline, Pool: pool, Runs: runs, Repairs: repairs,
		NeedsYou: needsYou,
	}), nil
}

// PreviewStorageCleanup says what a storage cleanup would remove.
func (a *Operator) PreviewStorageCleanup(ctx context.Context) (acquisition.StorageCleanupPreview, error) {
	if a.c.StorageCleanup == nil {
		return acquisition.StorageCleanupPreview{}, nil
	}
	return a.c.StorageCleanup.Preview(ctx)
}

// CleanupStorage removes what PreviewStorageCleanup listed.
func (a *Operator) CleanupStorage(ctx context.Context) (acquisition.StorageCleanupResult, error) {
	if a.c.StorageCleanup == nil {
		return acquisition.StorageCleanupResult{}, nil
	}
	return a.c.StorageCleanup.Clean(ctx)
}

// FetchStatus is the live auto-fetch limit status.
func (a *Operator) FetchStatus(ctx context.Context) (acquisition.FetchStatus, error) {
	if a.c.AutoFetch == nil {
		return acquisition.FetchStatus{}, nil
	}
	return a.c.AutoFetch.Status(ctx)
}

// Fetch runs the auto-fetch now, for one source or for every due one.
func (a *Operator) Fetch(ctx context.Context, sourceID string) (acquisition.FetchResult, error) {
	if a.c.AutoFetch == nil {
		return acquisition.FetchResult{}, ErrIngestUnavailable
	}
	if sourceID != "" {
		return a.c.AutoFetch.RunSource(ctx, sourceID)
	}
	return a.c.AutoFetch.Run(ctx)
}

// Rewind re-runs a clip's ladder from a stage.
func (a *Operator) Rewind(ctx context.Context, hash string, from pipelinestate.StageID, force bool) error {
	if a.c.Pipeline == nil {
		return errors.New("filler pipeline is not configured")
	}
	return a.c.Pipeline.Rewind(ctx, hash, from, force)
}

// RetryFailure retries a clip's failed stage.
func (a *Operator) RetryFailure(ctx context.Context, hash string) error {
	if a.c.Pipeline == nil {
		return errors.New("filler pipeline is not configured")
	}
	return a.c.Pipeline.RetryFailure(ctx, hash)
}

// Sync scans the configured folders and libraries into the catalog.
func (a *Operator) Sync(ctx context.Context) (int, int, int, int, error) {
	res, err := a.c.Syncer.Sync(ctx)
	return res.Total, res.Added, res.Updated, res.Pruned, err
}

// Ingest downloads clips into the drop-folder (§10). It returns a job id immediately and
// does the work in the background: a playlist can take minutes to hours, so holding the
// HTTP request open would guarantee a gateway timeout on exactly the useful cases.
// Progress rides the SSE bus as `filler_ingest` frames — the same shape the §8.1 model
// pull uses, because it is the same problem (long external process, no request to hold).
// ⚠ Downloads only — it does NOT register a source. See `rememberSources`.
func (a *Operator) Ingest(ctx context.Context, urls []string) (string, error) {
	return a.ingest(ctx, acquisition.TriggerManual, "", acquisitionTargets("", "", urls), nil)
}

// IngestSource is the unattended registered-source path. It preserves source attribution through
// the downloader sidecar while durable acquisition state carries the Enrollment authority.
func (a *Operator) IngestSource(ctx context.Context, sourceID, sourceKind string, urls []string) (string, error) {
	return a.ingest(ctx, acquisition.TriggerSource, "", acquisitionTargets(sourceID, sourceKind, urls), nil)
}

// IngestSourceItems downloads the listed items of a registered source.
func (a *Operator) IngestSourceItems(ctx context.Context, sourceID, sourceKind string, items []source.Item) (string, error) {
	targets := make([]acquisition.AcquisitionTarget, 0, len(items))
	for _, item := range items {
		targets = append(targets, acquisition.AcquisitionTarget{
			SourceID: sourceID, RemoteID: item.ID, Kind: sourceKind, URL: item.URL,
			DurationMS: int64(item.DurationMS), Height: item.Height, Gap: item.Gap,
		})
	}
	return a.ingest(ctx, acquisition.TriggerSource, "", targets, nil)
}

// IngestAsked downloads AND remembers the target, for the one path where an operator named it.
func (a *Operator) IngestAsked(ctx context.Context, urls []string) (string, error) {
	a.rememberSources(ctx, urls)
	return a.ingest(ctx, acquisition.TriggerManual, "", acquisitionTargets("", "", urls), nil)
}

// IngestPull constructs one run for the exact plan, commits its approval before launch,
// and executes the same manifest-backed ingest used by other acquisition paths.
func (a *Operator) IngestPull(
	ctx context.Context,
	pullID string,
	targets []acquisition.AcquisitionTarget,
	commit func(context.Context, acquisition.AcquisitionRun) error,
) (string, error) {
	if commit == nil {
		return "", errors.New("pull approval commit is required")
	}
	return a.ingest(ctx, acquisition.TriggerPull, pullID, targets, commit)
}

func acquisitionTargets(sourceID, sourceKind string, urls []string) []acquisition.AcquisitionTarget {
	targets := make([]acquisition.AcquisitionTarget, 0, len(urls))
	for _, url := range urls {
		targets = append(targets, acquisition.AcquisitionTarget{SourceID: sourceID, Kind: sourceKind, URL: url})
	}
	return targets
}

func (a *Operator) ingest(
	ctx context.Context,
	trigger acquisition.Trigger,
	pullID string,
	targets []acquisition.AcquisitionTarget,
	commit func(context.Context, acquisition.AcquisitionRun) error,
) (string, error) {
	if a.c.Fetcher == nil {
		return "", ErrIngestUnavailable
	}
	if len(targets) == 0 {
		return "", errors.New("filler acquisition requires at least one target")
	}
	if a.c.Start == nil {
		return "", errors.New("filler acquisition lifecycle is not configured")
	}
	providerEnabled := map[string]bool{}
	if a.c.Sources != nil {
		providers, err := a.c.Sources.ListFillerProviders(ctx)
		if err != nil {
			return "", fmt.Errorf("read filler provider policy: %w", err)
		}
		for _, provider := range providers {
			providerEnabled[provider.Kind] = provider.Enabled
		}
	}
	jobID := a.c.NewID()
	sources := make([]fetch.Source, 0, len(targets))
	for _, target := range targets {
		kind := fetch.Kind(target.Kind)
		if kind == "" {
			// Only the deliberate one-off admin path lacks registered source policy.
			kind = fetch.KindForURL(target.URL)
		} else if !a.c.Providers.Fetched(kind) {
			return "", fmt.Errorf("unsupported registered filler source kind %q", target.Kind)
		}
		if enabled, known := providerEnabled[string(kind)]; known && !enabled {
			return "", fmt.Errorf("%w: %s", acquisition.ErrProviderPaused, kind)
		}
		sources = append(sources, fetch.Source{
			ID: target.SourceID, AcquisitionID: jobID,
			Kind: kind, URL: target.URL, RemoteID: target.RemoteID,
			DurationMS: target.DurationMS, Height: target.Height, Gap: target.Gap,
		})
	}
	sourceID := commonAcquisitionSource(targets)
	plan, err := a.c.Fetcher.Prepare(ctx, sources, storagegovernor.Automatic)
	if err != nil {
		return "", err
	}
	handedOff := false
	defer func() {
		if !handedOff {
			plan.Release()
		}
	}()

	now := a.clock()
	run := acquisition.AcquisitionRun{
		ID: jobID, Trigger: trigger, SourceID: sourceID, PullID: pullID,
		Status: acquisition.RunQueued, Requested: len(sources),
		StartedAt: now().UTC(), UpdatedAt: now().UTC(),
	}
	if commit != nil {
		if err := commit(ctx, run); err != nil {
			return "", fmt.Errorf("commit filler pull: %w", err)
		}
	} else if a.c.Acquisitions != nil {
		if err := a.c.Acquisitions.UpsertAcquisitionRun(ctx, run); err != nil {
			return "", fmt.Errorf("queue filler acquisition: %w", err)
		}
	}
	var res fetch.Result
	operationTimeout := a.c.Timeout * time.Duration(len(sources))
	err = a.c.Start(operationTimeout, func(operationCtx context.Context) error {
		run.Status = acquisition.RunRunning
		run.UpdatedAt = now().UTC()
		a.persistAcquisition(operationCtx, run)
		a.publishIngest(jobID, "starting", fetch.Result{}, "")
		res = plan.Run(operationCtx)
		run.Fetched, run.Skipped = res.Fetched, res.Skipped
		run.Failed, run.Empty = res.Failed, res.Empty
		if err := operationCtx.Err(); err != nil {
			return err
		}
		if res.Failed > 0 && res.Fetched == 0 {
			return errors.New("every source failed; check the URLs and the yt-dlp version")
		}
		if a.c.AfterIngest != nil {
			if err := a.c.AfterIngest(operationCtx); err != nil {
				return fmt.Errorf("downloaded files could not be catalogued; the scheduled sync will retry: %w", err)
			}
		}
		return nil
	}, func(completionCtx context.Context, runErr error) {
		if runErr != nil {
			run.Status = acquisition.RunError
			run.Error = runErr.Error()
		} else {
			run.Status = acquisition.RunSuccess
		}
		run.CompletedAt, run.UpdatedAt = now().UTC(), now().UTC()
		a.persistAcquisition(completionCtx, run)
		a.publishIngest(jobID, string(run.Status), res, run.Error)
	})
	if err != nil {
		run.Status = acquisition.RunError
		run.Error = err.Error()
		run.CompletedAt, run.UpdatedAt = now().UTC(), now().UTC()
		a.persistAcquisition(ctx, run)
		return "", err
	}
	handedOff = true
	return jobID, nil
}

func commonAcquisitionSource(targets []acquisition.AcquisitionTarget) string {
	if len(targets) == 0 {
		return ""
	}
	sourceID := targets[0].SourceID
	for _, target := range targets[1:] {
		if target.SourceID != sourceID {
			return ""
		}
	}
	return sourceID
}

func (a *Operator) persistAcquisition(ctx context.Context, run acquisition.AcquisitionRun) {
	if a.c.Acquisitions == nil {
		return
	}
	// The run was durably queued before the goroutine started. Later snapshots are best-effort:
	// a transient store failure must not cancel downloaded bytes or bypass pipeline reconciliation.
	if err := a.c.Acquisitions.UpsertAcquisitionRun(ctx, run); err != nil && a.c.Log != nil {
		a.c.Log.Error("could not persist filler acquisition state",
			"acquisition", run.ID, "status", run.Status, "err", err)
	}
}

// publishIngest emits one ingest-progress frame (§7, type=filler_ingest). Empty counts
// sources that yielded nothing without erroring — a typo'd Archive id returns 200 with no
// items, so without surfacing it the operator sees "fetched:0 failed:0" and no reason.
func (a *Operator) publishIngest(jobID, status string, res fetch.Result, errMsg string) {
	if a.c.Events == nil {
		return
	}
	a.c.Events.IngestProgress(jobID, status, res, errMsg)
}

// Split starts compilation detection on one clip (§10, V34). It returns a job id
// immediately — a full-decode detection pass runs minutes per file — and reports
// over the SSE bus as `filler_split` frames, the same shape as Ingest above. The
// terminal frame carries the proposal id the review UI then reads back.
//
// The clip's existence is checked SYNCHRONOUSLY: "that clip is gone" is an
// answer the caller should get as a 404, not as an SSE error frame seconds later.
func (a *Operator) Split(ctx context.Context, clipID string) (string, error) {
	if a.c.Splitter == nil {
		return "", ErrSplitUnavailable
	}
	if a.c.Start == nil {
		return "", errors.New("filler split lifecycle is not configured")
	}
	if a.c.Operations == nil {
		return "", errors.New("filler split operation store is not configured")
	}
	if _, found, err := a.c.SplitClips.GetClip(ctx, clipID); err != nil {
		return "", err
	} else if !found {
		return "", store.ErrNotFound
	}
	jobID := a.c.NewID()
	now := a.clock()
	operation := store.InteractiveOperation{
		ID: jobID, Kind: store.InteractiveOperationFillerSplit, Subject: clipID,
		Status: store.InteractiveOperationQueued, StartedAt: now().UTC(), UpdatedAt: now().UTC(),
	}
	if err := a.c.Operations.UpsertInteractiveOperation(ctx, operation); err != nil {
		return "", fmt.Errorf("queue filler split: %w", err)
	}
	var proposal *filler.SplitProposal
	err := a.c.Start(a.c.Timeout, func(operationCtx context.Context) error {
		operation.Status = store.InteractiveOperationRunning
		operation.UpdatedAt = now().UTC()
		a.persistInteractiveOperation(operationCtx, operation)
		a.publishSplit(jobID, clipID, "running", "", 0, "")
		p, err := a.c.Splitter.Propose(operationCtx, clipID)
		if err != nil {
			return err
		}
		proposal = p
		// ⚠ Un-park the reel, for the same reason ConfirmSplit enrols its cuts (§10 V54a): the
		// operator-triggered path must leave the clip where the unattended one would. A reel parked
		// at `split`/`review` is claimed by nothing — `ListPipelineWork` takes `running` only — so
		// without this a re-detect writes a freshly scored proposal that no rung will ever read,
		// and the remedy §10 names for a stale proposal silently does half its job.
		//
		// ⚠ STRICTLY AFTER `Propose`. Un-parking first makes the row claimable while detection is
		// still running, and the rung would then ground the outgoing segment list or re-`Propose`
		// the same reel concurrently. A failed detection (above) returns before reaching here, so
		// the row keeps its original state.
		//
		// ⚠ The error is DROPPED, per this adapter's convention: it
		// has no logger and reports through the event bus, whose `filler_split` frames describe
		// DETECTION — publishing "error" here would report a detection that in fact succeeded and
		// whose proposal is saved. `Requeue` logs its own outcome, and the failure mode is the
		// state the reel was already in: parked, reviewable by hand, nothing lost.
		if a.c.Pipeline != nil {
			_, _ = a.c.Pipeline.Requeue(operationCtx, clipID)
		}
		return nil
	}, func(completionCtx context.Context, runErr error) {
		operation.CompletedAt, operation.UpdatedAt = now().UTC(), now().UTC()
		if runErr != nil {
			operation.Status = store.InteractiveOperationError
			operation.Error = runErr.Error()
			a.persistInteractiveOperation(completionCtx, operation)
			a.publishSplit(jobID, clipID, "error", "", 0, runErr.Error())
			return
		}
		operation.Status = store.InteractiveOperationSuccess
		operation.ResultID = proposal.ID
		a.persistInteractiveOperation(completionCtx, operation)
		a.publishSplit(jobID, clipID, "success", proposal.ID, len(proposal.Segments), "")
	})
	if err != nil {
		operation.Status = store.InteractiveOperationError
		operation.Error = err.Error()
		operation.CompletedAt, operation.UpdatedAt = now().UTC(), now().UTC()
		a.persistInteractiveOperation(ctx, operation)
		return "", err
	}
	return jobID, nil
}

func (a *Operator) persistInteractiveOperation(ctx context.Context, operation store.InteractiveOperation) {
	if err := a.c.Operations.UpsertInteractiveOperation(ctx, operation); err != nil && a.c.Log != nil {
		a.c.Log.Error("could not persist interactive operation state",
			"operation", operation.ID, "kind", operation.Kind, "status", operation.Status, "err", err)
	}
}

// ConfirmSplit commits the operator's reviewed cut list (§10, V34) — straight
// through to the splitter, whose validation errors (filler.ErrSplitValidation),
// filed parent (filler.ErrSplitParentNotHeld) and missing-proposal
// (store.ErrNotFound) the API maps to 422/409/404.
func (a *Operator) ConfirmSplit(ctx context.Context, proposalID string, segments []split.Segment) error {
	if a.c.Splitter == nil {
		return ErrSplitUnavailable
	}
	// Confirm owns the complete parent/child durable batch, including terminal parent pipeline
	// completion. The adapter must not add a fallible write after the generation commits: reporting an
	// error then would invite an operator retry of an operation that already succeeded.
	_, err := a.c.Splitter.Confirm(ctx, proposalID, segments)
	return err
}

// publishSplit emits one split-detection frame (§7, type=filler_split). The
// terminal "success" frame is what hands the review UI its proposal id.
// ⚠ The clip field is a HASH and is named like one: calling it a path invites storing a path
// in a field the proposal lookup needs a hash for (§10 V51a).
func (a *Operator) publishSplit(jobID, clipHash, status, proposalID string, segments int, errMsg string) {
	if a.c.Events == nil {
		return
	}
	a.c.Events.SplitProgress(jobID, clipHash, status, proposalID, segments, errMsg)
}

// rememberSources records an archive.org COLLECTION an operator asked for (§10, V33).
//
// ⚠ **Only from `IngestAsked`** — never auto-fetch or an approved pull, which carry the URLs of
// individual ITEMS inside a collection that is already registered. Calling it from those turned
// 5 source rows into 35, one per downloaded clip.
//
// ⚠ Best-effort: it runs before the download and a failure is dropped. Bookkeeping must never be
// able to stop a clip being fetched.
//
// ⚠ Archive only. A YouTube URL is a video, not a source with state worth remembering.
func (a *Operator) rememberSources(ctx context.Context, urls []string) {
	if a.c.Sources == nil {
		return
	}
	for _, u := range urls {
		if fetch.KindForURL(u) != fetch.Archive {
			continue
		}
		id := archiveIDFrom(u)
		if id == "" {
			continue
		}
		now := a.clock()
		// Upsert by id: re-adding a source an operator already has is not an error, and
		// UpsertFillerSource deliberately leaves last_fetched_at alone (see the store).
		// ⚠ The error is deliberately DROPPED, not logged: this adapter has no logger (it
		// reports through the event bus, which is for the download itself), and inventing a
		// second channel for bookkeeping noise would be worse than the silence. The download
		// proceeds either way, which is the property that matters.
		// NewFillerSource, not a struct literal: `Enabled` is a bool, so a literal that omits
		// it registers the source SWITCHED OFF (see the store).
		_ = a.c.Sources.UpsertFillerSource(ctx, records.NewFillerSource(id, string(fetch.Archive), u, id, now()))
	}
}

// archiveIDFrom pulls the identifier out of an archive.org URL. Returns "" for anything that
// is not one, which the caller skips.
func archiveIDFrom(raw string) string {
	const marker = "archive.org/details/"
	i := strings.Index(raw, marker)
	if i < 0 {
		return ""
	}
	id := raw[i+len(marker):]
	if j := strings.IndexAny(id, "/?#"); j >= 0 {
		id = id[:j]
	}
	return id
}
