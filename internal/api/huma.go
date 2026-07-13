package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"

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
