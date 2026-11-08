// Package schemadoc holds the plain-words description of every table and column Loomarr's
// migrations create, and renders it as docs/reference/database.md for someone browsing the
// database (`make db-view`, sqlite3, any SQL client). On Postgres the same text becomes
// COMMENT ON TABLE/COLUMN, so a client shows it beside the data.
//
// The text lives in areas/*.yaml, one file per area of the app, rendered in file-name order.
// Everything structural (column order, types, keys, references) comes from the live schema the
// caller introspects, never from the YAML, so only the words can drift, and Check catches that:
// a table or column the migrations create but the YAML doesn't describe, or a description of
// one that no longer exists.
package schemadoc

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed areas/*.yaml intro.md
var files embed.FS

// Doc is every area's description, in rendering order.
type Doc struct {
	Areas []Area
}

// Area is one part of the app and the tables it keeps.
type Area struct {
	Title  string  `yaml:"area"`
	About  string  `yaml:"about"`
	Tables []Table `yaml:"tables"`
}

// Table describes one table and each of its columns, keyed by column name.
type Table struct {
	Name      string            `yaml:"name"`
	About     string            `yaml:"about"`
	Retention string            `yaml:"retention"` // how long rows are kept, in plain words
	Columns   map[string]string `yaml:"columns"`
}

// LiveTable is one table as the database reports it.
type LiveTable struct {
	Name    string
	Columns []LiveColumn
	// ForeignKeys lists each foreign key's columns, in key order (several for a composite key).
	ForeignKeys [][]string
	// Indexes lists each index's columns in index order, the primary key's among them.
	Indexes [][]string
}

// LiveColumn is one column as the database reports it, in table order.
type LiveColumn struct {
	Name       string
	Type       string
	NotNull    bool
	PrimaryKey bool
	References string // "table.column" for a foreign key, else ""
	Checked    bool   // named inside one of the table's CHECK constraints
}

// Comment is one Postgres COMMENT ON target: a table when Column is empty, else a column.
type Comment struct {
	Table, Column, Text string
}

// LibraryOwned reports a table a library creates and documents itself: goose's migration ledger
// and River's job tables. The reference names them without describing their columns.
func LibraryOwned(table string) bool {
	return table == "goose_db_version" || strings.HasPrefix(table, "river_")
}

// Load reads every area file. A file with an unknown key fails, so a typo can't hide text.
func Load() (Doc, error) {
	entries, err := fs.ReadDir(files, "areas")
	if err != nil {
		return Doc{}, err
	}
	var doc Doc
	for _, e := range entries { // ReadDir sorts by name: the files' numeric prefixes set area order
		raw, err := files.ReadFile(path.Join("areas", e.Name()))
		if err != nil {
			return Doc{}, err
		}
		dec := yaml.NewDecoder(bytes.NewReader(raw))
		dec.KnownFields(true)
		var a Area
		if err := dec.Decode(&a); err != nil {
			return Doc{}, fmt.Errorf("schemadoc: %s: %w", e.Name(), err)
		}
		doc.Areas = append(doc.Areas, a)
	}
	return doc, nil
}

// Check lists every way doc and live disagree. Empty means every table and column the live
// schema has is described, once, and nothing described is gone.
func Check(doc Doc, live []LiveTable) []string {
	var problems []string
	described := map[string]Table{}
	for _, a := range doc.Areas {
		if strings.TrimSpace(a.Title) == "" || strings.TrimSpace(a.About) == "" {
			problems = append(problems, fmt.Sprintf("area %q needs a title and an about", a.Title))
		}
		for _, t := range a.Tables {
			if _, dup := described[t.Name]; dup {
				problems = append(problems, fmt.Sprintf("table %s is described twice", t.Name))
			}
			described[t.Name] = t
			if strings.TrimSpace(t.About) == "" {
				problems = append(problems, fmt.Sprintf("table %s has no about", t.Name))
			}
		}
	}
	seen := map[string]bool{}
	for _, lt := range live {
		if LibraryOwned(lt.Name) {
			continue
		}
		seen[lt.Name] = true
		t, ok := described[lt.Name]
		if !ok {
			problems = append(problems, fmt.Sprintf("table %s is not described", lt.Name))
			continue
		}
		have := map[string]bool{}
		for _, c := range lt.Columns {
			have[c.Name] = true
			if strings.TrimSpace(t.Columns[c.Name]) == "" {
				problems = append(problems, fmt.Sprintf("column %s.%s is not described", lt.Name, c.Name))
			}
		}
		for name := range t.Columns {
			if !have[name] {
				problems = append(problems, fmt.Sprintf("column %s.%s is described but doesn't exist", lt.Name, name))
			}
		}
	}
	for name := range described {
		if !seen[name] {
			problems = append(problems, fmt.Sprintf("table %s is described but doesn't exist", name))
		}
	}
	slices.Sort(problems)
	return problems
}

// Comments returns the COMMENT ON targets for the tables and columns live actually has, so a
// comment never names an object that isn't there.
func Comments(doc Doc, live []LiveTable) []Comment {
	byName := map[string]LiveTable{}
	for _, lt := range live {
		byName[lt.Name] = lt
	}
	var out []Comment
	for _, a := range doc.Areas {
		for _, t := range a.Tables {
			lt, ok := byName[t.Name]
			if !ok {
				continue
			}
			out = append(out, Comment{Table: t.Name, Text: oneLine(t.About)})
			for _, c := range lt.Columns {
				if text := oneLine(t.Columns[c.Name]); text != "" {
					out = append(out, Comment{Table: t.Name, Column: c.Name, Text: text})
				}
			}
		}
	}
	return out
}

// Markdown renders the reference: the hand-written introduction, a contents list, then each
// area's tables with their columns in the order the database keeps them.
func Markdown(doc Doc, live []LiveTable) ([]byte, error) {
	intro, err := files.ReadFile("intro.md")
	if err != nil {
		return nil, err
	}
	byName := map[string]LiveTable{}
	var owned []string
	for _, lt := range live {
		byName[lt.Name] = lt
		if LibraryOwned(lt.Name) {
			owned = append(owned, lt.Name)
		}
	}
	var b bytes.Buffer
	b.WriteString("# Database reference\n\n")
	b.WriteString("<!-- GENERATED by `make db-docs` from internal/platform/store/schemadoc/. DO NOT EDIT. -->\n\n")
	b.Write(bytes.TrimSpace(intro))
	b.WriteString("\n\n## Contents\n\n")
	for _, a := range doc.Areas {
		fmt.Fprintf(&b, "- [%s](#%s):", a.Title, anchor(a.Title))
		for i, t := range a.Tables {
			sep := ","
			if i == 0 {
				sep = ""
			}
			fmt.Fprintf(&b, "%s [`%s`](#%s)", sep, t.Name, anchor(t.Name))
		}
		b.WriteString("\n")
	}
	if len(owned) > 0 {
		b.WriteString("- [Tables other libraries own](#tables-other-libraries-own)\n")
	}
	for _, a := range doc.Areas {
		fmt.Fprintf(&b, "\n## %s\n\n%s\n", a.Title, oneLine(a.About))
		for _, t := range a.Tables {
			fmt.Fprintf(&b, "\n### %s\n\n%s\n\n", t.Name, oneLine(t.About))
			if r := oneLine(t.Retention); r != "" {
				fmt.Fprintf(&b, "**Kept:** %s\n\n", r)
			}
			b.WriteString("| Column | What it holds | Type |\n| --- | --- | --- |\n")
			for _, c := range byName[t.Name].Columns {
				fmt.Fprintf(&b, "| `%s` | %s | %s |\n", c.Name, cell(t.Columns[c.Name]), shape(c))
			}
		}
	}
	if len(owned) > 0 {
		slices.Sort(owned)
		b.WriteString("\n## Tables other libraries own\n\n")
		b.WriteString("These come from the tools Loomarr is built on, which document them. Leave them alone.\n\n")
		for _, name := range owned {
			fmt.Fprintf(&b, "- `%s`: %s\n", name, ownerOf(name))
		}
	}
	return b.Bytes(), nil
}

func ownerOf(table string) string {
	if table == "goose_db_version" {
		return "which database migrations have run ([goose](https://github.com/pressly/goose))."
	}
	return "Loomarr's background job queue ([River](https://riverqueue.com/docs))."
}

// shape is the Type cell: the declared type, then what a browser needs to read the value.
func shape(c LiveColumn) string {
	parts := []string{"`" + strings.ToUpper(c.Type) + "`"}
	switch {
	case c.PrimaryKey:
		parts = append(parts, "key")
	case !c.NotNull:
		parts = append(parts, "may be empty")
	}
	if c.References != "" {
		parts = append(parts, "→ `"+c.References+"`")
	}
	return strings.Join(parts, " · ")
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func cell(s string) string { return strings.ReplaceAll(oneLine(s), "|", `\|`) }

// anchor is GitHub's heading slug for a heading made only of words, spaces and punctuation.
func anchor(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}
