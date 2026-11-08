package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // registered for the probe's own connection
)

// Preflight checks a candidate Postgres target BEFORE anything is copied (§18, V11).
//
// The point is to fail while failing is free. Every check here is something that would
// otherwise surface halfway through a copy, with a half-written destination and an
// operator who has already taken a backup and committed to the maintenance window.
//
// It opens its OWN connection and closes it: the target is not yet this install's
// database, and holding a pool open to a database we may not migrate to would be a
// side effect the operator did not ask for.

// PreflightCheck is one named check. Detail is operator-facing prose, not a log line —
// it is rendered verbatim under the check name in the stepper.
type PreflightCheck struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
	OK     bool   `json:"ok"`
}

// minPostgresMajor is the oldest Postgres the schema is exercised against. 13 is where
// the SKIP LOCKED + CTE claim path used by the Postgres backend is reliably available,
// and it is what the mock's copy states.
const minPostgresMajor = 13

// Preflight runs every check against dsn and returns them in display order. The error
// return is reserved for "the checks could not be run at all"; a check that ran and
// failed comes back as OK:false, because the operator needs to see WHICH one.
func Preflight(ctx context.Context, dsn string) ([]PreflightCheck, error) {
	if !strings.HasPrefix(dsn, "postgres://") && !strings.HasPrefix(dsn, "postgresql://") {
		return nil, fmt.Errorf("preflight: target must be a postgres:// URL")
	}

	started := time.Now()
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return []PreflightCheck{{Name: "Reachable", Detail: err.Error()}}, nil
	}
	defer func() { _ = db.Close() }()

	if err := db.PingContext(ctx); err != nil {
		// The one check that short-circuits: every check below needs the connection, and
		// reporting four more failures caused by the first would bury the real answer.
		return []PreflightCheck{{
			Name:   "Reachable",
			Detail: unwrapConnErr(err),
		}}, nil
	}

	checks := []PreflightCheck{{
		Name:   "Reachable",
		Detail: fmt.Sprintf("connected in %dms", time.Since(started).Milliseconds()),
		OK:     true,
	}}

	checks = append(checks,
		checkVersion(ctx, db),
		checkEmpty(ctx, db),
		checkPrivileges(ctx, db),
		checkEncoding(ctx, db),
	)
	return checks, nil
}

// PreflightPassed reports whether every check passed — the gate the migrate endpoint
// applies. A helper rather than an inline loop at each call site so "passed" has one
// definition.
func PreflightPassed(checks []PreflightCheck) bool {
	if len(checks) == 0 {
		return false
	}
	for _, c := range checks {
		if !c.OK {
			return false
		}
	}
	return true
}

func checkVersion(ctx context.Context, db *sql.DB) PreflightCheck {
	var full string
	if err := db.QueryRowContext(ctx, `SHOW server_version`).Scan(&full); err != nil {
		return PreflightCheck{Name: "Version", Detail: err.Error()}
	}
	major, err := strconv.Atoi(strings.SplitN(strings.TrimSpace(full), ".", 2)[0])
	if err != nil {
		// Unparseable is not a failure to migrate — it is a failure to read a banner.
		// Report it honestly rather than blocking on a string format.
		return PreflightCheck{Name: "Version", Detail: "PostgreSQL " + full, OK: true}
	}
	if major < minPostgresMajor {
		return PreflightCheck{
			Name:   "Version",
			Detail: fmt.Sprintf("PostgreSQL %s — needs %d or newer", full, minPostgresMajor),
		}
	}
	return PreflightCheck{
		Name:   "Version",
		Detail: fmt.Sprintf("PostgreSQL %s — needs %d or newer", full, minPostgresMajor),
		OK:     true,
	}
}

