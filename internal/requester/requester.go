// Package requester is the Requester port (design §2, §6): it asks a downstream
// service to acquire a title. The default adapter is Seerr; a direct
// Sonarr/Radarr adapter is the alternative (§6, §15 footnote). Writes never
// client-retry (§6) — recovery is the reconciler's job (Phase 7).
package requester

import (
	"context"

	"github.com/mantonx/loomarr/internal/provision"
)

// Requester submits acquisition requests for titles (§2 boundary).
type Requester interface {
	// Request asks the downstream to acquire the title. It is idempotent: a
	// title already requested/available is success, not an error (§6). Returns
	// nil on acceptance; a non-nil error means the submission was not accepted
	// and the record stays `wanted` for the reconciler to retry.
	Request(ctx context.Context, t provision.Title) error
}
