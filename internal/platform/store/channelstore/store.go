// Package channelstore persists channels (the channels table, §9): the approved lineup, the desired
// schedule, the optimistic revision, and the reconcile deadline the sweep claims by.
//
// It is a cluster of the store, built on the storedb kernel and never importing the store: the
// store embeds a Store in its aggregate and re-exports Channel and its errors by alias, so every
// caller of store.Store keeps working unchanged. The proposal-approval transaction and the
// library-reference rewrite write channels inside their own transactions through SaveChannelIn,
// ChannelForJobIn, ScanChannels and the Columns/Select statements; the dialect claim statements are
// passed to New. internal/architecture_test.go enforces the direction.
package channelstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/storedb"
	"github.com/loomarr/loomarr/internal/provision"
)

// ErrJobChannelGone reports a refine or re-curation job whose channel was removed
// before the job's proposal could be approved. It is not ErrNotFound: that answer
// means "no channel yet" and makes approval create one.
var ErrJobChannelGone = errors.New("store: the job's channel no longer exists")

// ErrChannelConflict reports that a channel write lost a uniqueness race (normally
// number or non-empty intent binding). Approval transactions have rolled back
// completely when returning it, so their caller may safely reload and replan.
var ErrChannelConflict = errors.New("store: channel conflict")

// ErrChannelStale reports that a channel mutation was planned from an older
// revision than the row now holds. The row still exists; callers may reload and
// either reapply their domain merge or surface a conflict to the operator.
var ErrChannelStale = errors.New("store: stale channel revision")

// Invalidations builds the commit-ordered Postgres wake-ups a channel write and delete publish
// (the store's durable invalidation payloads). A nil Invalidations, which is every SQLite store,
// publishes nothing.
type Invalidations interface {
	// Saved is the payload for a channel write.
	Saved(ch Channel) (string, error)
	// Deleted is the payload for removing the channel id.
	Deleted(id string) (string, error)
	// NotifyChannel is the pg_notify channel both payloads go to.
	NotifyChannel() string
}

// Store is the channel persistence over one storedb.Handle.
type Store struct {
	h        storedb.Handle
	claimSQL string
	inv      Invalidations
}

// New returns the channel store for h. claimSQL is the dialect's ClaimDueChannels statement (already
// rebound; placeholders 1=leaseUntil, 2=now, 3=limit) and inv builds the Postgres invalidations, nil
// on SQLite.
func New(h storedb.Handle, claimSQL string, inv Invalidations) *Store {
	return &Store{h: h, claimSQL: claimSQL, inv: inv}
}

// The two legal channel broadcast codecs (§9.1 V50). Kept as plain strings to match
// how codecs flow everywhere else (playout.MediaFormat.VideoCodec, the plan capability
// maps) — a fresh typed enum here would only need converting at every boundary. H264 is
// the migration default, so an un-backfilled channel broadcasts h264/TS exactly as today.
const (
	BroadcastCodecH264 = "h264"
	BroadcastCodecHEVC = "hevc"
)

// Channel is the persisted form of a scheduler channel (§9). It carries the
// domain schedule.Channel fields plus persistence concerns: the approved lineup
// and last-computed desired lineup (stored as JSON blobs, like titles.title_json)
// and the reconcile deadline that drives the sweep's ClaimDueChannels. The
// reconcile engine maps between this and the pure schedule.* types.
type Channel struct {
	schedule.Channel
	// Revision is the optimistic concurrency token for every channel-row mutation.
	// Zero is the create sentinel accepted by SaveChannel; persisted rows start at 1.
	Revision int64
	// IntentJSON is the approved intent: the request this channel was made from, without any
	// one-run refinement. It lives on the channel, not on the job that produced it, because
	// housekeeping purges finished jobs while a refine or a re-curation still needs the request.
	// Empty for a hand-made channel and for one whose job was purged before the column existed.
	IntentJSON string
	// RequestedBy is the user id of the person who made the request, set when the channel is
	// created and never changed by a refine or a re-curation. Empty when no member asked.
	RequestedBy string
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
	// BroadcastCodec is the channel's uniform broadcast video codec (§9.1 V50): the
	// codec every program on the timeline is normalized TO so the stream stays
	// single-codec (which is what makes HEVC-fMP4 legal — fMP4 can't switch codec
	// mid-stream). Derived from the majority of the library titles' codecs at curation,
	// not probed at runtime. One of BroadcastCodecH264 / BroadcastCodecHEVC; empty
	// (an un-backfilled row read before the DDL default applies) reads as H264.
	BroadcastCodec string
	// PlayoutAnchor is the immutable wall-clock origin of this channel's accepted
	// broadcast cycle. Building/empty channels may be zero until first activation.
	PlayoutAnchor time.Time
	// ReconcileDeadline: the channel is due for a sweep reconcile at/before this
	// time. Leased forward on claim (§9/§18).
	ReconcileDeadline time.Time
	// CreatedAt is when the channel row was first saved (Home's "New channel", #1663). SaveChannel
	// stamps it on create when zero and never changes it after. Zero on a channel older than the
	// column that no approval dates.
	CreatedAt time.Time
}

