package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

type healthSettingStore struct{ fail bool }

func (s *healthSettingStore) GetSetting(context.Context, string) (string, error) {
	if s.fail {
		return "", errors.New("database unavailable")
	}
	return "", nil
}

func TestCompleteStartupIntegrationsDistinguishesUnconfiguredAndUnavailable(t *testing.T) {
	now := time.Unix(100, 0)
	startup := diagnostics.NewStartup(now, 1, "dev", []diagnostics.StartupCheck{
		{Key: diagnostics.StartupCheckMediaServer, Required: false},
		{Key: diagnostics.StartupCheckTunarr, Required: false},
		{Key: diagnostics.StartupCheckRequester, Required: false},
		{Key: diagnostics.StartupCheckLLM, Required: false},
		{Key: diagnostics.StartupCheckTMDB, Required: false},
	}, func() time.Time { return now })
	set := visionSet(t, map[string]string{"library.url": "http://media.example"})
	completeStartupIntegrations(context.Background(), startup, set, map[string]func(context.Context) (bool, string){
		diagnostics.StartupCheckMediaServer: func(context.Context) (bool, string) {
			return false, "could not reach the media server"
		},
	})

	report := startup.Snapshot()
	if report.State != diagnostics.StartupDegraded {
		t.Fatalf("state = %q, want degraded", report.State)
	}
	if report.Checks[0].Status != diagnostics.StartupWarning ||
		report.Checks[0].Detail != "configured but unavailable" {
		t.Fatalf("configured check = %+v", report.Checks[0])
	}
	for _, check := range report.Checks[1:] {
		if check.Status != diagnostics.StartupSkipped || check.Detail != "not configured" {
			t.Errorf("unconfigured check = %+v", check)
		}
	}
}

func TestCompleteStartupIntegrationsUsesExistingProbeSuccess(t *testing.T) {
	now := time.Unix(100, 0)
	startup := diagnostics.NewStartup(now, 1, "dev", []diagnostics.StartupCheck{
		{Key: diagnostics.StartupCheckTunarr, Required: false},
	}, func() time.Time { return now })
	set := visionSet(t, map[string]string{"tunarr.url": "http://tunarr.example"})
	completeStartupIntegrations(context.Background(), startup, set, map[string]func(context.Context) (bool, string){
		diagnostics.StartupCheckTunarr: func(context.Context) (bool, string) { return true, "" },
	})
	check := startup.Snapshot().Checks[0]
	if check.Status != diagnostics.StartupPassed || check.Detail != "available" {
		t.Fatalf("check = %+v", check)
	}
}

func TestCurrentHealthRunnerConfirmsDatabaseFailureAndRecovers(t *testing.T) {
	now := time.Unix(100, 0)
	health := diagnostics.NewStartup(now, 1, "dev", []diagnostics.StartupCheck{{
		Key: diagnostics.StartupCheckDatabase, Required: true,
		Mode: diagnostics.HealthCheckContinuous, FreshFor: time.Minute,
	}}, func() time.Time { return now })
	health.Complete(diagnostics.StartupCheckDatabase, diagnostics.StartupPassed, "available", "", "")
	st := &healthSettingStore{fail: true}
	runner := newCurrentHealthRunner(health, st, visionSet(t, nil), nil)

	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := health.Health().State; got != diagnostics.HealthDegraded {
		t.Fatalf("first failed probe state = %q, want degraded", got)
	}
	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := health.Health().State; got != diagnostics.HealthUnhealthy {
		t.Fatalf("confirmed failed probe state = %q, want unhealthy", got)
	}

	st.fail = false
	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := health.Health().State; got != diagnostics.HealthHealthy {
		t.Fatalf("recovered probe state = %q, want healthy", got)
	}
}

func TestCurrentHealthFreshnessTracksConfiguredSchedule(t *testing.T) {
	now := time.Date(2026, 8, 23, 12, 0, 1, 0, time.UTC)
	if got, want := currentHealthFreshness("", now), time.Minute+startupIntegrationTimeout; got != want {
		t.Fatalf("default freshness = %s, want %s", got, want)
	}
	if got, want := currentHealthFreshness("0 */5 * * * *", now), 10*time.Minute+startupIntegrationTimeout; got != want {
		t.Fatalf("freshness = %s, want %s", got, want)
	}
	if got := currentHealthFreshness("not cron", now); got != 3*time.Minute {
		t.Fatalf("invalid-cron fallback = %s", got)
	}
}
