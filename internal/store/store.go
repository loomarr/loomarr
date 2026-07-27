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
	// UpdateTitleProgress writes only the poll-updated download fields (§18.1 arr-queue-poll),
	// leaving state-machine columns untouched so it never races the state Upsert.
	UpdateTitleProgress(ctx context.Context, key provision.Key, progress float64, eta, status string) error
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
	// PutChannelIcon stores (or replaces) a channel's uploaded icon bytes + MIME (the
	// upload icon source). One per channel; updatedAt drives cache-busting on serve.
	PutChannelIcon(ctx context.Context, channelID, contentType string, data []byte, updatedAt time.Time) error
	// GetChannelIcon returns a channel's uploaded icon. ok=false when none is stored (the
	// channel uses a TMDB/URL logo, or has no icon). The serve endpoint 404s on !ok.
	GetChannelIcon(ctx context.Context, channelID string) (contentType string, data []byte, updatedAt time.Time, ok bool, err error)
	// ClaimDueChannels atomically claims up to limit channels whose
	// reconcile_deadline is at/before now, for the periodic reconcile sweep
	// (§9). Like ClaimDueTitles it *leases* each claimed channel (deadline →
	// now+lease) so two replicas never reconcile one Tunarr channel at once
	// (§18: single-leader / per-channel row claiming). SQLite: guarded UPDATE.
	// Postgres: FOR UPDATE SKIP LOCKED. Detached channels are never claimed.
	ClaimDueChannels(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]Channel, error)

	// --- cached series episode lists (§5, §9 series expansion) ---
	//
	// A materialized answer, not a second source of truth: the media server still owns what
	// episodes exist. It exists because enumerating a show is one library call, and doing it
	// per series on every guide request was ~90% of that endpoint's latency.
	//
	// GetSeriesEpisodes returns ErrNotFound for a show never enumerated — deliberately
	// distinct from a cached EMPTY list, which is a real answer ("no episodes present yet").
	GetSeriesEpisodes(ctx context.Context, libraryID string) (SeriesEpisodes, error)
	UpsertSeriesEpisodes(ctx context.Context, se SeriesEpisodes) error
	// ListStaleSeriesEpisodes returns shows fetched before `before`, oldest first, for the
	// series-episode-refresh job (§18.1).
	ListStaleSeriesEpisodes(ctx context.Context, before time.Time, limit int) ([]SeriesEpisodes, error)

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

	// --- scheduled jobs (the background-job scheduler, §18.1) ---
	// UpsertScheduledJob writes a job's runtime state (last-run/result + next-run lease).
	UpsertScheduledJob(ctx context.Context, j ScheduledJob) error
	// GetScheduledJob returns one job's state, or ErrNotFound.
	GetScheduledJob(ctx context.Context, name string) (ScheduledJob, error)
	// ListScheduledJobs returns all job state rows for the Tasks page.
	ListScheduledJobs(ctx context.Context) ([]ScheduledJob, error)
	// ClaimDueScheduledJobs leases every job whose next_run is due (advancing next_run to
	// now+lease) so only one replica runs it per tick — same SKIP LOCKED idiom as titles.
	ClaimDueScheduledJobs(ctx context.Context, now time.Time, lease time.Duration) ([]ScheduledJob, error)

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
	// RecordClipPlay counts a filler clip having AIRED (V28). Written from playout only;
	// a missing clip is not an error (the catalog may have pruned it mid-schedule).
	RecordClipPlay(ctx context.Context, libraryItemID string, at time.Time) error
	// UpdateClipKind corrects a clip's kind (§10). Separate from UpdateClipTags because
	// the AI tagging job never sets kind — it classifies era/audience/category from text
	// signals, while kind is detected at sync and only a human corrects it (a trailer
	// scanned as a commercial being the likely case).
	UpdateClipKind(ctx context.Context, tunarrProgramID, kind string, updatedAt time.Time) error
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

	// --- observability counts (§17 /metrics state gauges) ---
	// Read on scrape by the metrics collector, never on the write path.
	// CountTitlesByState returns the record count per provisioning state; a
	// state with no rows is omitted (the collector zero-fills the known set).
	CountTitlesByState(ctx context.Context) (map[provision.State]int, error)
	// CountJobsByStatus returns the suggester-job count per status
	// (queued/running/done/failed) — the queue-depth gauge.
	CountJobsByStatus(ctx context.Context) (map[string]int, error)
	// CountActiveSessions returns the number of unexpired sessions as of now.
	CountActiveSessions(ctx context.Context, now time.Time) (int, error)

	// Close releases the underlying database handle.
	Close() error
}
