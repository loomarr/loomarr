package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// /v1/system/database* — the SQLite → PostgreSQL migration stepper (§18, V11).
//
// The operator-facing flow is connect → preflight → backup → migrate-and-restart.
// Migrate only asks the process owner to begin the atomic operation; it returns before
// the current generation drains. The process then closes the live store, copies and
// verifies the data, writes the bootstrap target, and starts a fresh generation.
//
// ⚠ **The backup gate is enforced HERE, on the server.** The mock disables the Migrate
// button until a backup exists, but a disabled button is a hint, not a gate — anything
// that can be satisfied by the client can be skipped by the client. `Migrate` refuses
// unless the service can see a backup written for THIS migration, which is why
// `WriteBackup` had to write a real file instead of streaming one to the browser.

// Sentinels the DatabaseService returns; handlers map them to HTTP status.
var (
	// ErrNoBackup: Migrate was called before a backup was taken (→ 409).
	ErrNoBackup = errors.New("a backup is required before migrating")
	// ErrPreflightFailed: Migrate was called with a target that has not passed
	// preflight (→ 409).
	ErrPreflightFailed = errors.New("target has not passed preflight")
	// ErrNotSQLite: the install is already on Postgres, so there is nothing to
	// migrate (→ 409).
	ErrNotSQLite = errors.New("this install is not on SQLite")
	// ErrMigrationRunning: a migration is already in flight (→ 409).
	ErrMigrationRunning = errors.New("a migration is already running")
	// ErrDatabaseURLPinned: an environment pin would override the migrated target on
	// restart, so the atomic operation is refused before it is queued (→ 409).
	ErrDatabaseURLPinned = errors.New("DATABASE_URL is pinned by the environment")
	// ErrMigrationUnavailable: this handler has no process-level migration callback
	// behind it, so accepting the request would be a lie (→ 501).
	ErrMigrationUnavailable = errors.New("atomic database migration is unavailable")
	// ErrMigrationNotVerified: the compatibility switchover route was called without
	// an exact, verified in-memory migration result (→ 409).
	ErrMigrationNotVerified = errors.New("the target has not been verified for switchover")
)

// DatabaseService backs /v1/system/database*. Implemented in the composition root over
// the store's migration engine plus the settings/bootstrap writers; the API layer speaks
// plain view structs so it stays decoupled from store types (same shape as
// SystemLLMService).
type DatabaseService interface {
	// Status reports the active backend and where a migration (if any) has got to.
	Status(ctx context.Context) (DatabaseStatus, error)
	// Preflight probes a candidate Postgres target. Checks that RAN and failed come
	// back in the result with ok:false — an error means the probe could not run.
	Preflight(ctx context.Context, dsn string) ([]DatabaseCheck, error)
	// Backup writes a server-side snapshot and returns what it wrote. This is the
	// gate Migrate enforces, not a convenience.
	Backup(ctx context.Context) (DatabaseBackup, error)
	// Migrate validates the server-owned preconditions and queues one atomic
	// migrate-and-restart request. It does not perform the copy on the HTTP request.
	Migrate(ctx context.Context, dsn string) error
	// Switchover is retained for wire compatibility. The atomic Migrate path does not
	// need it; implementations must fail closed unless this exact target is verified.
	Switchover(ctx context.Context, dsn string) error
}

// DatabaseCheck is one preflight result, rendered verbatim under its name.
type DatabaseCheck struct {
	Name   string `json:"name" doc:"Check name, e.g. Target is empty"`
	Detail string `json:"detail" doc:"Operator-facing detail — shown under the name"`
	OK     bool   `json:"ok" doc:"Whether this check passed"`
}

// DatabaseTable is one table's row counts — the unit parity is asserted in and the unit
// the stepper renders a bar for.
type DatabaseTable struct {
	Table  string `json:"table" doc:"Table name"`
	Source int64  `json:"source" doc:"Rows in the source database"`
	Copied int64  `json:"copied" doc:"Rows copied into the target so far"`
}

// DatabaseBackup describes a server-written backup file.
type DatabaseBackup struct {
	Path      string `json:"path" doc:"Where the backup was written"`
	Bytes     int64  `json:"bytes" doc:"Size on disk"`
	WrittenAt int64  `json:"writtenAt" doc:"Unix seconds"`
}

