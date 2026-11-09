package connectiontest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/platform/llm"
)

func openRouterSelection() func() llm.Selection {
	return func() llm.Selection {
		return llm.Selection{
			Provider: "openrouter", URL: "https://openrouter.ai/api/v1",
			Model: "openai/gpt-4o-mini", APIKey: "provider-secret",
		}
	}
}

func TestLLMConnectionCheck_ListsModelsWithTheSelectionsKey(t *testing.T) {
	var gotKey string
	probes := New(Config{
		Selection: openRouterSelection(),
		ListModels: func(_ context.Context, _, apiKey string) error {
			gotKey = apiKey
			return nil
		},
	})
	ok, detail := probes["llm"](context.Background())
	if !ok {
		t.Fatalf("configured OpenRouter reported unavailable: %s", detail)
	}
	if gotKey != "provider-secret" {
		t.Errorf("listed models with key %q, want the selection's key", gotKey)
	}
}

// #2735: AI settings showed "Connection OK" above "No models returned". A hosted provider whose
// model list cannot be read is not a working connection.
func TestLLMConnectionCheck_HostedProviderWithNoModelsIsNotOK(t *testing.T) {
	probes := New(Config{
		Selection: openRouterSelection(),
		ListModels: func(context.Context, string, string) error {
			return errors.New("the provider returned no models")
		},
	})
	ok, detail := probes["llm"](context.Background())
	if ok {
		t.Fatal("hosted provider with no models reported OK")
	}
	if !strings.Contains(detail, "no models came back") {
		t.Errorf("detail = %q, want it to say no models came back", detail)
	}
}

// Current Health runs this probe every 30 seconds: a successful listing is reused for the same
// address and key, a failure never is, and a new key is listed at once.
func TestHostedModelCheck_ReusesOnlySuccessForTheSameKey(t *testing.T) {
	now := time.Unix(1000, 0)
	calls := 0
	fail := false
	check := &hostedModelCheck{now: func() time.Time { return now }, list: func(context.Context, string, string) error {
		calls++
		if fail {
			return errors.New("down")
		}
		return nil
	}}
	ctx := context.Background()
	_ = check.check(ctx, "http://a/v1", "k1")
	_ = check.check(ctx, "http://a/v1", "k1")
	if calls != 1 {
		t.Fatalf("calls = %d, want the second check to reuse the first success", calls)
	}
	_ = check.check(ctx, "http://a/v1", "k2")
	if calls != 2 {
		t.Fatalf("calls = %d, want a new key listed at once", calls)
	}
	now = now.Add(hostedModelReuse + time.Second)
	fail = true
	if err := check.check(ctx, "http://a/v1", "k2"); err == nil {
		t.Fatal("expired success was reused")
	}
	if err := check.check(ctx, "http://a/v1", "k2"); err == nil || calls != 4 {
		t.Fatalf("a failure was remembered (calls = %d)", calls)
	}
}
