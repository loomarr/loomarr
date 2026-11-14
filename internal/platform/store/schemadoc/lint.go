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
//     sees the whole set. A column holding another system's words as it sent them is listed in
//     FreeForm instead.
//   - retention: the table says how long its rows are kept, so nothing grows without anyone
//     having decided it should.
//   - unbounded: a table whose retention admits that nothing deletes its rows ("Unbounded: ...")
//     is a gap until something does.
//   - unit: a number column documented in milliseconds ends in _ms, one in nanoseconds in _ns,
//     and a Unix-seconds one in neither, so a reader never multiplies by the wrong thousand.
//   - reference: a column named for another row (_id, _by or _ref) is a foreign key (#2793), so it
//     can't name a row that isn't there. One that must not be (another system's id, a log tag, a
//     column naming rows of several tables) is listed in NonKeys with why.
//   - json: a column named _json, or one defaulting to a JSON object or array, is declared JSON
//     (CHECK (json_valid(column)), jsonb in Postgres), so neither engine stores text that isn't.
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
	found, freeSeen, nonKeySeen := map[string]bool{}, map[string]bool{}, map[string]bool{}
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
			if t, ok := described[lt.Name]; ok && !unitNamed(c, t.Columns[c.Name]) {
				found["unit "+lt.Name+"."+c.Name] = true
			}
			if holdsJSON(c) && !c.JSON {
				found["json "+lt.Name+"."+c.Name] = true
			}
			if referenceNamed(c) && c.References == "" {
				if _, non := NonKeys[lt.Name+"."+c.Name]; non {
					nonKeySeen[lt.Name+"."+c.Name] = true
				} else {
					found["reference "+lt.Name+"."+c.Name] = true
				}
			}
			if !stateColumn(c) || c.Checked {
				continue
			}
			if _, free := FreeForm[lt.Name+"."+c.Name]; free {
				freeSeen[lt.Name+"."+c.Name] = true
			} else {
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
	for column := range FreeForm {
		if !freeSeen[column] {
			problems = append(problems, column+" is listed in FreeForm but is no longer an unchecked state column")
		}
	}
	for column := range NonKeys {
		if !nonKeySeen[column] {
			problems = append(problems, column+" is listed in NonKeys but is a foreign key now, or gone")
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

// unitFix is how a column named against its unit is fixed.
const unitFix = "end the name in its documented unit: _ms for milliseconds, _ns for nanoseconds, neither for seconds"

// unitNamed reports whether an integer column's name carries the unit its description gives it.
// Any other column is skipped: a unit in a text or blob column's description is of values inside
// it (a JSON offset, a packed keyframe index).
func unitNamed(c LiveColumn, about string) bool {
	if !strings.EqualFold(c.Type, "INTEGER") && !strings.EqualFold(c.Type, "BIGINT") {
		return true
	}
	ms, ns := strings.HasSuffix(c.Name, "_ms"), strings.HasSuffix(c.Name, "_ns")
	switch {
	case strings.Contains(about, "nanoseconds"):
		return ns
	case strings.Contains(about, "milliseconds"):
		return ms
	case strings.Contains(about, "Unix seconds"):
		return !ms && !ns
	}
	return true
}

// holdsJSON reports a text column that, by its name or its default, holds a JSON document.
func holdsJSON(c LiveColumn) bool {
	if !strings.EqualFold(c.Type, "TEXT") {
		return false
	}
	return strings.HasSuffix(c.Name, "_json") || strings.HasPrefix(c.Default, "'{") || strings.HasPrefix(c.Default, "'[")
}

// referenceNamed reports a column whose name says it points at another row (_id, _by, _ref) and
// that is not its own table's primary key.
func referenceNamed(c LiveColumn) bool {
	if c.PrimaryKey {
		return false
	}
	return strings.HasSuffix(c.Name, "_id") || strings.HasSuffix(c.Name, "_by") || strings.HasSuffix(c.Name, "_ref")
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
	case strings.HasPrefix(gap, "unit "):
		return unitFix
	case strings.HasPrefix(gap, "json "):
		return "declare it JSON: CHECK (json_valid(column)) in SQLite, jsonb in Postgres"
	case strings.HasPrefix(gap, "reference "):
		return "make it a foreign key, or list it in NonKeys with why it must not be one"
	case strings.HasPrefix(gap, "unbounded "):
		return "delete old rows (housekeeping or a retention setting), then say how long they're kept"
	default:
		return "add a retention line to its table in internal/platform/store/schemadoc/areas/"
	}
}

// Unbounded opens the retention of a table nothing prunes yet; Lint reports each one as a gap.
const Unbounded = "Unbounded:"

// KnownGaps are the lint problems the schema had when the lint arrived, each with the work that
// removes it. Every one has been fixed (#2528); add nothing here: fix the schema instead.
var KnownGaps = map[string]string{}

// FreeForm are state-named columns that hold another system's words as it sent them, so a CHECK
// would refuse whatever that system adds next. Each says whose words they are.
var FreeForm = map[string]string{
	"inventory_items.kind":   "the media server's item type, lower-cased",
	"titles.download_status": "the download app's queue status text, shown as it is",
}

// NonKeys are columns named for another row that must not be foreign keys, each with why (#2793).
// Anything else so named is a key.
var NonKeys = map[string]string{
	// Another system's ids: nothing in Loomarr holds the rows they name.
	"channels.tunarr_id":                                 "Tunarr's channel id, from the import",
	"clips.tunarr_program_id":                            "Tunarr's program id, from the import",
	"filler_acquisition_artifacts.remote_id":             "the provider's own id for the item downloaded",
	"invitations.library_user_id":                        "the media server's account id",
	"library_candidates.external_id":                     "a catalog's id for the candidate",
	"library_collections.imported_id":                    "the media server's collection id",
	"library_episode_orders.order_id":                    "a catalog's episode-order id",
	"library_filmography.external_id":                    "a catalog's id for the work",
	"library_people.external_id":                         "a catalog's id for the person",
	"library_answers.external_id":                        "a catalog's id for the answer",
	"library_streams.external_ref":                       "the media server's stream reference",
	"notification_delivery_attempts.provider_message_id": "the delivery provider's message id",
	"filler_inference_evaluations.generation_id":         "the model provider's generation id",
	"filler_structure_window_call_ledger.assessor_id":    "the model that assessed, by name",
	// Names rows of several tables, so no one key fits.
	"activity.subject_id":                            "a channel, title or proposal",
	"filler_pulls.proposed_by":                       "a person's id or a job's name",
	"filler_ready_events.enrollment_ref":             "a folder, source or pull, by enrollment_kind",
	"notification_delivery_attempts.destination_ref": "a destination, or invitation: or person: and an id",
	"notification_intents.recipient_id":              "an invitation, a person, approvers or operators",
	"notification_intents.reference_id":              "a title, channel or proposal, by reference kind",
	// Not a reference at all.
	"library_answers.answered_by": "who answered: you or search",
	// Outlives what it names on purpose, so deleting that must not touch it.
	"filler_acquisition_artifacts.source_id":    "part of the already-downloaded identity, kept after its source is deleted",
	"filler_split_shadow_decisions.proposal_id": "a shadow decision outlives the split proposal it judged",
	"viewings.channel_id":                       "watch history outlives the channel",
	// Log correlation tags, written from whatever a log line carried, and kept past what they name.
	"diagnostic_events.actor_id":                "a log tag",
	"diagnostic_events.channel_id":              "a log tag",
	"diagnostic_events.instance_id":             "a log tag",
	"diagnostic_events.job_id":                  "a log tag",
	"diagnostic_events.playback_session_id":     "a log tag",
	"diagnostic_events.process_run_id":          "a log tag",
	"diagnostic_events.request_id":              "a log tag",
	"diagnostic_events.schedule_block_id":       "a log tag",
	"diagnostic_process_runs.channel_id":        "a log tag",
	"diagnostic_process_runs.instance_id":       "a log tag",
	"diagnostic_process_runs.job_id":            "a log tag",
	"diagnostic_process_runs.output_ref":        "a log tag",
	"diagnostic_process_runs.parent_run_id":     "a log tag",
	"diagnostic_process_runs.schedule_block_id": "a log tag",
}
