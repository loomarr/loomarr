package filler

import (
	"context"
	"log/slog"
	"time"

	"github.com/mantonx/loomarr/internal/taxonomy"
)

// The taxonomy reindex job (§10 V45a).
//
// `clip_tags` is a DERIVED cache of (clips × taxonomy graph): each clip's asserted leaves expanded to
// their ancestor rollups, so a curation query `WHERE taxon = 'food'` is one index hit. When an
// operator edits the graph — remaps a parent, renames, deletes a taxon — every clip's rollups may be
// stale. This job rebuilds them from the current graph.
//
// ⚠ **It is a SIBLING of the transcribe/vision jobs in WIRING and LIFECYCLE, not in body** (§10).
// Those jobs loop-and-batch because each clip costs whisper- or vision-seconds; this one does not
// touch a clip in Go at all. The rollup derivation (leaf → ancestors) is a set operation SQL can
// express as a join over the closure table, so the whole rebuild is TWO bulk statements —
// RebuildClosure (from the Go Forest, the single owner of the graph walk) then RebuildRollups (one
// INSERT..SELECT over the whole catalog) — regardless of whether the catalog is 200 clips or 200,000.
// A per-clip loop here would be an N+1 that holds the job worker once the catalog balloons past the
// `filler.fetch.max_catalog_clips` auto-fetch throttle; the doc rejects it for exactly that reason.
//
// ⚠ Why a JOB and not a synchronous action on the graph edit: same reconcile-on-a-timer shape the
// provisioner uses. The graph edit is the TRIGGER; the work runs decoupled, so a bulk edit that
// touches many taxa reindexes ONCE on the next pass rather than per row, and a rebuild that fails
// (a transient DB error) simply retries next pass rather than failing the operator's edit. When the
// taxonomy CRUD API lands (§10 V45a item 3) it may ALSO kick a rebuild for immediacy; this job is
// what guarantees eventual convergence with or without that.
//
// The future re-embed job is a lifecycle sibling of THIS job (cron, off by default) but keeps a
// per-clip batched body — embedding is a model call per clip, not a set operation. "Sibling" means
// same wiring, not same body; see the §10 note.

// ReindexStore is the narrow slice of the store this job needs. Declared here (mirroring
// TranscribeStore/VisionStore) so filler does not import store; the adapter in main bridges them.
type ReindexStore interface {
	// ListTaxa returns the whole graph — built into a Forest so RebuildClosure can walk it. The walk
	// stays in Go (the Forest is the single owner); the closure it produces makes the rollup rebuild
	// plain SQL.
	ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error)
	// RebuildClosure recomputes taxa_closure from the forest. Run first — RebuildRollups reads it.
	RebuildClosure(ctx context.Context, forest *taxonomy.Forest, at time.Time) error
	// RebuildRollups recomputes every clip's rollup rows from the closure in one set-based statement,
	// preserving asserted leaves.
	RebuildRollups(ctx context.Context) error
}

// ReindexResult is what one pass did. Deliberately coarse — the work is two bulk statements, not a
// per-clip tally, so there is nothing per-clip to count. `Ran` distinguishes "off" from "did work".
type ReindexResult struct {
	Ran bool // false when the job is switched off (the opt-in gate), true when a rebuild ran
}

// ReindexJob rebuilds the denormalised taxonomy rollups from the current graph.
type ReindexJob struct {
	store ReindexStore
	// enabled reports whether the reindex is switched on. Read live on every pass so a settings change
	// applies on the next one (config-design §3 hot-apply). nil or false ⇒ Run is a no-op, so an
	// install that has not opted in pays nothing — the same visible-but-idle contract the siblings use.
	enabled func() bool
	now     func() time.Time
	log     *slog.Logger
}

// NewReindexJob builds the job. A nil store or a nil/false `enabled` makes Run a no-op.
func NewReindexJob(store ReindexStore, enabled func() bool, now func() time.Time, log *slog.Logger) *ReindexJob {
	return &ReindexJob{store: store, enabled: enabled, now: now, log: log}
}

// Run rebuilds the taxonomy closure and every clip's rollups from the current graph.
func (j *ReindexJob) Run(ctx context.Context) (ReindexResult, error) {
	var res ReindexResult
	if j.store == nil {
		return res, nil
	}
	if j.enabled == nil || !j.enabled() {
		return res, nil // off — the opt-in gate, read live
	}

	taxa, err := j.store.ListTaxa(ctx)
	if err != nil {
		return res, err
	}
	forest := taxonomy.New(taxa)

	// Closure first, then rollups — the rollup rebuild joins against the closure, so a stale closure
	// yields stale rollups. Two statements, both bulk; no per-clip iteration.
	if err := j.store.RebuildClosure(ctx, forest, j.now()); err != nil {
		return res, err
	}
	if err := j.store.RebuildRollups(ctx); err != nil {
		return res, err
	}
	res.Ran = true

	if j.log != nil {
		j.log.Info("filler taxonomy reindex run", "taxa", len(taxa))
	}
	return res, nil
}
