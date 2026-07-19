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

	// --- scheduler / channels (§9) ---
	GetChannel(ctx context.Context, id string) (Channel, error)
	GetChannelByNumber(ctx context.Context, number int) (Channel, error)
	UpsertChannel(ctx context.Context, ch Channel) error
	ListChannels(ctx context.Context) ([]Channel, error)
	DeleteChannel(ctx context.Context, id string) error
	// ClaimDueChannels atomically claims up to limit channels whose
	// reconcile_deadline is at/before now, for the periodic reconcile sweep
	// (§9). Like ClaimDueTitles it *leases* each claimed channel (deadline →
	// now+lease) so two replicas never reconcile one Tunarr channel at once
	// (§18: single-leader / per-channel row claiming). SQLite: guarded UPDATE.
	// Postgres: FOR UPDATE SKIP LOCKED. Detached channels are never claimed.
	ClaimDueChannels(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Channel, error)

	// --- suggester jobs & proposals (§8) ---
	CreateJob(ctx context.Context, j Job) error
	GetJob(ctx context.Context, id string) (Job, error)
	UpdateJob(ctx context.Context, j Job) error
	// ClaimDueJobs atomically claims up to limit queued jobs whose deadline is
	// at/before now, for the worker pool (§8). Leases each (deadline → now+lease)
	// so two workers/replicas never run one job twice — SQLite guarded UPDATE,
	// Postgres FOR UPDATE SKIP LOCKED (§18).
	ClaimDueJobs(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Job, error)
	// FindJobByIntentHash returns a recent job with the same intent hash (§8
	// proposal cache), or ErrNotFound. `since` bounds the cache TTL.
	FindJobByIntentHash(ctx context.Context, hash string, since time.Time) (Job, error)

	CreateProposal(ctx context.Context, p Proposal) error
	GetProposal(ctx context.Context, id string) (Proposal, error)
	UpdateProposal(ctx context.Context, p Proposal) error
	ListProposalsByStatus(ctx context.Context, status string) ([]Proposal, error)
	ListProposalsByCreator(ctx context.Context, userID string) ([]Proposal, error)

	// --- users & sessions (§11) ---
	GetUser(ctx context.Context, id string) (User, error)
	// GetUserByName resolves a username to its allowlist row (§11 local login).
	GetUserByName(ctx context.Context, name string) (User, error)
	UpsertUser(ctx context.Context, u User) error
	ListUsers(ctx context.Context) ([]User, error)
	CountAdmins(ctx context.Context) (int, error)
	CreateSession(ctx context.Context, sess Session) error
	GetSession(ctx context.Context, tokenHash string, now time.Time) (Session, error)
	ListSessionsForUser(ctx context.Context, userID string, now time.Time) ([]Session, error)
	TouchSession(ctx context.Context, tokenHash string, expiresAt time.Time) error
	RevokeSession(ctx context.Context, tokenHash string) error
	RevokeSessionsForUser(ctx context.Context, userID string) error
	PurgeExpiredSessions(ctx context.Context, now time.Time) (int, error)

	// --- filler clips (§10) ---
	UpsertClip(ctx context.Context, c Clip) error
	GetClip(ctx context.Context, libraryItemID string) (Clip, error)
	// ListClips returns clips matching the filter (any zero-value field is a
	// wildcard). Used by /v1/filler and by pod assembly's catalog load.
	ListClips(ctx context.Context, filter ClipFilter) ([]Clip, error)
	// UpdateClipTags edits a clip's era/audience/category (+ ai flag) — the tag
	// editor (§10) and the AI-tagging job. Returns ErrNotFound if absent.
	UpdateClipTags(ctx context.Context, libraryItemID string, era int, audience, category string, aiTagged bool, updatedAt time.Time) error
	// DeleteClipsNotIn removes clips whose id isn't in the given set — the sync's
	// prune step (a clip removed from the media server's filler library is gone).
	DeleteClipsNotIn(ctx context.Context, keepIDs []string) (int, error)
	// ListUntaggedCommercials returns commercials missing match tags — the AI
	// tagging job's work list (§10). Sugar over ListClips(UntaggedOnly).
	ListUntaggedCommercials(ctx context.Context) ([]Clip, error)

	// --- settings KV (§5): instance id, per-app webhook last-received, etc. ---
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
	// ListSettings returns every persisted override with its audit metadata
	// (config-design §3). The settings service loads this into its snapshot; the
	// API surfaces updatedBy/updatedAt per field.
	ListSettings(ctx context.Context) ([]SettingRow, error)
	// UpsertSetting writes an override, stamping updated_at (epoch) and updated_by
	// (the admin who changed it; empty ⇒ NULL for env/migration/system writes).
	// This is the audited write path; SetSetting stays the un-audited system path
	// (instance id, webhook timestamps, the §8.1 model selection).
	UpsertSetting(ctx context.Context, row SettingRow) error
	// DeleteSetting removes an override so the key reverts to env/default
	// (config-design §9: an empty PATCH on an optional key clears it).
	DeleteSetting(ctx context.Context, key string) error

	// Close releases the underlying database handle.
	Close() error
}
