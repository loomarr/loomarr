package catalogstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/broadcaststation"
	"github.com/loomarr/loomarr/internal/store/storedb"
)

// The broadcast station directory (#2060). It is shipped data: boot applies the newest unapplied
// seed revision by replacing every row, and nothing else writes it.

// ListBroadcastStations returns every station with its call signs, ordered by id and call-sign start.
func (s *Store) ListBroadcastStations(ctx context.Context) ([]broadcaststation.Station, error) {
	rows, err := s.h.QueryContext(ctx, `SELECT s.id, s.country, s.city, s.region, s.latitude, s.longitude, c.call_sign, c.from_date, c.to_date
		FROM broadcast_stations s JOIN broadcast_station_call_signs c ON c.station_id = s.id
		ORDER BY s.id, c.from_date, c.call_sign`)
	if err != nil {
		return nil, fmt.Errorf("list broadcast stations: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []broadcaststation.Station
	for rows.Next() {
		var st broadcaststation.Station
		var c broadcaststation.CallSign
		if err := rows.Scan(&st.ID, &st.Country, &st.City, &st.Region, &st.Latitude, &st.Longitude, &c.Sign, &c.From, &c.To); err != nil {
			return nil, err
		}
		if n := len(out); n == 0 || out[n-1].ID != st.ID {
			out = append(out, st)
		}
		last := &out[len(out)-1]
		last.CallSigns = append(last.CallSigns, c)
	}
	return out, rows.Err()
}

// ConvergeBroadcastStationSeed applies the newest shipped revision the store has not recorded. Older
// unapplied revisions are skipped: each one is a complete directory, not a delta.
func (s *Store) ConvergeBroadcastStationSeed(ctx context.Context, revisions []broadcaststation.SeedRevision, at time.Time) error {
	if len(revisions) == 0 {
		return nil
	}
	latest := revisions[len(revisions)-1]
	var applied int
	if err := s.h.QueryRowContext(ctx, s.h.Rebind(`SELECT COUNT(*) FROM broadcast_station_seed_revisions WHERE version >= ?`), latest.Version).Scan(&applied); err != nil {
		return fmt.Errorf("converge broadcast station seed: read marker: %w", err)
	}
	if applied > 0 {
		return nil
	}
	stations, err := latest.Load()
	if err != nil {
		return fmt.Errorf("converge broadcast station seed v%d: %w", latest.Version, err)
	}
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("converge broadcast station seed: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// Two Postgres replicas can boot together. The lock serialises them and the second re-reads the
	// marker, finding the first's revision applied; SQLite already admits one writer at a time.
	if s.h.Dialect() == storedb.DialectPostgres {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('loomarr-broadcast-stations'))`); err != nil {
			return fmt.Errorf("converge broadcast station seed: lock: %w", err)
		}
	}
	if err := tx.QueryRowContext(ctx, s.h.Rebind(`SELECT COUNT(*) FROM broadcast_station_seed_revisions WHERE version >= ?`), latest.Version).Scan(&applied); err != nil {
		return fmt.Errorf("converge broadcast station seed: re-read marker: %w", err)
	}
	if applied > 0 {
		return nil
	}
	for _, stmt := range []string{`DELETE FROM broadcast_station_call_signs`, `DELETE FROM broadcast_stations`} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("converge broadcast station seed: clear: %w", err)
		}
	}
	if err := s.insertBroadcastStations(ctx, tx, stations); err != nil {
		return fmt.Errorf("converge broadcast station seed v%d: %w", latest.Version, err)
	}
	if _, err := tx.ExecContext(ctx, s.h.Rebind(`INSERT INTO broadcast_station_seed_revisions (version, applied_at) VALUES (?, ?)`), latest.Version, epoch(at)); err != nil {
		return fmt.Errorf("converge broadcast station seed: record v%d: %w", latest.Version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("converge broadcast station seed: commit: %w", err)
	}
	return nil
}

// Rows per INSERT: the seed is a few thousand rows applied on every fresh store, test stores
// included, so one statement per row would dominate Postgres store setup.
const broadcastStationBatch = 400

func (s *Store) insertBroadcastStations(ctx context.Context, tx storedb.Tx, stations []broadcaststation.Station) error {
	var stationRows, callSignRows [][]any
	for _, st := range stations {
		stationRows = append(stationRows, []any{st.ID, st.Country, st.City, st.Region, st.Latitude, st.Longitude})
		for _, c := range st.CallSigns {
			callSignRows = append(callSignRows, []any{st.ID, c.Sign, c.From, c.To})
		}
	}
	if err := s.insertBatched(ctx, tx, `INSERT INTO broadcast_stations (id, country, city, region, latitude, longitude) VALUES `, stationRows); err != nil {
		return fmt.Errorf("insert stations: %w", err)
	}
	if err := s.insertBatched(ctx, tx, `INSERT INTO broadcast_station_call_signs (station_id, call_sign, from_date, to_date) VALUES `, callSignRows); err != nil {
		return fmt.Errorf("insert call signs: %w", err)
	}
	return nil
}

func (s *Store) insertBatched(ctx context.Context, tx storedb.Tx, prefix string, rows [][]any) error {
	for start := 0; start < len(rows); start += broadcastStationBatch {
		batch := rows[start:min(start+broadcastStationBatch, len(rows))]
		var query strings.Builder
		query.WriteString(prefix)
		args := make([]any, 0, len(batch)*len(batch[0]))
		for i, row := range batch {
			if i > 0 {
				query.WriteString(", ")
			}
			query.WriteString("(" + strings.TrimSuffix(strings.Repeat("?, ", len(row)), ", ") + ")")
			args = append(args, row...)
		}
		if _, err := tx.ExecContext(ctx, s.h.Rebind(query.String()), args...); err != nil {
			return err
		}
	}
	return nil
}
