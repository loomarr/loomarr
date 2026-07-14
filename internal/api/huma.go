package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/store"
)

// Server holds the API dependencies and builds the Huma API on a stdlib mux
// (§7.1, §14: code-first OpenAPI 3.1 via humago — no third-party router).
type Server struct {
	store store.Store
	auth  Authorizer
	log   *slog.Logger
	// backupSQLite is set when the store is the SQLite backend (GET /v1/backup —
	// §16). Nil for Postgres, which returns 501 + a pg_dump docs pointer.
	backupSQLite BackupStreamer
	// login/sessions wire /v1/auth/* (§11); nil until Phase 9 is configured.
	login        LoginService
	sessions     SessionManager
	userSync     UserSyncer
	cookieSecure string // COOKIE_SECURE: auto|true|false (§11)
	// channels/livetv wire /v1/channels* and /v1/setup/* (§7/§9); nil until
	// Phase 10 is configured.
	channels ChannelService
	livetv   LiveTVService
	// suggest/search wire /v1/suggestions*, /v1/search (§7.2/§8); nil until
	// Phase 11 is configured.
	suggest SuggestService
	search  SearchService
	events  EventSource   // /v1/events SSE (Phase 11); nil ⇒ route 501
	filler  FillerService // /v1/filler* (Phase 12); nil ⇒ sync/tag routes 501
	// systemLLM wires /v1/system/llm* (§8.1 model selection); nil ⇒ routes 501.
	systemLLM SystemLLMService
}

// FillerService backs the filler ingestion routes (§10): catalog sync and the AI
// tagging job. Implemented by filler.Syncer + filler.Tagger. list/patch read/write
// the store directly (no service needed).
type FillerService interface {
	// Sync reconciles the clip catalog from the media server's filler library.
	Sync(ctx context.Context) (total, added, updated, pruned int, err error)
	// Tag runs AI classification over untagged commercials.
	Tag(ctx context.Context) (considered, tagged, partial, skipped int, err error)
}

// SuggestService is the suggestion surface the API depends on (§8). Implemented
// by suggest.Service + the store; abstracted so the API doesn't couple to the
// worker internals.
type SuggestService interface {
	// Submit enqueues a suggestion job for an intent and returns the job id.
	Submit(ctx context.Context, description, era, tone string, mustInclude, mustExclude []string, maxAcquire int, createdBy string) (jobID string, err error)
}

// SearchService backs GET /v1/search (§7.2) — the SAME catalog impl as the LLM
// grounding tool. Returns candidates as generic maps so the API layer needn't
// import the catalog types (kept dependency-light).
type SearchService interface {
	Search(ctx context.Context, query, scope string, limit int) ([]SearchCandidate, error)
}

// SearchCandidate is the API view of a catalog candidate (§7.2).
type SearchCandidate struct {
	MediaType     string `json:"mediaType"`
	TMDBID        int    `json:"tmdbId,omitempty"`
	TVDBID        int    `json:"tvdbId,omitempty"`
	Name          string `json:"name"`
	Year          int    `json:"year,omitempty"`
	InLibrary     bool   `json:"inLibrary"`
	LibraryItemID string `json:"libraryItemId,omitempty"`
}

// ChannelService is the channel-management surface the API depends on (§9).
// Implemented by channels.Engine + the store; abstracted so the API doesn't
// couple to the reconcile internals.
type ChannelService interface {
	// Reconcile forces a desired→Tunarr reconciliation for one channel (§9,
	// POST /v1/channels/{id}/reconcile).
	Reconcile(ctx context.Context, channelID string) error
}

// LiveTVService backs the Live TV setup routes (§6/§7): idempotent connect and
// the "wired?" status check. Implemented by setup.LiveTVConnector.
type LiveTVService interface {
	Connect(ctx context.Context) (tunerAdded, listingAdded bool, err error)
	Wired(ctx context.Context) (bool, error)
}

// UserSyncer imports users from the media server (§11). Returns the count synced.
type UserSyncer interface {
	Sync(ctx context.Context) (int, error)
}

// LoginService verifies credentials and issues a session (Phase 9, §11).
type LoginService interface {
	Login(ctx context.Context, username, password, rateKey string) (token string, expires time.Time, u store.User, err error)
	Disable(ctx context.Context, userID string) error
}

// SessionManager revokes sessions (logout) (§11).
type SessionManager interface {
	Revoke(ctx context.Context, token string) error
}

// BackupStreamer streams a consistent DB snapshot (§16). Implemented by the
// SQLite backend via VACUUM INTO. Matches store.SQLiteBackuper's return type.
type BackupStreamer interface {
	StreamBackup(ctx context.Context, w io.Writer) error
}

// Options configures the API server.
type Options struct {
	Store        store.Store
	Auth         Authorizer
	Log          *slog.Logger
	BackupSQLite BackupStreamer // nil ⇒ /v1/backup returns 501 (Postgres)
	Ingest       http.Handler   // POST /hooks/arr (Phase 6); mounted outside Huma
	Ready        ReadyFunc
	Login        LoginService     // /v1/auth/login + user disable (Phase 9); nil ⇒ routes absent
	Sessions     SessionManager   // /v1/auth/logout (Phase 9)
	UserSync     UserSyncer       // POST /v1/users/sync (Phase 9); nil ⇒ route absent
	CookieSecure string           // COOKIE_SECURE: auto|true|false (§11)
	Channels     ChannelService   // /v1/channels* reconcile (Phase 10); nil ⇒ reconcile route absent
	LiveTV       LiveTVService    // /v1/setup/* (Phase 10); nil ⇒ setup routes absent
	Suggest      SuggestService   // /v1/suggestions submit (Phase 11); nil ⇒ submit route 501
	Search       SearchService    // /v1/search (Phase 11); nil ⇒ search route 501
	Events       EventSource      // /v1/events SSE (Phase 11); nil ⇒ route 501
	Filler       FillerService    // /v1/filler sync/tag (Phase 12); nil ⇒ those routes 501
	SystemLLM    SystemLLMService // /v1/system/llm* model selection (§8.1); nil ⇒ routes 501
}

// humaConfig builds the OpenAPI 3.1 config with our metadata (§7.1).
func humaConfig() huma.Config {
	cfg := huma.DefaultConfig("Loomarr API", "0.1.0")
	cfg.Info.Description = "Turn a sentence into a self-maintaining Tunarr channel. " +
		"Every /v1 route requires a session cookie or Authorization: Bearer API_TOKEN (§7)."
	// Serve our own docs assets offline; Huma's default loads Stoplight from a
	// CDN which violates the offline rule (§7.1). We disable the built-in docs
	// path and mount our own at /docs (docs.go).
	cfg.DocsPath = ""
	return cfg
}
