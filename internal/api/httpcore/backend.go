package httpcore

import (
	"context"
	"net/http"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
)

// BackendCheckpoint is the transport-layer projection of the durable transition state.
type BackendCheckpoint struct {
	Applied           string
	Prepared          string
	PublishedInternal bool
}

// Backend reads the checkpoint every channel route and playout route judges playability by. Source
// is the durable transition state; without one, the applied backend setting stands in.
type Backend struct {
	Source   func(context.Context) (BackendCheckpoint, error)
	Settings settings.Reader
}

// Checkpoint reads the current checkpoint, or a 503 problem when the durable state cannot be read.
func (b Backend) Checkpoint(ctx context.Context) (BackendCheckpoint, error) {
	if b.Source != nil {
		checkpoint, err := b.Source(ctx)
		if err != nil {
			return BackendCheckpoint{}, ProblemWithCause(http.StatusServiceUnavailable,
				"Playout state unavailable",
				"Loomarr couldn't read the current playout state. Try again in a moment.", err)
		}
		return checkpoint, nil
	}
	return b.legacy(), nil
}

func (b Backend) legacy() BackendCheckpoint {
	applied := ""
	if b.Settings != nil {
		applied = settings.PlayoutBackend.Get(b.Settings)
	}
	return BackendCheckpoint{
		Applied: applied, PublishedInternal: applied == schedule.PlayoutBackendInternal,
	}
}

// PlaysInternallyAt reports whether ch plays on Loomarr's own engine under the checkpoint.
func PlaysInternallyAt(ch store.Channel, checkpoint BackendCheckpoint) bool {
	return schedule.PlaysInternally(ch.Policy, checkpoint.Applied)
}

// TransportPlayableAt reports whether ch's internal transport may be served under the checkpoint.
func TransportPlayableAt(ch store.Channel, checkpoint BackendCheckpoint) bool {
	return schedule.InternalTransportPlayable(ch.Status, ch.Policy, checkpoint.PublishedInternal)
}

// InAppPlayableAt reports whether the in-app player can watch ch under the checkpoint.
func InAppPlayableAt(ch store.Channel, checkpoint BackendCheckpoint) bool {
	if !PlaysInternallyAt(ch, checkpoint) {
		return false
	}
	if !ch.Status.Reconcilable() || ch.Status == schedule.StatusEmpty {
		return false
	}
	return true
}