// requesterValue writes a channel's requester: the member, or NULL when no member has that id.
// requested_by is a foreign key whose person going leaves the channel with no requester (SET
// NULL, #2793); a request outlives its author, and a channel read before they went may be saved
// after, so a save records the same "no requester" rather than failing.
const requesterValue = `(SELECT id FROM users WHERE id = ?)`

// ChannelRequesters maps each channel made from a member's request to that person's name, read
// from the channel's own `requested_by`. One query for every channel; a hand-made channel, or
// one whose requester no longer exists, has no entry (#1663).
func (s *Store) ChannelRequesters(ctx context.Context) (map[string]string, error) {
	rows, err := s.h.QueryContext(ctx,
		`SELECT c.id, u.name FROM channels c
		 JOIN users u ON u.id = c.requested_by`)
	if err != nil {
		return nil, fmt.Errorf("channel requesters: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

func (s *Store) GetChannel(ctx context.Context, id string) (Channel, error) {
	row := s.h.QueryRowContext(ctx, s.h.Rebind(Select(s)+` WHERE id = ?`), id)
	return ScanChannel(row)
}

func (s *Store) GetChannelByNumber(ctx context.Context, number int) (Channel, error) {
	row := s.h.QueryRowContext(ctx, s.h.Rebind(Select(s)+` WHERE number = ?`), number)
	return ScanChannel(row)
}

// GetChannelByIntentRef finds the channel bound to a suggestion job. A channel's `intent_ref` IS
// the job id that produced it, so this answers "which channel does this proposal belong to?" —
// asked once per bind and once per auto-curate consideration.
//
// ⚠ Indexed (00037), and it replaces two byte-identical `ListChannels`-then-linear-scan helpers
// that had been copy-pasted into `binder` and `recurate`. Two packages walking the whole channel
// table for one row is the shape a missing store method leaves behind.
//
// ⚠ Returns ErrNotFound rather than a zero Channel. Both former callers treated "no match" as
// benign (a job with no channel is simply not an auto-curate candidate), and they still can — but
// that has to be the CALLER's decision: a lookup that silently answers with an empty struct is one
// a caller can forget to check, and the zero Channel has an empty ID that reads as valid.
func (s *Store) GetChannelByIntentRef(ctx context.Context, intentRef string) (Channel, error) {
	row := s.h.QueryRowContext(ctx, s.h.Rebind(Select(s)+` WHERE intent_ref = ?`), intentRef)
	return ScanChannel(row)
}

// GetChannelForJob finds the channel a suggestion job's proposal belongs to: the channel a refine
// or re-curation job names in `proposal_jobs.channel_id`, or else the channel the job originated
// (`intent_ref`). It answers "which channel does approving this proposal change?" for the binder,
// the auto-curate grant and the job's journey.
//
// ErrNotFound means no channel is bound to the job yet — a first approval creates one. A job that
// names a channel which has since been removed returns ErrJobChannelGone instead, so a caller
// cannot mistake it for a first approval and create a second channel.
func (s *Store) GetChannelForJob(ctx context.Context, jobID string) (Channel, error) {
	return ChannelForJobIn(ctx, s, s.h, jobID)
}

// ChannelForJobIn is GetChannelForJob on q, which is a caller's transaction or the pool, so the
// approval transaction and the job's Journey read see the same channel it does. A function, not a
// method, like SaveChannelIn.
func ChannelForJobIn(ctx context.Context, s *Store, q DB, jobID string) (Channel, error) {
	ph := s.h.Rebind
	var channelID string
	err := q.QueryRowContext(ctx, ph(`SELECT channel_id FROM proposal_jobs WHERE id = ?`), jobID).Scan(&channelID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Channel{}, fmt.Errorf("read job %s channel: %w", jobID, err)
	}
	if channelID != "" {
		ch, err := ScanChannel(q.QueryRowContext(ctx, ph(Select(s)+` WHERE id = ?`), channelID))
		if errors.Is(err, storedb.ErrNotFound) {
			return Channel{}, fmt.Errorf("%w: job %s names channel %s", ErrJobChannelGone, jobID, channelID)
		}
		return ch, err
	}
	return ScanChannel(q.QueryRowContext(ctx, ph(Select(s)+` WHERE intent_ref = ?`), jobID))
}

// ChannelHasUnfinishedJob reports whether one of the channel's refine or re-curation jobs is
// still in flight: queued, running, or done with a proposal no one has decided yet. A new
// channel job is refused while one is, so two runs never race to replace each other's lineup and
// unapproved re-curations do not pile up. A failed job is finished: it leaves nothing to decide.
func (s *Store) ChannelHasUnfinishedJob(ctx context.Context, channelID string) (bool, error) {
	var n int
	err := s.h.QueryRowContext(ctx, s.h.Rebind(
		`SELECT COUNT(*) FROM proposal_jobs j
		  WHERE j.channel_id = ?
		    AND (j.status IN ('queued', 'running')
		         OR (j.status = 'done' AND EXISTS (
		               SELECT 1 FROM proposals p WHERE p.job_id = j.id AND p.status = 'submitted')))`),

		channelID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("channel %s unfinished jobs: %w", channelID, err)
	}
	return n > 0, nil
}

// SaveChannel writes the channel row and its lineup rows in one transaction; a live channel's
// job also reaches its Journey milestone in it.
func (s *Store) SaveChannel(ctx context.Context, ch Channel) (Channel, error) {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return Channel{}, fmt.Errorf("save channel %s: begin: %w", ch.ID, err)
	}
	defer func() { _ = tx.Rollback() }()
	saved, err := s.saveChannel(ctx, tx, ch)
	if storedb.IsConstraintViolation(err) {
		return Channel{}, fmt.Errorf("%w: %v", ErrChannelConflict, err)
	}
	if err != nil {
		return Channel{}, err
	}
	if (ch.Status == schedule.StatusLive || ch.Status == schedule.StatusDrifted) && ch.IntentRef != "" {
		if _, err := tx.ExecContext(ctx, s.h.Rebind(
			`UPDATE proposal_jobs SET reached_live=TRUE WHERE id=?`),
			ch.IntentRef); err != nil {
			return Channel{}, fmt.Errorf("save live channel %s Journey milestone: %w", ch.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return Channel{}, fmt.Errorf("save channel %s: commit: %w", ch.ID, err)
	}
	return saved, nil
}

type DB interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// SaveChannelIn writes ch on exec, which must be a caller's transaction (the lineup rows are written
// beside the channel row), with the same
// serialization and SQL as Store.SaveChannel. The proposal-approval transaction and the
// library-reference rewrite use it so their atomic paths are not a subtly different channel adapter.
// It is a function, not a method, so it does not widen the store's method set.
func SaveChannelIn(ctx context.Context, s *Store, exec DB, ch Channel) (Channel, error) {
	return s.saveChannel(ctx, exec, ch)
}

// saveChannel is shared by ordinary channel writes and SaveChannelIn. Keeping the serialization and
// SQL in one place prevents the atomic path from becoming a subtly different channel adapter.
func (s *Store) saveChannel(ctx context.Context, exec DB, ch Channel) (Channel, error) {
	desiredBlob, err := json.Marshal(orEmptySlots(ch.Desired))
	if err != nil {
		return Channel{}, fmt.Errorf("marshal desired: %w", err)
	}
	// Policy is a single object (not a slice), so it serializes to "{}" when zero —
	// the DDL default — never "null"; json.Marshal of a struct already yields "{}".
	policyBlob, err := json.Marshal(ch.Policy)
	if err != nil {
		return Channel{}, fmt.Errorf("marshal policy: %w", err)
	}
	// An empty BroadcastCodec (a Channel built without setting it) persists as the
	// DDL default h264, so the round-trip never writes "" — matching what an
	// un-backfilled row reads back as.
	broadcastCodec := ch.BroadcastCodec
	if broadcastCodec == "" {
		broadcastCodec = BroadcastCodecH264
	}
	args := []any{ch.IntentRef, storedb.OptionalJSON(ch.IntentJSON), storedb.OptionalID(ch.RequestedBy), ch.Name, ch.Number, ch.Group, ch.Logo,
		string(ch.Strategy), ch.TunarrID, string(ch.Status), ch.Shuffle.Seed,
		string(desiredBlob), string(policyBlob), broadcastCodec,
		storedb.Epoch(ch.PlayoutAnchor), storedb.Epoch(ch.ReconcileDeadline), ch.UpdatedAt}
	invalidation := ""
	if s.inv != nil {
		invalidation, err = s.inv.Saved(ch)
		if err != nil {
			return Channel{}, err
		}
	}
	if ch.Revision == 0 {
		// Creation is stamped here, once; the UPDATE below never writes created_at (#1663).
		if ch.CreatedAt.IsZero() {
			ch.CreatedAt = time.Now().UTC().Truncate(time.Second)
		}
		query :=
			`INSERT INTO channels
			   (id, intent_ref, intent_json, requested_by, name, number, grp, logo, strategy,
			    tunarr_id, status, shuffle_seed, desired_json, policy_json, broadcast_codec,
			    playout_anchor, reconcile_deadline, updated_at, revision, created_at)
			 VALUES (?, ?, ?, ` + requesterValue + `, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)
			 RETURNING revision`
		queryArgs := append(append([]any{ch.ID}, args...), storedb.Epoch(ch.CreatedAt))
		if s.inv != nil {
			query += `, pg_notify('` + s.inv.NotifyChannel() + `', ?)`
			queryArgs = append(queryArgs, invalidation)
			var notified any
			err = exec.QueryRowContext(ctx, s.h.Rebind(query), queryArgs...).Scan(&ch.Revision, &notified)
		} else {
			err = exec.QueryRowContext(ctx, s.h.Rebind(query), queryArgs...).Scan(&ch.Revision)
		}
		if err != nil {
			return Channel{}, fmt.Errorf("insert channel %s: %w", ch.ID, err)
		}
		return ch, s.writeLineup(ctx, exec, ch.ID, ch.Lineup)
	}
	if ch.Revision < 0 {
		return Channel{}, fmt.Errorf("save channel %s: revision must not be negative", ch.ID)
	}
	expected := ch.Revision
	query :=
		`UPDATE channels SET
		   intent_ref=?, intent_json=?, requested_by=` + requesterValue + `, name=?, number=?, grp=?, logo=?, strategy=?,
		   tunarr_id=?, status=?, shuffle_seed=?, desired_json=?,
		   policy_json=?, broadcast_codec=?, playout_anchor=?, reconcile_deadline=?, updated_at=?,
		   revision=revision+1
		 WHERE id=? AND revision=?
		 RETURNING revision`
	queryArgs := append(args, ch.ID, expected)
	if s.inv != nil {
		query += `, pg_notify('` + s.inv.NotifyChannel() + `', ?)`
		queryArgs = append(queryArgs, invalidation)
		var notified any
		err = exec.QueryRowContext(ctx, s.h.Rebind(query), queryArgs...).Scan(&ch.Revision, &notified)
	} else {
		err = exec.QueryRowContext(ctx, s.h.Rebind(query), queryArgs...).Scan(&ch.Revision)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, channelRevisionMiss(ctx, exec, s.h.Rebind, ch.ID, expected)
	}
	if err != nil {
		return Channel{}, fmt.Errorf("save channel %s: %w", ch.ID, err)
	}
	return ch, s.writeLineup(ctx, exec, ch.ID, ch.Lineup)
}

func channelRevisionMiss(ctx context.Context, q DB, ph storedb.Placeholder, id string, expected int64) error {
	var current int64
	err := q.QueryRowContext(ctx, ph(`SELECT revision FROM channels WHERE id = ?`), id).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return storedb.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read channel %s revision: %w", id, err)
	}
	return fmt.Errorf("%w: channel %s expected %d, current %d", ErrChannelStale, id, expected, current)
}

func channelMutationMiss(ctx context.Context, q DB, ph storedb.Placeholder, id, reason string) error {
	var revision int64
	err := q.QueryRowContext(ctx, ph(`SELECT revision FROM channels WHERE id = ?`), id).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return storedb.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read channel %s revision: %w", id, err)
	}
	return fmt.Errorf("%w: channel %s %s at revision %d", ErrChannelStale, id, reason, revision)
}

func (s *Store) AttachTunarrChannel(
	ctx context.Context,
	id, expectedTunarrID, newTunarrID string,
	expectedNumber, newNumber int,
) (int64, error) {
	var revision int64
	err := s.h.QueryRowContext(ctx, s.h.Rebind(
		`UPDATE channels SET tunarr_id = ?, number = ?, revision = revision + 1
		 WHERE id = ? AND tunarr_id = ? AND number = ? RETURNING revision`),

		newTunarrID, newNumber, id, expectedTunarrID, expectedNumber).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, channelMutationMiss(ctx, s.h, s.h.Rebind, id, "tunarr id or number changed")
	}
	if storedb.IsConstraintViolation(err) {
		return 0, fmt.Errorf("%w: attach Tunarr channel %s: %v", ErrChannelConflict, id, err)
	}
	if err != nil {
		return 0, fmt.Errorf("attach Tunarr channel %s: %w", id, err)
	}
	return revision, nil
}

// SetChannelBroadcastCodec updates ONLY the channel's broadcast_codec column (§9.1 V50) — the
// uniform codec the timeline normalizes to, computed from the library titles' codecs at curation.
//
// A targeted single-column UPDATE rather than a read-modify-write SaveChannel: the codec is
// derived state written after the lineup is bound, and the binder is the ONE lineup writer.
// Reusing SaveChannel here would mean re-reading the whole row and
// racing the reconciler's own writes for no benefit — this only ever touches the one derived column.
func (s *Store) SetChannelBroadcastCodec(ctx context.Context, id string, expectedRevision int64, codec string) (int64, error) {
	if codec == "" {
		codec = BroadcastCodecH264
	}
	var revision int64
	err := s.h.QueryRowContext(ctx, s.h.Rebind(
		`UPDATE channels SET broadcast_codec = ?, revision = revision + 1
		 WHERE id = ? AND revision = ? RETURNING revision`),

		codec, id, expectedRevision).Scan(&revision)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, channelRevisionMiss(ctx, s.h, s.h.Rebind, id, expectedRevision)
	}
	if err != nil {
		return 0, fmt.Errorf("set broadcast_codec %s: %w", id, err)
	}
	return revision, nil
}

