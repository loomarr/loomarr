package testkit

import (
	"context"
	"sync"

	"github.com/loomarr/loomarr/internal/quality"
)

// QualityRecorder is the shared test double for the privacy-safe discovery
// quality sink. It copies observations so callers can assert the typed boundary
// without reaching into the ledger's private receipt table.
type QualityRecorder struct {
	mu  sync.Mutex
	Err error
	got []quality.Observation
}

func (r *QualityRecorder) RecordQualityObservation(_ context.Context, observation quality.Observation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, observation)
	return r.Err
}

func (r *QualityRecorder) Observations() []quality.Observation {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]quality.Observation(nil), r.got...)
}
