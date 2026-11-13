package api_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/channels/schedule"
)

// A channel saved before rules that never fire were refused still holds one. Over HTTP, every write
// path that checks a policy treats it the same way: an edit that leaves the rule alone goes through
// (PATCH, and both draft previews, which exist to show what a save would do), and one that touches
// it is refused with a 422 that names the rule rather than the audience and filler options.
func TestLegacyDeadRuleDoesNotBlockUnrelatedEdits(t *testing.T) {
	harness := newChannelsHarness(t)
	srv, st := harness.Server, harness.Store
	mkChannel(t, srv, "c1", "Cartoons", 5)

	ctx := context.Background()
	ch, err := st.GetChannel(ctx, "c1")
	if err != nil {
		t.Fatal(err)
	}
	ch.Policy.Rules = []schedule.SchedulingRule{
		{ID: "r1", Label: "Weekend cartoons", Source: schedule.RuleSourceOperator, When: schedule.WhenPredicate{Weekend: true, Weekday: true}},
		{ID: "r2", Label: "One hour", Source: schedule.RuleSourceOperator, When: schedule.WhenPredicate{HourFrom: 5, HourTo: 5}},
	}
	if _, err := st.SaveChannel(ctx, ch); err != nil {
		t.Fatal(err)
	}

	const (
		r1 = `{"id":"r1","label":"Weekend cartoons","source":"operator","when":{"weekend":true,"weekday":true}}`
		r2 = `{"id":"r2","label":"One hour","source":"operator","when":{"hourFrom":5,"hourTo":5}}`
	)
	rules := `"rules":[` + r1 + `,` + r2 + `]`
	touched := `"rules":[` + strings.Replace(r1, "Weekend cartoons", "Renamed", 1) + `,` + r2 + `]`
	bodyOf := func(rules string) string {
		return `{"policy":{"audience":{"ceiling":"TV-PG"},` + rules + `}}`
	}

	t.Run("PATCH with the rules left alone", func(t *testing.T) {
		resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken, channelPatchBody(t, st, "c1", bodyOf(rules)))
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("PATCH → %d %s, want 200", resp.StatusCode, body)
		}
		got, _ := st.GetChannel(ctx, "c1")
		if len(got.Policy.Rules) != 2 || got.Policy.Audience.Ceiling != "TV-PG" {
			t.Fatalf("stored policy = %+v, want the edit saved and both rules kept as they were", got.Policy)
		}
	})
	t.Run("both previews with the rules left alone", func(t *testing.T) {
		for _, path := range []string{"/v1/channels/c1/programming/preview", "/v1/channels/c1/programming/changes"} {
			if resp := do(t, srv, http.MethodPost, path, adminToken, bodyOf(rules)); resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("%s → %d %s, want 200", path, resp.StatusCode, body)
			}
		}
	})
	t.Run("PATCH and both previews name a rule the edit touches", func(t *testing.T) {
		for method, path := range map[string]string{
			"PATCH": "/v1/channels/c1", "POST1": "/v1/channels/c1/programming/preview", "POST2": "/v1/channels/c1/programming/changes",
		} {
			verb, body := http.MethodPost, bodyOf(touched)
			if method == "PATCH" {
				verb, body = http.MethodPatch, channelPatchBody(t, st, "c1", bodyOf(touched))
			}
			resp := do(t, srv, verb, path, adminToken, body)
			text, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusUnprocessableEntity {
				t.Errorf("%s %s → %d, want 422", verb, path, resp.StatusCode)
				continue
			}
			for _, want := range []string{"Rule 1", "Renamed", "never"} {
				if !strings.Contains(string(text), want) {
					t.Errorf("%s %s detail = %s, want it to say %q", verb, path, text, want)
				}
			}
			if strings.Contains(string(text), "audience and filler") {
				t.Errorf("%s %s still points at the audience and filler options: %s", verb, path, text)
			}
		}
	})
}
