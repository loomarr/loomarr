package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// Migration 00230 folds a stored scope `era` into `dates`, as the retired decode alias did
// (schedule.withEra): ANDed axis by axis, an empty axis becomes [era], an axis the era misses
// becomes [era], and the any-era range is dropped.
func TestFoldScopeEraMigration(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "fold-scope-era.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testFoldScopeEraMigration(t, db, DialectSQLite, "migrations/sqlite")
}

type eraFoldCase struct {
	name  string
	scope string // the stored scope JSON
	want  *schedule.DateScope
}

var eraFoldCases = []eraFoldCase{
	{"era alone", `{"era":{"from":1980,"to":1989},"collections":["comedy"]}`, &schedule.DateScope{
		MovieRelease: []schedule.Range{{From: 1980, To: 1989}}, SeriesPremiere: []schedule.Range{{From: 1980, To: 1989}},
		SeriesAiring: []schedule.Range{{From: 1980, To: 1989}},
	}},
	{"open-ended era", `{"era":{"from":1990}}`, &schedule.DateScope{
		MovieRelease: []schedule.Range{{From: 1990}}, SeriesPremiere: []schedule.Range{{From: 1990}},
		SeriesAiring: []schedule.Range{{From: 1990}},
	}},
	{"any era", `{"era":{},"collections":["comedy"]}`, nil},
	{"era with dates", `{"era":{"from":1985,"to":1995},"dates":{
		"movieRelease":[{"from":1970,"to":1979},{"from":1990,"to":2000}],
		"seriesPremiere":[{"to":1988}],
		"seriesAiring":[{"from":2001}]}}`, &schedule.DateScope{
		MovieRelease:   []schedule.Range{{From: 1990, To: 1995}},
		SeriesPremiere: []schedule.Range{{From: 1985, To: 1988}},
		SeriesAiring:   []schedule.Range{{From: 1985, To: 1995}}, // missed entirely: the era wins
	}},
	{"era with a partly empty dates", `{"era":{"to":1999},"dates":{"movieRelease":[{"from":1980}]}}`, &schedule.DateScope{
		MovieRelease: []schedule.Range{{From: 1980, To: 1999}}, SeriesPremiere: []schedule.Range{{To: 1999}},
		SeriesAiring: []schedule.Range{{To: 1999}},
	}},
}

func testFoldScopeEraMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 229); err != nil {
		t.Fatalf("migrate through 229: %v", err)
	}
	ph := placeholder(storedb.Passthrough)
	if dialect == DialectPostgres {
		ph = storedb.PgPlaceholders
	}
	untouched := `{"scope":{"collections":["drama"]},"filler":{"era":{"from":1980,"to":1989}}}`
	for i, c := range eraFoldCases {
		// Each case sits on a channel's scope, a channel rule, a proposal's scope and a proposal rule;
		// a rule without an era keeps its own scope.
		policy := `{"scope":` + c.scope + `,"rules":[{"what":{"collections":["drama"]}},{"what":` + c.scope + `}]}`
		if _, err := db.ExecContext(ctx, ph(`INSERT INTO channels (id, name, number, strategy, status, policy_json)
			VALUES (?, ?, ?, 'sequential', 'live', ?)`), c.name, c.name, i+1, policy); err != nil {
			t.Fatalf("seed channel %s: %v", c.name, err)
		}
		if _, err := db.ExecContext(ctx, ph(`INSERT INTO proposals (id, status, proposal_json) VALUES (?, 'submitted', ?)`),
			c.name, `{"policy":`+policy+`,"scores":{"era":{"score":1}}}`); err != nil {
			t.Fatalf("seed proposal %s: %v", c.name, err)
		}
	}
	if _, err := db.ExecContext(ctx, ph(`INSERT INTO channels (id, name, number, strategy, status, policy_json)
		VALUES ('no-era', 'no-era', 99, 'sequential', 'live', ?)`), untouched); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.UpTo(ctx, 230); err != nil {
		t.Fatalf("apply 00230: %v", err)
	}

	read := func(query, id string) map[string]json.RawMessage {
		t.Helper()
		var raw string
		if err := db.QueryRowContext(ctx, ph(query), id).Scan(&raw); err != nil {
			t.Fatalf("%s %s: %v", query, id, err)
		}
		var doc map[string]json.RawMessage
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			t.Fatal(err)
		}
		return doc
	}
	for _, c := range eraFoldCases {
		channel := read(`SELECT policy_json FROM channels WHERE id = ?`, c.name)
		proposal := read(`SELECT proposal_json FROM proposals WHERE id = ?`, c.name)
		if string(proposal["scores"]) != `{"era":{"score":1}}` && string(proposal["scores"]) != `{"era": {"score": 1}}` {
			t.Errorf("%s: proposal scores changed to %s", c.name, proposal["scores"])
		}
		for where, policyJSON := range map[string]json.RawMessage{"channel": mustJSON(t, channel), "proposal": proposal["policy"]} {
			var policy struct {
				Scope map[string]json.RawMessage `json:"scope"`
				Rules []struct {
					What map[string]json.RawMessage `json:"what"`
				} `json:"rules"`
			}
			if err := json.Unmarshal(policyJSON, &policy); err != nil {
				t.Fatal(err)
			}
			if len(policy.Rules) != 2 || string(policy.Rules[0].What["collections"]) == "" || policy.Rules[0].What["dates"] != nil {
				t.Errorf("%s %s: the rule without an era changed: %+v", where, c.name, policy.Rules)
				continue
			}
			for part, scope := range map[string]map[string]json.RawMessage{"scope": policy.Scope, "rule": policy.Rules[1].What} {
				if _, ok := scope["era"]; ok {
					t.Errorf("%s %s %s still has era: %v", where, c.name, part, scope)
				}
				var got *schedule.DateScope
				if raw, ok := scope["dates"]; ok {
					got = &schedule.DateScope{}
					if err := json.Unmarshal(raw, got); err != nil {
						t.Fatal(err)
					}
				}
				if !reflect.DeepEqual(got, c.want) {
					t.Errorf("%s %s %s dates = %+v, want %+v", where, c.name, part, got, c.want)
				}
			}
		}
	}
	if got := read(`SELECT policy_json FROM channels WHERE id = ?`, "no-era"); string(got["filler"]) == "" ||
		string(got["scope"]) != `{"collections":["drama"]}` && string(got["scope"]) != `{"collections": ["drama"]}` {
		t.Errorf("a channel without a scope era changed: %v", got)
	}
}

func mustJSON(t *testing.T, doc map[string]json.RawMessage) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
