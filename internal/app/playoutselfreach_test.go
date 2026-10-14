package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

// deadPublicURL refuses connections at once: the shape of a public URL whose IP the host no longer
// has, without the multi-second dial timeout a black-holed address would add to the test.
const deadPublicURL = "http://127.0.0.1:1"

func TestPublicURLProbeNamesAnUnreachableAddress(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/healthz" {
			t.Errorf("probed %q, want the liveness route", r.URL.Path)
		}
	}))
	t.Cleanup(up.Close)
	client := &http.Client{Timeout: time.Second}

	if ok, _ := publicURLProbe(visionSet(t, map[string]string{"server.public_url": up.URL}), client)(context.Background()); !ok {
		t.Fatal("a reachable public address must pass")
	}
	if ok, _ := publicURLProbe(visionSet(t, map[string]string{"server.public_url": deadPublicURL}), client)(context.Background()); ok {
		t.Fatal("an unreachable public address must fail")
	}

	got := publicURLFailureDetail("http://user:secret@10.0.0.9:8080")
	if !strings.Contains(got, "http://10.0.0.9:8080") || !strings.Contains(got, "check SERVER_PUBLIC_URL") ||
		strings.Contains(got, "secret") {
		t.Fatalf("failure detail = %q", got)
	}
}

func TestCurrentHealthReportsAnUnreachablePublicURL(t *testing.T) {
	now := time.Unix(100, 0)
	health := diagnostics.NewStartup(now, 1, "dev", []diagnostics.StartupCheck{
		{Key: diagnostics.StartupCheckPublicURL, Mode: diagnostics.HealthCheckContinuous, FreshFor: time.Minute},
	}, func() time.Time { return now })
	set := visionSet(t, map[string]string{"server.public_url": deadPublicURL})
	runner := newCurrentHealthRunner(health, nil, set, map[string]func(context.Context) (bool, string){
		diagnostics.StartupCheckPublicURL: publicURLProbe(set, &http.Client{Timeout: time.Second}),
	})
	if err := runner.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, check := range health.Health().Checks {
		if check.Key != diagnostics.StartupCheckPublicURL {
			continue
		}
		found = true
		if check.Status != diagnostics.HealthFailed || !strings.Contains(check.Detail, deadPublicURL) ||
			!strings.Contains(check.Detail, "check SERVER_PUBLIC_URL") {
			t.Fatalf("public URL check = %+v", check)
		}
	}
	if !found {
		t.Fatal("public URL check missing from Current Health")
	}
}
