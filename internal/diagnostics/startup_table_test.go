package diagnostics

import (
	"strings"
	"testing"
)

func TestStartupTableSnapshots(t *testing.T) {
	base := StartupReport{
		Generation: 3, Version: "v1.2.3", DurationMillis: 1250,
		Checks: []StartupCheck{
			{Key: "configuration", Label: "Configuration", Required: true, Status: StartupPassed, EndedAt: 1, DurationMillis: 25, Detail: "valid"},
			{Key: "database", Label: "Database and migrations", Required: true, Status: StartupPassed, EndedAt: 1, DurationMillis: 1225, Detail: "SQLite ready"},
		},
	}
	cases := []struct {
		name     string
		state    StartupState
		status   StartupCheckStatus
		width    int
		detail   string
		contains []string
	}{
		{"ready", StartupReady, StartupPassed, 100, "ready", []string{"READY", "PASSED", "Configuration", "1.25s"}},
		{"degraded", StartupDegraded, StartupWarning, 100, "optional service unavailable", []string{"DEGRADED", "WARNING", "optional service unavailable"}},
		{"blocked", StartupBlocked, StartupFailed, 100, "migration failed", []string{"BLOCKED", "FAILED", "migration failed"}},
		{"narrow-long-detail", StartupBlocked, StartupFailed, 48, strings.Repeat("long detail ", 20), []string{"BLOCKED", "FAILED", "long", "detail"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := base
			report.State = tc.state
			report.Checks = append([]StartupCheck(nil), base.Checks...)
			report.Checks[1].Status, report.Checks[1].Detail = tc.status, tc.detail
			got := RenderStartupTable(report, StartupTableOptions{Width: tc.width, Color: false})
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Fatalf("snapshot missing %q:\n%s", want, got)
				}
			}
			if strings.Contains(got, "\x1b[") {
				t.Fatalf("no-color snapshot contains ANSI: %q", got)
			}
			if tc.width == 48 {
				for _, line := range strings.Split(got, "\n") {
					if len([]rune(line)) > 58 {
						t.Fatalf("narrow snapshot line is %d columns:\n%s", len([]rune(line)), got)
					}
				}
			}
		})
	}
}

func TestStartupTableColorIsProjectionOnly(t *testing.T) {
	report := StartupReport{State: StartupBlocked, Checks: []StartupCheck{{Label: "Database", Status: StartupFailed}}}
	plain := RenderStartupTable(report, StartupTableOptions{Color: false})
	colored := RenderStartupTable(report, StartupTableOptions{Color: true})
	if strings.Contains(plain, "\x1b[") || !strings.Contains(colored, "\x1b[") {
		t.Fatalf("plain/color projection mismatch: plain=%q colored=%q", plain, colored)
	}
	if report.Checks[0].Status != StartupFailed {
		t.Fatal("renderer mutated report")
	}
}
