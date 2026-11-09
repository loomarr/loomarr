package system

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/playout"
)

// BackupStreamer streams a consistent DB snapshot (§16). Implemented by the
// SQLite backend via VACUUM INTO. Matches store.SQLiteBackuper's return type.
type BackupStreamer interface {
	StreamBackup(ctx context.Context, w io.Writer) error
}

// InteractiveOperations reads a recorded long operation, such as a model pull.
type InteractiveOperations interface {
	GetInteractiveOperation(ctx context.Context, id string) (store.InteractiveOperation, error)
}

// SessionObserver counts the live encoders the restart page warns about.
type SessionObserver interface {
	// Stats snapshots every live encoder.
	Stats(now time.Time) []playout.SessionStat
}

// Deps is what the system routes need from the rest of the server. Every field is optional: a nil
// service leaves its routes answering 501 or unmounted, as the API's nil-semantics describe.
type Deps struct {
	Log *slog.Logger
	// StartedAt is when this handler build began, so a restart resets the uptime About reports.
	StartedAt time.Time
	// Ready reports readiness for GET /v1/system/version; nil reads as ready.
	Ready func() (ready bool, detail string)
	// StoreFacts reports the schema version and backend name, or is nil on a store-less boot.
	StoreFacts func() (schemaVersion int64, backend string)
	// Operations backs the model-pull operation route.
	Operations InteractiveOperations
	// BackupSQLite streams the SQLite snapshot; nil on Postgres.
	BackupSQLite BackupStreamer

	Backups    BackupsService
	Database   DatabaseService
	Encryption EncryptionService
	Jobs       JobService
	SystemLLM  SystemLLMService
	Restart    RestartService
	// RestartDrift lists the settings that changed since boot and need a restart to apply.
	RestartDrift func() []string
	// PlayoutObserver lets the restart page count streaming channels.
	PlayoutObserver SessionObserver
	// ConnectionChecks runs the connection probes the restart page shows.
	ConnectionChecks func(ctx context.Context) []httpcore.SetupCheck
}

// Handlers serves the system routes.
type Handlers struct {
	log                *slog.Logger
	startedAt          time.Time
	ready              func() (bool, string)
	storeFacts         func() (int64, string)
	operations         InteractiveOperations
	backupSQLite       BackupStreamer
	backups            BackupsService
	database           DatabaseService
	encryption         EncryptionService
	jobs               JobService
	systemLLM          SystemLLMService
	restart            RestartService
	restartDrift       func() []string
	playoutObserver    SessionObserver
	runConnectionCheck func(ctx context.Context) []httpcore.SetupCheck

	// schemaOnly is set only by the OpenAPI exporter: it mounts every route whatever is configured,
	// so the generated spec lists them all.
	schemaOnly bool
}

// New returns the system handlers over d. schemaOnly mounts every route regardless of which
// services d carries; only the OpenAPI exporter sets it.
func New(d Deps, schemaOnly bool) *Handlers {
	return &Handlers{
		log: d.Log, startedAt: d.StartedAt, ready: d.Ready, storeFacts: d.StoreFacts,
		operations: d.Operations, backupSQLite: d.BackupSQLite,
		backups: d.Backups, database: d.Database, encryption: d.Encryption, jobs: d.Jobs,
		systemLLM: d.SystemLLM, restart: d.Restart, restartDrift: d.RestartDrift,
		playoutObserver: d.PlayoutObserver, runConnectionCheck: d.ConnectionChecks,
		schemaOnly: schemaOnly,
	}
}

// Register mounts every system route on api.
func (s *Handlers) Register(api huma.API) {
	s.registerHelp(api)
	s.registerJobs(api)
	s.registerSystemLLM(api)
	s.registerSystemDatabase(api)
	s.registerSystemSecurity(api)
	s.registerSystemBackups(api)
	s.registerSystemRestart(api)
}

func (s *Handlers) runConnectionChecks(ctx context.Context) []httpcore.SetupCheck {
	if s.runConnectionCheck == nil {
		return []httpcore.SetupCheck{}
	}
	return s.runConnectionCheck(ctx)
}

func (s *Handlers) writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	httpcore.WriteProblem(s.log, w, r, status, title, detail)
}
