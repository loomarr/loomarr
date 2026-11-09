package api

import (
	"context"
	"net/http"

	"github.com/loomarr/loomarr/internal/api/httpcore"
)

// checkpoint reads the backend checkpoint every channel route judges playability by.
func (s *Server) checkpoint(ctx context.Context) (httpcore.BackendCheckpoint, error) {
	return httpcore.Backend{Source: s.backendCheckpoint, Settings: s.liveSettings}.Checkpoint(ctx)
}

// writeProblem sends an RFC 7807 problem+json body from a plain mux handler, logging through the
// server's logger (httpcore.WriteProblem).
func (s *Server) writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	httpcore.WriteProblem(s.log, w, r, status, title, detail)
}
