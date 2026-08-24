package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

func TestStartupTableNeverWritesToNonInteractiveJSONOutput(t *testing.T) {
	var output bytes.Buffer
	printStartupReport(&output, diagnostics.StartupReport{State: diagnostics.StartupReady}, false, 100, false)
	if output.Len() != 0 {
		t.Fatalf("non-interactive output gained multiline side channel: %q", output.String())
	}
	printStartupReport(&output, diagnostics.StartupReport{Version: "dev", State: diagnostics.StartupReady}, true, 100, false)
	if !strings.Contains(output.String(), "Loomarr dev") {
		t.Fatalf("interactive startup report = %q", output.String())
	}
}

func TestGenerationHealthDeclaresLiveChecksBeforeBoot(t *testing.T) {
	health := newGenerationHealth(time.Unix(100, 0), 3).Health()
	if health.Generation != 3 || len(health.Checks) != 10 {
		t.Fatalf("health = %+v", health)
	}
	continuous := map[string]bool{}
	for _, check := range health.Checks {
		if check.Mode == diagnostics.HealthCheckContinuous {
			continuous[check.Key] = true
		}
	}
	for _, key := range []string{
		diagnostics.StartupCheckDatabase,
		diagnostics.StartupCheckMediaServer,
		diagnostics.StartupCheckTunarr,
		diagnostics.StartupCheckRequester,
		diagnostics.StartupCheckLLM,
		diagnostics.StartupCheckTMDB,
	} {
		if !continuous[key] {
			t.Errorf("%s was not declared continuous", key)
		}
	}
}
