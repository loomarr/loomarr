package store

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/storetest"
)

// planRecorder watches every statement TestSQLiteConformance makes the store run.
var planRecorder = storetest.NewRecorder("sqlite-plan-recorder-store")

// openRecordingSQLite is Open for the conformance factory, except that each cloned store runs on
// the recording driver. The template build (autoMigrate) goes through the ordinary Open.
func openRecordingSQLite(ctx context.Context, databaseURL string, autoMigrate bool) (Store, error) {
	if autoMigrate {
		return Open(ctx, databaseURL, true)
	}
	return OpenSQLiteDriver(ctx, "sqlite-plan-recorder-store", databaseURL)
}

// checkQueryPlans proves every statement the conformance suite made the store run finds its rows
// through an index, unless scannedWhole says why it reads a table whole.
func checkQueryPlans(t *testing.T) {
	storetest.CheckQueryPlans(t, newSQLiteStore(t).(*sqlStore).db, planRecorder.Statements(), scannedWhole, 300)
}

// scannedWhole names the statements allowed to read a table whole, keyed "VERB table", each with
// why. Everything else the store runs must find its rows through an index. The list only shrinks:
// an entry no statement needs any more fails the check, so a fixed scan cannot stay excused.
var scannedWhole = map[string]string{
	// Bounded: a household has a handful of each, so reading them whole is the cheap plan.
	"SELECT settings":                  "bounded: one row per setting; the whole set is the answer",
	"SELECT users":                     "bounded: one row per household member",
	"SELECT channels":                  "bounded: one row per channel a household made",
	"UPDATE channels":                  "bounded: the reconcile tick picks due channels from a household's few",
	"SELECT holidays":                  "bounded: the calendar of named holidays, read whole in order",
	"SELECT taxa":                      "bounded: the clip taxonomy, read whole in order",
	"UPDATE taxa":                      "bounded: re-parenting a taxon touches the taxonomy's few dozen rows",
	"INSERT taxa_closure":              "bounded: the taxonomy's closure, joined whole when tags are re-derived",
	"SELECT secret_data_keys":          "bounded: one row per data-key rotation",
	"UPDATE secret_data_keys":          "bounded: rotation retires the one active key among a few",
	"SELECT notification_destinations": "bounded: one row per place a household sends notices",
	"SELECT invitations":               "bounded: one row per invitation, listed whole",
	"DELETE invitations":               "bounded sweep: expired and settled invitations",
	"DELETE password_recoveries":       "bounded sweep: expired and settled recoveries",
	"DELETE library_roots":             "bounded: one row per media folder",
	"DELETE library_root_paths":        "bounded: one row per media folder a source reports",
	"INSERT library_root_paths":        "bounded: one row per media folder a source reports",

	// Sweeps: retention and orphan passes run off the request path and read their table whole by
	// design; the row they keep or drop is decided by a condition no index narrows.
	"DELETE interactive_operations":         "retention sweep: settled operations past their window",
	"DELETE inventory_item_origins":         "retention sweep: origins missing past their window",
	"DELETE inventory_items":                "retention sweep: items no origin holds any more",
	"DELETE inventory_source_origins":       "retention sweep: origins missing past their window",
	"DELETE inventory_sources":              "retention sweep: sources no origin holds any more",
	"DELETE library_files":                  "retention sweep: files missing past their window",
	"DELETE library_people":                 "orphan sweep: people no credit names",
	"DELETE discovery_feedback":             "retention sweep: keeps the newest feedback per target",
	"DELETE clip_tags":                      "rebuild: re-deriving the ancestor tags drops every non-leaf row",
	"SELECT library_items":                  "orphan sweep: items nothing references",
	"SELECT library_people":                 "sweep: people whose filmography is due, by a condition across credits",
	"SELECT quality_run_snapshots":          "orphan sweep: snapshots no receipt or aggregate keeps",
	"SELECT images":                         "orphan sweep: images no reference or derivative keeps",
	"UPDATE notification_intents":           "sweep: settles intents whose attempts have all finished",
	"UPDATE notification_delivery_attempts": "startup recovery: attempts a crashed worker left sending",
	"SELECT titles":                         "re-key: reads every title's library id when ids are re-mapped",
	"DELETE viewings":                       "clear all: an admin clearing everyone's watch history removes every row",
	"UPDATE library_files":                  "refresh everything: an admin asks for every present file to be written again",

	// Totals: the answer is an aggregate over every row.
	"SELECT diagnostic_retained_bytes":      "total: diagnostics' retained bytes, summed",
	"SELECT image_derivatives":              "total: derivative bytes, summed for the storage report",
	"SELECT library_catalog":                "total: an import's progress counts every catalog record due as new, by a condition across columns",
	"SELECT library_files":                  "total: the library status counts every present file",
	"SELECT notification_delivery_attempts": "total: each destination's last delivery and failure, over retained attempts",
}
