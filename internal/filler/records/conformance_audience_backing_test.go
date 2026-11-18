package records

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
)

// A clip's audience is backed or a lone guess (#3838 decision 7). The store decides on every read
// from the evidence rows: an operator, a listing, a source or a rule cue backs it, and so does a
// cue from a second source that agrees with the model; one model's answer alone does not. A guess
// that a late-night cue contradicts reads as late-night.
func testAudienceBacking(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	at := time.Unix(1_700_000_000, 0).UTC()

	state := func(hash string, axis enrichment.Axis, value enrichment.Value, kind enrichment.EvidenceKind, producer string) enrichment.State {
		return enrichment.State{ClipHash: hash, Axis: axis, Status: enrichment.StatusComplete, Value: value,
			Evidence: enrichment.Evidence{Kind: kind, Reference: "test." + producer, Confidence: 90,
				Producer: producer, ProducerVersion: "1", ObservedAt: at}}
	}
	kids := enrichment.Value{Text: "kids"}
	cue := func(slug string) enrichment.Value { return enrichment.Value{Tags: []string{slug}} }

	for _, tc := range []struct {
		name         string
		states       func(hash string) []enrichment.State
		wantGuess    bool
		wantAudience clips.Audience
	}{
		{"no evidence at all", func(string) []enrichment.State { return nil }, true, clips.Kids},
		{"a model's answer alone", func(h string) []enrichment.State {
			return []enrichment.State{state(h, enrichment.AxisAudience, kids, enrichment.EvidenceInference, "text-model")}
		}, true, clips.Kids},
		{"a cue the same model gave is not a second signal", func(h string) []enrichment.State {
			return []enrichment.State{
				state(h, enrichment.AxisAudience, kids, enrichment.EvidenceInference, "text-model"),
				state(h, enrichment.AxisAudienceCue, cue("kids-cue"), enrichment.EvidenceInference, "text-model"),
			}
		}, true, clips.Kids},
		{"a rule cue backs it", func(h string) []enrichment.State {
			return []enrichment.State{
				state(h, enrichment.AxisAudience, kids, enrichment.EvidenceInference, "text-model"),
				state(h, enrichment.AxisAudienceCue, cue("kids-cue"), enrichment.EvidenceItem, "deterministic"),
			}
		}, false, clips.Kids},
		{"a frame-read cue is a second signal", func(h string) []enrichment.State {
			return []enrichment.State{
				state(h, enrichment.AxisAudience, kids, enrichment.EvidenceInference, "text-model"),
				state(h, enrichment.AxisAudienceCue, cue("family-cue"), enrichment.EvidenceContent, "vision"),
			}
		}, false, clips.Kids},
		{"an operator backs it", func(h string) []enrichment.State {
			return []enrichment.State{state(h, enrichment.AxisAudience, kids, enrichment.EvidenceOperator, "operator")}
		}, false, clips.Kids},
		{"a late-night cue contradicts a guess, and the stricter reading wins", func(h string) []enrichment.State {
			return []enrichment.State{
				state(h, enrichment.AxisAudience, kids, enrichment.EvidenceInference, "text-model"),
				state(h, enrichment.AxisAudienceCue, cue("late-night-cue"), enrichment.EvidenceItem, "deterministic"),
			}
		}, true, clips.LateNight},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStore(t)
			c := sampleClip("aud-"+tc.name, "Audience", clips.Commercial, 1992, clips.Kids, "")
			putClip(t, s, c)
			for _, st := range tc.states(c.Hash) {
				if _, _, err := s.ApplyFillerEnrichment(ctx, st, at); err != nil {
					t.Fatal(err)
				}
			}
			got, err := s.GetClip(ctx, c.Hash)
			if err != nil {
				t.Fatal(err)
			}
			if got.AudienceGuess != tc.wantGuess || got.Audience != tc.wantAudience {
				t.Errorf("guess/audience = %v/%q, want %v/%q", got.AudienceGuess, got.Audience, tc.wantGuess, tc.wantAudience)
			}
		})
	}
}
