package api

import (
	"net/http"
)

// backupHandler serves GET /v1/backup: a binary DB snapshot streamed straight to the client
// rather than a typed body (§16). SQLite streams VACUUM INTO; Postgres → 501.
//
// ⚠ **No authorization check in this body, deliberately.** It is registered as a rawOp
// (registerSystemBackups) declaring RoleAdmin, and the shared middleware refuses before the
// handler runs. The guard that used to live here is the reason rawOp exists: an inline check
// here and another in events.go had already drifted apart on whether a nil authorizer denies
// or allows — one rule, enforced in one place, cannot disagree with itself (§11).
func (s *Server) backupHandler(w http.ResponseWriter, r *http.Request) {
	if s.backupSQLite == nil {
		s.writeProblem(w, r, http.StatusNotImplemented, "Backup unavailable",
			"In-app backup is available on SQLite installs. On Postgres, back up the database with your usual Postgres tools.")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="loomarr-backup.db"`)
	if err := s.backupSQLite.StreamBackup(r.Context(), w); err != nil {
		s.log.Error("backup stream", "err", err)
		// Headers may already be sent; best-effort error.
		http.Error(w, "backup failed", http.StatusInternalServerError)
	}
}