func (s *Store) ListChannels(ctx context.Context) ([]Channel, error) {
	rows, err := s.h.QueryContext(ctx, s.h.Rebind(Select(s)+` ORDER BY number`))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return ScanChannels(rows)
}

func (s *Store) DeleteChannel(ctx context.Context, id string, expectedRevision int64) error {
	tx, err := s.h.Begin(ctx)
	if err != nil {
		return fmt.Errorf("delete channel %s: begin: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	if s.inv != nil {
		payload, err := s.inv.Deleted(id)
		if err != nil {
			return err
		}
		var notified any
		err = tx.QueryRowContext(ctx,
			`DELETE FROM channels WHERE id = $1 AND revision = $2
			 RETURNING pg_notify('`+s.inv.NotifyChannel()+`', $3)`,
			id, expectedRevision, payload).Scan(&notified)
		if errors.Is(err, sql.ErrNoRows) {
			return channelRevisionMiss(ctx, tx, s.h.Rebind, id, expectedRevision)
		}
		if err != nil {
			return err
		}
	} else {
		result, err := tx.ExecContext(ctx, s.h.Rebind(`DELETE FROM channels WHERE id = ? AND revision = ?`), id, expectedRevision)
		if err != nil {
			return err
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("delete channel %s: affected rows: %w", id, err)
		}
		if deleted == 0 {
			return channelRevisionMiss(ctx, tx, s.h.Rebind, id, expectedRevision)
		}
	}
	// The channel's feedback went with the row (discovery_feedback.scope_id cascades, #2793).
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("delete channel %s: commit: %w", id, err)
	}
	// The channel's icon and watermark refs went with the row (channel_images cascades, #2793), so
	// the GC can collect them; a deleted channel can no longer pin its images.
	return nil
}

