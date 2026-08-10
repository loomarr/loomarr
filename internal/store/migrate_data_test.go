//go:build integration

package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// The V11 gate: an established SQLite install migrates to Postgres with row-count
// parity, and every failure leaves the operator still running on SQLite.
//
// Integration-tagged because it needs a real Postgres — the divergences this is
// guarding against (BOOLEAN vs INTEGER, BYTEA vs BLOB) exist only between real
// drivers, so a fake destination would assert nothing.

// seedForMigration writes rows into every table whose type handling differs between
// the dialects, plus enough ordinary rows that a count is meaningful.
//
// ⚠ `users.disabled` and `channel_icons.bytes` are the two that MATTER: disabled is retired-ok
// INTEGER in SQLite and BOOLEAN in Postgres, and bytes is BLOB vs BYTEA. A migration
// that gets every other table right and drops these is the realistic bug.
func seedForMigration(t *testing.T, s Store) {
	t.Helper()
	ctx := context.Background()

	for i := 0; i < 7; i++ {
		title := provision.Title{
			MediaType: provision.Movie,
			TMDBID:    900000 + i,
			Name:      "Migrated Movie " + string(rune('A'+i)),
			Year:      1990 + i,
		}
		key, err := title.Key()
		if err != nil {
			t.Fatalf("key %d: %v", i, err)
		}
		rec := provision.Record{
			Key:         key,
			Title:       title,
			State:       provision.Wanted,
			RequestedAt: time.Unix(1_700_000_000, 0).UTC(),
			UpdatedAt:   time.Unix(1_700_000_000, 0).UTC(),
		}
		if err := s.UpsertTitle(ctx, rec); err != nil {
			t.Fatalf("seed title %d: %v", i, err)
		}
	}

	// A disabled user AND an enabled one, so the boolean coercion is exercised in BOTH
	// directions — a converter that hardcodes `false` passes a single-row test. Same for
	// AutoApprove, deliberately set opposite to Disabled so a transposition shows up.
	for _, u := range []User{
		{ID: "u-alice", Name: "alice", Role: RoleAdmin, Disabled: false, AutoApprove: true, PasswordHash: "hash-alice"},
		{ID: "u-bob", Name: "bob", Role: RoleMember, Disabled: true, AutoApprove: false, PasswordHash: "hash-bob"},
	} {
		u.CreatedAt, u.UpdatedAt = time.Now(), time.Now()
		if err := s.UpsertUser(ctx, u); err != nil {
			t.Fatalf("seed user %s: %v", u.Name, err)
		}
	}

	// ⚠ A session, because `sessions.user_id REFERENCES users(id)` is the schema's ONLY
	// foreign key — and tables are copied in ALPHABETICAL order, which puts `sessions`
	// before `users`. Without a row here the constraint is never exercised and the copy
	// order looks fine. With one, an FK-enforcing copy fails outright.
	if err := s.CreateSession(ctx, Session{
		TokenHash: "hash-alice-session",
		UserID:    "u-alice",
		CreatedAt: time.Unix(1_700_000_000, 0).UTC(),
		ExpiresAt: time.Unix(1_900_000_000, 0).UTC(),
	}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	if err := s.SetSetting(ctx, "setup.completed", "true"); err != nil {
		t.Fatalf("seed setting: %v", err)
	}
}

// TestMigrateSQLiteToPostgres is the phase gate itself.
func TestMigrateSQLiteToPostgres(t *testing.T) {
	ctx := context.Background()
	src := newSQLiteStore(t)
	seedForMigration(t, src)

	dst, err := Open(ctx, startPostgres(t), true)
	if err != nil {
		t.Fatalf("open destination: %v", err)
	}
	t.Cleanup(func() { _ = dst.Close() })

	prog, err := MigrateData(ctx, src, dst, nil)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !prog.Done {
		t.Error("progress must report done on success")
	}

	// Parity is the gate, and it re-counts both sides independently.
	stats, err := VerifyParity(ctx, src, dst)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if bad := ParityMismatches(stats); len(bad) > 0 {
		t.Fatalf("row-count parity failed: %+v", bad)
	}

	// Parity alone would pass if every value were mangled identically, so assert the
	// data actually survived — especially the boolean that differs between dialects.
	users, err := dst.ListUsers(ctx)
	if err != nil {
		t.Fatalf("list migrated users: %v", err)
	}
	if len(users) != 2 {
		t.Fatalf("migrated users = %d, want 2", len(users))
	}
	var sawDisabled, sawEnabled bool
	for _, u := range users {
		if u.Name == "bob" && u.Disabled && !u.AutoApprove {
			sawDisabled = true
		}
		if u.Name == "alice" && !u.Disabled && u.AutoApprove {
			sawEnabled = true
		}
	}
	if !sawDisabled || !sawEnabled {
		t.Errorf("booleans did not survive (bob-disabled=%v alice-enabled=%v) — the SQLite INTEGER/Postgres BOOLEAN divergence; a transposition would also land here",
			sawDisabled, sawEnabled)
	}

	if v, err := dst.GetSetting(ctx, "setup.completed"); err != nil || v != "true" {
		t.Errorf("setup.completed = %q (err %v), want \"true\" — wizard state must survive", v, err)
	}
}

// Every table in the destination schema must be visited. A migrator that silently skips
// a table nobody remembered is the failure mode a hardcoded list produces — this repo's
// own TRUNCATE list had already drifted to 8 of 10 when this was written.
func TestMigrateCoversEveryTable(t *testing.T) {
	ctx := context.Background()
	src := newSQLiteStore(t)
	dst, err := Open(ctx, startPostgres(t), true)
	if err != nil {
		t.Fatalf("open destination: %v", err)
	}
	t.Cleanup(func() { _ = dst.Close() })

	prog, err := MigrateData(ctx, src, dst, nil)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}

	visited := map[string]bool{}
	for _, st := range prog.Tables {
		visited[st.Table] = true
	}
	// Derived from the destination catalog, which goose just built from the embedded
	// migrations — so this assertion cannot drift from the schema.
	want, err := userTables(ctx, dst.(*sqlStore))
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	if len(want) == 0 {
		t.Fatal("no tables found in the destination — the catalog query is wrong")
	}
	for _, table := range want {
		if !visited[table] {
			t.Errorf("table %q was never copied", table)
		}
	}
	if visited["goose_db_version"] {
		t.Error("goose_db_version must NOT be copied — the destination earns its own")
	}
}

