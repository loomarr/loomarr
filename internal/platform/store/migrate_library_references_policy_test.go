package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

func TestLibraryReferencesPolicyMigration(t *testing.T) {
	// Foreign keys on, as production opens SQLite.
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "library-references-policy.db")+"?_pragma=foreign_keys(on)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testLibraryReferencesPolicyMigration(t, db, storedb.DialectSQLite, "migrations/sqlite")
}

// policyFixtures are channels' policies in the shapes ChannelPolicy writes, plus fields it does
// not know, which the migration must carry through untouched. col-a and col-b are collections;
// every dead-* id names none.
var policyFixtures = []struct {
	id, desired, policy string
	// named is the channel_scope_collections rows the channel gets.
	named []string
}{
	{
		id:      "ch-full",
		desired: `[{"Kind":"program","LibraryItemID":"lib-film","Start":"2026-01-01T00:00:00Z"},{"Kind":"filler"}]`,
		policy: `{"scope":{"series":["series:tvdb:90000701"],"collections":["col-a","dead-1","col-b","col-a","dead-1"],` +
			`"seasons":{"from":1,"to":3},"dates":{"movieRelease":[{"from":1980,"to":1989}]},` +
			`"genres":{"include":["Comedy"],"exclude":["Horror"]},"runtimeMax":3600},` +
			`"audience":{"ceiling":"PG"},"separation":{"minGapHours":1.50},"ordering":"shuffle",` +
			`"seasonal":{"holidays":["christmas"]},` +
			`"rules":[` +
			`{"source":"operator","label":"Weekend","priority":2,"when":{"days":[6,0]},` +
			`"what":{"collections":["dead-2","col-a"],"genres":{"include":["Action"]}},"how":{"ordering":"sequential"},"window":86400},` +
			`{"label":"No narrowing","when":{"hours":{"from":18,"to":22}}},` +
			`{"label":"Genres only","what":{"genres":{"include":["Drama"]}}},` +
			`{"label":"Empty collections","what":{"collections":[]}},` +
			`{"label":"All dead","what":{"collections":["dead-3"]}},` +
			`{"label":"What null","what":null}],` +
			`"filler":{"theme":"retro","always":["clip-1"]},"operatorSet":["scope"],` +
			`"futureField":{"nested":[1,2.50,true,null,"ü \"quoted\""]}}`,
		named: []string{"col-a", "col-b"},
	},
	{
		id:      "ch-no-collections",
		desired: `[]`,
		policy:  `{"scope":{"genres":{"include":["Comedy"]}},"rules":[{"label":"x","what":{"genres":{"include":["Drama"]}}}]}`,
	},
	{id: "ch-empty-policy", desired: `null`, policy: `{}`},
	{id: "ch-empty-arrays", desired: `[]`, policy: `{"scope":{"collections":[]},"rules":[]}`},
	{id: "ch-scope-all-dead", desired: `[]`, policy: `{"scope":{"collections":["dead-4"],"runtimeMax":1800}}`},
	{
		id:      "ch-only-kept",
		desired: `[{"Kind":"program","LibraryItemID":"dead-item"}]`,
		policy:  `{"scope":{"collections":["col-b"]},"rules":[{"what":{"collections":["col-a","col-b"]}}]}`,
		named:   []string{"col-a", "col-b"},
	},
	{id: "ch-rules-only", desired: `[]`, policy: `{"rules":[{"what":{"collections":["col-b","dead-5"],"runtimeMax":60}}]}`, named: []string{"col-b"}},
	// Shapes ChannelPolicy never writes: none may fail the migration, and anything that isn't an
	// array of ids is left as it is.
	{
		id:      "ch-odd-shapes",
		desired: `{"not":"a list"}`,
		policy:  `{"scope":{"collections":["col-a",null,5,"dead-6"]},"rules":[{"what":{"collections":null}},{"what":{"collections":"col-b"}},{"what":"x"}]}`,
		named:   []string{"col-a"},
	},
	{
		id:      "ch-bare-rule",
		desired: `[]`,
		policy:  `{"scope":{"collections":["col-b"]},"rules":["bare",{"what":{"collections":["col-a"]}}]}`,
		named:   []string{"col-a", "col-b"},
	},
	{
		id:      "ch-null-rule",
		desired: `["not a slot",{"Kind":"program","LibraryItemID":"lib-film"}]`,
		policy:  `{"rules":[null,true,7,"bare",{"what":{"collections":["col-a","dead-8"]}}]}`,
		named:   []string{"col-a"},
	},
	{id: "ch-scope-not-array", desired: `[]`, policy: `{"scope":{"collections":"col-a"},"rules":{"what":{"collections":["dead-7"]}}}`},
}

