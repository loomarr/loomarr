package api_test

import (
	"context"
	"io"
	"net/http"
	"testing"
)

// A channel edit naming a collection the library has no row for (one deleted in another tab, say)
// is refused with a 409 and saves nothing (#2793): the channel's named collections are foreign keys.
func TestChannelEditNamingAnUnknownCollectionIsRefused(t *testing.T) {
	harness := newChannelsHarness(t)
	srv, st := harness.Server, harness.Store
	mkChannel(t, srv, "c1", "Cartoons", 5)
	before, err := st.GetChannel(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}

	for name, policy := range map[string]string{
		"scope": `{"policy":{"scope":{"collections":["90000777"]}}}`,
		"rule":  `{"policy":{"rules":[{"id":"r1","label":"Weekends","source":"operator","when":{"weekend":true},"what":{"collections":["90000778"]}}]}}`,
	} {
		resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken, channelPatchBody(t, st, "c1", policy))
		if resp.StatusCode != http.StatusConflict {
			body, _ := io.ReadAll(resp.Body)
			t.Errorf("%s naming an unknown collection → %d %s, want 409", name, resp.StatusCode, body)
		}
	}
	after, err := st.GetChannel(context.Background(), "c1")
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || len(after.Policy.Scope.Collections) != 0 || len(after.Policy.Rules) != 0 {
		t.Errorf("channel after refused edits = revision %d, policy %+v; want it untouched at %d", after.Revision, after.Policy, before.Revision)
	}
}