// Preflight must refuse a target that already holds tables. "Wipe it and retry" is only
// safe advice because this check guarantees there was nothing there to lose.
func TestPreflightRefusesAPopulatedTarget(t *testing.T) {
	ctx := context.Background()
	dsn := startPostgres(t)

	checks, err := Preflight(ctx, dsn)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	if !PreflightPassed(checks) {
		t.Fatalf("an empty target must pass every check, got %+v", checks)
	}

	// Migrate the target so it now holds Loomarr's tables, then re-run.
	dst, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = dst.Close() })

	checks, err = Preflight(ctx, dsn)
	if err != nil {
		t.Fatalf("preflight (populated): %v", err)
	}
	if PreflightPassed(checks) {
		t.Fatal("a populated target must NOT pass preflight")
	}
	var found bool
	for _, c := range checks {
		if c.Name == "Target is empty" {
			found = true
			if c.OK {
				t.Error("the emptiness check itself must be the one that fails")
			}
			if !strings.Contains(c.Detail, "table") {
				t.Errorf("detail should name what was found, got %q", c.Detail)
			}
		}
	}
	if !found {
		t.Error("preflight must include a 'Target is empty' check")
	}
}

// An unreachable target reports one honest failure rather than five cascading ones.
func TestPreflightUnreachableReportsOneFailure(t *testing.T) {
	// Port 1 is reserved and never listening.
	checks, err := Preflight(context.Background(), "postgres://u:p@127.0.0.1:1/loomarr?sslmode=disable&connect_timeout=2")
	if err != nil {
		t.Fatalf("preflight should report a failed check, not an error: %v", err)
	}
	if PreflightPassed(checks) {
		t.Fatal("an unreachable target must not pass")
	}
	if len(checks) != 1 || checks[0].Name != "Reachable" {
		t.Errorf("want a single Reachable failure, got %+v", checks)
	}
}

