package schemadoc

import (
	"fmt"
	"slices"
	"strings"
)

// Lint holds the schema to the rules that keep it legible and safe to query (#2528). Each problem
// names its rule, table and column, so a new migration that breaks one fails with the fix in view:
//
//   - fk-index: a foreign key's columns lead an index (the primary key counts). Without one, every
//     delete of the parent scans the child table, and so does every lookup of a parent's children.
//   - check: a state column (state, status, outcome, kind, or a name ending in _state or _status)
//     lists its values in a CHECK, so a typo'd state can't be written and a reader of the schema
//     sees the whole set.
//   - retention: the table says how long its rows are kept, so nothing grows without anyone
//     having decided it should.
//   - unbounded: a table whose retention admits that nothing deletes its rows ("Unbounded: ...")
//     is a gap until something does.
//
// Gaps already in the schema are listed in KnownGaps with the work that removes them. The list may
// only shrink: a listed gap that no longer exists is itself a problem, so fixing one forces its
// entry out in the same change.
func Lint(doc Doc, live []LiveTable) []string {
	described := map[string]Table{}
	for _, a := range doc.Areas {
		for _, t := range a.Tables {
			described[t.Name] = t
		}
	}
	found := map[string]bool{}
	for _, lt := range live {
		if LibraryOwned(lt.Name) {
			continue
		}
		if t, ok := described[lt.Name]; ok {
			switch retention := strings.TrimSpace(t.Retention); {
			case retention == "":
				found["retention "+lt.Name] = true
			case strings.HasPrefix(retention, Unbounded):
				found["unbounded "+lt.Name] = true
			}
		}
		for _, fk := range lt.ForeignKeys {
			if !slices.ContainsFunc(lt.Indexes, func(index []string) bool { return leads(index, fk) }) {
				found["fk-index "+lt.Name+"."+strings.Join(fk, ",")] = true
			}
		}
		for _, c := range lt.Columns {
			if stateColumn(c) && !c.Checked {
				found["check "+lt.Name+"."+c.Name] = true
			}
		}
	}
	var problems []string
	for gap := range found {
		if _, known := KnownGaps[gap]; !known {
			problems = append(problems, gap+": "+lintFix(gap))
		}
	}
	for gap, why := range KnownGaps {
		if !found[gap] {
			problems = append(problems, fmt.Sprintf("%s is fixed: remove it from KnownGaps (was: %s)", gap, why))
		}
	}
	slices.Sort(problems)
	return problems
}

// leads reports an index whose first columns are exactly the key's, in any order, so finding a
// parent's children is a range of that index.
func leads(index, key []string) bool {
	return len(index) >= len(key) && !slices.ContainsFunc(index[:len(key)], func(c string) bool {
		return !slices.Contains(key, c)
	})
}

func stateColumn(c LiveColumn) bool {
	if !strings.EqualFold(c.Type, "TEXT") {
		return false
	}
	switch c.Name {
	case "state", "status", "outcome", "kind":
		return true
	}
	return strings.HasSuffix(c.Name, "_state") || strings.HasSuffix(c.Name, "_status")
}

func lintFix(gap string) string {
	switch {
	case strings.HasPrefix(gap, "fk-index "):
		return "add an index whose first column is this foreign key"
	case strings.HasPrefix(gap, "check "):
		return "list the allowed values in a CHECK (column IN (...))"
	case strings.HasPrefix(gap, "unbounded "):
		return "delete old rows (housekeeping or a retention setting), then say how long they're kept"
	default:
		return "add a retention line to its table in internal/platform/store/schemadoc/areas/"
	}
}

// Unbounded opens the retention of a table nothing prunes yet; Lint reports each one as a gap.
const Unbounded = "Unbounded:"

const (
	needsIndex = "#2528: the integrity migration indexes it"
	needsCheck = "#2528: the integrity migration lists its values in a CHECK"
	needsPrune = "#2528: nothing prunes it yet"
)

// KnownGaps are the lint problems the schema had when the lint arrived, each with the work that
// removes it. Add nothing here: fix the schema instead.
var KnownGaps = map[string]string{
	"unbounded device_pairings":                            needsPrune,
	"unbounded discovery_feedback":                         needsPrune,
	"unbounded filler_acquisition_artifacts":               needsPrune,
	"unbounded filler_acquisition_runs":                    needsPrune,
	"unbounded filler_context_reports":                     needsPrune,
	"unbounded filler_enrichment_passes":                   needsPrune,
	"unbounded filler_inference_evaluations":               needsPrune,
	"unbounded filler_pull_commits":                        needsPrune,
	"unbounded filler_pulls":                               needsPrune,
	"unbounded filler_ready_events":                        needsPrune,
	"unbounded filler_research_web_usage":                  needsPrune,
	"unbounded filler_split_shadow_decisions":              needsPrune,
	"unbounded filler_structure_window_call_ledger":        needsPrune,
	"unbounded interactive_operations":                     needsPrune,
	"unbounded inventory_item_origins":                     needsPrune,
	"unbounded inventory_source_origins":                   needsPrune,
	"unbounded inventory_sources":                          needsPrune,
	"unbounded library_catalog":                            needsPrune,
	"unbounded library_item_paths":                         needsPrune,
	"unbounded library_items":                              needsPrune,
	"unbounded notification_reference_recipients":          needsPrune,
	"unbounded proposals":                                  needsPrune,
	"unbounded series_episodes":                            needsPrune,
	"unbounded titles":                                     needsPrune,
	"fk-index client_crash_reports.user_id":                needsIndex,
	"fk-index device_pairings.user_id":                     needsIndex,
	"fk-index filler_acquisition_artifacts.acquisition_id": needsIndex,
	"fk-index inventory_external_ids.item_id":              needsIndex,
	"fk-index password_recoveries.user_id":                 needsIndex,
	"fk-index quality_receipts.run_snapshot_id":            needsIndex,
	"check activity.kind":                                  needsCheck,
	"check channels.status":                                needsCheck,
	"check diagnostic_process_runs.status":                 needsCheck,
	"check filler_acquisition_artifacts.state":             needsCheck,
	"check filler_acquisition_runs.status":                 needsCheck,
	"check filler_clip_defects.outcome":                    needsCheck,
	"check filler_clip_pipeline.status":                    needsCheck,
	"check filler_curated_offers.local_state":              needsCheck,
	"check filler_enrichment_axes.state":                   needsCheck,
	"check filler_inference_evaluations.outcome":           needsCheck,
	"check filler_inference_evaluations.state":             needsCheck,
	"check filler_pulls.status":                            needsCheck,
	"check filler_ready_events.outcome":                    needsCheck,
	"check filler_sources.kind":                            needsCheck,
	"check interactive_operations.kind":                    needsCheck,
	"check interactive_operations.status":                  needsCheck,
	"check inventory_items.kind":                           needsCheck,
	"check inventory_sources.kind":                         needsCheck,
	"check library_answers.kind":                           needsCheck,
	"check library_art.kind":                               needsCheck,
	"check library_candidates.kind":                        needsCheck,
	"check library_catalog.kind":                           needsCheck,
	"check library_files.kind":                             needsCheck,
	"check library_filmography.kind":                       needsCheck,
	"check library_id_map.kind":                            needsCheck,
	"check library_items.kind":                             needsCheck,
	"check library_markers.kind":                           needsCheck,
	"check library_streams.kind":                           needsCheck,
	"check proposal_jobs.kind":                             needsCheck,
	"check quality_daily_aggregates.outcome":               needsCheck,
	"check quality_receipts.outcome":                       needsCheck,
	"check titles.download_status":                         needsCheck,
}
