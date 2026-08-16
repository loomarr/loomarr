package metrics

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"

	"github.com/mantonx/loomarr/internal/provision"
)

// This file adds the first tranche of §17 *domain* metrics: the state gauges
// (records-by-state, job-queue depth, active sessions) via a pull-based
// collector, plus the two cleanest event counters (logins, webhook events).
// The latency/LLM/filler series in §17 remain staged follow-up instrumentation
// (documented in README Operations + PROGRESS) — several need their own timing
// wrappers around external calls, a different pattern from these.

// StoreCounts is the narrow read surface the state-gauge collector needs. The
// full store.Store satisfies it structurally, so wiring stays decoupled from the
// persistence package.
type StoreCounts interface {
	CountTitlesByState(ctx context.Context) (map[provision.State]int, error)
	CountJobsByStatus(ctx context.Context) (map[string]int, error)
	CountActiveSessions(ctx context.Context, now time.Time) (int, error)
}

// knownStates / knownJobStatuses are the label sets the collector zero-fills, so
// a dimension that empties out reports 0 instead of dropping off the dashboard.
var (
	knownStates = []provision.State{
		provision.Wanted, provision.Requested, provision.Downloading,
		provision.Available, provision.Unavailable,
	}
	knownJobStatuses = []string{"queued", "running", "done", "failed"}
)

var (
	// logins counts login attempts by outcome (§17 logins success/failure).
	logins = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loomarr", Subsystem: "auth", Name: "logins_total",
		Help: "Login attempts by result.",
	}, []string{"result"})

	// storeScrapeErrors records when a state-gauge query fails during a scrape,
	// so a silently-empty gauge is distinguishable from a broken one.
	storeScrapeErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loomarr", Subsystem: "metrics", Name: "scrape_errors_total",
		Help: "Store queries that failed during a /metrics scrape, by source.",
	}, []string{"source"})
)

// LoginResult records a login outcome (§17). success is true when credentials
// verified and a session was issued.
func LoginResult(success bool) {
	result := "failure"
	if success {
		result = "success"
	}
	logins.WithLabelValues(result).Inc()
}

// storeCollector emits the state gauges by querying the store at scrape time.
// Reading on scrape (not on every mutation) keeps the gauges correct without
// threading a recorder through the provisioning, job, and session write paths.
type storeCollector struct {
	binding  atomic.Pointer[storeCollectorBinding]
	titles   *prometheus.Desc
	jobs     *prometheus.Desc
	sessions *prometheus.Desc
}

// storeCollectorBinding is published as one immutable value so a scrape never
// combines a generation's store with another generation's clock.
type storeCollectorBinding struct {
	counts StoreCounts
	now    func() time.Time
}

func (c *storeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.titles
	ch <- c.jobs
	ch <- c.sessions
}

func (c *storeCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	binding := c.binding.Load()

	if byState, err := binding.counts.CountTitlesByState(ctx); err != nil {
		storeScrapeErrors.WithLabelValues("titles").Inc()
	} else {
		for _, st := range knownStates {
			ch <- prometheus.MustNewConstMetric(c.titles, prometheus.GaugeValue,
				float64(byState[st]), string(st))
		}
	}

	if byStatus, err := binding.counts.CountJobsByStatus(ctx); err != nil {
		storeScrapeErrors.WithLabelValues("jobs").Inc()
	} else {
		for _, status := range knownJobStatuses {
			ch <- prometheus.MustNewConstMetric(c.jobs, prometheus.GaugeValue,
				float64(byStatus[status]), status)
		}
	}

	if n, err := binding.counts.CountActiveSessions(ctx, binding.now()); err != nil {
		storeScrapeErrors.WithLabelValues("sessions").Inc()
	} else {
		ch <- prometheus.MustNewConstMetric(c.sessions, prometheus.GaugeValue, float64(n))
	}
}

// newStoreCollector builds the collector with its metric descriptors. Split from
// registration so tests can gather it through a private registry.
func newStoreCollector(counts StoreCounts, now func() time.Time) *storeCollector {
	c := &storeCollector{
		titles: prometheus.NewDesc("loomarr_titles",
			"Provisioning records currently in each state.", []string{"state"}, nil),
		jobs: prometheus.NewDesc("loomarr_jobs",
			"Suggester jobs currently in each status (queue depth).", []string{"status"}, nil),
		sessions: prometheus.NewDesc("loomarr_active_sessions",
			"Unexpired sessions right now.", nil, nil),
	}
	c.rebind(counts, now)
	return c
}

func (c *storeCollector) rebind(counts StoreCounts, now func() time.Time) {
	c.binding.Store(&storeCollectorBinding{counts: counts, now: now})
}

// RegisterStoreCollector wires the state-gauge collector into the default
// registry so /metrics includes it. App generations rebuild their stores while
// the registry lives for the process, so a duplicate registration rebinds the
// existing collector to the new store. Any other error is returned for the
// caller to log.
func RegisterStoreCollector(counts StoreCounts, now func() time.Time) error {
	return registerStoreCollector(prometheus.DefaultRegisterer, counts, now)
}

func registerStoreCollector(reg prometheus.Registerer, counts StoreCounts, now func() time.Time) error {
	candidate := newStoreCollector(counts, now)
	if err := reg.Register(candidate); err != nil {
		var already prometheus.AlreadyRegisteredError
		if errors.As(err, &already) {
			existing, ok := already.ExistingCollector.(*storeCollector)
			if !ok {
				return fmt.Errorf("store metric descriptors already owned by %T", already.ExistingCollector)
			}
			existing.rebind(counts, now)
			return nil
		}
		return err
	}
	return nil
}
