package metrics

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/mantonx/loomarr/internal/provision"
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

type trackingCounts struct {
	fakeCounts
	calls atomic.Int64
}

func (f *trackingCounts) CountTitlesByState(ctx context.Context) (map[provision.State]int, error) {
	f.calls.Add(1)
	return f.fakeCounts.CountTitlesByState(ctx)
}

func (f *trackingCounts) CountJobsByStatus(ctx context.Context) (map[string]int, error) {
	f.calls.Add(1)
	return f.fakeCounts.CountJobsByStatus(ctx)
}

func (f *trackingCounts) CountActiveSessions(ctx context.Context, now time.Time) (int, error) {
	f.calls.Add(1)
	return f.fakeCounts.CountActiveSessions(ctx, now)
}
func (f *trackingCounts) OldestProposalJobsByStatus(ctx context.Context) (map[string]time.Time, error) {
	f.calls.Add(1)
	return f.fakeCounts.OldestProposalJobsByStatus(ctx)
}
func (f *trackingCounts) CountProposalJobAttemptsByStatus(ctx context.Context) (map[string]int, error) {
	f.calls.Add(1)
	return f.fakeCounts.CountProposalJobAttemptsByStatus(ctx)
}
func (f *trackingCounts) CountFailedProposalJobsByCode(ctx context.Context) (map[string]int, error) {
	f.calls.Add(1)
	return f.fakeCounts.CountFailedProposalJobsByCode(ctx)
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

// The collector emits every known state/status even when the store reports none,
// so a dimension that empties out reads 0 rather than dropping off the graph.
func TestStoreCollectorZeroFills(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	c := newStoreCollector(fakeCounts{
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

// A failing store must degrade the scrape, not crash it: the collector emits
// nothing for the broken queries rather than panicking or reporting stale zeros.
func TestStoreCollectorScrapeError(t *testing.T) {
	c := newStoreCollector(fakeCounts{err: errors.New("db down")},
		func() time.Time { return time.Unix(0, 0).UTC() })
	if n := testutil.CollectAndCount(c); n != 0 {
		t.Errorf("on store error the collector emitted %d metrics, want 0", n)
	}
}

// A process restart rebuilds the app around a new store while Prometheus keeps
// its process-global registry. The registered collector must follow the current
// generation rather than retaining the closed store from the first one.
func TestRegisterStoreCollectorRebindsExistingCollector(t *testing.T) {
	reg := prometheus.NewRegistry()
	old := &trackingCounts{fakeCounts: fakeCounts{err: errors.New("closed old store")}}
	current := &trackingCounts{fakeCounts: fakeCounts{
		titles:   map[provision.State]int{},
		jobs:     map[string]int{},
		sessions: 7,
	}}

	if err := registerStoreCollector(reg, old, func() time.Time { return time.Unix(1, 0) }); err != nil {
		t.Fatal(err)
	}
	if err := registerStoreCollector(reg, current, func() time.Time { return time.Unix(2, 0) }); err != nil {
		t.Fatal(err)
	}

	want := `
# HELP loomarr_active_sessions Unexpired sessions right now.
# TYPE loomarr_active_sessions gauge
loomarr_active_sessions 7
`
	if err := testutil.GatherAndCompare(reg, strings.NewReader(want), "loomarr_active_sessions"); err != nil {
		t.Error(err)
	}
	if got := old.calls.Load(); got != 0 {
		t.Errorf("closed generation queried %d times after rebind, want 0", got)
	}
	if got := current.calls.Load(); got != 6 {
		t.Errorf("current generation queried %d times, want 6", got)
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
