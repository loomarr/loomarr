// Package fillerstore persists the filler pipeline's own state: the remote source registry, the
// pull approvals and the acquisition runs and artifacts they start, the hosted-inference
// accounting and the ledgers layered over it (spoken safety, structure assessment, structure
// windows) (§10).
//
// It extends the core store rather than standing beside it. Its tables live in the same database,
// it runs on the core's store.Handle, and every transaction it takes begins through
// Handle.Begin, so a write that also touches core tables can share one transaction. Dependencies
// point this way only: the core store never imports this package (#1747).
//
// Enrichment, research, admission decisions, split proposals and the per-clip ingest pipeline live
// here too, and write the clip columns they change only through the core's store.ClipTx, inside
// their own transaction (#1747).
//
// ⚠ The filler store overrides one core method, DeleteClipsNotIn, to prune the pipeline rows and
// split proposals a clip prune orphans. A caller that must keep that cleanup holds this Store, not
// store.Store.
package fillerstore

import (
	"context"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerdecision"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
	"github.com/loomarr/loomarr/internal/fillerresearch"
	"github.com/loomarr/loomarr/internal/fillersafety"
	"github.com/loomarr/loomarr/internal/fillerstructure"
	"github.com/loomarr/loomarr/internal/fillerstructurewindow"
	"github.com/loomarr/loomarr/internal/store"
)

// ErrInferenceNotReserved reports a settlement that lost the reserved-state
// compare-and-swap. A completed/failed/held evaluation is immutable accounting.
var ErrInferenceNotReserved = errors.New("store: inference evaluation is not reserved")

// ErrInferenceBudgetExceeded reports a provider charge above its pre-call
// reservation. The charged fact is still persisted and the evaluation is held.
var ErrInferenceBudgetExceeded = errors.New("store: inference budget exceeded")

// ErrPullNotPending reports a losing pull decision or a pending pull with prior acquisition work.
var ErrPullNotPending = errors.New("store: pull already decided or acquired")

// FillerPullStore is the filler approval gate (§10 V35).
//
// Separate from FillerSourceStore on purpose: a pull is an APPROVAL object that happens to
// reference sources, and folding it in would make "the thing that lists where clips come from"
// also the thing that records what a human agreed to download.
//
// ⚠ There is no Delete. A decided pull is KEPT — the queue's History answers "what did we agree
// to download, and when, and who said so", which a delete erases. Same reason §7 keeps deny
// reasons on title proposals.
type FillerPullStore interface {
	GetPull(ctx context.Context, id string) (filler.Pull, error)
	// ListPulls returns pulls with the given status, newest first; an empty status means all.
	ListPulls(ctx context.Context, status filler.PullStatus) ([]filler.Pull, error)
	UpsertPull(ctx context.Context, p filler.Pull) error
	// CommitPullApproval atomically saves the pending decision and its one queued run.
	// Existing historical runs and losing decisions return ErrPullNotPending.
	CommitPullApproval(ctx context.Context, p filler.Pull, run filler.AcquisitionRun) error
	// DismissPull compares-and-sets pending without overwriting a concurrent approval.
	DismissPull(ctx context.Context, p filler.Pull) error
}

// FillerAcquisitionStore is the reconnect truth for filler downloads and their resulting clip
// lifecycle. It is separate from sources and pulls because one run is an execution record, not a
// source definition or approval decision.
type FillerAcquisitionStore interface {
	// UpsertAcquisitionRun creates unbound runs and updates existing snapshots.
	// Pull-bound creation belongs to CommitPullApproval; execution ownership is immutable.
	UpsertAcquisitionRun(ctx context.Context, run filler.AcquisitionRun) error
	// UpsertAcquisitionArtifacts atomically records the exact downloaded-byte manifest before
	// publication makes any artifact eligible for intake.
	UpsertAcquisitionArtifacts(ctx context.Context, artifacts []filler.AcquisitionArtifact) error
	// AcquisitionArtifactForClip resolves provenance and recovery state for a discovered clip.
	AcquisitionArtifactForClip(ctx context.Context, mediaPath, clipHash string) (filler.AcquisitionArtifact, bool, error)
	// ListRecoverableAcquisitionArtifacts exposes bounded staged/published/repair work.
	ListRecoverableAcquisitionArtifacts(ctx context.Context, limit int) ([]filler.AcquisitionArtifact, error)
	// ListRecoverableAcquisitionArtifactsAfter continues a stable bounded recovery scan.
	ListRecoverableAcquisitionArtifactsAfter(ctx context.Context, after filler.AcquisitionArtifactCursor, limit int) ([]filler.AcquisitionArtifact, error)
	// ListAcquisitionRemoteStates is the acquisition planner's exact-item high-water mark.
	ListAcquisitionRemoteStates(ctx context.Context) (map[string]filler.ExistingRemoteState, error)
	// RecoverInterruptedAcquisitionRuns marks work orphaned by the previous process as failed.
	// The beta is single-replica; startup is therefore the exact ownership boundary.
	RecoverInterruptedAcquisitionRuns(ctx context.Context, at time.Time) (int, error)
	GetAcquisitionRun(ctx context.Context, id string, at time.Time) (filler.AcquisitionRun, error)
	ListAcquisitionRuns(ctx context.Context, limit int, at time.Time) ([]filler.AcquisitionRun, error)
	// AcquisitionRepairSummary reports all currently unresolved artifact repairs without loading
	// the bounded acquisition history page.
	AcquisitionRepairSummary(ctx context.Context) (filler.AcquisitionRepairSummary, error)
}

