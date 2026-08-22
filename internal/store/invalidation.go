package store

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/mantonx/loomarr/internal/schedule"
)

const (
	postgresInvalidationChannel = "loomarr_durable_change"
	// Bounds detection of a half-open listener connection. This probes connection liveness only;
	// lifecycle state still arrives through commit-triggered notifications and durable catch-up.
	postgresInvalidationHeartbeat = 5 * time.Second
)

// Invalidation is a commit-ordered Postgres wake-up carrying enough committed state to preserve
// a stop transition even when a later resume commits before a replica handles the first event.
// Durable rows remain truth; reconnect reconciliation is what closes notification-loss gaps.
type Invalidation struct {
	Kind      InvalidationKind       `json:"kind"`
	ChannelID string                 `json:"channelId,omitempty"`
	Status    schedule.ChannelStatus `json:"status,omitempty"`
	Backend   string                 `json:"backend,omitempty"`
	Key       string                 `json:"key,omitempty"`
	Value     string                 `json:"value,omitempty"`
	// ScheduleVersion fingerprints the committed Desired cycle. Process-local
	// encoders compare it with the last durable version they observed so a changed
	// lineup cuts over on every Postgres replica without restarting unchanged sweeps.
	ScheduleVersion string `json:"scheduleVersion,omitempty"`
}

// InvalidationKind identifies the durable record family changed by an invalidation.
type InvalidationKind string

const (
	InvalidationChannel InvalidationKind = "channel"
	InvalidationSetting InvalidationKind = "setting"
)

func channelInvalidation(ch Channel) (string, error) {
	backend := ""
	if ch.Policy.Playout != nil {
		backend = ch.Policy.Playout.Backend
	}
	return marshalInvalidation(Invalidation{
		Kind: InvalidationChannel, ChannelID: ch.ID, Status: ch.Status, Backend: backend,
		ScheduleVersion: ChannelScheduleVersion(ch),
	})
}

// ChannelScheduleVersion returns a stable fingerprint of the accepted broadcast
// cycle carried by a channel invalidation. The Desired JSON is already the durable
// commit boundary; hashing keeps NOTIFY payloads small even for large lineups.
func ChannelScheduleVersion(ch Channel) string {
	b, err := json.Marshal(ch.Desired)
	if err != nil {
		return "" // schedule.Slot is currently infallible; empty degrades to no cutover hint
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func settingInvalidation(key, value string) (string, error) {
	return marshalInvalidation(Invalidation{
		Kind: InvalidationSetting, Key: key, Value: value,
	})
}

func marshalInvalidation(event Invalidation) (string, error) {
	payload, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("marshal durable invalidation: %w", err)
	}
	return string(payload), nil
}

// ListenInvalidations owns one dedicated Postgres connection until ctx is cancelled or the
// connection fails. LISTEN is active before ready runs, so changes committed during ready's
// durable catch-up are queued and handled afterward. Callers reconnect after an error and run the
// same catch-up before reopening their local admission gate.
func ListenInvalidations(
	ctx context.Context,
	st Store,
	ready func(context.Context) error,
	handle func(context.Context, Invalidation) error,
) error {
	s, ok := st.(*sqlStore)
	if !ok || s.dialect != DialectPostgres || s.dsn == "" {
		return fmt.Errorf("listen durable invalidations: postgres store required")
	}
	conn, err := pgx.Connect(ctx, s.dsn)
	if err != nil {
		return fmt.Errorf("listen durable invalidations: connect: %w", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	if _, err := conn.Exec(ctx, "LISTEN "+postgresInvalidationChannel); err != nil {
		return fmt.Errorf("listen durable invalidations: subscribe: %w", err)
	}
	if ready != nil {
		if err := ready(ctx); err != nil {
			return fmt.Errorf("listen durable invalidations: initial reconciliation: %w", err)
		}
	}
	for {
		waitCtx, cancel := context.WithTimeout(ctx, postgresInvalidationHeartbeat)
		notification, err := conn.WaitForNotification(waitCtx)
		cancel()
		if notification != nil {
			var event Invalidation
			if err := json.Unmarshal([]byte(notification.Payload), &event); err != nil {
				return fmt.Errorf("listen durable invalidations: decode: %w", err)
			}
			if handle != nil {
				if err := handle(ctx, event); err != nil {
					return fmt.Errorf("listen durable invalidations: apply: %w", err)
				}
			}
			continue
		}
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
				pingCtx, pingCancel := context.WithTimeout(ctx, postgresInvalidationHeartbeat)
				pingErr := conn.Ping(pingCtx)
				pingCancel()
				if pingErr != nil {
					return fmt.Errorf("listen durable invalidations: heartbeat: %w", pingErr)
				}
				continue
			}
			if errors.Is(err, context.Canceled) && ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("listen durable invalidations: wait: %w", err)
		}
	}
}
