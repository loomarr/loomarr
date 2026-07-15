package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mantonx/loomarr/internal/schedule"
)

// Channel is the persisted form of a scheduler channel (§9). It carries the
// domain schedule.Channel fields plus persistence concerns: the approved lineup
// and last-computed desired lineup (stored as JSON blobs, like titles.title_json)
// and the reconcile deadline that drives the sweep's ClaimDueChannels. The
// reconcile engine maps between this and the pure schedule.* types.
type Channel struct {
	schedule.Channel
	// Lineup is the approved []LineupEntry (the intent-level "what should play").
	Lineup []schedule.LineupEntry
	// Desired is the last-computed []Slot (desired lineup) — persisted so the
	// sweep can diff without recomputing when nothing changed, and so a restart
	// resumes from the known desired state.
	Desired []schedule.Slot
	// Policy is the channel's ChannelPolicy (programming-design §2): scope/audience/
	// separation/ordering/seasonal + the last reconcile's applied relaxations.
	// Stored as policy_json, sparse (omitted fields resolve to built-in defaults).
	Policy schedule.ChannelPolicy
	// ReconcileDeadline: the channel is due for a sweep reconcile at/before this
	// time. Leased forward on claim (§9/§18).
	ReconcileDeadline time.Time
}

func (s *sqlStore) GetChannel(ctx context.Context, id string) (Channel, error) {
	row := s.db.QueryRowContext(ctx, s.ph(channelSelect+` WHERE id = ?`), id)
	return scanChannel(row)
}

func (s *sqlStore) GetChannelByNumber(ctx context.Context, number int) (Channel, error) {
	row := s.db.QueryRowContext(ctx, s.ph(channelSelect+` WHERE number = ?`), number)
	return scanChannel(row)
}

func (s *sqlStore) UpsertChannel(ctx context.Context, ch Channel) error {
	lineupBlob, err := json.Marshal(orEmptyEntries(ch.Lineup))
	if err != nil {
		return fmt.Errorf("marshal lineup: %w", err)
	}
	desiredBlob, err := json.Marshal(orEmptySlots(ch.Desired))
	if err != nil {
		return fmt.Errorf("marshal desired: %w", err)
	}
	// Policy is a single object (not a slice), so it serializes to "{}" when zero —
	// the DDL default — never "null"; json.Marshal of a struct already yields "{}".
	policyBlob, err := json.Marshal(ch.Policy)
	if err != nil {
		return fmt.Errorf("marshal policy: %w", err)
	}
	_, err = s.db.ExecContext(ctx, s.ph(
		`INSERT INTO channels
		   (id, intent_ref, name, number, grp, logo, strategy, filler_ref, tunarr_id,
		    status, shuffle_seed, lineup_json, desired_json, policy_json, reconcile_deadline, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   intent_ref=excluded.intent_ref, name=excluded.name, number=excluded.number,
		   grp=excluded.grp, logo=excluded.logo, strategy=excluded.strategy,
		   filler_ref=excluded.filler_ref, tunarr_id=excluded.tunarr_id, status=excluded.status,
		   shuffle_seed=excluded.shuffle_seed, lineup_json=excluded.lineup_json,
		   desired_json=excluded.desired_json, policy_json=excluded.policy_json,
		   reconcile_deadline=excluded.reconcile_deadline, updated_at=excluded.updated_at`),
		ch.ID, ch.IntentRef, ch.Name, ch.Number, ch.Group, ch.Logo, string(ch.Strategy),
		ch.FillerRef, ch.TunarrID, string(ch.Status), ch.Shuffle.Seed,
		string(lineupBlob), string(desiredBlob), string(policyBlob),
		epoch(ch.ReconcileDeadline), ch.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert channel %s: %w", ch.ID, err)
	}
	return nil
}

func (s *sqlStore) ListChannels(ctx context.Context) ([]Channel, error) {
	rows, err := s.db.QueryContext(ctx, s.ph(channelSelect+` ORDER BY number`))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return scanChannels(rows)
}

func (s *sqlStore) DeleteChannel(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, s.ph(`DELETE FROM channels WHERE id = ?`), id)
	return err
}

// ClaimDueChannels runs the dialect-specific channel-claim statement. Selects
// non-detached channels whose reconcile_deadline is at/before now, up to limit,
// and leases each by advancing its deadline to now+lease (§9/§18).
// Placeholder order (all dialects): 1=leaseUntil, 2=now, 3=limit.
func (s *sqlStore) ClaimDueChannels(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Channel, error) {
	leaseUntil := epoch(now.Add(lease))
	rows, err := s.db.QueryContext(ctx, s.channelClaimSQL, leaseUntil, epoch(now), limit)
	if err != nil {
		return nil, fmt.Errorf("claim due channels: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanChannels(rows)
}

// channelSelect is the shared column list; claim SQL RETURNs the same columns in
// this order so scanChannel serves both paths (mirrors scanTitle).
const channelSelect = `SELECT id, intent_ref, name, number, grp, logo, strategy, filler_ref,
	tunarr_id, status, shuffle_seed, lineup_json, desired_json, policy_json, reconcile_deadline, updated_at
	FROM channels`

func scanChannel(sc scannable) (Channel, error) {
	var (
		ch                                  Channel
		strategy, status                    string
		lineupBlob, desiredBlob, policyBlob string
		seed, deadline, updatedAt           int64
	)
	err := sc.Scan(&ch.ID, &ch.IntentRef, &ch.Name, &ch.Number, &ch.Group, &ch.Logo,
		&strategy, &ch.FillerRef, &ch.TunarrID, &status, &seed,
		&lineupBlob, &desiredBlob, &policyBlob, &deadline, &updatedAt)
	if err == sql.ErrNoRows {
		return Channel{}, ErrNotFound
	}
	if err != nil {
		return Channel{}, err
	}
	ch.Strategy = schedule.Strategy(strategy)
	ch.Status = schedule.ChannelStatus(status)
	ch.Shuffle.Seed = seed
	if err := json.Unmarshal([]byte(lineupBlob), &ch.Lineup); err != nil {
		return Channel{}, fmt.Errorf("unmarshal lineup %s: %w", ch.ID, err)
	}
	if err := json.Unmarshal([]byte(desiredBlob), &ch.Desired); err != nil {
		return Channel{}, fmt.Errorf("unmarshal desired %s: %w", ch.ID, err)
	}
	if err := json.Unmarshal([]byte(policyBlob), &ch.Policy); err != nil {
		return Channel{}, fmt.Errorf("unmarshal policy %s: %w", ch.ID, err)
	}
	ch.ReconcileDeadline = fromEpoch(deadline)
	ch.UpdatedAt = updatedAt
	return ch, nil
}

func scanChannels(rows *sql.Rows) ([]Channel, error) {
	var out []Channel
	for rows.Next() {
		ch, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// orEmptyEntries / orEmptySlots ensure a nil slice serializes to `[]` not `null`,
// so the stored JSON round-trips to an empty (non-nil) slice.
func orEmptyEntries(e []schedule.LineupEntry) []schedule.LineupEntry {
	if e == nil {
		return []schedule.LineupEntry{}
	}
	return e
}

func orEmptySlots(s []schedule.Slot) []schedule.Slot {
	if s == nil {
		return []schedule.Slot{}
	}
	return s
}
