package app

import (
	"context"
	"testing"

	"github.com/loomarr/loomarr/internal/playout"
)

func TestCertificationDeclaredTierUsesProductionPolicy(t *testing.T) {
	for _, tier := range []playout.Tier{playout.TierEfficient, playout.TierBalanced, playout.TierQuality} {
		config := PlayoutCertificationConfig{QualityTier: tier}
		if err := config.validateTier(); err != nil {
			t.Fatal(err)
		}
		if config.rendition() != playout.CanonicalPreparedRendition(tier) || config.sourceProfile() != playout.Resolve(tier, playout.EncoderSoftware, 0) {
			t.Fatalf("declared tier %s diverges from production policy", tier)
		}
	}
	if err := (PlayoutCertificationConfig{QualityTier: "unknown"}).validateTier(); err == nil {
		t.Fatal("unknown tier silently selected another profile")
	}
}

func TestCertificationLiveProfileFollowsTheAdmittedRung(t *testing.T) {
	resolver := syntheticLiveResolver{profile: func(rung int) playout.Profile {
		return playout.Resolve(playout.TierBalanced, playout.EncoderVAAPI, rung)
	}}
	initial := resolver.Profile(context.Background(), 0)
	dropped := resolver.Profile(context.Background(), 2)
	if initial.Height != 1080 || dropped.Height != 720 || initial.Encoder != playout.EncoderVAAPI || dropped.Encoder != initial.Encoder {
		t.Fatalf("rung did not reach encoder: initial=%+v dropped=%+v", initial, dropped)
	}
}

func TestCertificationProfileEvidenceKeepsProbeAndPreparationIndependent(t *testing.T) {
	evidence := certificationProfileEvidence(PlayoutCertificationConfig{QualityTier: playout.TierBalanced}, playout.DefaultProfile(),
		playout.Capacity{Chosen: playout.EncoderVAAPI, MaxChannels: 12})
	if evidence.Probe.Height != 720 || evidence.Prepared.Height != 1080 || evidence.Probe.VideoBitrateKbps == evidence.Prepared.VideoBitrateKbps {
		t.Fatalf("probe replaced the canonical prepared profile: %+v", evidence)
	}
}