// ⚠ There are no channel-icon byte accessors: icon bytes are image-service images (§22),
// content-addressed on disk and served by /v1/images/{hash} under an immutable cache. A channel
// only references its images through `channel_images`.

// ClaimDueChannels runs the dialect-specific channel-claim statement. Selects
// non-detached channels whose reconcile_deadline is at/before now, up to limit,
// and leases each by advancing its deadline to now+lease (§9/§18).
// Placeholder order (all dialects): 1=leaseUntil, 2=now, 3=limit.
func (s *Store) ClaimDueChannels(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Channel, error) {
	leaseUntil := storedb.Epoch(now.Add(lease))
	rows, err := s.h.QueryContext(ctx, s.claimSQL, leaseUntil, storedb.Epoch(now), limit)
	if err != nil {
		return nil, fmt.Errorf("claim due channels: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return ScanChannels(rows)
}

// Columns is the column list ScanChannel reads, in its order; lineup is the expression that
// rebuilds the lineup (SQLiteLineup, PostgresLineup). Every channel read, the claims' RETURNING
// included, goes through it, so none can miss the lineup. The Postgres claim spells the rest with
// its `c.` alias (postgres.go) and must stay in step.
func Columns(lineup string) string {
	return `id, intent_ref, intent_json, requested_by, name, number, grp, logo, strategy,
		tunarr_id, status, shuffle_seed, ` + lineup + `, desired_json, policy_json, broadcast_codec,
		playout_anchor, reconcile_deadline, updated_at, revision, created_at`
}

// SQLiteLineup and PostgresLineup rebuild a channel's lineup, in order, as the JSON list
// LineupEntry decodes from: each channel_lineup row's entry with its title key put back as Key
// (#2793). table is how the statement names the channels row.
func SQLiteLineup(table string) string {
	return `(SELECT json_group_array(json_set(l.entry, '$.Key', l.title_key) ORDER BY l.position)
		FROM channel_lineup l WHERE l.channel_id = ` + table + `.id)`
}

// PostgresLineup is SQLiteLineup for Postgres.
func PostgresLineup(table string) string {
	return `COALESCE((SELECT jsonb_agg(l.entry || jsonb_build_object('Key', l.title_key) ORDER BY l.position)
		FROM channel_lineup l WHERE l.channel_id = ` + table + `.id), '[]'::jsonb)`
}

// Select selects every channel column, the lineup rebuilt for s's engine. It is a function, not a
// method, for the reason SaveChannelIn is: the store embeds a Store, and a method would surface on
// the aggregate as a store method that store.Store does not declare.
func Select(s *Store) string {
	// The lineup rebuild is a JSON aggregate whose function names differ between the engines.
	lineup := storedb.ByDialect(s.h.Dialect(), SQLiteLineup("channels"), PostgresLineup("channels"))
	return `SELECT ` + Columns(lineup) + ` FROM channels`
}

// writeLineup replaces a channel's lineup rows with lineup, in order. It runs on the transaction
// that wrote the channel row, so a reader never sees a channel with half a lineup. A title no
// titles row records yet (one added by hand) gets a pending row first, because title_key is a
// foreign key (storedb.PendingTitle).
func (s *Store) writeLineup(ctx context.Context, exec DB, channelID string, lineup []schedule.LineupEntry) error {
	if _, err := exec.ExecContext(ctx, s.h.Rebind(`DELETE FROM channel_lineup WHERE channel_id = ?`), channelID); err != nil {
		return fmt.Errorf("clear lineup %s: %w", channelID, err)
	}
	now := time.Now().Unix()
	for i, e := range lineup {
		entry, err := lineupEntryDocument(e)
		if err != nil {
			return fmt.Errorf("marshal lineup %s entry %d: %w", channelID, i, err)
		}
		title, err := json.Marshal(pendingTitle(e))
		if err != nil {
			return fmt.Errorf("marshal lineup %s entry %d title: %w", channelID, i, err)
		}
		if _, err := exec.ExecContext(ctx, s.h.Rebind(`INSERT INTO titles (key, title_json, state, updated_at)
			VALUES (?, ?, ?, ?) ON CONFLICT (key) DO NOTHING`),
			string(e.Key), string(title), storedb.PendingTitle, now); err != nil {
			return fmt.Errorf("write lineup %s entry %d title: %w", channelID, i, err)
		}
		if _, err := exec.ExecContext(ctx, s.h.Rebind(`INSERT INTO channel_lineup (channel_id, position, title_key, entry)
			VALUES (?, ?, ?, ?)`), channelID, i, string(e.Key), entry); err != nil {
			return fmt.Errorf("write lineup %s entry %d: %w", channelID, i, err)
		}
	}
	return nil
}

// pendingTitle is what a lineup entry says of its title: the identity its key spells, its name
// and year. Whatever asks for the title later writes the full record over it.
func pendingTitle(e schedule.LineupEntry) provision.Title {
	t := provision.Title{Name: e.Title, Year: e.Year}
	if mt, provider, id, ok := provision.ParseKey(e.Key); ok {
		t.MediaType = mt
		if provider == "tvdb" {
			t.TVDBID = id
		} else {
			t.TMDBID = id
		}
	}
	return t
}

// lineupEntryDocument is an entry's attributes without its title key, which is its row's column.
func lineupEntryDocument(e schedule.LineupEntry) (string, error) {
	blob, err := json.Marshal(e)
	if err != nil {
		return "", err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(blob, &fields); err != nil {
		return "", err
	}
	delete(fields, "Key")
	blob, err = json.Marshal(fields)
	return string(blob), err
}

func ScanChannel(sc storedb.Scannable) (Channel, error) {
	var (
		ch                                  Channel
		strategy, status                    string
		lineupBlob, desiredBlob, policyBlob string
		seed, anchor, deadline, updatedAt   int64
		createdAt                           int64
	)
	err := sc.Scan(&ch.ID, &ch.IntentRef, (*storedb.OptionalJSON)(&ch.IntentJSON), (*storedb.OptionalID)(&ch.RequestedBy), &ch.Name, &ch.Number, &ch.Group, &ch.Logo,
		&strategy, &ch.TunarrID, &status, &seed,
		&lineupBlob, &desiredBlob, &policyBlob, &ch.BroadcastCodec, &anchor, &deadline, &updatedAt, &ch.Revision,
		&createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, storedb.ErrNotFound
	}
	if err != nil {
		return Channel{}, err
	}
	ch.Strategy = schedule.Strategy(strategy)
	ch.Status = schedule.ChannelStatus(status)
	ch.Shuffle.Seed = seed
	ch.PlayoutAnchor = storedb.FromEpoch(anchor)
	if err := json.Unmarshal([]byte(lineupBlob), &ch.Lineup); err != nil {
		return Channel{}, fmt.Errorf("unmarshal lineup %s: %w", ch.ID, err)
	}
	if err := json.Unmarshal([]byte(desiredBlob), &ch.Desired); err != nil {
		return Channel{}, fmt.Errorf("unmarshal desired %s: %w", ch.ID, err)
	}
	if err := json.Unmarshal([]byte(policyBlob), &ch.Policy); err != nil {
		return Channel{}, fmt.Errorf("unmarshal policy %s: %w", ch.ID, err)
	}
	ch.ReconcileDeadline = storedb.FromEpoch(deadline)
	ch.UpdatedAt = updatedAt
	ch.CreatedAt = storedb.FromEpoch(createdAt)
	return ch, nil
}

func ScanChannels(rows *sql.Rows) ([]Channel, error) {
	var out []Channel
	for rows.Next() {
		ch, err := ScanChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

// orEmptySlots ensures a nil slice serializes to `[]` not `null`, so the stored JSON round-trips
// to an empty (non-nil) slice.
func orEmptySlots(s []schedule.Slot) []schedule.Slot {
	if s == nil {
		return []schedule.Slot{}
	}
	return s
}
