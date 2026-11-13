package api_test

import (
	"net/http"
	"testing"
)

// A rule's weekday, hour and holiday values are closed sets. They used to validate when they named
// nothing and then never fired, so a channel silently never aired what its owner scheduled. Every
// route that accepts a policy says no, the same way: the channel edit, and both draft previews.
func TestPolicyRuleValuesAreRejectedEverywherePoliciesAreAccepted(t *testing.T) {
	rule := func(when string) string { return `{"policy":{"rules":[{"label":"x","when":` + when + `}]}}` }
	invalid := map[string]string{
		"weekday 9":                     rule(`{"days":[9]}`),
		"weekday 7":                     rule(`{"days":[7]}`),
		"weekday -1":                    rule(`{"days":[-1]}`),
		"a weekday twice":               rule(`{"days":[1,1]}`),
		"hourFrom 25 hourTo 30":         rule(`{"hourFrom":25,"hourTo":30}`),
		"hourFrom 24":                   rule(`{"hourFrom":24,"hourTo":5}`),
		"hourFrom -1":                   rule(`{"hourFrom":-1,"hourTo":5}`),
		"hourTo 25":                     rule(`{"hourFrom":1,"hourTo":25}`),
		"an empty window 5 to 5":        rule(`{"hourFrom":5,"hourTo":5}`),
		"dates reversed":                rule(`{"dateFrom":"2026-12-26T00:00:00Z","dateTo":"2026-12-24T00:00:00Z"}`),
		"an unknown holiday":            rule(`{"holiday":"not-a-holiday"}`),
		"a holiday spelled differently": rule(`{"holiday":"Christmas"}`),
		"a role eight times": `{"policy":{"filler":{"roles":["commercial","commercial","commercial","commercial",` +
			`"commercial","commercial","commercial","commercial"]}}}`,
		"a role twice": `{"policy":{"filler":{"roles":["commercial","bumper","commercial"]}}}`,
	}
	valid := map[string]string{
		"weekend days":          rule(`{"days":[0,6]}`),
		"every day":             rule(`{"days":[0,1,2,3,4,5,6]}`),
		"late night":            rule(`{"hourFrom":23,"hourTo":2}`),
		"to the end of the day": rule(`{"hourFrom":18,"hourTo":24}`),
		"a defined holiday":     rule(`{"holiday":"christmas"}`),
		"a date range":          rule(`{"dateFrom":"2026-12-24T00:00:00Z","dateTo":"2026-12-26T00:00:00Z"}`),
		"a one-day range":       rule(`{"dateFrom":"2026-12-24T00:00:00Z","dateTo":"2026-12-24T00:00:00Z"}`),
		"distinct roles":        `{"policy":{"filler":{"roles":["commercial","bumper","news"]}}}`,
	}

	harness := newChannelsHarness(t)
	srv, st := harness.Server, harness.Store
	mkChannel(t, srv, "c1", "Cold Relay", 5)

	for name, body := range invalid {
		t.Run("invalid/"+name, func(t *testing.T) {
			for route, send := range map[string]func() int{
				"edit channel": func() int {
					return do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken, channelPatchBody(t, st, "c1", body)).StatusCode
				},
				"programming preview": func() int {
					return do(t, srv, http.MethodPost, "/v1/channels/c1/programming/preview", adminToken, body).StatusCode
				},
				"programming changes": func() int {
					return do(t, srv, http.MethodPost, "/v1/channels/c1/programming/changes", adminToken, body).StatusCode
				},
			} {
				if got := send(); got != http.StatusUnprocessableEntity {
					t.Errorf("%s with %s = %d, want 422", route, name, got)
				}
			}
		})
	}
	for name, body := range valid {
		t.Run("valid/"+name, func(t *testing.T) {
			for route, send := range map[string]func() int{
				"edit channel": func() int {
					return do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken, channelPatchBody(t, st, "c1", body)).StatusCode
				},
				"programming preview": func() int {
					return do(t, srv, http.MethodPost, "/v1/channels/c1/programming/preview", adminToken, body).StatusCode
				},
			} {
				if got := send(); got != http.StatusOK {
					t.Errorf("%s with %s = %d, want 200", route, name, got)
				}
			}
		})
	}
}
