package records

import (
	"context"

	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/storetest"
)

// planRecorder watches every statement TestSQLiteConformance makes the filler store run, so
// storetest.CheckQueryPlans can prove each finds its rows through an index.
var planRecorder = storetest.NewRecorder("sqlite-plan-recorder-filler")

// openRecording is openExtended for the conformance factory, except that each cloned store runs
// on the recording driver. The template build (autoMigrate) goes through the ordinary open.
func openRecording(ctx context.Context, databaseURL string, autoMigrate bool) (Store, error) {
	if autoMigrate {
		return openExtended(ctx, databaseURL, true)
	}
	core, err := store.OpenSQLiteDriver(ctx, "sqlite-plan-recorder-filler", databaseURL)
	if err != nil {
		return nil, err
	}
	s := Extend(core)
	reopenURLs.Store(s, databaseURL)
	return s, nil
}

// scannedWhole names the filler statements allowed to read a table whole, keyed "VERB table",
// each with why. Everything else must find its rows through an index; the list only shrinks.
var scannedWhole = map[string]string{
	// Bounded: a household has a handful of each.
	"SELECT channels":                     "bounded: every channel's filler policy, one row per channel",
	"SELECT taxa":                         "bounded: the clip taxonomy, read whole in order",
	"UPDATE taxa":                         "bounded: re-parenting a taxon touches the taxonomy's few dozen rows",
	"INSERT taxa_closure":                 "bounded: the taxonomy's closure, joined whole when tags are re-derived",
	"SELECT filler_providers":             "bounded: one row per provider kind",
	"SELECT filler_sources":               "bounded: one row per filler source a household follows",
	"SELECT filler_source_check_outcomes": "bounded: one row per filler source",
	"SELECT filler_pulls":                 "bounded: the pull proposals, listed whole newest first",

	// The pool: playout builds its filler pool from every live clip and its history in one read, so
	// reading these whole is the query, not a missed lookup.
	"SELECT clips":                  "pool: every live clip with its split proposal",
	"SELECT filler_airings":         "pool: rotation history inside its retention window",
	"SELECT filler_clip_defects":    "pool: every clip's defect verdict and label",
	"SELECT filler_clip_variants":   "pool: every variant pair, so rotation spaces them",
	"SELECT filler_inferior_copies": "pool: every merged-away copy, so intake skips it",
	"SELECT filler_split_proposals": "pool: every split proposal, read whole in order",

	// Sweeps: retention, orphan and startup-recovery passes run off the request path and read their
	// table whole by design.
	"DELETE clips":                    "sync: drops catalog rows whose files are gone",
	"DELETE clip_tags":                "rebuild and orphan sweep: non-leaf tags and tags of deleted clips",
	"DELETE filler_acquisition_runs":  "retention sweep: settled runs past their window",
	"DELETE filler_airings":           "retention sweep: airings of deleted clips past their window",
	"DELETE filler_clip_defects":      "orphan sweep: defects of deleted clips",
	"DELETE filler_clip_fingerprints": "orphan sweep: fingerprints of deleted clips",
	"DELETE filler_clip_pipeline":     "orphan sweep: pipeline rows of deleted clips",
	"DELETE filler_clip_variants":     "orphan sweep: variant pairs of deleted clips",
	"DELETE filler_context_reports":   "retention sweep: superseded reports and reports of deleted clips",
	"DELETE filler_enrichment_axes":   "retention sweep: axes of deleted clips",
	"DELETE filler_enrichment_passes": "retention sweep: passes of deleted clips",
	"DELETE filler_split_proposals":   "orphan sweep: proposals of deleted clips",
	"UPDATE filler_acquisition_runs":  "startup recovery: runs a crashed process left running",
	"UPDATE interactive_operations":   "startup recovery: operations a crashed process left running",
}
