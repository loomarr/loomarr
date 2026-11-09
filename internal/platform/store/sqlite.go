package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
	_ "modernc.org/sqlite" // pure-Go driver, no cgo (§5, §16)
)

// SQLite claim: a guarded UPDATE that leases the due rows by pushing their
// deadline to ?1 (now+lease), so they aren't re-claimed until the reconciler
// acts or the lease lapses. A single instance is assumed (§5), so the UPDATE's
// atomicity suffices — no row-locking. RETURNING yields the leased deadline;
// the reconciler acts on the row's state (give-up/retry) and recomputes any new
// deadline itself, so it doesn't depend on the pre-lease value.
// Placeholders: ?1=leaseUntil, ?2=now, ?3=limit.
const sqliteClaimSQL = `
UPDATE titles SET deadline = ?1
WHERE key IN (
    SELECT key FROM titles
    WHERE state IN ('wanted','requested','downloading') AND deadline <= ?2 AND deadline > 0
    ORDER BY deadline LIMIT ?3
)
RETURNING ` + titleColumns

// SQLite channel claim: same guarded-UPDATE lease as titles, keyed on
// reconcile_deadline and excluding detached + paused channels (§9/§18) — both are
// off the sweep (detached = unmanaged; paused = deliberately off-air). RETURNs the
// full channel column set (channelstore.Select order) so channelstore.ScanChannel serves it.
// Placeholders: ?1=leaseUntil, ?2=now, ?3=limit.
//
// ⚠ **There is deliberately NO `reconcile_deadline > 0` guard. A zero deadline means DUE NOW**
// (§9 V54), and re-adding that guard reintroduces a permanent strand. The deadline's only writer
// is the LAST step of a SUCCESSFUL reconcile, so a channel whose first reconcile failed keeps 0 —
// with the guard it was invisible to the sweep forever, stuck in `building`, never pushed to
// Tunarr, while the binder's comment promised "the sweep retries". The sweep retried everything
// except the one case it existed for. Channels that opt out of reconciliation say so in `status`;
// that is the only exclusion, and a magic deadline value must never become a second one.
var sqliteChannelClaimSQL = `
UPDATE channels SET reconcile_deadline = ?1, revision = revision + 1
WHERE id IN (
    SELECT id FROM channels
    WHERE status NOT IN ('detached', 'paused') AND reconcile_deadline <= ?2
    ORDER BY reconcile_deadline LIMIT ?3
)
RETURNING ` + channelstore.Columns(channelstore.SQLiteLineup("channels"))

// sqliteMaxOpenConns is the pool size: enough that reads run beside the one writer and River's
// polls, small enough that a household server's file handles and memory don't notice.
const sqliteMaxOpenConns = 4

// sqliteDSN is the live store's connection string. Every _pragma applies to each new connection.
//   - journal_mode(WAL): readers and the writer don't block each other.
//   - busy_timeout(15000): a writer waits up to 15 s for the write lock instead of failing at once.
//     It is longer than River's 10 s fetch deadline on purpose, so a caller's context, which the
//     driver honours by interrupting the statement, is what bounds the wait.
//   - synchronous(NORMAL): the standard WAL setting. A commit no longer fsyncs; the WAL is synced at
//     checkpoints, so a power cut can lose the last commits but never corrupts the file.
//   - foreign_keys(on): enforce the schema's references.
//   - _txlock=immediate: see openSQLite.
func sqliteDSN(path string) string {
	return fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(15000)"+
		"&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(on)&_txlock=immediate", path)
}

// optimizeSQLite lets SQLite gather the planner statistics prod never had (no sqlite_stat1, #2446).
// 0x10002 is the documented form for a long-lived connection at open: analyze any table that
// needs it, with a cap on how much work one call does. Statistics only help the planner choose;
// without them every query still answers correctly, so Open does not fail when this does.
func optimizeSQLite(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, "PRAGMA optimize=0x10002")
	return err
}

// openSQLite opens the DB file with WAL + busy_timeout (§5) and returns a store
// wired with the SQLite claim SQL. dsn is the path after the sqlite:// scheme.
func openSQLite(ctx context.Context, path string) (*sqlStore, error) {
	return openSQLiteDriver(ctx, "sqlite", path)
}

// OpenSQLiteDriver opens an already-migrated SQLite database ("sqlite://path") through the named
// database/sql driver, which must wrap "sqlite". It exists for tests that watch the statements the
// store sends (storetest.Recorder, whose query-plan check proves lookups use indexes); production
// opens with Open.
func OpenSQLiteDriver(ctx context.Context, driverName, databaseURL string) (Store, error) {
	s, err := openSQLiteDriver(ctx, driverName, strings.TrimPrefix(databaseURL, "sqlite://"))
	if err != nil {
		return nil, err
	}
	_ = optimizeSQLite(ctx, s.db)
	return s, nil
}

// openSQLiteDriver is openSQLite over a named database/sql driver. Production always passes
// "sqlite".
func openSQLiteDriver(ctx context.Context, driverName, path string) (*sqlStore, error) {
	db, err := sql.Open(driverName, sqliteDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// ⚠ A few connections, not one (#2446). With one, every read in the process (each request,
	// River's 100 ms job polls, the guide) queued behind every other read and write; prod logged
	// hundreds of River "timed out after 10s" fetches and leader-election losses a week. Under WAL
	// readers on other connections never wait for the writer, and SQLite still admits one writer at
	// a time: `_txlock=immediate` makes each transaction take the write lock when it begins, so two
	// transactions never both read and then collide upgrading to write (SQLITE_BUSY mid-transaction,
	// which busy_timeout cannot retry), and a writer that must wait does so on busy_timeout.
	db.SetMaxOpenConns(sqliteMaxOpenConns)
	db.SetMaxIdleConns(sqliteMaxOpenConns)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return bindClusters(&sqlStore{db: db, dialect: DialectSQLite, path: path, ph: storedb.Passthrough, claimSQL: sqliteClaimSQL, channelClaimSQL: sqliteChannelClaimSQL}), nil
}

// openSQLiteReadOnly is the migration source opener. Both URI mode=ro and
// PRAGMA query_only are intentional: sql.TxOptions{ReadOnly:true} is advisory in the
// modernc driver, while the migration invariant requires the source to be incapable of
// accepting a write after the live generation has closed.
func openSQLiteReadOnly(ctx context.Context, path string) (*sqlStore, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=query_only(1)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open read-only sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping read-only sqlite: %w", err)
	}
	return bindClusters(&sqlStore{db: db, dialect: DialectSQLite, path: path, ph: storedb.Passthrough}), nil
}
