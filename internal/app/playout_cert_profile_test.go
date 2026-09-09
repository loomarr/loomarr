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
		if config.rendition() != playout.CanonicalPreparedRendition(tier) || config.sourceProfile() != playout.Resolve(tier, playout.EncoderSoftware, 2, 0) {
			t.Fatalf("declared tier %s diverges from production policy", tier)
		}
	}
	if err := (PlayoutCertificationConfig{QualityTier: "unknown"}).validateTier(); err == nil {
		t.Fatal("unknown tier silently selected another profile")
	}
}

func TestCertificationLiveProfileRespondsToLoad(t *testing.T) {
	active := 0
	resolver := syntheticLiveResolver{profile: func() playout.Profile {
		return playout.Resolve(playout.TierBalanced, playout.EncoderVAAPI, 12, active)
	}}
	initial := resolver.Profile(context.Background())
	active = 10
	loaded := resolver.Profile(context.Background())
	if initial.Height != 1080 || loaded.Height != 720 || initial.Encoder != playout.EncoderVAAPI || loaded.Encoder != initial.Encoder {
		t.Fatalf("load policy did not reach encoder: initial=%+v loaded=%+v", initial, loaded)
	}
}

func TestCertificationProfileEvidenceKeepsProbeAndPreparationIndependent(t *testing.T) {
	evidence := certificationProfileEvidence(PlayoutCertificationConfig{QualityTier: playout.TierBalanced}, playout.DefaultProfile(),
		playout.Capacity{Chosen: playout.EncoderVAAPI, MaxChannels: 12})
	if evidence.Probe.Height != 720 || evidence.Prepared.Height != 1080 || evidence.Probe.VideoBitrateKbps == evidence.Prepared.VideoBitrateKbps {
		t.Fatalf("probe replaced the canonical prepared profile: %+v", evidence)
	}
}