// DatabaseStatus is the stepper's whole view of the world.
type DatabaseStatus struct {
	Backend string `json:"backend" doc:"Active backend: sqlite or postgres"`
	// CanMigrate is false once the install is already on Postgres. The UI needs this
	// to explain the absence of the stepper rather than just not rendering it.
	CanMigrate bool            `json:"canMigrate" doc:"Whether a migration is offered at all"`
	Phase      string          `json:"phase" doc:"idle|migrating|verified|failed"`
	Tables     []DatabaseTable `json:"tables" doc:"Per-table progress; empty when idle"`
	// Parity is reported separately from Tables because it is the GATE, and a caller
	// should not have to re-derive it by comparing every row.
	Parity string          `json:"parity" doc:"unknown|match|mismatch"`
	Backup *DatabaseBackup `json:"backup,omitempty" doc:"The backup taken for this migration, if any"`
	Error  string          `json:"error,omitempty" doc:"Why the last migration failed, if it did"`
}

// registerSystemDatabase mounts /v1/system/database* (§18). Admin-only throughout: this
// moves every row the instance owns, and the backup it writes carries every secret.
func (s *Server) registerSystemDatabase(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-database-status", Method: http.MethodGet, Path: "/v1/system/database",
		Summary: "Active backend + migration progress",
		Description: "Admin only. Which backend is live, whether a migration is offered, and " +
			"how far one has got — including per-table row counts and the parity verdict.",
		Tags: []string{"system"},
	}, RoleAdmin), s.databaseStatus)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-database-preflight", Method: http.MethodPost, Path: "/v1/system/database/preflight",
		Summary: "Probe a candidate PostgreSQL target",
		Description: "Admin only. Runs every check against the target and returns them all — a check " +
			"that ran and failed comes back with ok:false rather than as an error, so the operator " +
			"sees which one. Nothing is written to either database.",
		Tags: []string{"system"},
	}, RoleAdmin), s.databasePreflight)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-database-backup", Method: http.MethodPost, Path: "/v1/system/database/backup",
		Summary: "Write the pre-migration backup",
		Description: "Admin only. Writes a server-side snapshot into the configured backup directory. " +
			"This is the gate the migrate call enforces — a backup is required, not suggested, because " +
			"it is the only thing that makes the move reversible.",
		Tags: []string{"system"},
	}, RoleAdmin), s.databaseBackup)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-database-migrate", Method: http.MethodPost, Path: "/v1/system/database/migrate",
		Summary: "Copy every table into the target and verify parity",
		Description: "Admin only. Refuses without a passing preflight and a backup. The source is only " +
			"ever read: on any failure the install is still running on SQLite and nothing was lost. " +
			"Progress streams over /v1/events as `database` frames.",
		Tags: []string{"system"},
	}, RoleAdmin), s.databaseMigrate)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-database-switchover", Method: http.MethodPost, Path: "/v1/system/database/switchover",
		Summary: "Point the next boot at the migrated database",
		Description: "Admin only. Persists the new DATABASE_URL to the bootstrap file. Takes effect on " +
			"restart; the SQLite file is left in place untouched, so reverting is a one-line change.",
		Tags: []string{"system"},
	}, RoleAdmin), s.databaseSwitchover)
}

type databaseStatusOutput struct {
	Body DatabaseStatus
}

func (s *Server) databaseStatus(ctx context.Context, _ *struct{}) (*databaseStatusOutput, error) {
	if s.database == nil {
		return nil, huma.Error501NotImplemented("database migration is not available on this build")
	}
	st, err := s.database.Status(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("database status", err)
	}
	return &databaseStatusOutput{Body: st}, nil
}

type databaseTargetInput struct {
	Body struct {
		DSN string `json:"dsn" required:"true" doc:"postgres:// connection string for the target"`
	}
}

type databasePreflightOutput struct {
	Body struct {
		Checks []DatabaseCheck `json:"checks"`
		Passed bool            `json:"passed" doc:"True only if every check passed"`
	}
}

