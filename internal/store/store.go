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

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/provision"
)

// ErrNotFound is returned by Get* methods when no row matches.
var ErrNotFound = errors.New("store: not found")

// TitleStore is the provisioning surface (§3–§4).
type TitleStore interface {
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
}

// ChannelStore is the scheduler/channel surface (§9), including channel icons.
type ChannelStore interface {
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
}

// SeriesEpisodeStore is the cached series episode lists (§5, §9 series expansion).
//
// A materialized answer, not a second source of truth: the media server still owns what
// episodes exist. It exists because enumerating a show is one library call, and doing it
// per series on every guide request was ~90% of that endpoint's latency.
type SeriesEpisodeStore interface {
	// GetSeriesEpisodes returns ErrNotFound for a show never enumerated — deliberately
	// distinct from a cached EMPTY list, which is a real answer ("no episodes present yet").
	GetSeriesEpisodes(ctx context.Context, libraryID string) (SeriesEpisodes, error)
	UpsertSeriesEpisodes(ctx context.Context, se SeriesEpisodes) error
	// ListStaleSeriesEpisodes returns shows fetched before `before`, oldest first, for the
	// series-episode-refresh job (§18.1).
	ListStaleSeriesEpisodes(ctx context.Context, before time.Time, limit int) ([]SeriesEpisodes, error)
}

// JobStore is the suggester job queue (§8).
type JobStore interface {
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
	// PurgeFinishedJobs removes done/failed jobs older than `before` (§5 JOBS_RETENTION).
	// In-flight jobs (queued/running) are never removed by age.
	PurgeFinishedJobs(ctx context.Context, before time.Time) (int, error)
}

// ProposalStore is the suggester proposal surface (§8).
type ProposalStore interface {
	CreateProposal(ctx context.Context, p Proposal) error
	GetProposal(ctx context.Context, id string) (Proposal, error)
	UpdateProposal(ctx context.Context, p Proposal) error
	ListProposalsByStatus(ctx context.Context, status string) ([]Proposal, error)
	ListProposalsByCreator(ctx context.Context, userID string) ([]Proposal, error)
	// PurgeDeniedProposals removes denied proposals older than `before` (§5
	// PROPOSALS_RETENTION). Approved proposals are the audit trail and are kept
	// indefinitely; submitted ones are still awaiting an answer.
	PurgeDeniedProposals(ctx context.Context, before time.Time) (int, error)
}

// ScheduledJobStore is the background-job scheduler's state (§18.1).
type ScheduledJobStore interface {
	// UpsertScheduledJob writes a job's runtime state (last-run/result + next-run lease).
	// ⚠ Never writes `paused` — see SetScheduledJobPaused.
	UpsertScheduledJob(ctx context.Context, j ScheduledJob) error
	// SetScheduledJobPaused pauses or resumes a job (§18.1). A paused job is never claimed by
	// ClaimDueScheduledJobs, so it simply does not run until resumed. Creates the row if the
	// job has not run yet, so a task can be paused before its first execution.
	SetScheduledJobPaused(ctx context.Context, name string, paused bool) error
	// GetScheduledJob returns one job's state, or ErrNotFound.
	GetScheduledJob(ctx context.Context, name string) (ScheduledJob, error)
	// ListScheduledJobs returns all job state rows for the Tasks page.
	ListScheduledJobs(ctx context.Context) ([]ScheduledJob, error)
	// ClaimDueScheduledJobs leases every job whose next_run is due (advancing next_run to
	// now+lease) so only one replica runs it per tick — same SKIP LOCKED idiom as titles.
	ClaimDueScheduledJobs(ctx context.Context, now time.Time, lease time.Duration) ([]ScheduledJob, error)
}

// UserStore is the users & sessions surface (§11).
type UserStore interface {
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
}

