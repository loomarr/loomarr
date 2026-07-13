// Package store is Loomarr's persistence abstraction (design §5): one Store
// interface, two first-class backends (SQLite via modernc.org/sqlite, Postgres
// via pgx's database/sql shim). Dialect differences live only in migrations and
// the ClaimDue* methods; everything else is shared code and one conformance
// suite runs against both backends (CLAUDE.md: never fork the assertions).
package store

import (
	"context"
	"errors"
	"time"

	"github.com/mantonx/loomarr/internal/provision"
)

// ErrNotFound is returned by Get* methods when no row matches.
var ErrNotFound = errors.New("store: not found")

// Store is the full persistence surface (§5). Methods are grouped by domain;
// each group is implemented by the phase that owns it. The provisioning group
// and settings KV are exercised in Phase 3; scheduler/filler/user/job/proposal
// behavior is filled by Phases 9–12, but the interface is defined here so those
// phases add behavior without reshaping callers.
type Store interface {
	// --- provisioning (§3–§4) ---
	GetTitle(ctx context.Context, key provision.Key) (provision.Record, error)
	UpsertTitle(ctx context.Context, rec provision.Record) error
	ListTitlesByState(ctx context.Context, state provision.State) ([]provision.Record, error)
	// ClaimDueTitles atomically claims up to limit non-terminal records
	// (wanted/requested/downloading) whose deadline is at/before now, for the
	// reconciler (§4: wanted→retry, in-flight→give-up; §5 concurrency).
	// Claiming *leases* each row by advancing its deadline to now+lease, so a
	// claimed row won't be handed out again (to a concurrent caller or the next
	// tick) until the reconciler either transitions it or the lease expires —
	// this is what prevents two replicas both firing a give-up/retry. SQLite: a
	// guarded UPDATE (single instance, §5). Postgres: FOR UPDATE SKIP LOCKED.
	ClaimDueTitles(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]provision.Record, error)

	// --- settings KV (§5): instance id, per-app webhook last-received, etc. ---
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error

	// Close releases the underlying database handle.
	Close() error
}
