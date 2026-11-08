package schemadoc

import (
	"fmt"
	"slices"
	"strings"
)

// noFlagCheck is the work that removes a KnownParityGaps entry.
const noFlagCheck = "#2793: SQLite flag without a CHECK (col IN (0, 1)); needs a table rebuild"

// KnownParityGaps are the differences the engines had when the parity test arrived, each with the
// work that removes it. Like KnownGaps it only shrinks: a listed difference that is gone fails too.
var KnownParityGaps = map[string]string{
	"column clips.held":                        noFlagCheck,
	"column filler_clip_pipeline.force_run":    noFlagCheck,
	"column filler_sources.enabled":            noFlagCheck,
	"column holidays.name_needs_corroboration": noFlagCheck,
	"column image_derivatives.animated":        noFlagCheck,
	"column images.animated":                   noFlagCheck,
	"column library_answers.ignored":           noFlagCheck,
	"column library_candidates.length_match":   noFlagCheck,
	"column library_candidates.name_match":     noFlagCheck,
	"column library_candidates.year_match":     noFlagCheck,
	"column library_files.held":                noFlagCheck,
	"column library_items.grounded":            noFlagCheck,
	"column library_items.needs_noticed":       noFlagCheck,
	"column library_root_paths.by_hand":        noFlagCheck,
	"column library_roots.by_hand":             noFlagCheck,
	"column library_roots.in_use":              noFlagCheck,
	"column library_roots.readable":            noFlagCheck,
	"column users.auto_approve":                noFlagCheck,
	"column users.disabled":                    noFlagCheck,
}

// Parity compares the schema SQLite and Postgres were migrated to (#2793): the same tables and
// columns, each with the same type, nullability, key, reference and CHECK, and the same foreign
// keys and indexes. The two engines are migrated by separate files, so nothing else keeps them
// in step; a difference is a bug in one of them. A difference listed in KnownParityGaps is left
// out, and a listed one that no longer exists is reported.
func Parity(sqlite, postgres []LiveTable) []string {
	found := parity(sqlite, postgres)
	var problems []string
	seen := map[string]bool{}
	for _, p := range found {
		subject, _, _ := strings.Cut(p, ":")
		if _, known := KnownParityGaps[subject]; known {
			seen[subject] = true
			continue
		}
		problems = append(problems, p)
	}
	for gap, why := range KnownParityGaps {
		if !seen[gap] {
			problems = append(problems, fmt.Sprintf("%s is fixed: remove it from KnownParityGaps (was: %s)", gap, why))
		}
	}
	slices.Sort(problems)
	return problems
}

func parity(sqlite, postgres []LiveTable) []string {
	var problems []string
	pg := map[string]LiveTable{}
	for _, t := range postgres {
		if !LibraryOwned(t.Name) {
			pg[t.Name] = t
		}
	}
	seen := map[string]bool{}
	for _, s := range sqlite {
		if LibraryOwned(s.Name) {
			continue
		}
		seen[s.Name] = true
		p, ok := pg[s.Name]
		if !ok {
			problems = append(problems, "table "+s.Name+": only in SQLite")
			continue
		}
		problems = append(problems, tableParity(s, p)...)
	}
	for name := range pg {
		if !seen[name] {
			problems = append(problems, "table "+name+": only in Postgres")
		}
	}
	slices.Sort(problems)
	return problems
}

func tableParity(s, p LiveTable) []string {
	var problems []string
	pc := map[string]LiveColumn{}
	for _, c := range p.Columns {
		pc[c.Name] = c
	}
	seen := map[string]bool{}
	for _, c := range s.Columns {
		seen[c.Name] = true
		q, ok := pc[c.Name]
		where := s.Name + "." + c.Name
		if !ok {
			problems = append(problems, "column "+where+": only in SQLite")
			continue
		}
		if !SameType(c, q.Type) {
			problems = append(problems, fmt.Sprintf("column %s: type %s in SQLite, %s in Postgres", where, c.Type, q.Type))
		}
		if c.NotNull != q.NotNull && !c.PrimaryKey {
			problems = append(problems, fmt.Sprintf("column %s: NOT NULL %t in SQLite, %t in Postgres", where, c.NotNull, q.NotNull))
		}
		if c.PrimaryKey != q.PrimaryKey {
			problems = append(problems, fmt.Sprintf("column %s: primary key %t in SQLite, %t in Postgres", where, c.PrimaryKey, q.PrimaryKey))
		}
		if !sameReference(c.References, q.References) {
			problems = append(problems, fmt.Sprintf("column %s: references %q in SQLite, %q in Postgres", where, c.References, q.References))
		}
		if stateColumn(c) && c.Checked != q.Checked {
			problems = append(problems, fmt.Sprintf("column %s: CHECKed %t in SQLite, %t in Postgres", where, c.Checked, q.Checked))
		}
	}
	for _, c := range p.Columns {
		if !seen[c.Name] {
			problems = append(problems, "column "+s.Name+"."+c.Name+": only in Postgres")
		}
	}
	for _, d := range setDiff(s.ForeignKeys, p.ForeignKeys) {
		problems = append(problems, fmt.Sprintf("foreign key %s(%s): %s", s.Name, d.cols, d.side))
	}
	for _, d := range setDiff(s.Indexes, p.Indexes) {
		problems = append(problems, fmt.Sprintf("index %s(%s): %s", s.Name, d.cols, d.side))
	}
	return problems
}

// SameType reports whether a Postgres column stores what its SQLite twin stores. SQLite keeps
// every integer in 64 bits and every REAL in 8 bytes, so the Postgres twin is bigint and double
// precision: an integer or real there would overflow or round what SQLite keeps whole. A 0/1
// flag (BOOLEAN, or an INTEGER CHECKed to its values) is boolean; text is text, never jsonb,
// which rewrites a document's spacing and key order on the way in.
func SameType(sqlite LiveColumn, postgres string) bool {
	pg := strings.ToLower(postgres)
	switch strings.ToUpper(sqlite.Type) {
	case "TEXT":
		return pg == "text"
	case "BOOLEAN":
		return pg == "boolean"
	case "INTEGER", "BIGINT":
		return pg == "bigint" || (pg == "boolean" && sqlite.Checked)
	case "REAL", "DOUBLE PRECISION":
		return pg == "double precision"
	case "BLOB":
		return pg == "bytea"
	}
	return false
}

// sameReference compares a SQLite reference, which shows a key naming no column as the parent
// table alone, with a Postgres one, which always names the column.
func sameReference(sqlite, postgres string) bool {
	if sqlite == postgres {
		return true
	}
	table, _, _ := strings.Cut(postgres, ".")
	return !strings.Contains(sqlite, ".") && sqlite == table
}

type keyDiff struct{ cols, side string }

// setDiff lists the column lists in only one of a and b, compared in order and without repeats.
func setDiff(a, b [][]string) []keyDiff {
	key := func(cols []string) string { return strings.Join(cols, ",") }
	inA, inB := map[string]bool{}, map[string]bool{}
	for _, c := range a {
		inA[key(c)] = true
	}
	for _, c := range b {
		inB[key(c)] = true
	}
	var out []keyDiff
	for k := range inA {
		if !inB[k] {
			out = append(out, keyDiff{k, "only in SQLite"})
		}
	}
	for k := range inB {
		if !inA[k] {
			out = append(out, keyDiff{k, "only in Postgres"})
		}
	}
	return out
}
