package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// Migration 00264 moves a channel's reconcile findings (`applied`, `unairable`) out of policy_json
// into reconcile_findings_json (#4706). The findings land in the new column, and every other byte of
// the policy stays as it was.
func TestReconcileFindingsMigration(t *testing.T) {
	t.Parallel()
	db, err := sql.Open("sqlite", sqliteDSNAt(t, 263, plainConnections))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testReconcileFindingsMigration(t, db, DialectSQLite, "migrations/sqlite")
}

type findingsMigrationCase struct {
	name     string
	policy   string // policy_json before the migration
	want     string // policy_json after it, byte for byte
	findings string // reconcile_findings_json after it, as JSON
}

const (
	appliedJSON = `[{"kind":"episodeNoRepeat","from":"168h0m0s","to":"84h0m0s"},{"kind":"seriesMinGap","from":"2h0m0s","to":"0s"}]`
	// The first title is as Go's encoder writes "Pots & Pans <Restored>": with &, < and > escaped.
	unairableJSON = `[{"key":"movie:tmdb:90000603","title":"Pots ` + escAmp + ` Pans ` + escLT + `Restored` + escGT +
		`","reason":"not_in_library"},{"key":"series:tvdb:90000007","title":"Café","reason":"over_ceiling"}]`
	escAmp, escLT, escGT = `\` + `u0026`, `\` + `u003c`, `\` + `u003e`
	decidedJSON          = `"scope":{"dates":{"movieRelease":[{"from":1980,"to":1989}]},"collections":["comedy"]},"audience":{"ceiling":"PG-13","allowUnrated":true},"separation":{"seriesMinGap":"2h0m0s"},"ordering":"syndication"`
	operatorJSON         = `"filler":{"pins":["clip_1"]},"window":"168h0m0s","growth":{"keep":true,"declined":["movie:tmdb:90000009"]}`
)

var findingsMigrationCases = []findingsMigrationCase{
	{"both findings",
		`{` + decidedJSON + `,` + operatorJSON + `,"applied":` + appliedJSON + `,"unairable":` + unairableJSON + `,"operatorSet":["scope","audience"]}`,
		`{` + decidedJSON + `,` + operatorJSON + `,"operatorSet":["scope","audience"]}`,
		`{"applied":` + appliedJSON + `,"unairable":` + unairableJSON + `}`},
	{"applied only",
		`{` + decidedJSON + `,"applied":` + appliedJSON + `}`,
		`{` + decidedJSON + `}`,
		`{"applied":` + appliedJSON + `}`},
	{"unairable only",
		`{"unairable":` + unairableJSON + `,` + operatorJSON + `}`,
		`{` + operatorJSON + `}`,
		`{"unairable":` + unairableJSON + `}`},
	{"empty arrays",
		`{"ordering":"shuffle","applied":[],"unairable":[]}`,
		`{"ordering":"shuffle"}`,
		`{"applied":[],"unairable":[]}`},
	{"only findings",
		`{"applied":` + appliedJSON + `}`,
		`{}`,
		`{"applied":` + appliedJSON + `}`},
	{"null findings",
		`{"applied":null,"ordering":"shuffle","unairable":null}`,
		`{"ordering":"shuffle"}`,
		`{}`},
	{"unknown keys around findings",
		`{"zeta":{"nested":{"applied":[1]},"ratio":0.25},"applied":` + appliedJSON + `,"alpha":"é\n","unairable":[],"n":-7}`,
		`{"zeta":{"nested":{"applied":[1]},"ratio":0.25},"alpha":"é\n","n":-7}`,
		`{"applied":` + appliedJSON + `,"unairable":[]}`},
	{"no findings, unknown keys", `{"futureKey":{"x":[1,2]},` + decidedJSON + `}`, `{"futureKey":{"x":[1,2]},` + decidedJSON + `}`, `{}`},
	{"empty policy", `{}`, `{}`, `{}`},
	{"not an object", `[{"applied":[]},"applied"]`, `[{"applied":[]},"applied"]`, `{}`},
	{"a string", `"applied"`, `"applied"`, `{}`},
}

// Malformed policies cannot reach the migration: policy_json is CHECKed with json_valid on SQLite
// and is jsonb on Postgres, so both refuse them on write.
var malformedPolicies = []string{`{"applied":[{"kind":"episodeNoRepeat"`, ``, `{"applied":[],}`}

func testReconcileFindingsMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 263); err != nil {
		t.Fatalf("migrate through 263: %v", err)
	}
	ph := placeholder(storedb.Passthrough)
	if dialect == DialectPostgres {
		ph = storedb.PgPlaceholders
	}
	for i, c := range findingsMigrationCases {
		if _, err := db.ExecContext(ctx, ph(`INSERT INTO channels (id, name, number, strategy, status, policy_json, revision)
			VALUES (?, ?, ?, 'sequential', 'live', ?, 7)`), c.name, c.name, i+1, c.policy); err != nil {
			t.Fatalf("seed channel %s: %v", c.name, err)
		}
	}
	for i, policy := range malformedPolicies {
		if _, err := db.ExecContext(ctx, ph(`INSERT INTO channels (id, name, number, strategy, status, policy_json)
			VALUES (?, ?, ?, 'sequential', 'live', ?)`), "malformed", "malformed", 100+i, policy); err == nil {
			t.Fatalf("the schema stored the malformed policy %q", policy)
		}
	}
	// stored is how the dialect renders a policy it holds: jsonb has its own spacing and key
	// order, so on Postgres "byte for byte" means the bytes jsonb gives back for the wanted policy.
	stored := func(policy string) string {
		if dialect != DialectPostgres {
			return policy
		}
		var out string
		if err := db.QueryRowContext(ctx, `SELECT CAST($1 AS jsonb)::text`, policy).Scan(&out); err != nil {
			t.Fatalf("render %s as jsonb: %v", policy, err)
		}
		return out
	}
	before := map[string]string{}
	for _, c := range findingsMigrationCases {
		var policy string
		if err := db.QueryRowContext(ctx, ph(`SELECT policy_json FROM channels WHERE id = ?`), c.name).Scan(&policy); err != nil {
			t.Fatal(err)
		}
		if policy != stored(c.policy) {
			t.Fatalf("%s: seeded policy reads back as %s", c.name, policy)
		}
		before[c.name] = policy
	}

	if _, err := provider.UpTo(ctx, 264); err != nil {
		t.Fatalf("apply 00264: %v", err)
	}

	for _, c := range findingsMigrationCases {
		var policy, findings string
		var revision int64
		if err := db.QueryRowContext(ctx, ph(`SELECT policy_json, reconcile_findings_json, revision FROM channels WHERE id = ?`),
			c.name).Scan(&policy, &findings, &revision); err != nil {
			t.Fatalf("read %s: %v", c.name, err)
		}
		if want := stored(c.want); policy != want {
			t.Errorf("%s: policy_json\n got %s\nwant %s\n was %s", c.name, policy, want, before[c.name])
		}
		if got, want := decodeJSON(t, findings), decodeJSON(t, c.findings); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: reconcile_findings_json = %s, want %s", c.name, findings, c.findings)
		}
		var typed schedule.ReconcileFindings
		if err := json.Unmarshal([]byte(findings), &typed); err != nil {
			t.Errorf("%s: findings do not decode as ReconcileFindings: %v", c.name, err)
		}
		if revision != 7 {
			t.Errorf("%s: revision = %d, want 7 (moving findings is not an edit)", c.name, revision)
		}
	}

	var raw string
	if err := db.QueryRowContext(ctx, ph(`SELECT reconcile_findings_json FROM channels WHERE id = ?`), "both findings").
		Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var both schedule.ReconcileFindings
	if err := json.Unmarshal([]byte(raw), &both); err != nil {
		t.Fatal(err)
	}
	if len(both.Applied) != 2 || both.Applied[0].Kind != "episodeNoRepeat" || len(both.Unairable) != 2 ||
		both.Unairable[0].Title != "Pots & Pans <Restored>" || both.Unairable[1].Reason != schedule.UnairableOverCeiling {
		t.Errorf("both findings decode to %+v", both)
	}
}

func decodeJSON(t *testing.T, raw string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("decode %q: %v", raw, err)
	}
	return v
}
