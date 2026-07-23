package api

import (
	"net/http"
)

// backupHandler serves GET /v1/backup as a plain mux handler (not Huma) because
// it streams a binary DB snapshot rather than returning a typed body (§16). It
// is mounted on the mux and documented in the OpenAPI spec via a webhook-free
// note; auth is checked inline. SQLite streams VACUUM INTO; Postgres → 501.
func (s *Server) backupHandler(w http.ResponseWriter, r *http.Request) {
	if s.auth == nil || s.auth.Authorize(r) != RoleAdmin {
		s.writeProblem(w, r, http.StatusForbidden, "Not allowed", "This action needs an admin account.")
		return
	}
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
