package playoutresolve

import (
	"context"
	"errors"
	"testing"
	"time"
)

// reports collects what an AirFaults handed to the catalog.
type reports struct{ ch chan [2]string }

func newReports() *reports { return &reports{ch: make(chan [2]string, 8)} }

func (r *reports) report(_ context.Context, hash, detail string) error {
	r.ch <- [2]string{hash, detail}
	return nil
}

func (r *reports) none(t *testing.T) {
	t.Helper()
	select {
	case got := <-r.ch:
		t.Fatalf("reported %v, want nothing yet", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func (r *reports) one(t *testing.T) [2]string {
	t.Helper()
	select {
	case got := <-r.ch:
		return got
	case <-time.After(2 * time.Second):
		t.Fatal("nothing reported")
		return [2]string{}
	}
}

// A clip is fed back only after it fails to encode AirFaultStrikes airings in a row; a good airing
// in between starts the count again, so a clip that failed once in a while stays on air (#3855).
func TestAirFaults_ReportsAfterConsecutiveEncodeFailures(t *testing.T) {
	var a AirFaults
	r := newReports()
	a.SetReporter(r.report)
	boom := errors.New("ended without a frame")

	a.Faulted("90003001", boom)
	a.Faulted("90003001", boom)
	a.Healthy("90003001")
	a.Faulted("90003001", boom)
	a.Faulted("90003001", boom)
	r.none(t)

	a.Faulted("90003001", boom)
	if got := r.one(t); got[0] != "90003001" || got[1] == "" {
		t.Fatalf("reported %v, want the clip and the cause", got)
	}
	// The count starts over after a report.
	a.Faulted("90003001", boom)
	r.none(t)
}

// Clips are counted apart, and with no reporter wired nothing happens.
func TestAirFaults_CountsEachClipApartAndNeedsAReporter(t *testing.T) {
	var a AirFaults
	boom := errors.New("x")
	for range AirFaultStrikes {
		a.Faulted("90003002", boom) // no reporter yet: must not panic
	}
	r := newReports()
	a.SetReporter(r.report)
	for range AirFaultStrikes - 1 {
		a.Faulted("90003003", boom)
		a.Faulted("90003004", boom)
	}
	r.none(t)
}
