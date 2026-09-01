package metrics

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/loomarr/loomarr/internal/provision"
)

// This file owns Store-backed current-state gauges and authentication outcomes.
// The pull collector reads retained truth at scrape time rather than attempting
// to mirror every Store mutation in an in-memory gauge.

// StoreCounts is the narrow read surface the state-gauge collector needs. The
// full store.Store satisfies it structurally, so wiring stays decoupled from the
// persistence package.
type StoreCounts interface {
	CountTitlesByState(ctx context.Context) (map[provision.State]int, error)
	CountJobsByStatus(ctx context.Context) (map[string]int, error)
	OldestProposalJobsByStatus(ctx context.Context) (map[string]time.Time, error)
	CountProposalJobAttemptsByStatus(ctx context.Context) (map[string]int, error)
	CountFailedProposalJobsByCode(ctx context.Context) (map[string]int, error)
	CountActiveSessions(ctx context.Context, now time.Time) (int, error)
}

// knownStates / knownJobStatuses are the label sets the collector zero-fills, so
// a dimension that empties out reports 0 instead of dropping off the dashboard.
var (
	knownStates = []provision.State{
		provision.Wanted, provision.Requested, provision.Downloading,
		provision.Available, provision.Unavailable,
	}
	knownJobStatuses             = []string{"queued", "running", "done", "failed"}
	knownProposalAttemptOutcomes = []string{"succeeded", "failed", "interrupted", "other"}
	knownProposalFailureCodes    = []string{"no_grounded_titles", "generation_failed", "other"}
)

// storeCollector emits the state gauges by querying the store at scrape time.
// Reading on scrape (not on every mutation) keeps the gauges correct without
// threading a recorder through the provisioning, job, and session write paths.
type storeCollector struct {
	counts            StoreCounts
	now               func() time.Time
	scrapeErrors      *prometheus.CounterVec
	titles            *prometheus.Desc
	jobs              *prometheus.Desc
	sessions          *prometheus.Desc
	proposalOldestAge *prometheus.Desc
	proposalAttempts  *prometheus.Desc
	proposalFailures  *prometheus.Desc
}

func (c *storeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.titles
	ch <- c.jobs
	ch <- c.sessions
	ch <- c.proposalOldestAge
	ch <- c.proposalAttempts
	ch <- c.proposalFailures
}

func (c *storeCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if byState, err := c.counts.CountTitlesByState(ctx); err != nil {
		c.scrapeErrors.WithLabelValues("titles").Inc()
	} else {
		for _, st := range knownStates {
			ch <- prometheus.MustNewConstMetric(c.titles, prometheus.GaugeValue,
				float64(byState[st]), string(st))
		}
	}

	if byStatus, err := c.counts.CountJobsByStatus(ctx); err != nil {
		c.scrapeErrors.WithLabelValues("jobs").Inc()
	} else {
		for _, status := range knownJobStatuses {
			ch <- prometheus.MustNewConstMetric(c.jobs, prometheus.GaugeValue,
				float64(byStatus[status]), status)
		}
	}

	if n, err := c.counts.CountActiveSessions(ctx, c.now()); err != nil {
		c.scrapeErrors.WithLabelValues("sessions").Inc()
	} else {
		ch <- prometheus.MustNewConstMetric(c.sessions, prometheus.GaugeValue, float64(n))
	}

	if oldest, err := c.counts.OldestProposalJobsByStatus(ctx); err != nil {
		c.scrapeErrors.WithLabelValues("proposal_job_age").Inc()
	} else {
		now := c.now()
		for _, status := range []string{"queued", "running"} {
			age := 0.0
			if createdAt, ok := oldest[status]; ok {
				age = max(0, now.Sub(createdAt).Seconds())
			}
			ch <- prometheus.MustNewConstMetric(c.proposalOldestAge, prometheus.GaugeValue, age, status)
		}
	}

	if raw, err := c.counts.CountProposalJobAttemptsByStatus(ctx); err != nil {
		c.scrapeErrors.WithLabelValues("proposal_job_attempts").Inc()
	} else {
		bounded := boundCounts(raw, knownProposalAttemptOutcomes[:len(knownProposalAttemptOutcomes)-1])
		for _, outcome := range knownProposalAttemptOutcomes {
			ch <- prometheus.MustNewConstMetric(c.proposalAttempts, prometheus.GaugeValue,
				float64(bounded[outcome]), outcome)
		}
	}

	if raw, err := c.counts.CountFailedProposalJobsByCode(ctx); err != nil {
		c.scrapeErrors.WithLabelValues("proposal_job_failures").Inc()
	} else {
		bounded := boundCounts(raw, knownProposalFailureCodes[:len(knownProposalFailureCodes)-1])
		for _, code := range knownProposalFailureCodes {
			ch <- prometheus.MustNewConstMetric(c.proposalFailures, prometheus.GaugeValue,
				float64(bounded[code]), code)
		}
	}
}

func boundCounts(raw map[string]int, known []string) map[string]int {
	out := make(map[string]int, len(known)+1)
	for label, n := range raw {
		matched := false
		for _, allowed := range known {
			if label == allowed {
				out[label] += n
				matched = true
				break
			}
		}
		if !matched {
			out["other"] += n
		}
	}
	return out
}

func newStoreCollectorWithErrors(
	counts StoreCounts,
	now func() time.Time,
	scrapeErrors *prometheus.CounterVec,
) *storeCollector {
	c := &storeCollector{
		counts:       counts,
		now:          now,
		scrapeErrors: scrapeErrors,
		titles: prometheus.NewDesc("loomarr_titles",
			"Provisioning records currently in each state.", []string{"state"}, nil),
		jobs: prometheus.NewDesc("loomarr_jobs",
			"Suggester jobs currently in each status (queue depth).", []string{"status"}, nil),
		sessions: prometheus.NewDesc("loomarr_active_sessions",
			"Unexpired sessions right now.", nil, nil),
		proposalOldestAge: prometheus.NewDesc("loomarr_proposal_job_oldest_age_seconds",
			"Age of the oldest retained nonterminal Proposal Job.", []string{"status"}, nil),
		proposalAttempts: prometheus.NewDesc("loomarr_proposal_job_attempts",
			"Retained terminal Proposal Job Attempts by bounded outcome.", []string{"outcome"}, nil),
		proposalFailures: prometheus.NewDesc("loomarr_proposal_job_failures",
			"Retained failed Proposal Jobs by bounded requester-safe code.", []string{"code"}, nil),
	}
	return c
}