// FillerInferenceStore owns append-only call attribution and the atomic budget
// reservation that must succeed before hosted inference starts (§10 V62).
type FillerInferenceStore interface {
	ReserveInferenceEvaluation(ctx context.Context, evaluation InferenceEvaluation, budget InferenceBudget) (InferenceEvaluation, error)
	SettleInferenceEvaluation(ctx context.Context, id string, settlement InferenceSettlement) (InferenceEvaluation, error)
	GetInferenceEvaluation(ctx context.Context, id string) (InferenceEvaluation, error)
	ListInferenceEvaluations(ctx context.Context, filter InferenceEvaluationFilter) ([]InferenceEvaluation, error)
}

// FillerStructureAssessmentStore owns the structure-specific journal layered over shared filler
// inference accounting. Duplicate requests remain visible conflicts rather than implicit retries.
type FillerStructureAssessmentStore interface {
	ReserveStructureAssessment(context.Context, fillerstructure.AssessmentReservation, InferenceBudget) (fillerstructure.AssessmentReservationState, error)
	SettleStructureAssessment(context.Context, fillerstructure.AssessmentRecord) error
	GetStructureAssessmentLedgerEntry(context.Context, string) (fillerstructure.AssessmentLedgerEntry, error)
	ListOpenStructureAssessmentLedgerEntries(context.Context, int) ([]fillerstructure.AssessmentLedgerEntry, error)
	ReserveStructureWindowCall(context.Context, fillerstructurewindow.CallReservation, InferenceBudget) (fillerstructurewindow.CallReservationState, error)
	SettleStructureWindowCall(context.Context, fillerstructurewindow.CallRecord) error
	GetStructureWindowCallLedgerEntry(context.Context, string) (fillerstructurewindow.CallLedgerEntry, error)
	ListOpenStructureWindowCallLedgerEntries(context.Context, int) ([]fillerstructurewindow.CallLedgerEntry, error)
}

// FillerSafetyStore owns the path-free, append-only execution ledger for the
// spoken-safety shadow cascade. It is distinct from terminal admission policy.
type FillerSafetyStore interface {
	fillersafety.LedgerRepository
	fillersafety.ExecutionRepository
	ReserveSpokenSafetyInference(context.Context, SpokenSafetyInferenceReservation, InferenceEvaluation, InferenceBudget) (InferenceEvaluation, fillersafety.LedgerEvent, error)
	SettleSpokenSafetyInference(context.Context, SpokenSafetyInferenceSettlement, InferenceSettlement) (InferenceEvaluation, fillersafety.LedgerEvent, error)
	RecoverInterruptedSpokenSafetyRuns(context.Context, time.Time) (int, error)
}

// FillerDecisionStore owns immutable V63 admission results and append-only
// operator actions. Projection rules remain in fillerdecision.Service. An applied action's effect on
// the clip and its pipeline row goes through the core's store.ClipTx, inside the action's transaction.
type FillerDecisionStore interface {
	fillerdecision.Repository
	fillerdecision.AppliedActionRepository
}

// FillerEnrichmentStore owns the accepted per-axis descriptive evidence for clips. Applying a
// candidate is rank-aware and idempotent inside the adapter so no caller can overwrite an item fact
// with weaker inference by choosing a different write path. What it projects onto the clip itself
// goes through the core's store.ClipTx, inside the same transaction.
type FillerEnrichmentStore interface {
	ListFillerEnrichment(ctx context.Context, clipHash string) ([]fillerenrichment.State, error)
	ApplyFillerEnrichment(ctx context.Context, candidate fillerenrichment.State, updatedAt time.Time) (fillerenrichment.State, bool, error)
	ListFillerEnrichmentCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int) ([]store.Clip, error)
	ListFillerEnrichmentCapabilityCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int) ([]store.Clip, error)
	ApplyFillerEnrichmentPass(ctx context.Context, pass fillerenrichment.Pass) (int, error)
}

