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
		{Name: "jobs", Retention: "Deleted after a month."},
		{Name: "events", Retention: "Unbounded: nothing deletes them."},
		{Name: "notes"},
	}}}}
	live := []LiveTable{
		{Name: "jobs", Columns: []LiveColumn{
			{Name: "id", Type: "INTEGER", PrimaryKey: true},
			{Name: "status", Type: "TEXT"},
			{Name: "kind", Type: "TEXT", Checked: true},
			{Name: "run_state", Type: "INTEGER"}, // not text: a number, not a named state
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
		"retention notes: add a retention line to its table in internal/platform/store/schemadoc/areas/",
		"unbounded events: delete old rows (housekeeping or a retention setting), then say how long they're kept",
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