// The source is only ever READ. This is what makes "roll back by reverting one config
// line" true: whatever happens to the destination, the SQLite database the operator is
// still running on must be byte-identical afterwards.
func TestMigrateLeavesTheSourceUntouched(t *testing.T) {
	ctx := context.Background()
	src := newSQLiteStore(t)
	seedForMigration(t, src)

	before, err := VerifyParity(ctx, src, src)
	if err != nil {
		t.Fatalf("count source: %v", err)
	}

	dst, err := Open(ctx, startPostgres(t), true)
	if err != nil {
		t.Fatalf("open destination: %v", err)
	}
	t.Cleanup(func() { _ = dst.Close() })

	if _, err := MigrateData(ctx, src, dst, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	after, err := VerifyParity(ctx, src, src)
	if err != nil {
		t.Fatalf("recount source: %v", err)
	}
	if len(before) != len(after) {
		t.Fatalf("source table count changed: %d -> %d", len(before), len(after))
	}
	for i := range before {
		if before[i].Source != after[i].Source {
			t.Errorf("source table %q changed: %d -> %d rows",
				before[i].Table, before[i].Source, after[i].Source)
		}
	}
}

// Migrating onto the same dialect is refused — it is never what someone meant, and
// letting it run would duplicate every row into the database being read.
func TestMigrateRefusesSameDialect(t *testing.T) {
	ctx := context.Background()
	a, b := newSQLiteStore(t), newSQLiteStore(t)
	if _, err := MigrateData(ctx, a, b, nil); err == nil {
		t.Fatal("migrating SQLite -> SQLite must be refused")
	}
}

// Progress must report per-table counts as it goes: the stepper renders a bar per table,
// and a callback that only fires at the end would make the UI's progress fictional.
func TestMigrateReportsProgress(t *testing.T) {
	ctx := context.Background()
	src := newSQLiteStore(t)
	seedForMigration(t, src)

	dst, err := Open(ctx, startPostgres(t), true)
	if err != nil {
		t.Fatalf("open destination: %v", err)
	}
	t.Cleanup(func() { _ = dst.Close() })

	var updates int
	var sawTitles bool
	if _, err := MigrateData(ctx, src, dst, func(p MigrationProgress) {
		updates++
		for _, st := range p.Tables {
			if st.Table == "titles" && st.Copied == 7 {
				sawTitles = true
			}
		}
	}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if updates < 2 {
		t.Errorf("progress fired %d times, want per-table updates", updates)
	}
	if !sawTitles {
		t.Error("never saw titles reach its full count of 7")
	}
}

// ⚠ **`TestIconBytesSurviveMigration` was deleted in V52 phase 8, and what it proved is now
// unproven — which is worth stating rather than quietly dropping.** retired-ok
//
// It seeded a PNG (with deliberately invalid UTF-8 bytes) into the one binary column in the
// schema and asserted it survived the SQLite→Postgres copy without being mangled by a round trip
// through a Go string. That column was `channel_icons.bytes`, and dropping the table left the retired-ok
// schema with **no BLOB/BYTEA column at all** — so the test could not be rewritten against a real
// table, only against a synthetic one that would have to be added to both dialects' migrations for
// the benefit of a test.
//
// The consequence: `copyTable`'s binary-coercion branch still exists and is now reachable by
// nothing. It is defensive rather than dead — the next binary column added would depend on it, and
// deleting it would mean that column silently arrives corrupted — but it is no longer exercised by
// any test, and the comments in migrate_data.go that cite `channel_icons.bytes` as the motivating retired-ok
// example now cite something that does not exist.
//
// Whether to keep an unexercised path or remove it with the table it served is a maintainer call,
// not one this deletion should make by default.

// The copy order must place a table after everything it references. This is asserted
// directly rather than only implied by the migration succeeding, because the failure it
// guards against is silent under a seed with no sessions — which is exactly how the bug
// was originally missed.
func TestCopyOrderPutsParentsFirst(t *testing.T) {
	ctx := context.Background()
	dst, err := Open(ctx, startPostgres(t), true)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = dst.Close() })
	d := dst.(*sqlStore)

	tables, err := userTables(ctx, d)
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	pos := map[string]int{}
	for i, tb := range tables {
		pos[tb] = i
	}
	parents, err := fkParents(ctx, d)
	if err != nil {
		t.Fatalf("read fks: %v", err)
	}
	if len(parents) == 0 {
		t.Fatal("no foreign keys found — the catalog query is wrong, so this test proves nothing")
	}
	for child, ps := range parents {
		for _, parent := range ps {
			if pos[parent] > pos[child] {
				t.Errorf("%s is copied before %s, which it references", child, parent)
			}
		}
	}
	// The specific pair that motivated this: alphabetically sessions precedes users.
	if pos["users"] > pos["sessions"] {
		t.Error("users must be copied before sessions (sessions.user_id references it)")
	}
}
