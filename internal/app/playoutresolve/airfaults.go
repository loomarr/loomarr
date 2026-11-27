package playoutresolve

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AirFaultStrikes is how many airings in a row a clip may fail to encode before it is fed back to
// the catalog as unplayable.
const AirFaultStrikes = 3

// AirFaults counts, per filler clip, the airings that failed to encode, and feeds a clip that keeps
// failing back to the catalog (#3855). A clip that airs ends its run of failures. The zero value is
// ready to use and reports nothing until SetReporter is called.
type AirFaults struct {
	mu      sync.Mutex
	strikes map[string]int
	report  func(ctx context.Context, hash, detail string) error
}

// SetReporter names where a failing clip goes: the filler pipeline's RejectAirFault. The reporter
// owns logging its own errors.
func (a *AirFaults) SetReporter(report func(ctx context.Context, hash, detail string) error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.report = report
}

// Faulted counts one failed airing of the clip. At AirFaultStrikes in a row the clip is reported,
// off the caller's goroutine, since the packager calls this while it is airing, and the count
// starts over.
func (a *AirFaults) Faulted(hash string, cause error) {
	a.mu.Lock()
	if a.strikes == nil {
		a.strikes = map[string]int{}
	}
	a.strikes[hash]++
	if a.strikes[hash] < AirFaultStrikes || a.report == nil {
		a.mu.Unlock()
		return
	}
	delete(a.strikes, hash)
	report := a.report
	a.mu.Unlock()
	detail := fmt.Sprintf("failed to encode at air time %d times in a row: %v", AirFaultStrikes, cause)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = report(ctx, hash, detail)
	}()
}

// Healthy ends the clip's run of failures: it aired.
func (a *AirFaults) Healthy(hash string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.strikes, hash)
}