func (s *Server) databasePreflight(ctx context.Context, in *databaseTargetInput) (*databasePreflightOutput, error) {
	if s.database == nil {
		return nil, huma.Error501NotImplemented("database migration is not available on this build")
	}
	checks, err := s.database.Preflight(ctx, in.Body.DSN)
	if err != nil {
		if errors.Is(err, ErrNotSQLite) {
			return nil, huma.Error409Conflict("this install is not on SQLite")
		}
		return nil, huma.Error422UnprocessableEntity("preflight", err)
	}
	out := &databasePreflightOutput{}
	out.Body.Checks = checks
	out.Body.Passed = len(checks) > 0
	for _, c := range checks {
		if !c.OK {
			out.Body.Passed = false
		}
	}
	return out, nil
}

type databaseBackupOutput struct {
	Body DatabaseBackup
}

func (s *Server) databaseBackup(ctx context.Context, _ *struct{}) (*databaseBackupOutput, error) {
	if s.database == nil {
		return nil, huma.Error501NotImplemented("database migration is not available on this build")
	}
	bk, err := s.database.Backup(ctx)
	if err != nil {
		if errors.Is(err, ErrNotSQLite) {
			return nil, huma.Error409Conflict("this install is not on SQLite — use your PostgreSQL backup tooling")
		}
		return nil, huma.Error500InternalServerError("write backup", err)
	}
	return &databaseBackupOutput{Body: bk}, nil
}

type databaseMigrateOutput struct {
	Body DatabaseStatus
}

func (s *Server) databaseMigrate(ctx context.Context, in *databaseTargetInput) (*databaseMigrateOutput, error) {
	if s.database == nil {
		return nil, huma.Error501NotImplemented("database migration is not available on this build")
	}
	if err := s.database.Migrate(ctx, in.Body.DSN); err != nil {
		switch {
		case errors.Is(err, ErrNoBackup):
			// 409, not 403: the request is well-formed and the caller is allowed — the
			// instance is simply not in a state where it can be honored yet.
			return nil, huma.Error409Conflict("a backup is required before migrating — it is the only thing that makes this reversible")
		case errors.Is(err, ErrPreflightFailed):
			return nil, huma.Error409Conflict("run preflight against the target first")
		case errors.Is(err, ErrNotSQLite):
			return nil, huma.Error409Conflict("this install is not on SQLite")
		case errors.Is(err, ErrMigrationRunning):
			return nil, huma.Error409Conflict("a migration is already running")
		case errors.Is(err, ErrDatabaseURLPinned):
			return nil, huma.Error409Conflict("DATABASE_URL is pinned by the environment — change it where Loomarr is launched, then restart before migrating")
		case errors.Is(err, ErrMigrationUnavailable):
			return nil, huma.Error501NotImplemented("atomic database migration is not available on this build")
		}
		return nil, huma.Error409Conflict("migration request was not accepted; Loomarr is still running on SQLite: " + err.Error())
	}
	st, err := s.database.Status(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("database status", err)
	}
	return &databaseMigrateOutput{Body: st}, nil
}

type databaseSwitchoverOutput struct {
	Body struct {
		RestartRequired bool   `json:"restartRequired" doc:"Always true — DATABASE_URL is a boot-time setting"`
		Note            string `json:"note" doc:"What happens next, in operator-facing prose"`
	}
}

func (s *Server) databaseSwitchover(ctx context.Context, in *databaseTargetInput) (*databaseSwitchoverOutput, error) {
	if s.database == nil {
		return nil, huma.Error501NotImplemented("database migration is not available on this build")
	}
	if err := s.database.Switchover(ctx, in.Body.DSN); err != nil {
		if errors.Is(err, ErrMigrationNotVerified) || errors.Is(err, ErrDatabaseURLPinned) || errors.Is(err, ErrNotSQLite) {
			return nil, huma.Error409Conflict(err.Error())
		}
		return nil, huma.Error500InternalServerError("switchover", err)
	}
	out := &databaseSwitchoverOutput{}
	out.Body.RestartRequired = true
	out.Body.Note = "Loomarr will use the migrated database on its next start. " +
		"Your SQLite file is left in place, untouched, as a fallback."
	return out, nil
}
