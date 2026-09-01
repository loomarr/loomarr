package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/pressly/goose/v3"
)

// TestDowngradeGuard: if the DB records a schema version newer than the binary
// embeds, Open must refuse (§16) instead of limping into an unknown schema.
func TestDowngradeGuard(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "future.db")

	// Open+migrate normally so goose's version table exists and is current.
	s, err := Open(context.Background(), "sqlite://"+path, true)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	// Forge a from-the-future version row (higher than any embedded migration).
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(
		`INSERT INTO goose_db_version (version_id, is_applied) VALUES (99999, 1)`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// Re-open: the guard must fire.
	_, err = Open(context.Background(), "sqlite://"+path, true)
	if err == nil {
		t.Fatal("expected downgrade guard to refuse a from-the-future DB")
	}
}

func TestMigrationProviderIsDialectLocal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "provider.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}

	// Changing the legacy process-global dialect must not affect this provider.
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Up(context.Background()); err != nil {
		t.Fatalf("migrate SQLite after changing legacy dialect: %v", err)
	}

	got, err := provider.GetDBVersion(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want, err := highestMigration("migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("schema version = %d, want %d", got, want)
	}
}

func TestMigrationProviderScopesEmbeddedMigrations(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "sources.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	sources := provider.ListSources()
	if len(sources) == 0 {
		t.Fatal("provider has no migration sources")
	}

	want, err := highestMigration("migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if got := sources[len(sources)-1].Version; got != want {
		t.Fatalf("highest provider source = %d, want %d", got, want)
	}
}

func TestUserMediaServerLinkedMigrationBackfillsOldImportedRows(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "user-linkage.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 74); err != nil {
		t.Fatalf("migrate through 74: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO users (id, name, role, password_hash) VALUES
			('imported', 'Emby user', 'member', NULL),
			('local', 'Local user', 'admin', '$2a$04$abcdefghijklmnopqrstuuuuuuuuuuuuuuuuuuuuuuuuuuuuu')
	`); err != nil {
		t.Fatalf("seed old users: %v", err)
	}
	if _, err := provider.UpTo(ctx, 75); err != nil {
		t.Fatalf("apply linkage migration: %v", err)
	}

	var importedLinked, localLinked bool
	if err := db.QueryRowContext(ctx,
		`SELECT media_server_linked FROM users WHERE id = 'imported'`).Scan(&importedLinked); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT media_server_linked FROM users WHERE id = 'local'`).Scan(&localLinked); err != nil {
		t.Fatal(err)
	}
	if !importedLinked {
		t.Fatal("old imported row was not marked media-server linked")
	}
	if localLinked {
		t.Fatal("old local row was incorrectly marked media-server linked")
	}
}

func TestUserContactAddressMigrationLeavesExistingUsersContactless(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "user-contacts.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 75); err != nil {
		t.Fatalf("migrate through 75: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO users (id, name, role, password_hash, media_server_linked)
		VALUES ('existing', 'Existing user', 'member', NULL, 1)
	`); err != nil {
		t.Fatalf("seed existing user: %v", err)
	}
	if _, err := provider.UpTo(ctx, 76); err != nil {
		t.Fatalf("apply contact-address migration: %v", err)
	}

	var contacts int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM user_contact_addresses WHERE user_id = 'existing'`).Scan(&contacts); err != nil {
		t.Fatal(err)
	}
	if contacts != 0 {
		t.Fatalf("existing user contacts = %d, want contactless", contacts)
	}
}