// FillerResearchStore owns cited context reports and their narrowly bounded country projection.
type FillerResearchStore interface {
	ListFillerResearchCandidates(ctx context.Context, producer, producerVersion, adapter, adapterVersion string, limit int) ([]fillerresearch.Candidate, error)
	SaveFillerResearchReport(ctx context.Context, report fillerresearch.Report) error
	PromoteStoredFillerResearchCountries(ctx context.Context, limit int) (int, error)
	LatestFillerResearchReport(ctx context.Context, clipHash string) (fillerresearch.Report, error)
	ReserveFillerResearchWebRequest(ctx context.Context, month string, provider fillerresearch.WebProvider, limit int, attempt fillerresearch.WebAttempt) (fillerresearch.WebUsage, error)
	CompleteFillerResearchWebRequest(ctx context.Context, month string, success bool, at time.Time) error
	FillerResearchWebUsage(ctx context.Context, month string) (fillerresearch.WebUsage, error)
}

// FillerSplitProposalStore is the persisted split-proposal surface (§10, V34) —
// detector-authored, reviewer-edited cut lists that are NOT clips until
// confirmed. One proposal per compilation clip (re-detection replaces). Confirmation's clip and
// pipeline writes go through the core's store.ClipTx, inside the confirmation's transaction.
type FillerSplitProposalStore interface {
	UpsertSplitProposal(ctx context.Context, p filler.SplitProposal) error
	// GetSplitProposal reads one proposal by id (the review's reconnect truth).
	GetSplitProposal(ctx context.Context, id string) (filler.SplitProposal, error)
	AcquireSplitProposalClaim(ctx context.Context, id, token string, at, expiresAt time.Time) (filler.SplitProposal, error)
	RenewSplitProposalClaim(ctx context.Context, id, token string, expiresAt time.Time) error
	ReleaseSplitProposalClaim(ctx context.Context, id, token string) error
	// ListSplitProposals returns every pending proposal, oldest first — the Incoming tab's
	// "reels" (V35). One read behind that tab, so a restart cannot lose the queue.
	ListSplitProposals(ctx context.Context) ([]filler.SplitProposal, error)
	// ListReadySplitProposalsAfter is the bounded, newest-first Needs-help read. Detection
	// checkpoints are skipped without consuming the page limit.
	ListReadySplitProposalsAfter(ctx context.Context, cursor filler.SplitProposalCursor, limit int) ([]filler.SplitProposal, error)
	CountReadySplitProposals(ctx context.Context) (int, error)
	// CountIncomingConveyorBySource counts, per source, what the Incoming belt shows: held clips
	// plus running/review pipeline rows, minus composites whose split proposal is ready. A plain
	// HeldOnly count would disagree with the page it links to.
	CountIncomingConveyorBySource(ctx context.Context) (map[string]int, error)
	// DeleteSplitProposal removes a proposal after confirm or on reject.
	DeleteSplitProposal(ctx context.Context, id string) error
	// UpdateSplitProposal replaces an EXISTING proposal document; ErrNotFound if the row is gone.
	// Never inserts — see the implementation for why that matters (§10 V54).
	UpdateSplitProposal(ctx context.Context, p filler.SplitProposal) error
	CompletePartialSplitConfirmation(ctx context.Context, completion filler.SplitPartialCompletion) error
	// ListSweepableSplitProposals finds reels whose leftover cuts nobody reviewed inside the
	// window AND which have already produced clips — the only ones the sweep may retire (§10 V54).
	ListSweepableSplitProposals(ctx context.Context, before time.Time) ([]SweepableProposal, error)
	// CompleteSplitConfirmation atomically transitions a fully reviewed split proposal, retained
	// parent, replacement pipelines, and selected child generation (§10 V65).
	CompleteSplitConfirmation(ctx context.Context, completion filler.SplitCompletion) (int, error)
	// Put/ListStructureSplitShadowDecisions own the immutable V67 compatibility-versus-complete-
	// plan history. It survives proposal consumption so publication cannot erase disagreement.
	PutStructureSplitShadowDecision(ctx context.Context, decision filler.StructureSplitShadowDecision) error
	GetStructureSplitShadowDecision(ctx context.Context, id string) (filler.StructureSplitShadowDecision, bool, error)
	ListStructureSplitShadowDecisions(ctx context.Context, clipHash string, limit int) ([]filler.StructureSplitShadowDecision, error)
}