// ClipStore is the filler clip catalog (§10).
type ClipStore interface {
	UpsertClip(ctx context.Context, c Clip) error
	GetClip(ctx context.Context, libraryItemID string) (Clip, error)
	// ListClips returns clips matching the filter (any zero-value field is a
	// wildcard). Used by /v1/filler and by pod assembly's catalog load.
	ListClips(ctx context.Context, filter ClipFilter) ([]Clip, error)
	// UpdateClipTags edits a clip's era/audience/category (+ ai flag) — the tag
	// editor (§10) and the AI-tagging job. suggestedEra records an UNGROUNDED
	// AI-proposed era (§10 V34) for operator confirmation; writing an era clears
	// it in the same write, and a write with neither leaves it alone. Returns
	// ErrNotFound if absent.
	UpdateClipTags(ctx context.Context, libraryItemID string, era int, audience, category string, suggestedEra int, aiTagged bool, updatedAt time.Time) error
	// UpdateClipKind corrects a clip's kind (§10). Separate from UpdateClipTags because
	// the AI tagging job never sets kind — it classifies era/audience/category from text
	// signals, while kind is detected at sync and only a human corrects it (a trailer
	// scanned as a commercial being the likely case).
	UpdateClipKind(ctx context.Context, tunarrProgramID, kind string, updatedAt time.Time) error
	// DeleteClipsNotIn removes clips whose id isn't in the given set — the sync's
	// prune step (a clip removed from the media server's filler library is gone).
	DeleteClipsNotIn(ctx context.Context, keepIDs []string) (int, error)
	// DeleteClip removes ONE clip by identity — the split-confirm path drops the
	// compilation row it just cut into segments (§10 V34). ErrNotFound if absent.
	DeleteClip(ctx context.Context, libraryItemID string) error
	// ListUntaggedCommercials returns commercials missing match tags — the AI
	// tagging job's work list (§10). Sugar over ListClips(UntaggedOnly).
	ListUntaggedCommercials(ctx context.Context) ([]Clip, error)
}

// SplitProposalStore is the persisted split-proposal surface (§10, V34) —
// detector-authored, reviewer-edited cut lists that are NOT clips until
// confirmed. One proposal per compilation clip (re-detection replaces).
type SplitProposalStore interface {
	UpsertSplitProposal(ctx context.Context, p filler.SplitProposal) error
	// GetSplitProposal reads one proposal by id (the review's reconnect truth).
	GetSplitProposal(ctx context.Context, id string) (filler.SplitProposal, error)
	// DeleteSplitProposal removes a proposal after confirm or on reject.
	DeleteSplitProposal(ctx context.Context, id string) error
}

// FillerSourceStore is the persisted REMOTE filler-source registry (§10, V33).
//
// ⚠ Remote sources only. The drop-folder and the media-server library stay DERIVED from config
// (see `GET /v1/filler/sources`, V28) — they answer "you could set one up but have not", which
// rows cannot express. These describe the specific archive.org collections an operator added, and
// they nest under that read-model's `remote` row rather than replacing any of it.
type FillerSourceStore interface {
	// ListFillerSources returns every registered remote source, oldest first, so the UI order
	// is stable across reloads.
	ListFillerSources(ctx context.Context) ([]FillerSource, error)
	// UpsertFillerSource adds or updates one source by id.
	UpsertFillerSource(ctx context.Context, src FillerSource) error
	// DeleteFillerSource removes a source. Clips it already brought in are NOT deleted:
	// they are files in the drop-folder, and forgetting where something came from is not a
	// reason to throw it away.
	DeleteFillerSource(ctx context.Context, id string) error
	// MarkFillerSourceFetched stamps a successful fetch, for the Sources tab's "last fetched".
	MarkFillerSourceFetched(ctx context.Context, id string, at time.Time) error
	// SetFillerSourceEnabled switches a source on or off (V35). ⚠ Disabling is NOT deleting:
	// the row keeps its licence and fetch history, and clips it already brought in stay in the
	// catalog. It only withdraws the source from future searching and downloading.
	SetFillerSourceEnabled(ctx context.Context, id string, enabled bool) error
}