// checkEmpty refuses a target that already holds Loomarr tables.
//
// ⚠ This is a data-safety check, not tidiness. Migrating into a populated database would
// either collide on primary keys halfway through (leaving both databases in play and the
// operator unsure which is real) or, worse, succeed at inserting alongside somebody
// else's rows. "Wipe it and retry" is a safe instruction only because this check
// guarantees there was nothing to lose.
func checkEmpty(ctx context.Context, db *sql.DB) PreflightCheck {
	rows, err := db.QueryContext(ctx,
		`SELECT tablename FROM pg_tables WHERE schemaname = current_schema()`)
	if err != nil {
		return PreflightCheck{Name: "Target is empty", Detail: err.Error()}
	}
	defer func() { _ = rows.Close() }()

	var found []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return PreflightCheck{Name: "Target is empty", Detail: err.Error()}
		}
		found = append(found, name)
	}
	if err := rows.Err(); err != nil {
		return PreflightCheck{Name: "Target is empty", Detail: err.Error()}
	}
	if len(found) > 0 {
		return PreflightCheck{
			Name: "Target is empty",
			Detail: fmt.Sprintf("%d table(s) already present (%s%s) — migrate into an empty database",
				len(found), strings.Join(first(found, 3), ", "), more(len(found), 3)),
		}
	}
	return PreflightCheck{Name: "Target is empty", Detail: "no tables found", OK: true}
}

// checkPrivileges proves the grant by USING it, not by reading a grant table: goose is
// about to create ten tables and their indexes, and `has_schema_privilege` says what the
// catalog believes rather than what the connection can do.
func checkPrivileges(ctx context.Context, db *sql.DB) PreflightCheck {
	const probe = `loomarr_preflight_probe`
	if _, err := db.ExecContext(ctx, `CREATE TABLE `+probe+` (id INTEGER PRIMARY KEY)`); err != nil {
		return PreflightCheck{Name: "Privileges", Detail: "cannot create tables: " + err.Error()}
	}
	// Best-effort drop: a leaked probe table would fail the "target is empty" check on the
	// next run, which is a confusing way to learn about it.
	if _, err := db.ExecContext(ctx, `DROP TABLE `+probe); err != nil {
		return PreflightCheck{
			Name:   "Privileges",
			Detail: "created a table but could not drop it: " + err.Error(),
		}
	}
	var user string
	_ = db.QueryRowContext(ctx, `SELECT current_user`).Scan(&user)
	return PreflightCheck{
		Name:   "Privileges",
		Detail: fmt.Sprintf("user %q can create and drop tables", user),
		OK:     true,
	}
}

// checkEncoding requires UTF8. Channel names, intents and LLM output are free text; a
// LATIN1 target would corrupt them on insert rather than reject them, which is the worst
// failure mode available — silent, and only discovered by reading the data later.
func checkEncoding(ctx context.Context, db *sql.DB) PreflightCheck {
	var enc, coll string
	err := db.QueryRowContext(ctx,
		`SELECT pg_encoding_to_char(encoding), datcollate FROM pg_database WHERE datname = current_database()`,
	).Scan(&enc, &coll)
	if err != nil {
		return PreflightCheck{Name: "Encoding", Detail: err.Error()}
	}
	if !strings.EqualFold(enc, "UTF8") {
		return PreflightCheck{
			Name:   "Encoding",
			Detail: fmt.Sprintf("%s — Loomarr needs UTF8 (channel names and intents are free text)", enc),
		}
	}
	return PreflightCheck{Name: "Encoding", Detail: enc + " · " + coll, OK: true}
}

// unwrapConnErr keeps a connection failure readable. pgx wraps dial errors several layers
// deep, and the operator needs "connection refused", not a struct dump.
func unwrapConnErr(err error) string {
	msg := err.Error()
	for {
		u := errors.Unwrap(err)
		if u == nil {
			break
		}
		err = u
		msg = err.Error()
	}
	return msg
}

func first(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func more(total, shown int) string {
	if total <= shown {
		return ""
	}
	return fmt.Sprintf(", +%d more", total-shown)
}
