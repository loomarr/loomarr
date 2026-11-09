package api

import (
	"net/http"

	"github.com/loomarr/loomarr/internal/api/httpcore"
)

// writeProblem sends an RFC 7807 problem+json body from a plain mux handler, logging through the
// server's logger (httpcore.WriteProblem).
func (s *Server) writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	httpcore.WriteProblem(s.log, w, r, status, title, detail)
}
