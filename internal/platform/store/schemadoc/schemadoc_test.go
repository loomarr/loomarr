package schemadoc

import (
	"slices"
	"strings"
	"testing"
)

func TestCheckNamesEveryGapBetweenTheWordsAndTheSchema(t *testing.T) {
	doc := Doc{Areas: []Area{{Title: "People", About: "Who uses it.", Tables: []Table{
		{Name: "users", About: "One person.", Columns: map[string]string{"id": "Identifies them.", "nickname": "Gone."}},
		{Name: "retired", About: "Dropped long ago.", Columns: map[string]string{"id": "x"}},
		{Name: "blank", Columns: map[string]string{"id": "  "}},
	}}}}
	live := []LiveTable{
		{Name: "users", Columns: []LiveColumn{{Name: "id"}, {Name: "email"}}},
		{Name: "blank", Columns: []LiveColumn{{Name: "id"}}},
		{Name: "added", Columns: []LiveColumn{{Name: "id"}}},
		{Name: "goose_db_version", Columns: []LiveColumn{{Name: "id"}}},
		{Name: "river_job", Columns: []LiveColumn{{Name: "id"}}},
	}
	want := []string{
		"column blank.id is not described",
		"column users.email is not described",
		"column users.nickname is described but doesn't exist",
		"table added is not described",
		"table blank has no about",
		"table retired is described but doesn't exist",
	}
	if got := Check(doc, live); !slices.Equal(got, want) {
		t.Fatalf("Check =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestMarkdownKeepsTheDatabaseColumnOrderAndEscapesCells(t *testing.T) {
	doc := Doc{Areas: []Area{{Title: "People and sign-in", About: "Who.", Tables: []Table{{
		Name: "users", About: "One\n  person.",
		Columns: map[string]string{"id": "Identifies them.", "role": "admin | member", "invited_by": "Who asked them."},
	}}}}}
	live := []LiveTable{
		{Name: "users", Columns: []LiveColumn{
			{Name: "id", Type: "text", PrimaryKey: true},
			{Name: "role", Type: "TEXT", NotNull: true},
			{Name: "invited_by", Type: "TEXT", References: "users.id"},
		}},
		{Name: "river_job"},
	}
	md, err := Markdown(doc, live)
	if err != nil {
		t.Fatal(err)
	}
	s := string(md)
	for _, want := range []string{
		"- [People and sign-in](#people-and-sign-in): [`users`](#users)\n",
		"### users\n\nOne person.\n",
		"| `id` | Identifies them. | `TEXT` · key |\n| `role` | admin \\| member | `TEXT` |\n| `invited_by` | Who asked them. | `TEXT` · may be empty · → `users.id` |\n",
		"- `river_job`: Loomarr's background job queue",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("markdown lacks %q", want)
		}
	}
}

// Every shipped area file parses with no unknown keys (a typo'd key would silently drop text).
func TestShippedAreasLoad(t *testing.T) {
	doc, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Areas) == 0 {
		t.Fatal("no areas loaded")
	}
}

