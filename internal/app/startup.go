package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/adhocore/gronx"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/scheduler"
	"github.com/loomarr/loomarr/internal/store"
)

const startupIntegrationTimeout = 5 * time.Second
const systemHealthDefaultCron = "0 * * * * *"

type startupIntegration struct {
	key         string
	configured  bool
	remediation string
}

func startupIntegrations(set resolved) []startupIntegration {
	return []startupIntegration{
		{diagnostics.StartupCheckMediaServer,
			set.str("library.flavor") != "" || set.str("library.url") != "" || set.str("library.token") != "",
			"/settings/connections"},
		{diagnostics.StartupCheckTunarr, set.str("tunarr.url") != "", "/settings/connections"},
		{diagnostics.StartupCheckRequester,
			set.str("seerr.url") != "" || set.str("sonarr.url") != "" || set.str("radarr.url") != "",
			"/settings/connections"},
		{diagnostics.StartupCheckLLM,
			set.str("llm.url") != "" || set.str("llm.model") != "" || set.str("llm.api_key") != "",
			"/settings/ai"},
		{diagnostics.StartupCheckTMDB, set.str("tmdb.api_key") != "", "/settings/connections"},
	}
}

// completeStartupIntegrations projects the existing setup probes into the startup report. It
// does not invent a second health implementation: connectionTests remains the one probe truth.
// Independent probes run concurrently so one slow optional service costs at most one timeout.
func completeStartupIntegrations(
	parent context.Context,
	startup *diagnostics.Startup,
	set resolved,
	probes map[string]func(context.Context) (bool, string),
) {
	var wg sync.WaitGroup
	for _, check := range startupIntegrations(set) {
		if !check.configured {
			startup.Complete(check.key, diagnostics.StartupSkipped, "not configured", check.remediation, "")
			continue
		}
		probe := probes[check.key]
		if probe == nil {
			startup.Complete(check.key, diagnostics.StartupWarning, "health probe unavailable", check.remediation, "")
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(parent, startupIntegrationTimeout)
			defer cancel()
			ok, _ := probe(ctx)
			if ok {
				startup.Complete(check.key, diagnostics.StartupPassed, "available", "", "")
				return
			}
			// Setup probes may include upstream URLs in their operator-facing explanation. The
			// startup report is retained and downloadable, so keep its detail deliberately
			// credential-free and route the operator to the richer live probe instead.
			detail := "configured but unavailable"
			if ctx.Err() != nil {
				detail = "health probe timed out"
			}
			startup.Complete(check.key, diagnostics.StartupWarning, detail, check.remediation, "")
		}()
	}
	wg.Wait()
}

// currentHealthRunner is the one continuous probe implementation used by the scheduler and manual
// refresh. Probe-specific detail stays private because setup probes may include URLs or credentials.
type currentHealthRunner struct {
	mu     sync.Mutex
	health *diagnostics.Startup
	store  interface {
		GetSetting(context.Context, string) (string, error)
	}
	set    resolved
	probes map[string]func(context.Context) (bool, string)
}

func newCurrentHealthRunner(
	health *diagnostics.Startup,
	st interface {
		GetSetting(context.Context, string) (string, error)
	},
	set resolved,
	probes map[string]func(context.Context) (bool, string),
) *currentHealthRunner {
	return &currentHealthRunner{health: health, store: st, set: set, probes: probes}
}

func (r *currentHealthRunner) Run(parent context.Context) error {
	if r == nil || r.health == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := parent.Err(); err != nil {
		return err
	}
	type probe struct {
		key         string
		configured  bool
		remediation string
		fn          func(context.Context) bool
	}
	checks := []probe{{
		key: diagnostics.StartupCheckDatabase, configured: r.store != nil,
		remediation: "/settings/system/database",
		fn: func(ctx context.Context) bool {
			_, err := r.store.GetSetting(ctx, "healthcheck_probe")
			return err == nil || errors.Is(err, store.ErrNotFound)
		},
	}}
	freshFor := currentHealthFreshness(r.set.str("job.system_health.schedule"), time.Now())
	for _, integration := range startupIntegrations(r.set) {
		probeFn := r.probes[integration.key]
		checks = append(checks, probe{
			key: integration.key, configured: integration.configured,
			remediation: integration.remediation,
			fn: func(ctx context.Context) bool {
				if probeFn == nil {
					return false
				}
				ok, _ := probeFn(ctx)
				return ok
			},
		})
	}

	var wg sync.WaitGroup
	for _, check := range checks {
		if !check.configured {
			r.health.Observe(check.key, diagnostics.HealthObservation{
				Status: diagnostics.HealthSkipped, Detail: "not configured",
				RemediationRoute: check.remediation,
				FreshFor:         freshFor,
			})
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(parent, startupIntegrationTimeout)
			defer cancel()
			if check.fn(ctx) {
				r.health.Observe(check.key, diagnostics.HealthObservation{
					Status: diagnostics.HealthPassed, Detail: "available",
					FreshFor: freshFor,
				})
				return
			}
			detail := "configured but unavailable"
			if check.key == diagnostics.StartupCheckDatabase {
				detail = "database health check failed"
			} else if ctx.Err() != nil {
				detail = "health probe timed out"
			}
			r.health.Observe(check.key, diagnostics.HealthObservation{
				Status: diagnostics.HealthFailed, Detail: detail,
				RemediationRoute: check.remediation,
				FreshFor:         freshFor,
			})
		}()
	}
	wg.Wait()
	return parent.Err()
}

func currentHealthFreshness(expression string, now time.Time) time.Duration {
	if expression == "" {
		expression = systemHealthDefaultCron
	}
	first, err := gronx.NextTickAfter(expression, now, false)
	if err != nil {
		return 3 * time.Minute
	}
	second, err := gronx.NextTickAfter(expression, first, false)
	if err != nil || !second.After(first) {
		return 3 * time.Minute
	}
	// Two missed scheduled observations plus one probe deadline is stale. This tolerates one
	// delayed scheduler claim while still preventing old green state from lingering.
	return 2*second.Sub(first) + startupIntegrationTimeout
}

func (r *currentHealthRunner) Refresh(ctx context.Context) (diagnostics.HealthReport, error) {
	if r == nil || r.health == nil {
		return diagnostics.HealthReport{State: diagnostics.HealthUnhealthy}, errors.New("current health is unavailable")
	}
	err := r.Run(ctx)
	return r.health.Health(), err
}

func (r *currentHealthRunner) Job() scheduler.Job {
	return scheduler.Job{
		Name: "system-health", Group: scheduler.GroupSystem, Title: "Check app health",
		Description: "Checks Loomarr's database and configured connections and refreshes Current Health.",
		DefaultCron: systemHealthDefaultCron, ScheduleKey: "job.system_health.schedule",
		Timeout: 10 * time.Second, Run: r.Run,
	}
}
