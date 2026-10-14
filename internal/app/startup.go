package app

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/adhocore/gronx"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/scheduler"
	"github.com/loomarr/loomarr/internal/store"
)

const startupIntegrationTimeout = 5 * time.Second
const systemHealthDefaultCron = "*/30 * * * * *"

type startupIntegration struct {
	key         string
	configured  bool
	remediation string
}

// unavailableDetail is the operator-facing reason for a configured check that failed. Most
// checks stay deliberately generic (their probes may echo credentials); the public address has
// exactly one fix, so it names the setting to change.
func unavailableDetail(set resolved, key string) string {
	if key == diagnostics.StartupCheckPublicURL {
		return publicURLFailureDetail(set.str("server.public_url"))
	}
	return "configured but unavailable"
}

// publicURLFailureDetail names the address and the setting. Userinfo is stripped: the retained,
// downloadable startup report must never carry a credential typed into the URL.
func publicURLFailureDetail(raw string) string {
	shown := strings.TrimSpace(raw)
	if u, err := url.Parse(shown); err == nil && u.User != nil {
		u.User = nil
		shown = u.String()
	}
	return "This server can't reach its public address " + shown + " — check SERVER_PUBLIC_URL"
}

// publicURLProbe is the health probe for server.public_url: the server must answer its own
// unauthenticated liveness route there, the same path a media server or Tunarr would dial.
func publicURLProbe(set resolved, client *http.Client) func(context.Context) (bool, string) {
	return func(ctx context.Context) (bool, string) {
		base := strings.TrimRight(strings.TrimSpace(set.str("server.public_url")), "/")
		if base == "" {
			return false, "set the public address"
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/healthz", nil)
		if err != nil {
			return false, "the public address is not a valid URL"
		}
		resp, err := client.Do(req)
		if err != nil {
			return false, "could not reach the public address"
		}
		_ = resp.Body.Close()
		return resp.StatusCode >= 200 && resp.StatusCode < 300, ""
	}
}

func startupIntegrations(set resolved) []startupIntegration {
	return []startupIntegration{
		{diagnostics.StartupCheckPublicURL, strings.TrimSpace(set.str("server.public_url")) != "",
			"/settings/system/playback"},
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
		if check.key == diagnostics.StartupCheckPublicURL {
			// The server dials itself here, and this runs during assembly, before the listener
			// exists: probing now would report every install as unreachable. Current Health
			// runs the same probe once the server is up.
			startup.Complete(check.key, diagnostics.StartupSkipped,
				"checked by Current Health once the server is listening", check.remediation, "")
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
			detail := unavailableDetail(set, check.key)
			if ctx.Err() != nil && check.key != diagnostics.StartupCheckPublicURL {
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
			detail := unavailableDetail(r.set, check.key)
			if check.key == diagnostics.StartupCheckDatabase {
				detail = "database health check failed"
			} else if ctx.Err() != nil && check.key != diagnostics.StartupCheckPublicURL {
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
