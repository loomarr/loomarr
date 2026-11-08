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
	found, freeSeen := map[string]bool{}, map[string]bool{}
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

// mediaServerCache is a cache of the media server's catalog, deleted whole with the Emby catalog
// code once the scanner is the catalog.
const mediaServerCache = "#2151: the media server's cache; deleted with the Emby catalog after the cutover"

// scannerCatalog is the scanner's own catalog. It keeps a title whose files have gone on purpose,
// behind the mass-disappearance guard, so what removes a title for good is the scanner's to decide
// as the one catalog lands.
const scannerCatalog = "#2151/#2528: the scanner's catalog; retention lands with the one catalog at the cutover"

// KnownGaps are the lint problems the schema had when the lint arrived, each with the work that
// removes it. Add nothing here: fix the schema instead.
var KnownGaps = map[string]string{
	"unbounded inventory_item_origins":   scannerCatalog,
	"unbounded inventory_source_origins": scannerCatalog,
	"unbounded inventory_sources":        scannerCatalog,
	"unbounded library_catalog":          scannerCatalog,
	"unbounded library_item_paths":       mediaServerCache,
	"unbounded library_items":            scannerCatalog,
	"unbounded series_episodes":          mediaServerCache,
}

// FreeForm are state-named columns that hold another system's words as it sent them, so a CHECK
// would refuse whatever that system adds next. Each says whose words they are.
var FreeForm = map[string]string{
	"inventory_items.kind":   "the media server's item type, lower-cased",
	"titles.download_status": "the download app's queue status text, shown as it is",
}