// FillerPipelineStore is the per-clip ingest pipeline (§10 V51b, migration 00044) and terminal
// readiness, the one non-composite publication path.
//
// ⚠ A SIBLING of `clips`, never columns on it: `clips` is a synced cache that has been dropped
// and recreated twice, and these rows record that Whisper seconds and a paid vision call have
// ALREADY been spent. This surface is the table's only writer, so unlike the clip columns there is
// no DO UPDATE omission list to keep in step. The clip columns a transition changes go through the
// core's store.ClipTx, inside the transition's transaction.
type FillerPipelineStore interface {
	// UpsertClipPipeline writes an ordinary runner transition.
	UpsertClipPipeline(ctx context.Context, p filler.ClipPipeline) error
	// RetryClipPipeline writes the recovery transition and, for an exhausted terminal failure,
	// restores the catalog tombstone while holding the clip in the same transaction.
	RetryClipPipeline(ctx context.Context, failed, retry filler.ClipPipeline, restore bool) error
	// GetClipPipeline reads one row. Absence is ordinary (an un-enrolled clip), not an error.
	GetClipPipeline(ctx context.Context, hash string) (filler.ClipPipeline, bool, error)
	// MarkPipelineComplete gives a processed composite its distinct non-playable terminal state.
	MarkPipelineComplete(ctx context.Context, hash string, at time.Time) error
	// ListPipelineWork returns non-terminal rows due at or before `now`, oldest first, with a
	// total order so one clip cannot starve while another is worked repeatedly.
	ListPipelineWork(ctx context.Context, now time.Time, limit int) ([]filler.ClipPipeline, error)
	// PipelineOverview groups the durable state through filler.ClipPipeline.Lifecycle so API,
	// runner telemetry and persistence cannot acquire separate ownership predicates.
	PipelineOverview(ctx context.Context, at time.Time) (filler.PipelineOverview, error)
	// ListClipPipelines serves the Incoming read model — what is moving, and what was refused.
	ListClipPipelines(ctx context.Context, f filler.PipelineFilter) ([]filler.ClipPipeline, error)
	// CountClipPipelines shares ListClipPipelines' lifecycle predicate while ignoring its cursor
	// and limit, so a bounded page and its total cannot describe different populations.
	CountClipPipelines(ctx context.Context, f filler.PipelineFilter) (int, error)
	// ListPreparationWork joins bounded pipeline facts to only the clip duration needed for local
	// progress calibration. It never exposes paths or descriptive media content.
	ListPreparationWork(ctx context.Context, f filler.PipelineFilter) ([]filler.PreparationWork, error)
	// ListClipsWithoutPipeline returns catalogued clips with no pipeline row yet, so enrolment is
	// lazy and self-healing rather than a data migration.
	ListClipsWithoutPipeline(ctx context.Context, limit int) ([]filler.StoreClip, error)
	// CommitFillerReady atomically stores Placement, releases the held clip, settles its conveyor
	// row, and appends the effective Ready event. It is the only non-composite publication path.
	CommitFillerReady(ctx context.Context, commit filler.ReadyCommit) error
	GetFillerReadyEvent(ctx context.Context, clipHash string) (filler.ReadyEvent, bool, error)
}

