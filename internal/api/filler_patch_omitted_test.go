package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
)

// An edit that names neither era nor audience leaves both alone and records no operator evidence
// for them: an erased era or audience would cost a channel its fit, and operator evidence outranks
// every later machine guess.
func TestFiller_PatchWithoutEraOrAudienceKeepsBoth(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"role only", `{"hash":"o1","role":"psa"}`},
		{"brand only", `{"hash":"o1","brand":"Acme"}`},
		{"name only", `{"hash":"o1","name":"A new name"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, st, _ := newFillerServer(t)
			seedClip(t, st, "o1", clips.Commercial, 1994, clips.Kids, "toys")

			if resp := do(t, srv, http.MethodPatch, "/v1/filler/tags", adminToken, tc.body); resp.StatusCode != http.StatusOK {
				t.Fatalf("patch → %d, want 200", resp.StatusCode)
			}
			got, err := st.GetClip(context.Background(), "o1")
			if err != nil {
				t.Fatal(err)
			}
			if got.Era != 1994 || got.Audience != clips.Kids {
				t.Errorf("era/audience = %d/%q, want 1994/kids left alone", got.Era, got.Audience)
			}
			states, err := st.ListFillerEnrichment(context.Background(), "o1")
			if err != nil {
				t.Fatal(err)
			}
			for _, state := range states {
				if state.Evidence.Kind == enrichment.EvidenceOperator &&
					(state.Axis == enrichment.AxisEra || state.Axis == enrichment.AxisAudience) {
					t.Errorf("an edit that never named %s recorded operator evidence for it: %+v", state.Axis, state)
				}
			}
		})
	}
}

// A field the edit names is written alone: era leaves the audience, audience leaves the era, and an
// explicit 0 or empty string clears just that field and records the operator's choice.
func TestFiller_PatchWritesOnlyTheNamedField(t *testing.T) {
	for _, tc := range []struct {
		name         string
		body         string
		wantEra      int
		wantAudience clips.Audience
		wantAxis     enrichment.Axis
		otherAxis    enrichment.Axis
	}{
		{"era only", `{"hash":"w1","era":1988}`, 1988, clips.Kids, enrichment.AxisEra, enrichment.AxisAudience},
		{"audience only", `{"hash":"w1","audience":"general"}`, 1994, clips.General, enrichment.AxisAudience, enrichment.AxisEra},
		{"era cleared", `{"hash":"w1","era":0}`, 0, clips.Kids, enrichment.AxisEra, enrichment.AxisAudience},
		{"audience cleared", `{"hash":"w1","audience":""}`, 1994, "", enrichment.AxisAudience, enrichment.AxisEra},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, st, _ := newFillerServer(t)
			seedClip(t, st, "w1", clips.Commercial, 1994, clips.Kids, "toys")
			if resp := do(t, srv, http.MethodPatch, "/v1/filler/tags", adminToken, tc.body); resp.StatusCode != http.StatusOK {
				t.Fatalf("patch → %d, want 200", resp.StatusCode)
			}
			got, err := st.GetClip(context.Background(), "w1")
			if err != nil {
				t.Fatal(err)
			}
			if got.Era != tc.wantEra || got.Audience != tc.wantAudience {
				t.Errorf("era/audience = %d/%q, want %d/%q", got.Era, got.Audience, tc.wantEra, tc.wantAudience)
			}
			states, err := st.ListFillerEnrichment(context.Background(), "w1")
			if err != nil {
				t.Fatal(err)
			}
			recorded := map[enrichment.Axis]bool{}
			for _, state := range states {
				if state.Evidence.Kind == enrichment.EvidenceOperator {
					recorded[state.Axis] = true
				}
			}
			if !recorded[tc.wantAxis] || recorded[tc.otherAxis] {
				t.Errorf("operator evidence on %v, want it on %s alone", recorded, tc.wantAxis)
			}
		})
	}
}