// AiringStore records what actually went to air — written from playout only.
type AiringStore interface {
	// RecordClipPlay counts a filler clip having AIRED (V28). Written from playout only;
	// a missing clip is not an error (the catalog may have pruned it mid-schedule).
	RecordClipPlay(ctx context.Context, libraryItemID string, at time.Time) error
	// RecordAiring stamps that a PROGRAMME aired on a channel (§5, programming-design §3.1) —
	// the programme analogue of RecordClipPlay. Written from playout only, when a programme is
	// actually resolved for streaming; upserts one row per (channel, key) holding the LAST
	// airing, because the only question asked of it is "when did this last air here?".
	RecordAiring(ctx context.Context, channelID string, key provision.Key, libraryItemID string, at time.Time) error
	// LastAiredByChannel returns the most recent airing per key on one channel, for
	// recency-aware placement (programming-design §3.1). A key that has never aired is simply
	// absent — callers treat absence as "least recently aired", which sorts it first.
	LastAiredByChannel(ctx context.Context, channelID string) (map[provision.Key]time.Time, error)
}

// ActivityStore is the Dashboard feed (§5, §12, V32).
type ActivityStore interface {
	// RecordActivity appends one Dashboard feed row. Best-effort by contract: callers log
	// the error and carry on — recording that something happened must never be able to
	// stop it happening.
	RecordActivity(ctx context.Context, a Activity) error
	// ListActivity returns the newest feed rows first, capped at limit.
	ListActivity(ctx context.Context, limit int) ([]Activity, error)
	// PurgeActivity deletes feed rows older than `before` (§18.1 activity-purge). The feed
	// is the one append-only table here, so it is the one that needs a purge.
	PurgeActivity(ctx context.Context, before time.Time) (int, error)
}

// SettingStore is the settings KV (§5): instance id, per-app webhook last-received, etc.
type SettingStore interface {
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
	// SetSettingEnvOverride claims a key for the app or hands it back to the
	// environment (config-design §3.1). Distinct from UpsertSetting because it
	// writes AUTHORITY, not a value: a plain save must never disturb the flag.
	// `seed` is the value to store when the row does not exist yet (the env value
	// being taken over, so unlocking does not blank the setting; empty for secrets,
	// which never seed). Existing rows keep their stored value.
	SetSettingEnvOverride(ctx context.Context, key string, on bool, seed, by string) error
}

// CountStore is the §17 /metrics state gauges. Read on scrape by the metrics
// collector, never on the write path.
type CountStore interface {
	// CountTitlesByState returns the record count per provisioning state; a
	// state with no rows is omitted (the collector zero-fills the known set).
	CountTitlesByState(ctx context.Context) (map[provision.State]int, error)
	// CountJobsByStatus returns the suggester-job count per status
	// (queued/running/done/failed) — the queue-depth gauge.
	CountJobsByStatus(ctx context.Context) (map[string]int, error)
	// CountActiveSessions returns the number of unexpired sessions as of now.
	CountActiveSessions(ctx context.Context, now time.Time) (int, error)
}

// Store is the full persistence surface (§5) — the union of the per-domain
// interfaces above, which is what the composition root and the conformance suite
// hold. Callers that need one domain should depend on that domain's interface
// instead: the narrow role interfaces domain packages already declare for
// themselves (binder.Store, filler.Store, scheduler.ScheduleStore, …) are the
// pattern, and these groups exist so a consumer has something to name without
// re-declaring one.
//
// ⚠ The grouping is by DOMAIN, not by caller. A new method belongs in the group
// that owns its table; a group existing to serve one consumer would drift back
// into the 68-method union this replaced the moment a second consumer appeared.
type Store interface {
	TitleStore
	ChannelStore
	SeriesEpisodeStore
	JobStore
	ProposalStore
	ScheduledJobStore
	UserStore
	ClipStore
	FillerSourceStore
	SplitProposalStore
	AiringStore
	ActivityStore
	SettingStore
	CountStore

	// Close releases the underlying database handle.
	Close() error
}
