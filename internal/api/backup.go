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
		http.Error(w, "admin role required", http.StatusForbidden)
		return
	}
	if s.backupSQLite == nil {
		http.Error(w,
			"backup is SQLite-only; on Postgres use pg_dump against the database directly (docs/design.md §16)",
			http.StatusNotImplemented)
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
