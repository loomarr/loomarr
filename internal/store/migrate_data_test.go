//go:build integration

package store

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/contact"
	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/provision"
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
// ⚠ `users.disabled` is the one that MATTERS here: INTEGER in SQLite, BOOLEAN in Postgres, and a
// migration that gets every other table right and drops it is the realistic bug. The BLOB/BYTEA
// divergence used to be covered alongside it by `channel_icons.bytes` (retired-ok), which V52
// phase 8 dropped — TestBinaryColumnsSurviveMigration now covers that half against a probe table
// it creates itself, so no production column has to exist for a test's benefit.
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
		{ID: "u-bob", Name: "bob", Role: RoleMember, Disabled: true, AutoApprove: false, MediaServerLinked: true, PasswordHash: "hash-bob"},
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

	intent := notifications.Intent{
		ID: "migration-notification", Topic: notifications.TopicAccountInvitation,
		RecipientKind: notifications.RecipientInvitation, RecipientID: "migration-invitation",
		ReferenceKind: notifications.ReferenceInvitation, ReferenceID: "migration-invitation",
		Policy: notifications.PolicyMandatoryAccount, Template: notifications.TemplateData{RecipientName: "Ada"},
		IdempotencyKey: "migration-invitation:created", CreatedAt: time.Unix(1_700_000_000, 0).UTC(),
	}
	attempt := notifications.Attempt{
		ID: "migration-notification-attempt", IntentID: intent.ID, Means: notifications.MeansEmail,
		DestinationRef: "migration-contact", DestinationRedacted: "a***@example.com",
		Status: notifications.StatusQueued, AttemptNumber: 1,
		AvailableAt: intent.CreatedAt, CreatedAt: intent.CreatedAt,
	}
	if _, created, err := s.CreateNotificationIntent(ctx, intent, []notifications.Attempt{attempt}); err != nil || !created {
		t.Fatalf("seed notification: created=%t err=%v", created, err)
	}

	invited := invitation.Invitation{
		ID: "migration-invitation", Kind: invitation.KindLocal, Username: "Ada",
		IdentityKey: "ada", Role: invitation.RoleMember, Status: invitation.StatusPending,
		CreatedAt: intent.CreatedAt, ExpiresAt: intent.CreatedAt.Add(invitation.Expiry),
	}
	address := &contact.Address{
		OwnerKind: contact.OwnerInvitation, OwnerID: invited.ID, Email: "Ada@Example.com",
		Normalized: "ada@example.com", Status: contact.StatusPending,
		Provenance: contact.ProvenanceAdmin, CreatedAt: intent.CreatedAt,
	}
	if err := s.CreateInvitation(ctx, invited, address); err != nil {
		t.Fatalf("seed invitation: %v", err)
	}
	grant := invitation.Grant{
		TokenHash: invitation.HashGrant("migration-bearer"), InvitationID: invited.ID,
		Kind: invitation.GrantActivation, Conveyance: invitation.ConveyanceEmail,
		CreatedAt: intent.CreatedAt, ExpiresAt: invited.ExpiresAt,
	}
	if err := s.ReplaceInvitationGrant(ctx, invited.ID, grant, grant.CreatedAt); err != nil {
		t.Fatalf("seed invitation grant: %v", err)
	}

	// The copier discovers the destination's live column set. Saving twice gives
	// revision a non-default value, proving it is copied rather than merely filled
	// by Postgres's migration default.
	channel := mustSaveChannel(t, s, sampleChannel("migration-revision", 701, time.Time{}))
	channel.Name = "revision two"
	channel = mustSaveChannel(t, s, channel)
	if channel.Revision != 2 {
		t.Fatalf("migration source channel revision = %d, want 2", channel.Revision)
	}
}

