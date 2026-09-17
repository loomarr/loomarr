package clipfetch

import (
	"context"
	"errors"
	"fmt"

	"github.com/loomarr/loomarr/internal/storagegovernor"
)

var ErrEstimateUnavailable = errors.New("clipfetch: media size estimate unavailable")

// Estimator reads provider metadata without downloading media. Implementations
// return the governor-owned write ceiling and full processing reservation.
type Estimator interface {
	Estimate(ctx context.Context, source Source) (storagegovernor.MediaBudget, error)
}

func mediaBudget(declaredBytes, durationMS int64, height int) (storagegovernor.MediaBudget, error) {
	budget, ok := storagegovernor.EstimateMedia(storagegovernor.MediaEstimate{
		DeclaredBytes: declaredBytes, DurationMS: durationMS, Height: height,
	})
	if !ok {
		return storagegovernor.MediaBudget{}, ErrEstimateUnavailable
	}
	return budget, nil
}

func addMediaBudget(total, next storagegovernor.MediaBudget) (storagegovernor.MediaBudget, error) {
	const maximum = int64(^uint64(0) >> 1)
	if next.WriteCeilingBytes <= 0 || next.ReservationBytes <= 0 ||
		total.WriteCeilingBytes > maximum-next.WriteCeilingBytes ||
		total.ReservationBytes > maximum-next.ReservationBytes {
		return storagegovernor.MediaBudget{}, fmt.Errorf("%w: aggregate overflow", ErrEstimateUnavailable)
	}
	total.WriteCeilingBytes += next.WriteCeilingBytes
	total.ReservationBytes += next.ReservationBytes
	return total, nil
}
