// Package crashstore persists the crash reports paired apps send (#1816). The diagnostics module
// validates and scrubs a report before it gets here; the store only persists, pages and purges.
//
// It is a cluster of the store, built on the storedb kernel and never importing the store: the
// store embeds a Store in its aggregate, so every caller of store.Store keeps working unchanged.
// internal/architecture_test.go enforces the direction.
package crashstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
)

// Store is the crash-report persistence over one storedb.Handle.
type Store struct{ h storedb.Handle }

// New returns the crash-report store for h.
func New(h storedb.Handle) *Store { return &Store{h: h} }

const clientCrashSummaryColumns = `id, received_at_ms, occurred_at_ms, COALESCE(user_id, ''), device_id,
	device_name, kind, platform, app_version, app_build, os_version, device_model, message`

func (s *Store) InsertClientCrashReport(ctx context.Context, r diagnostics.ClientCrashRecord) error {
	if r.ID == "" || r.ReceivedAt <= 0 {
		return fmt.Errorf("insert client crash report: id and received time are required")
	}
	var userID any
	if r.UserID != "" {
		userID = r.UserID
	}
	breadcrumbs := r.BreadcrumbsJSON
	if breadcrumbs == "" {
		breadcrumbs = "[]"
	}
	if _, err := s.h.ExecContext(ctx, s.h.Rebind(`INSERT INTO client_crash_reports (id, received_at_ms,
		occurred_at_ms, user_id, device_id, device_name, kind, platform, app_version, app_build,
		os_version, device_model, message, stack, breadcrumbs_json, native_payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),

		r.ID, r.ReceivedAt, r.OccurredAt, userID, r.DeviceID, r.DeviceName, r.Kind, r.Platform,
		r.AppVersion, r.AppBuild, r.OSVersion, r.DeviceModel, r.Message, r.Stack, breadcrumbs,
		r.NativePayload); err != nil {
		return fmt.Errorf("insert client crash report: %w", err)
	}
	return nil
}

func (s *Store) QueryClientCrashReports(
	ctx context.Context, query diagnostics.ClientCrashStoreQuery,
) ([]diagnostics.ClientCrashRecord, error) {
	if query.Limit < 1 || query.Limit > 101 {
		return nil, fmt.Errorf("query client crash reports: invalid module query")
	}
	where, args := "", []any{}
	if query.CursorID != "" {
		where = `WHERE received_at_ms < ? OR (received_at_ms = ? AND id < ?)`
		args = append(args, query.CursorReceivedAt, query.CursorReceivedAt, query.CursorID)
	}
	args = append(args, query.Limit)
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(`SELECT `+clientCrashSummaryColumns+`
		FROM client_crash_reports `+where+` ORDER BY received_at_ms DESC, id DESC LIMIT ?`),
		args...)
	if err != nil {
		return nil, fmt.Errorf("query client crash reports: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := make([]diagnostics.ClientCrashRecord, 0, query.Limit)
	for rows.Next() {
		var r diagnostics.ClientCrashRecord
		if err := rows.Scan(&r.ID, &r.ReceivedAt, &r.OccurredAt, &r.UserID, &r.DeviceID, &r.DeviceName,
			&r.Kind, &r.Platform, &r.AppVersion, &r.AppBuild, &r.OSVersion, &r.DeviceModel, &r.Message); err != nil {
			return nil, fmt.Errorf("scan client crash report: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) GetClientCrashReport(ctx context.Context, id string) (diagnostics.ClientCrashRecord, bool, error) {
	var r diagnostics.ClientCrashRecord
	err := s.h.QueryRowContext(ctx, s.h.Rebind(`SELECT `+clientCrashSummaryColumns+`,
		stack, breadcrumbs_json, native_payload FROM client_crash_reports WHERE id = ?`),
		id).Scan(
		&r.ID, &r.ReceivedAt, &r.OccurredAt, &r.UserID, &r.DeviceID, &r.DeviceName, &r.Kind, &r.Platform,
		&r.AppVersion, &r.AppBuild, &r.OSVersion, &r.DeviceModel, &r.Message, &r.Stack, &r.BreadcrumbsJSON,
		&r.NativePayload)
	if errors.Is(err, sql.ErrNoRows) {
		return diagnostics.ClientCrashRecord{}, false, nil
	}
	if err != nil {
		return diagnostics.ClientCrashRecord{}, false, fmt.Errorf("get client crash report %s: %w", id, err)
	}
	return r, true, nil
}

// PurgeClientCrashReports applies the age window, then the row cap. The cap finds the newest
// kept row with an OFFSET read, then deletes everything older than it, which is portable where
// SQLite's `LIMIT -1 OFFSET n` and Postgres's `LIMIT ALL` are not. The table is bounded by both
// the cap and the per-device rate limit, so one statement each never holds the connection long.
func (s *Store) PurgeClientCrashReports(ctx context.Context, before time.Time, keep int) (int, error) {
	if keep < 1 {
		return 0, fmt.Errorf("purge client crash reports: keep must be positive")
	}
	result, err := s.h.ExecContext(ctx, s.h.Rebind(`DELETE FROM client_crash_reports WHERE received_at_ms < ?`),
		before.UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("purge expired client crash reports: %w", err)
	}
	expired, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("purge expired client crash reports: %w", err)
	}
	var cutAt int64
	var cutID string
	err = s.h.QueryRowContext(ctx, s.h.Rebind(`SELECT received_at_ms, id FROM client_crash_reports
		ORDER BY received_at_ms DESC, id DESC LIMIT 1 OFFSET ?`),
		keep-1).Scan(&cutAt, &cutID)
	if errors.Is(err, sql.ErrNoRows) {
		return int(expired), nil
	}
	if err != nil {
		return int(expired), fmt.Errorf("find client crash report cap: %w", err)
	}
	result, err = s.h.ExecContext(ctx, s.h.Rebind(`DELETE FROM client_crash_reports
		WHERE received_at_ms < ? OR (received_at_ms = ? AND id < ?)`),
		cutAt, cutAt, cutID)
	if err != nil {
		return int(expired), fmt.Errorf("purge client crash reports over cap: %w", err)
	}
	over, err := result.RowsAffected()
	if err != nil {
		return int(expired), fmt.Errorf("purge client crash reports over cap: %w", err)
	}
	return int(expired + over), nil
}
