package metrics

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/loomarr/loomarr/internal/provision"
)

type fakeCounts struct {
	titles   map[provision.State]int
	jobs     map[string]int
	sessions int
	oldest   map[string]time.Time
	attempts map[string]int
	failures map[string]int
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
func (f fakeCounts) OldestProposalJobsByStatus(context.Context) (map[string]time.Time, error) {
	return f.oldest, f.err
}
func (f fakeCounts) CountProposalJobAttemptsByStatus(context.Context) (map[string]int, error) {
	return f.attempts, f.err
}
func (f fakeCounts) CountFailedProposalJobsByCode(context.Context) (map[string]int, error) {
	return f.failures, f.err
}

func testStoreCollector(counts StoreCounts, now func() time.Time) *storeCollector {
	errors := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "test_scrape_errors_total"}, []string{"source"})
	return newStoreCollectorWithErrors(counts, now, errors)
}

// The collector emits every known state/status even when the store reports none,
// so a dimension that empties out reads 0 rather than dropping off the graph.
func TestStoreCollectorZeroFills(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	c := testStoreCollector(fakeCounts{
		titles:   map[provision.State]int{provision.Requested: 2, provision.Available: 1},
		jobs:     map[string]int{"queued": 3},
		sessions: 4,
		oldest:   map[string]time.Time{"queued": now.Add(-90 * time.Second)},
		attempts: map[string]int{"succeeded": 2, "interrupted": 1, "future_outcome": 3},
		failures: map[string]int{"no_grounded_titles": 4, "private_provider_reason": 2},
	}, func() time.Time { return now })

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
# HELP loomarr_proposal_job_attempts Retained terminal Proposal Job Attempts by bounded outcome.
# TYPE loomarr_proposal_job_attempts gauge
loomarr_proposal_job_attempts{outcome="failed"} 0
loomarr_proposal_job_attempts{outcome="interrupted"} 1
loomarr_proposal_job_attempts{outcome="other"} 3
loomarr_proposal_job_attempts{outcome="succeeded"} 2
# HELP loomarr_proposal_job_failures Retained failed Proposal Jobs by bounded requester-safe code.
# TYPE loomarr_proposal_job_failures gauge
loomarr_proposal_job_failures{code="generation_failed"} 0
loomarr_proposal_job_failures{code="no_grounded_titles"} 4
loomarr_proposal_job_failures{code="other"} 2
# HELP loomarr_proposal_job_oldest_age_seconds Age of the oldest retained nonterminal Proposal Job.
# TYPE loomarr_proposal_job_oldest_age_seconds gauge
loomarr_proposal_job_oldest_age_seconds{status="queued"} 90
loomarr_proposal_job_oldest_age_seconds{status="running"} 0
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

// A source that fails before any successful scrape emits no invented gauges.
func TestStoreCollectorScrapeError(t *testing.T) {
	c := testStoreCollector(fakeCounts{err: errors.New("db down")},
		func() time.Time { return time.Unix(0, 0).UTC() })
	if n := testutil.CollectAndCount(c); n != 0 {
		t.Errorf("on store error the collector emitted %d metrics, want 0", n)
	}
}
