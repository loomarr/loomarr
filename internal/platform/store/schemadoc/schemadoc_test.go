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