// FillerSourceStore is the persisted REMOTE filler-source registry (§10, V33).
//
// ⚠ Remote sources only. The drop-folder and the media-server library stay DERIVED from config
// (see `GET /v1/filler/sources`, V28) — they answer "you could set one up but have not", which
// rows cannot express. These describe the specific archive.org collections an operator added, and
// they nest under that read-model's `remote` row rather than replacing any of it.
type FillerSourceStore interface {
	// ListFillerProviders returns the closed provider policy set in stable order.
	ListFillerProviders(ctx context.Context) ([]FillerProvider, error)
	// SetFillerProviderEnabled pauses or resumes future work through one provider without
	// rewriting any child source choice or removing already downloaded clips.
	SetFillerProviderEnabled(ctx context.Context, kind string, enabled bool) error
	// ListFillerSources returns every registered remote source, oldest first, so the UI order
	// is stable across reloads.
	ListFillerSources(ctx context.Context) ([]FillerSource, error)
	// UpsertFillerSource adds or updates one source by id.
	UpsertFillerSource(ctx context.Context, src FillerSource) error
	// DeleteFillerSource removes a source. Clips it already brought in are NOT deleted:
	// they are files in the drop-folder, and forgetting where something came from is not a
	// reason to throw it away.
	DeleteFillerSource(ctx context.Context, id string) error
	// MarkFillerSourceFetched stamps a successful fetch, for the Sources tab's "last fetched".
	MarkFillerSourceFetched(ctx context.Context, id string, at time.Time) error
	// Claim/complete/fail are the durable source-check lease and retry boundary shared by the
	// scheduler and the manual Look for new clips command.
	ClaimFillerSourceCheck(ctx context.Context, id string, observedLastCheck, now, leaseUntil time.Time) (bool, error)
	CompleteFillerSourceCheck(ctx context.Context, id string, leaseUntil time.Time, completion filler.SourceCheckCompletion) error
	FailFillerSourceCheck(ctx context.Context, id string, leaseUntil, retryAt time.Time) error
	// SetFillerSourceFetchPolicy writes one source's per-source fetch overrides (§10 V38c).
	//
	// ⚠ The ONLY writer of those columns, like SetFillerSourceEnabled owns `enabled` — the upsert
	// omits them so a re-register cannot blank an operator's tuning. nil clears an override back
	// to "inherit the global", which is a real action and must be expressible.
	SetFillerSourceFetchPolicy(ctx context.Context, id string, everySeconds, maxPerRun *int) error
	SetFillerSourceGeography(ctx context.Context, id string, geography filler.Geography) error
	// SetFillerSourceEnabled switches a source on or off (V35). ⚠ Disabling is NOT deleting:
	// the row keeps its licence and fetch history, and clips it already brought in stay in the
	// catalog. It only withdraws the source from future searching and downloading.
	SetFillerSourceEnabled(ctx context.Context, id string, enabled bool) error
}

// Store is the core store extended with the filler store: the one value the composition root
// opens and hands to the app, the API and the filler runtime. Callers keep calling one value;
// which half owns a method is this package's business.
type Store interface {
	store.Store
	FillerInferenceStore
	FillerStructureAssessmentStore
	FillerSafetyStore
	FillerSourceStore
	FillerPullStore
	FillerAcquisitionStore
	FillerEnrichmentStore
	FillerResearchStore
	FillerDecisionStore
	FillerSplitProposalStore
	FillerPipelineStore

	// Core returns the store this one extends, for the core functions that need its adapter
	// (backups, migration, schema version); store.Store's own methods are already promoted.
	Core() store.Store
}

// sqlStore implements the filler half over the core store's handle. The receiver name matches the
// core adapter's, so the methods read the same on both sides of the split.
type sqlStore struct {
	db      store.Handle
	dialect store.Dialect
	ph      placeholder
	// core reads clips and the taxonomy through the core's own methods, which own how they are read.
	core store.Store
}

// extended is Store: the core store's methods promoted, the filler half's methods beside them.
type extended struct {
	store.Store
	*sqlStore
}

func (e extended) Core() store.Store { return e.Store }

// Extend builds the filler store over core's handle and returns the two as one Store. An
// already-extended store is returned as it is, and no store extends to none: app.Build without a
// database still serves readiness.
func Extend(core store.Store) Store {
	if core == nil {
		return nil
	}
	if s, ok := core.(Store); ok {
		return s
	}
	h := store.HandleOf(core)
	return extended{Store: core, sqlStore: &sqlStore{db: h, dialect: h.Dialect(), ph: h.Rebind, core: core}}
}

// Open opens the core store (store.Open) and extends it. With autoMigrate it then runs this
// half's boot backfill, after the core's own boot seeds, as store.Open did while enrichment lived
// there. Every opener that migrates opens here, or the backfill would run late, on clips a seed
// tool wrote before the server first opened the database.
func Open(ctx context.Context, databaseURL string, autoMigrate bool) (Store, error) {
	core, err := store.Open(ctx, databaseURL, autoMigrate)
	if err != nil {
		return nil, err
	}
	st := Extend(core)
	if autoMigrate {
		if err := st.(extended).backfillFillerEnrichment(ctx, time.Now()); err != nil {
			_ = core.Close()
			return nil, err
		}
	}
	return st, nil
}

// placeholder rewrites a query written with `?` markers into the dialect's style.
type placeholder func(query string) string

// scannable is the one method *sql.Row and *sql.Rows share.
type scannable = store.Scannable

// The core's Unix-seconds time-column codec, under the names the moved code already uses. The
// nanosecond codec the decision and safety ledgers use is this package's own (fillerDecisionEpoch).
var (
	epoch     = store.Epoch
	fromEpoch = store.FromEpoch
)
