// Package fillerstore persists the filler pipeline's own state: the remote source registry, the
// hosted-inference accounting and the ledgers layered over it (spoken safety, structure
// assessment, structure windows) (§10).
//
// It extends the core store rather than standing beside it. Its tables live in the same database,
// it runs on the core's store.Handle, and every transaction it takes begins through
// Handle.Begin, so a write that also touches core tables can share one transaction. Dependencies
// point this way only: the core store never imports this package (#1747).
//
// The filler state still in internal/store (clips' pipeline, acquisitions, pulls, decisions,
// enrichment, research, split proposals) moves here in later steps of #1747.
package fillerstore

import (
	"context"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
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
	return extended{Store: core, sqlStore: &sqlStore{db: h, dialect: h.Dialect(), ph: h.Rebind}}
}

// Open opens the core store (store.Open) and extends it.
func Open(ctx context.Context, databaseURL string, autoMigrate bool) (Store, error) {
	core, err := store.Open(ctx, databaseURL, autoMigrate)
	if err != nil {
		return nil, err
	}
	return Extend(core), nil
}

// placeholder rewrites a query written with `?` markers into the dialect's style.
type placeholder func(query string) string

// scannable is the one method *sql.Row and *sql.Rows share.
type scannable = store.Scannable

// The core's time-column codecs, under the names the moved code already uses.
var (
	epoch                   = store.Epoch
	fromEpoch               = store.FromEpoch
	fillerDecisionEpoch     = store.EpochNano
	fromFillerDecisionEpoch = store.FromEpochNano
)