func TestLintNamesEachRuleAndHoldsKnownGapsToShrinking(t *testing.T) {
	savedGaps, savedFree := KnownGaps, FreeForm
	t.Cleanup(func() { KnownGaps, FreeForm = savedGaps, savedFree })
	FreeForm = map[string]string{"events.app_status": "another app's words", "jobs.kind": "now checked"}
	KnownGaps = map[string]string{
		"check jobs.status":           "listed, still a gap",
		"fk-index jobs.old_id":        "listed, since fixed",
		"fk-index jobs.user_id,shelf": "listed, still a gap",
	}
	doc := Doc{Areas: []Area{{Tables: []Table{
		{Name: "jobs", Retention: "Deleted after a month.", Columns: map[string]string{
			"took_millis":  "How long it took, in milliseconds.",               // a gap: not _ms
			"ran_at":       "When it ran, in Unix milliseconds.",               // a gap: not _ms
			"seen_at_ms":   "When it was seen, in Unix seconds.",               // a gap: seconds named _ms
			"queued_at_ns": "When it was queued, in Unix nanoseconds.",         // named for its unit
			"plan":         "JSON steps, each with its start in milliseconds.", // text: the unit is inside it
		}},
		{Name: "events", Retention: "Unbounded: nothing deletes them."},
		{Name: "notes"},
	}}}}
	live := []LiveTable{
		{Name: "jobs", Columns: []LiveColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "status", Type: "TEXT"},
			{Name: "kind", Type: "TEXT", Checked: true},
			{Name: "run_state", Type: "INTEGER"}, // not text: a number, not a named state
			{Name: "took_millis", Type: "BIGINT"},
			{Name: "ran_at", Type: "INTEGER"},
			{Name: "seen_at_ms", Type: "INTEGER"},
			{Name: "queued_at_ns", Type: "BIGINT"},
			{Name: "plan", Type: "TEXT"},
			{Name: "steps_json", Type: "TEXT"},                            // a gap: named JSON, not declared
			{Name: "tags", Type: "TEXT", Default: "'[]'"},                 // a gap: a JSON default, not declared
			{Name: "meta", Type: "TEXT", Default: "'{}'", JSON: true},     // declared JSON
			{Name: "notes_json", Type: "TEXT", Checked: true, JSON: true}, // declared JSON
		},
			ForeignKeys: [][]string{
				{"user_id"},               // only second in an index: a gap
				{"old_id"},                // leads one: fixed
				{"parent_id", "position"}, // a composite key the primary key leads, in another order
				{"user_id", "shelf"},      // a composite key no index leads: a listed gap
			},
			Indexes: [][]string{{"position", "parent_id", "id"}, {"old_id", "kind"}, {"shelf", "kind", "user_id"}, {"kind", "user_id"}},
		},
		{Name: "events", Columns: []LiveColumn{{Name: "outcome", Type: "text"}, {Name: "app_status", Type: "TEXT"}}},
		{Name: "notes"},
		{Name: "river_job", Columns: []LiveColumn{{Name: "state", Type: "TEXT"}}},
	}
	want := []string{
		"check events.outcome: list the allowed values in a CHECK (column IN (...))",
		"fk-index jobs.old_id is fixed: remove it from KnownGaps (was: listed, since fixed)",
		"fk-index jobs.user_id: add an index whose first column is this foreign key",
		"jobs.kind is listed in FreeForm but is no longer an unchecked state column",
		"json jobs.steps_json: declare it JSON: CHECK (json_valid(column)) in SQLite, jsonb in Postgres",
		"json jobs.tags: declare it JSON: CHECK (json_valid(column)) in SQLite, jsonb in Postgres",
		"retention notes: add a retention line to its table in internal/platform/store/schemadoc/areas/",
		"unbounded events: delete old rows (housekeeping or a retention setting), then say how long they're kept",
		"unit jobs.ran_at: " + unitFix,
		"unit jobs.seen_at_ms: " + unitFix,
		"unit jobs.took_millis: " + unitFix,
	}
	if got := Lint(doc, live); !slices.Equal(got, want) {
		t.Fatalf("Lint =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestMarkdownShowsHowLongRowsAreKept(t *testing.T) {
	doc := Doc{Areas: []Area{{Title: "Jobs", About: "Work.", Tables: []Table{{
		Name: "jobs", About: "One job.", Retention: "Deleted\n  after a month.",
	}}}}}
	md, err := Markdown(doc, []LiveTable{{Name: "jobs"}})
	if err != nil {
		t.Fatal(err)
	}
	if want := "### jobs\n\nOne job.\n\n**Kept:** Deleted after a month.\n\n| Column"; !strings.Contains(string(md), want) {
		t.Errorf("markdown lacks %q", want)
	}
}

// TestCheckHoldsWideTablesToGroupedColumns: past WideTable columns a table must group them, and a
// grouped table places every column once, in groups that say why they stay.
func TestCheckHoldsWideTablesToGroupedColumns(t *testing.T) {
	wide := LiveTable{Name: "wide"}
	described := map[string]string{}
	for i := range WideTable + 1 {
		name := "c" + strings.Repeat("x", i)
		wide.Columns = append(wide.Columns, LiveColumn{Name: name})
		described[name] = "A column."
	}
	check := func(groups []Group) []string {
		doc := Doc{Areas: []Area{{Title: "A", About: "B", Tables: []Table{{Name: "wide", About: "Wide.", Columns: described, Groups: groups}}}}}
		return Check(doc, []LiveTable{wide})
	}
	if got := check(nil); len(got) != 1 || !strings.Contains(got[0], "group them") {
		t.Fatalf("an ungrouped wide table = %v, want one problem asking for groups", got)
	}
	all := make([]string, 0, len(wide.Columns))
	for _, c := range wide.Columns {
		all = append(all, c.Name)
	}
	if got := check([]Group{{Title: "All", Why: "One row, one thing.", Columns: all}}); len(got) != 0 {
		t.Fatalf("a fully grouped table = %v, want no problems", got)
	}
	got := check([]Group{
		{Title: "First", Why: "Because.", Columns: append([]string{"gone"}, all[1:]...)},
		{Title: "Second", Columns: all[1:2]},
	})
	want := []string{
		`column wide.c is in no group`,
		`column wide.cx is in groups "First" and "Second"`,
		`group "First" of wide names column gone, which doesn't exist`,
		`table wide has a group without a title and a why`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Check =\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

func TestMarkdownRendersEachGroupWithItsWhy(t *testing.T) {
	doc := Doc{Areas: []Area{{Title: "A", About: "B", Tables: []Table{{
		Name: "clips", About: "One clip.", Columns: map[string]string{"hash": "Its id.", "name": "Its name.", "held": "Not ready."},
		Groups: []Group{
			{Title: "Whether it can air", Why: "Read first.", Columns: []string{"held"}},
			{Title: "The clip", Why: "Every read needs it.", Columns: []string{"name", "hash"}},
		},
	}}}}}
	live := []LiveTable{{Name: "clips", Columns: []LiveColumn{{Name: "hash", Type: "TEXT"}, {Name: "name", Type: "TEXT"}, {Name: "held", Type: "INTEGER"}}}}
	md, err := Markdown(doc, live)
	if err != nil {
		t.Fatal(err)
	}
	s := string(md)
	air := strings.Index(s, "**Whether it can air.** Read first.")
	clip := strings.Index(s, "**The clip.** Every read needs it.")
	if air < 0 || clip < air {
		t.Fatalf("groups missing or out of order:\n%s", s)
	}
	// Inside a group, columns keep the database's order, not the group's listing.
	if h, n := strings.Index(s, "| `hash`"), strings.Index(s, "| `name`"); h < clip || n < h {
		t.Fatalf("group columns not in database order:\n%s", s)
	}
}