func TestNotificationMigrationAddsEmptyDurableQueueWithoutChangingUsers(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 76); err != nil {
		t.Fatalf("migrate through 76: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO users (id, name, role, password_hash, media_server_linked)
		VALUES ('existing', 'Existing user', 'member', NULL, 1)
	`); err != nil {
		t.Fatalf("seed existing user: %v", err)
	}
	if _, err := provider.UpTo(ctx, 77); err != nil {
		t.Fatalf("apply notification migration: %v", err)
	}

	for _, table := range []struct{ name, query string }{
		{"notification_intents", `SELECT COUNT(*) FROM notification_intents`},
		{"notification_delivery_attempts", `SELECT COUNT(*) FROM notification_delivery_attempts`},
	} {
		var rows int
		if err := db.QueryRowContext(ctx, table.query).Scan(&rows); err != nil {
			t.Fatalf("query %s: %v", table.name, err)
		}
		if rows != 0 {
			t.Fatalf("new %s rows = %d, want empty", table.name, rows)
		}
	}
	var users int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = 'existing'`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if users != 1 {
		t.Fatalf("existing users after notification migration = %d, want 1", users)
	}
}

func TestNotificationProductVocabularyMigrationPreservesAccountDelivery(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "notification-product-vocabulary.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 80); err != nil {
		t.Fatalf("migrate through 80: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO notification_intents
			(id, topic, recipient_kind, recipient_id, reference_kind, reference_id,
			 recipient_policy, template_json, idempotency_key, created_at, terminal_at)
		VALUES
			('intent-existing', 'account_invitation', 'invitation', 'invitation-existing',
			 'invitation', 'invitation-existing', 'mandatory_account',
			 '{"recipientName":"Ada"}', 'invitation-existing:created', 1900000000, 0);
		INSERT INTO notification_delivery_attempts
			(id, intent_id, means, destination_ref, destination_redacted, status,
			 attempt_number, available_at, created_at)
		VALUES
			('attempt-existing', 'intent-existing', 'email', 'contact-existing',
			 'a***@example.com', 'queued', 1, 1900000000, 1900000000)
	`); err != nil {
		t.Fatalf("seed account delivery: %v", err)
	}
	if _, err := provider.UpTo(ctx, 81); err != nil {
		t.Fatalf("apply product vocabulary migration: %v", err)
	}

	var topic, template, means, status string
	if err := db.QueryRowContext(ctx, `
		SELECT intent.topic, intent.template_json, attempt.means, attempt.status
		FROM notification_intents intent
		JOIN notification_delivery_attempts attempt ON attempt.intent_id = intent.id
		WHERE intent.id = 'intent-existing'
	`).Scan(&topic, &template, &means, &status); err != nil {
		t.Fatal(err)
	}
	if topic != "account_invitation" || template != `{"recipientName":"Ada"}` || means != "email" || status != "queued" {
		t.Fatalf("preserved delivery = topic %q template %q means %q status %q", topic, template, means, status)
	}
}

func TestFillerDecisionApplicationModeMigrationBackfillsShadow(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "decision-mode.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 70); err != nil {
		t.Fatalf("migrate through 70: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO filler_admission_decisions (
		id, clip_hash, evidence_hash, evidence_version, schema_version, policy_version,
		taxonomy_version, outcome_kind, verdict, result_json, created_at
	) VALUES ('pre-mode', 'clip-1', 'evidence-1', 'e1', 2, 'p1', 't1',
		'semantic', 'admit', '{}', 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 71); err != nil {
		t.Fatalf("migrate through 71: %v", err)
	}
	var mode string
	if err := db.QueryRowContext(ctx,
		`SELECT application_mode FROM filler_admission_decisions WHERE id = 'pre-mode'`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "shadow" {
		t.Fatalf("pre-migration application mode = %q, want shadow", mode)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE filler_admission_decisions SET application_mode = 'automatic' WHERE id = 'pre-mode'`); err == nil {
		t.Fatal("application mode CHECK accepted an unknown value")
	}
}

func TestUniqueChannelIntentRefMigrationPreservesDuplicateChannels(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "intent-ref.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 54); err != nil {
		t.Fatalf("migrate through 54: %v", err)
	}
	insert := func(id, intentRef string, number int, updatedAt int64) {
		t.Helper()
		if _, err := db.ExecContext(ctx,
			`INSERT INTO channels (id, intent_ref, name, number, strategy, status, updated_at)
			 VALUES (?, ?, ?, ?, 'sequential', 'building', ?)`,
			id, intentRef, id, number, updatedAt); err != nil {
			t.Fatal(err)
		}
	}
	insert("older", "job-one", 1, 10)
	insert("newer", "job-one", 2, 20)
	insert("tie-a", "job-tie", 3, 30)
	insert("tie-z", "job-tie", 4, 30)
	insert("empty-a", "", 5, 40)
	insert("empty-b", "", 6, 50)
	insert("active-old", "job-detached", 7, 10)
	insert("detached-new", "job-detached", 8, 100)
	if _, err := db.ExecContext(ctx, `UPDATE channels SET status='detached' WHERE id='detached-new'`); err != nil {
		t.Fatal(err)
	}

	if _, err := provider.UpTo(ctx, 55); err != nil {
		t.Fatalf("migrate through 55: %v", err)
	}
	refs := map[string]string{}
	rows, err := db.QueryContext(ctx, `SELECT id, intent_ref FROM channels`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, ref string
		if err := rows.Scan(&id, &ref); err != nil {
			t.Fatal(err)
		}
		refs[id] = ref
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(refs) != 8 {
		t.Fatalf("channels after migration = %d, want all 8 preserved", len(refs))
	}
	if refs["newer"] != "job-one" || refs["older"] != "" {
		t.Errorf("updated_at winner refs = %+v", refs)
	}
	if refs["tie-z"] != "job-tie" || refs["tie-a"] != "" {
		t.Errorf("id tie-break winner refs = %+v", refs)
	}
	if refs["empty-a"] != "" || refs["empty-b"] != "" {
		t.Errorf("empty refs changed = %+v", refs)
	}
	if refs["active-old"] != "job-detached" || refs["detached-new"] != "" {
		t.Errorf("managed row did not win over newer detached duplicate: %+v", refs)
	}

	insert("empty-c", "", 9, 60)
	if _, err := db.ExecContext(ctx,
		`INSERT INTO channels (id, intent_ref, name, number, strategy, status)
		 VALUES ('duplicate', 'job-one', 'duplicate', 10, 'sequential', 'building')`); err == nil {
		t.Fatal("partial unique index accepted a second non-empty intent_ref")
	}
}

func TestChannelRevisionMigrationBackfillsExistingRows(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "channel-revision.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 55); err != nil {
		t.Fatalf("migrate through 55: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO channels (id, name, number, strategy, status)
		 VALUES ('pre-revision', 'Pre revision', 156, 'sequential', 'building')`); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 56); err != nil {
		t.Fatalf("migrate through 56: %v", err)
	}
	var revision int64
	if err := db.QueryRowContext(ctx, `SELECT revision FROM channels WHERE id='pre-revision'`).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if revision != 1 {
		t.Fatalf("backfilled revision = %d, want 1", revision)
	}
	if _, err := db.ExecContext(ctx, `UPDATE channels SET revision=0 WHERE id='pre-revision'`); err == nil {
		t.Fatal("channel revision CHECK accepted zero")
	}
}

func TestChannelPlayoutAnchorMigrationSeedsExistingRows(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "channel-anchor.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	provider, err := newMigrationProvider(db, DialectSQLite, "migrations/sqlite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 61); err != nil {
		t.Fatalf("migrate through 61: %v", err)
	}
	const updatedAt int64 = 1_800_000_123
	if _, err := db.ExecContext(ctx,
		`INSERT INTO channels (id, name, number, strategy, status, updated_at)
		 VALUES ('pre-anchor', 'Pre anchor', 162, 'sequential', 'live', ?)`, updatedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, 62); err != nil {
		t.Fatalf("migrate through 62: %v", err)
	}
	var anchor int64
	if err := db.QueryRowContext(ctx, `SELECT playout_anchor FROM channels WHERE id='pre-anchor'`).Scan(&anchor); err != nil {
		t.Fatal(err)
	}
	if anchor != updatedAt {
		t.Fatalf("backfilled playout anchor = %d, want %d", anchor, updatedAt)
	}
}

func TestMigrationProviderRejectsUnknownDialect(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "unknown.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if _, err := newMigrationProvider(db, Dialect("unknown"), "migrations/sqlite"); err == nil {
		t.Fatal("expected an unknown dialect error")
	}
}
