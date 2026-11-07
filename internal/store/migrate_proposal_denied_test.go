package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/store/storedb"
)

// proposalDeniedMigration is the migration that renames `proposals.approved_by` and the
// `proposal_declined` topic (#2323).
const proposalDeniedMigration = 164

func TestProposalDeniedTopicMigration(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.Join(t.TempDir(), "proposal-denied.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	testProposalDeniedTopicMigration(t, db, DialectSQLite, "migrations/sqlite")
}

// testProposalDeniedTopicMigration proves the rename moves what is already stored: an intent
// queued under the old topic and a destination that chose it both carry the new name, and the
// decider survives the column rename.
func testProposalDeniedTopicMigration(t *testing.T, db *sql.DB, dialect Dialect, dir string) {
	t.Helper()
	ctx := context.Background()
	provider, err := newMigrationProvider(db, dialect, dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.UpTo(ctx, proposalDeniedMigration-1); err != nil {
		t.Fatalf("migrate through %d: %v", proposalDeniedMigration-1, err)
	}
	ph := placeholder(storedb.Passthrough)
	if dialect == DialectPostgres {
		ph = storedb.PgPlaceholders
	}
	for _, stmt := range []string{
		`INSERT INTO proposals (id, job_id, status, created_by, approved_by, deny_reason, mod_summary, note,
			proposal_json, approved_at, created_at, updated_at)
			VALUES ('p-denied', 'j', 'denied', 'kid', 'boss', 'no', '', '', '', 0, 1, 1)`,
		`INSERT INTO notification_intents (id, topic, recipient_kind, recipient_id, reference_kind, reference_id,
			recipient_policy, template_json, idempotency_key, created_at)
			VALUES ('i-denied', 'proposal_declined', 'person', 'kid', 'proposal', 'p-denied', 'configurable',
			'{}', 'proposal:p-denied:declined', 1)`,
		`INSERT INTO notification_intents (id, topic, recipient_kind, recipient_id, reference_kind, reference_id,
			recipient_policy, template_json, idempotency_key, created_at)
			VALUES ('i-approved', 'proposal_approved', 'person', 'kid', 'proposal', 'p-other', 'configurable',
			'{}', 'proposal:p-other:approved', 1)`,
		`INSERT INTO notification_destinations (id, means, label, scope, owner_id, audience, topics_json, enabled,
			configuration_json, credentials_encrypted, created_at, updated_at)
			VALUES ('d-kid', 'ntfy', 'Phone', 'person', 'kid', 'person',
			'["proposal_approved","proposal_declined"]', 1, '{}', '', 1, 1)`,
	} {
		if _, err := db.ExecContext(ctx, ph(stmt)); err != nil {
			t.Fatalf("seed before %d: %v", proposalDeniedMigration, err)
		}
	}
	if _, err := provider.UpTo(ctx, proposalDeniedMigration); err != nil {
		t.Fatalf("apply %d: %v", proposalDeniedMigration, err)
	}

	var decidedBy string
	if err := db.QueryRowContext(ctx, `SELECT decided_by FROM proposals WHERE id = 'p-denied'`).Scan(&decidedBy); err != nil {
		t.Fatal(err)
	}
	if decidedBy != "boss" {
		t.Errorf("decided_by = %q, want the denier kept", decidedBy)
	}
	topics := map[string]string{}
	rows, err := db.QueryContext(ctx, `SELECT id, topic FROM notification_intents`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, topic string
		if err := rows.Scan(&id, &topic); err != nil {
			t.Fatal(err)
		}
		topics[id] = topic
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if topics["i-denied"] != "proposal_denied" || topics["i-approved"] != "proposal_approved" {
		t.Errorf("intent topics = %v, want the declined one renamed and the rest untouched", topics)
	}
	var chosen string
	if err := db.QueryRowContext(ctx, `SELECT topics_json FROM notification_destinations WHERE id = 'd-kid'`).Scan(&chosen); err != nil {
		t.Fatal(err)
	}
	if chosen != `["proposal_approved","proposal_denied"]` {
		t.Errorf("destination topics = %s, want the declined topic renamed", chosen)
	}
	if _, err := db.ExecContext(ctx, ph(`INSERT INTO notification_intents (id, topic, recipient_kind, recipient_id,
		reference_kind, reference_id, recipient_policy, template_json, idempotency_key, created_at)
		VALUES ('i-old', 'proposal_declined', 'person', 'kid', 'proposal', 'p-x', 'configurable', '{}', 'k-old', 1)`)); err == nil {
		t.Error("the old topic is still accepted; the CHECK constraint was not replaced")
	}
}