// testLibraryReferencesPolicyMigration proves the migration takes out of each channel's policy
// exactly the collection ids the library has no row for, in its scope and in each rule's what,
// and changes nothing else in the policy or the desired schedule.
func testLibraryReferencesPolicyMigration(t *testing.T, db *sql.DB, dialect storedb.Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, libraryReferencesMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", libraryReferencesMigration-1, err)
	}
	for _, stmt := range []string{
		`INSERT INTO library_items (id, kind, name, sort_name, updated_at_ms) VALUES ('lib-film', 'movie', 'Film', 'film', 1)`,
		`INSERT INTO library_collections (id, name, created_at_ms, updated_at_ms) VALUES ('col-a', 'A', 1, 1), ('col-b', 'B', 1, 1)`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed before %d: %v", libraryReferencesMigration, err)
		}
	}
	ph := placeholder(storedb.Passthrough)
	if dialect == storedb.DialectPostgres {
		ph = storedb.PgPlaceholders
	}
	for i, f := range policyFixtures {
		if _, err := db.ExecContext(ctx, ph(`INSERT INTO channels (id, name, number, strategy, status, desired_json, policy_json)
			VALUES (?, ?, ?, 'shuffle', 'live', ?, ?)`), f.id, f.id, 100+i, f.desired, f.policy); err != nil {
			t.Fatalf("seed channel %s: %v", f.id, err)
		}
	}
	before := channelDocuments(t, db)
	if _, err := provider.UpTo(ctx, libraryReferencesMigration); err != nil {
		t.Fatalf("apply %d: %v", libraryReferencesMigration, err)
	}
	after := channelDocuments(t, db)

	live := map[string]bool{"col-a": true, "col-b": true}
	for _, f := range policyFixtures {
		was, now := before[f.id], after[f.id]
		if now.desired != was.desired {
			t.Errorf("%s desired_json = %s, want it untouched (%s)", f.id, now.desired, was.desired)
		}
		want := decodeExact(t, was.policy)
		dropDeadCollections(want, live)
		if got := decodeExact(t, now.policy); !reflect.DeepEqual(got, want) {
			t.Errorf("%s policy_json:\n got %s\nwant %s\n(was %s)", f.id, now.policy, encode(t, want), was.policy)
		}
	}
	// Removing a dead id is the only change: a policy with none reads back the same document.
	for _, id := range []string{"ch-no-collections", "ch-empty-policy", "ch-empty-arrays", "ch-only-kept", "ch-bare-rule", "ch-scope-not-array"} {
		if got, want := decodeExact(t, after[id].policy), decodeExact(t, before[id].policy); !reflect.DeepEqual(got, want) {
			t.Errorf("%s policy_json changed with no dead id in it: %s, was %s", id, after[id].policy, before[id].policy)
		}
	}

	named := map[string][]string{}
	rows, err := db.QueryContext(ctx, `SELECT channel_id, collection_id FROM channel_scope_collections ORDER BY channel_id, collection_id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var ch, col string
		if err := rows.Scan(&ch, &col); err != nil {
			t.Fatal(err)
		}
		named[ch] = append(named[ch], col)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()
	for _, f := range policyFixtures {
		if !slices.Equal(named[f.id], f.named) {
			t.Errorf("%s channel_scope_collections = %v, want %v", f.id, named[f.id], f.named)
		}
	}
	var planned string
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) || ':' || MIN(channel_id) || ':' || MIN(library_item_id) FROM channel_planned_items`).Scan(&planned); err != nil {
		t.Fatal(err)
	}
	if planned != "2:ch-full:lib-film" {
		t.Errorf("channel_planned_items = %s, want ch-full's and ch-null-rule's lib-film", planned)
	}
}

type channelDocument struct{ desired, policy string }

func channelDocuments(t *testing.T, db *sql.DB) map[string]channelDocument {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `SELECT id, desired_json, policy_json FROM channels`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]channelDocument{}
	for rows.Next() {
		var id string
		var d channelDocument
		if err := rows.Scan(&id, &d.desired, &d.policy); err != nil {
			t.Fatal(err)
		}
		out[id] = d
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// decodeExact decodes a document keeping each number's literal, so a rewritten number shows.
func decodeExact(t *testing.T, doc string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(doc))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("decode %s: %v", doc, err)
	}
	return v
}

func encode(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// dropDeadCollections removes, in place, every id live lacks from the policy's
// scope.collections and each rule's what.collections, where those are arrays.
func dropDeadCollections(policy any, live map[string]bool) {
	filter := func(scope any) {
		m, ok := scope.(map[string]any)
		if !ok {
			return
		}
		ids, ok := m["collections"].([]any)
		if !ok {
			return
		}
		kept := []any{}
		for _, id := range ids {
			if s, ok := id.(string); ok && live[s] {
				kept = append(kept, id)
			}
		}
		m["collections"] = kept
	}
	doc, ok := policy.(map[string]any)
	if !ok {
		return
	}
	filter(doc["scope"])
	rules, _ := doc["rules"].([]any)
	for _, r := range rules {
		if rule, ok := r.(map[string]any); ok {
			filter(rule["what"])
		}
	}
}
