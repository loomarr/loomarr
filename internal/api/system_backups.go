package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"github.com/danielgtaylor/huma/v2"
)

// /v1/system/backups* — the backups already on disk (§16, V12).
//
// Distinct from `GET /v1/backup`, which GENERATES a fresh snapshot and keeps nothing.
// These two endpoints are about the files the scheduled `backup` job (§18.1) left in
// `backup.dir`: what is there, and fetching one of them.
//
// ⚠ The download takes a CLIENT-SUPPLIED filename. It is validated against the same
// `loomarr-<timestamp>.db` pattern the writer produces (`store.IsBackupName`) and then
// resolved inside the configured directory — never joined and hoped for. A path segment
// from a request is the classic traversal vector, and "../../etc/passwd" reaching
// os.Open is the difference between an admin-only download and an admin-only arbitrary
// file read.

// BackupsService backs /v1/system/backups*. Implemented in the composition root over the
// store's backup helpers plus the settings registry; the API layer speaks plain view
// structs so it stays decoupled from store types (same shape as DatabaseService).
type BackupsService interface {
	// List returns the backups on disk, newest first, plus the policy in force.
	List(ctx context.Context) (BackupList, error)
	// Open resolves one backup by name for download. Returns ErrBackupNotFound if the
	// name is not ours or not present.
	Open(ctx context.Context, name string) (BackupContent, error)
	// Run writes one backup now and prunes to the retention policy — the same work the
	// scheduled job does, on demand.
	Run(ctx context.Context) (BackupEntry, error)
}

// BackupEntry is one backup file on disk.
type BackupEntry struct {
	Name      string `json:"name" doc:"Filename, e.g. loomarr-2026-07-29-033000.db"`
	Bytes     int64  `json:"bytes" doc:"Size on disk"`
	WrittenAt int64  `json:"writtenAt" doc:"Unix seconds"`
}

// BackupList is the Backup page's whole view of the world.
type BackupList struct {
	// Supported is false on Postgres, where in-app backup is deliberately not offered
	// (§16). The UI needs this to EXPLAIN the empty table rather than render one — an
	// empty list alone reads as "backups are broken" on the one install where the
	// operator is correctly using pg_dump.
	Supported bool          `json:"supported" doc:"Whether in-app backup is available on this backend"`
	Dir       string        `json:"dir" doc:"Where backups are written (backup.dir)"`
	Schedule  string        `json:"schedule" doc:"Cron for the scheduled backup (backup.schedule)"`
	Retain    int           `json:"retain" doc:"How many backups are kept; 0 keeps everything"`
	Backups   []BackupEntry `json:"backups" doc:"On disk, newest first"`
}

// BackupContent is one backup opened for download.
type BackupContent struct {
	Name  string
	Path  string
	Bytes int64
}

// registerSystemBackups mounts the typed half of /v1/system/backups (§16). The download
// is a plain mux handler (it streams a file) and is mounted in Router alongside
// /v1/backup. Admin-only: a backup carries every secret the instance holds.
func (s *Server) registerSystemBackups(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "system-backups-list", Method: http.MethodGet, Path: "/v1/system/backups",
		Summary: "Backups on disk, newest first",
		Description: "Admin only. What the scheduled backup job has written into the configured " +
			"directory, plus the schedule and retention in force. A directory that does not exist " +
			"yet is an empty list, not an error — nothing written yet is the normal state of a " +
			"fresh install.",
		Tags: []string{"system"},
	}, s.systemBackupsList)

	huma.Register(api, huma.Operation{
		OperationID: "system-backups-run", Method: http.MethodPost, Path: "/v1/system/backups",
		Summary: "Write a backup now",
		Description: "Admin only. Writes one snapshot into the configured directory and prunes to " +
			"the retention policy — the same work the scheduled job does, without waiting for it.",
		Tags: []string{"system"},
	}, s.systemBackupsRun)
}

type systemBackupsListOutput struct {
	Body BackupList
}

func (s *Server) systemBackupsList(ctx context.Context, _ *struct{}) (*systemBackupsListOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.backups == nil {
		return nil, huma.Error501NotImplemented("in-app backup is not available on this build")
	}
	list, err := s.backups.List(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("list backups", err)
	}
	// A nil slice serializes as `null`, which a client has to special-case before it can
	// map over it. An empty list is the honest and useful shape here.
	if list.Backups == nil {
		list.Backups = []BackupEntry{}
	}
	return &systemBackupsListOutput{Body: list}, nil
}

type systemBackupsRunOutput struct {
	Body BackupEntry
}

func (s *Server) systemBackupsRun(ctx context.Context, _ *struct{}) (*systemBackupsRunOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.backups == nil {
		return nil, huma.Error501NotImplemented("in-app backup is not available on this build")
	}
	entry, err := s.backups.Run(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("write backup", err)
	}
	return &systemBackupsRunOutput{Body: entry}, nil
}

// downloadBackupHandler serves GET /v1/system/backups/{name} — an already-written file,
// as opposed to /v1/backup which makes a new one. A plain mux handler because it streams
// bytes rather than returning a typed body (§16).
func (s *Server) downloadBackupHandler(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil || s.auth.Authorize(r) != RoleAdmin {
		s.writeProblem(w, r, http.StatusForbidden, "Not allowed", "This action needs an admin account.")
		return
	}
	if s.backups == nil {
		s.writeProblem(w, r, http.StatusNotImplemented, "Backup unavailable",
			"In-app backup is available on SQLite installs. On Postgres, back up the database with your usual Postgres tools.")
		return
	}
	name := r.PathValue("name")
	content, err := s.backups.Open(r.Context(), name)
	if err != nil {
		// Deliberately one answer for "not a backup name" and "no such backup": both
		// mean the caller cannot have this file, and distinguishing them would confirm
		// which paths exist.
		s.writeProblem(w, r, http.StatusNotFound, "Backup not found",
			"That backup isn't on disk. It may have been pruned by the retention policy.")
		return
	}
	// content.Path came from the service, which resolved it inside backup.dir after
	// validating the name — filepath.Clean here is belt-and-braces for the linter's
	// benefit, not the security boundary.
	f, err := os.Open(filepath.Clean(content.Path))
	if err != nil {
		s.log.Error("open backup", "err", err)
		s.writeProblem(w, r, http.StatusNotFound, "Backup not found",
			"That backup isn't on disk. It may have been pruned by the retention policy.")
		return
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		s.log.Error("stat backup", "err", err)
		http.Error(w, "backup unreadable", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+content.Name+`"`)
	// ServeContent over a bare io.Copy: it sets Content-Length and honours Range, so a
	// large backup download can resume rather than restarting from zero.
	http.ServeContent(w, r, content.Name, info.ModTime(), f)
}