// TestMigrateSQLiteToPostgres is the phase gate itself.
func TestMigrateSQLiteToPostgres(t *testing.T) {
	ctx := context.Background()
	src := newSQLiteStore(t)
	seedForMigration(t, src)
	dsn := startPostgres(t)

	prog, err := MigrateToPostgres(ctx, "sqlite://"+SQLitePath(src), dsn, nil)
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	dst, err := Open(ctx, dsn, false)
	if err != nil {
		t.Fatalf("open migrated destination: %v", err)
	}
	t.Cleanup(func() { _ = dst.Close() })
	if !prog.Done {
		t.Error("progress must report done on success")
	}

	// MigrateToPostgres does not return success until it has independently
	// re-counted both sides inside the source snapshot.
	if bad := ParityMismatches(prog.Tables); len(bad) > 0 {
		t.Fatalf("row-count parity failed: %+v", bad)
	}
	migratedChannel, err := dst.GetChannel(ctx, "migration-revision")
	if err != nil {
		t.Fatalf("get migrated channel: %v", err)
	}
	if migratedChannel.Revision != 2 || migratedChannel.Name != "revision two" {
		t.Fatalf("migrated channel revision = %d name=%q, want 2/revision two",
			migratedChannel.Revision, migratedChannel.Name)
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
		if u.Name == "bob" && u.Disabled && !u.AutoApprove && u.MediaServerLinked {
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
	migratedIntent, err := dst.GetNotificationIntent(ctx, "migration-notification")
	if err != nil || migratedIntent.Template.RecipientName != "Ada" || !migratedIntent.TerminalAt.IsZero() {
		t.Errorf("migrated notification intent = %+v, err %v", migratedIntent, err)
	}
	migratedAttempts, err := dst.ListNotificationAttempts(ctx, "migration-notification")
	if err != nil || len(migratedAttempts) != 1 || migratedAttempts[0].Status != notifications.StatusQueued ||
		migratedAttempts[0].DestinationRef != "migration-contact" {
		t.Errorf("migrated notification attempts = %+v, err %v", migratedAttempts, err)
	}
	migratedInvitation, err := dst.GetInvitation(ctx, "migration-invitation", time.Unix(1_700_000_000, 0).UTC())
	if err != nil || migratedInvitation.Username != "Ada" || migratedInvitation.Role != invitation.RoleMember {
		t.Errorf("migrated invitation = %+v, err %v", migratedInvitation, err)
	}
	migratedContact, err := dst.GetInvitationContactAddress(ctx, "migration-invitation")
	if err != nil || migratedContact.Normalized != "ada@example.com" || migratedContact.Status != contact.StatusPending {
		t.Errorf("migrated invitation contact = %+v, err %v", migratedContact, err)
	}
	migratedGrants, err := dst.ListInvitationGrants(ctx, "migration-invitation")
	if err != nil || len(migratedGrants) != 1 || migratedGrants[0].TokenHash != invitation.HashGrant("migration-bearer") {
		t.Errorf("migrated invitation grants = %+v, err %v", migratedGrants, err)
	}
}

// A migration is one point-in-time view of SQLite, not one view per table. A live
// source can change while a large copy is running; rows written after the snapshot
// belong to the old generation and must not leak into only the later-copied tables.
func TestMigrateToPostgresUsesOneSourceSnapshot(t *testing.T) {
	ctx := context.Background()
	src := newSQLiteStore(t)
	if err := src.SetSetting(ctx, "snapshot.probe", "before"); err != nil {
		t.Fatal(err)
	}
	dsn := startPostgres(t)

	var mutationErr error
	mutated := false
	_, err := MigrateToPostgres(ctx, "sqlite://"+SQLitePath(src), dsn, func(p MigrationProgress) {
		if mutated || p.Current == "" {
			return
		}
		mutated = true
		// Use a second pool deliberately. The production Store has one connection,
		// while a distinct connection can write beside a WAL reader and prove that
		// every source query stays on the original snapshot.
		db, openErr := sql.Open("sqlite", "file:"+SQLitePath(src))
		if openErr != nil {
			mutationErr = openErr
			return
		}
		defer func() { _ = db.Close() }()
		_, mutationErr = db.ExecContext(ctx,
			`UPDATE settings SET value = 'after' WHERE key = 'snapshot.probe'`)
	})
	if mutationErr != nil {
		t.Fatalf("mutate live SQLite source: %v", mutationErr)
	}
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if !mutated {
		t.Fatal("progress never fired, so the concurrent-write probe did not run")
	}

	dst, err := Open(ctx, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dst.Close() }()
	if got, err := dst.GetSetting(ctx, "snapshot.probe"); err != nil || got != "before" {
		t.Fatalf("migrated snapshot.probe = %q (err %v), want pre-snapshot value", got, err)
	}
	if got, err := src.GetSetting(ctx, "snapshot.probe"); err != nil || got != "after" {
		t.Fatalf("live SQLite snapshot.probe = %q (err %v), want concurrent value", got, err)
	}
}

func TestMigrateToPostgresLocksTargetThroughParityAndCommit(t *testing.T) {
	ctx := context.Background()
	src := newSQLiteStore(t)
	if err := src.SetSetting(ctx, "source.row", "copied"); err != nil {
		t.Fatal(err)
	}
	dsn := startPostgres(t)
	writer, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Close() }()

	var once sync.Once
	writerDone := make(chan error, 1)
	finishedBeforeCommit := false
	_, err = MigrateToPostgres(ctx, "sqlite://"+SQLitePath(src), dsn, func(p MigrationProgress) {
		if p.Current == "" {
			return
		}
		once.Do(func() {
			go func() {
				_, err := writer.ExecContext(ctx,
					`INSERT INTO settings (key, value, updated_at) VALUES ($1, $2, $3)`,
					"external.row", "after", time.Now().Unix())
				writerDone <- err
			}()
			select {
			case <-writerDone:
				finishedBeforeCommit = true
			case <-time.After(100 * time.Millisecond):
				// Expected: ACCESS EXCLUSIVE holds the writer until migration commits.
			}
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if finishedBeforeCommit {
		t.Fatal("external target write completed before verified migration commit")
	}
	select {
	case err := <-writerDone:
		if err != nil {
			t.Fatalf("external target write after commit: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("external target write stayed blocked after migration commit")
	}
}

func TestMigrationTargetAdvisoryLockRejectsAConcurrentOwner(t *testing.T) {
	ctx := context.Background()
	dsn := startPostgres(t)
	release, err := lockPostgresMigrationTarget(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockPostgresMigrationTarget(ctx, dsn); err == nil {
		release()
		t.Fatal("second migration acquired an already-owned target")
	}
	release()

	releaseAgain, err := lockPostgresMigrationTarget(ctx, dsn)
	if err != nil {
		t.Fatalf("target stayed locked after release: %v", err)
	}
	releaseAgain()
}

func TestMigrateToPostgresRechecksTargetAtPointOfUse(t *testing.T) {
	ctx := context.Background()
	src := newSQLiteStore(t)
	dsn := startPostgres(t)

	checks, err := Preflight(ctx, dsn)
	if err != nil || !PreflightPassed(checks) {
		t.Fatalf("initial preflight = %+v, err %v", checks, err)
	}
	// The target changes after the caller's earlier preflight. The deep migration
	// operation must not trust that stale authorization or clear what appeared.
	existing, err := Open(ctx, dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = existing.Close() }()
	if err := existing.SetSetting(ctx, "other.owner", "keep"); err != nil {
		t.Fatal(err)
	}

	if _, err := MigrateToPostgres(ctx, "sqlite://"+SQLitePath(src), dsn, nil); err == nil {
		t.Fatal("migration accepted a target populated after the earlier preflight")
	}
	if got, err := existing.GetSetting(ctx, "other.owner"); err != nil || got != "keep" {
		t.Fatalf("pre-existing target value = %q (err %v), want untouched", got, err)
	}
}

func TestMigrateToPostgresRollsBackTheWholeDataCopy(t *testing.T) {
	ctx := context.Background()
	src := newSQLiteStore(t)
	seedForMigration(t, src)
	if err := src.UpsertFillerSource(ctx, FillerSource{
		ID: "operator:keep", Kind: "archive", URI: "operator_collection",
		Label: "Operator source", CreatedAt: time.Now(), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	// SQLite accepts this integer, while the destination BOOLEAN coercion rejects
	// it. filler_sources sorts before users, so a per-table commit would strand the
	// operator row in an otherwise failed destination.
	if _, err := src.(*sqlStore).db.ExecContext(ctx,
		`UPDATE users SET disabled = 2 WHERE id = 'u-bob'`); err != nil {
		t.Fatal(err)
	}
	dsn := startPostgres(t)

	if _, err := MigrateToPostgres(ctx, "sqlite://"+SQLitePath(src), dsn, nil); err == nil {
		t.Fatal("migration with an invalid destination boolean unexpectedly succeeded")
	}
	dst, err := Open(ctx, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dst.Close() }()
	sources, err := dst.ListFillerSources(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range sources {
		if source.ID == "operator:keep" {
			t.Fatal("failed migration committed an earlier table's operator row")
		}
	}
	if len(sources) != 6 {
		t.Fatalf("target contains %d filler sources after rollback, want only 6 SQL seeds", len(sources))
	}
	taxa, err := dst.ListTaxa(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(taxa) != 0 {
		t.Fatalf("target contains %d Go-runtime-seeded taxa after rollback, want none", len(taxa))
	}
}

func TestMigrateDataNeverClearsUnexpectedTargetRows(t *testing.T) {
	ctx := context.Background()
	src := newSQLiteStore(t)
	dst, err := openPostgresForDataMigration(ctx, startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dst.Close() }()
	if err := dst.SetSetting(ctx, "other.owner", "keep"); err != nil {
		t.Fatal(err)
	}

	if _, err := MigrateData(ctx, src, dst, nil); err == nil {
		t.Fatal("migration accepted an unexpected target row")
	}
	if got, err := dst.GetSetting(ctx, "other.owner"); err != nil || got != "keep" {
		t.Fatalf("unexpected target value = %q (err %v), want untouched", got, err)
	}
}

// Every table in the destination schema must be visited. A migrator that silently skips
// a table nobody remembered is the failure mode a hardcoded list produces — this repo's
// own TRUNCATE list had already drifted to 8 of 10 when this was written.
func TestMigrateCoversEveryTable(t *testing.T) {
	ctx := context.Background()
	src := newSQLiteStore(t)
	dst, err := openPostgresForDataMigration(ctx, startPostgres(t))
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
	want, err := userTables(ctx, dst)
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

	before := paritySnapshot(t, ctx, src)

	dst, err := openPostgresForDataMigration(ctx, startPostgres(t))
	if err != nil {
		t.Fatalf("open destination: %v", err)
	}
	t.Cleanup(func() { _ = dst.Close() })

	if _, err := MigrateData(ctx, src, dst, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	after := paritySnapshot(t, ctx, src)
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

func paritySnapshot(t *testing.T, ctx context.Context, st Store) []TableStat {
	t.Helper()
	s, ok := st.(*sqlStore)
	if !ok {
		t.Fatalf("snapshot source is %T, want SQL store", st)
	}
	tables, err := userTables(ctx, s)
	if err != nil {
		t.Fatalf("list source tables: %v", err)
	}
	stats, err := verifyParity(ctx, s.db, s.db, tables)
	if err != nil {
		t.Fatalf("count source: %v", err)
	}
	return stats
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

	dst, err := openPostgresForDataMigration(ctx, startPostgres(t))
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

// Binary columns must survive the copy: a BLOB routed through a Go string corrupts every byte that
// is not valid UTF-8, and `coerce`'s BYTEA/BLOB arm is the two lines that prevent it.
//
// ⚠ **This test CREATES ITS OWN TABLE, and that is the whole design rather than a shortcut.**
// It replaces `TestIconBytesSurviveMigration`, which asserted the same property against
// `channel_icons.bytes` — the schema's only binary column until V52 phase 8 dropped the table retired-ok
// (retired-ok). That left the coercion branch defensive but unexercised, and the two obvious fixes
// are both wrong: deleting the branch means the next binary column silently arrives mangled, and
// adding a binary column to both dialects' migrations puts a table that exists for a test's benefit
// into every operator's database.
//
// `MigrateData` enumerates tables from the DESTINATION catalog (`userTables`) and reads column
// types from the destination too, so a table created here on both sides travels the identical code
// path a real one would — same `copyTable`, same `describe`, same `coerce`. The probe is scoped to
// this test's own containers and never reaches a migration.
//
// ⚠ The destination column is BYTEA against a source BLOB deliberately: it is the DESTINATION's
// type that selects the coercion, which is the asymmetry the whole migrator is built around.
func TestBinaryColumnsSurviveMigration(t *testing.T) {
	ctx := context.Background()
	src := newSQLiteStore(t)
	dst, err := openPostgresForDataMigration(ctx, startPostgres(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dst.Close() })

	// A PNG header plus bytes that are invalid UTF-8 — precisely the case a string trip mangles.
	// A payload of only printable ASCII would pass even with the coercion branch deleted, which
	// would make this test a decoration.
	raw := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0xFF, 0xFE, 0x00, 0x80, 0xC3}

	if _, err := src.(*sqlStore).db.ExecContext(ctx,
		`CREATE TABLE binary_probe (id TEXT PRIMARY KEY, payload BLOB NOT NULL)`); err != nil {
		t.Fatalf("create source probe: %v", err)
	}
	if _, err := src.(*sqlStore).db.ExecContext(ctx,
		`INSERT INTO binary_probe (id, payload) VALUES (?, ?)`, "one", raw); err != nil {
		t.Fatalf("seed probe: %v", err)
	}
	if _, err := dst.db.ExecContext(ctx,
		`CREATE TABLE binary_probe (id TEXT PRIMARY KEY, payload BYTEA NOT NULL)`); err != nil {
		t.Fatalf("create destination probe: %v", err)
	}

	if _, err := MigrateData(ctx, src, dst, nil); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var got []byte
	if err := dst.db.QueryRowContext(ctx,
		`SELECT payload FROM binary_probe WHERE id = $1`, "one").Scan(&got); err != nil {
		t.Fatalf("read migrated bytes: %v", err)
	}
	if !bytes.Equal(got, raw) {
		t.Errorf("binary column corrupted:\n got %v\nwant %v", got, raw)
	}
}

// The copy order must place a table after everything it references. This is asserted
// directly rather than only implied by the migration succeeding, because the failure it
// guards against is silent under a seed with no sessions — which is exactly how the bug
// was originally missed.
func TestCopyOrderPutsParentsFirst(t *testing.T) {
	ctx := context.Background()
	dst, err := openPostgresForDataMigration(ctx, startPostgres(t))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = dst.Close() })
	d := dst

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
