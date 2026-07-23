package metrics

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/mantonx/loomarr/internal/provision"
)

type fakeCounts struct {
	titles   map[provision.State]int
	jobs     map[string]int
	sessions int
	err      error
}

func (f fakeCounts) CountTitlesByState(context.Context) (map[provision.State]int, error) {
	return f.titles, f.err
}
func (f fakeCounts) CountJobsByStatus(context.Context) (map[string]int, error) {
	return f.jobs, f.err
}
func (f fakeCounts) CountActiveSessions(context.Context, time.Time) (int, error) {
	return f.sessions, f.err
}

// The collector emits every known state/status even when the store reports none,
// so a dimension that empties out reads 0 rather than dropping off the graph.
func TestStoreCollectorZeroFills(t *testing.T) {
	c := newStoreCollector(fakeCounts{
		titles:   map[provision.State]int{provision.Requested: 2, provision.Available: 1},
		jobs:     map[string]int{"queued": 3},
		sessions: 4,
	}, func() time.Time { return time.Unix(0, 0).UTC() })

	want := `
# HELP loomarr_active_sessions Unexpired sessions right now.
# TYPE loomarr_active_sessions gauge
loomarr_active_sessions 4
# HELP loomarr_jobs Suggester jobs currently in each status (queue depth).
# TYPE loomarr_jobs gauge
loomarr_jobs{status="done"} 0
loomarr_jobs{status="failed"} 0
loomarr_jobs{status="queued"} 3
loomarr_jobs{status="running"} 0
# HELP loomarr_titles Provisioning records currently in each state.
# TYPE loomarr_titles gauge
loomarr_titles{state="available"} 1
loomarr_titles{state="downloading"} 0
loomarr_titles{state="requested"} 2
loomarr_titles{state="unavailable"} 0
loomarr_titles{state="wanted"} 0
`
	if err := testutil.CollectAndCompare(c, strings.NewReader(want)); err != nil {
		t.Error(err)
	}
}

// A failing store must degrade the scrape, not crash it: the collector emits
// nothing for the broken queries rather than panicking or reporting stale zeros.
func TestStoreCollectorScrapeError(t *testing.T) {
	c := newStoreCollector(fakeCounts{err: errors.New("db down")},
		func() time.Time { return time.Unix(0, 0).UTC() })
	if n := testutil.CollectAndCount(c); n != 0 {
		t.Errorf("on store error the collector emitted %d metrics, want 0", n)
	}
}

// LoginResult increments its labelled counter. Deltas are used because the vec
// lives on the process-global default registry.
func TestEventCounters(t *testing.T) {
	before := testutil.ToFloat64(logins.WithLabelValues("success"))
	LoginResult(true)
	if got := testutil.ToFloat64(logins.WithLabelValues("success")); got != before+1 {
		t.Errorf("LoginResult(true): success counter %v, want %v", got, before+1)
	}
}
