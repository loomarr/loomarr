package llm

import (
	"errors"
	"testing"
)

func TestRoleResolverUsesExactCertifiedRouteAndLanePolicy(t *testing.T) {
	resolver, err := NewRoleResolver(RolePolicySnapshot{
		Version: "policy-2026-08-24", CertificationArtifact: "filler-bakeoff-v1",
		Routes: map[InferenceRole]RoleRoute{
			RoleFillerVideo: {
				Provider: "openrouter", Model: "google/gemini-3.7-flash", UpstreamProvider: "Google AI Studio",
				Modalities: []string{"text", "video", "audio"}, StructuredOutput: true, RequireZDR: true,
				AllowResilienceFallbacks: true,
				Limits:                   InferenceLimits{MaxInputBytes: 12 << 20, MaxDurationMS: 60_000, MaxOutputTokens: 512, MaxAttempts: 2, MaxChargeNanoUSD: 20_000_000},
			},
		},
	}, CompatibilitySnapshot{
		Version: "openrouter-models-2026-08-24",
		Models: []ModelCompatibility{{
			Provider: "openrouter", Model: "google/gemini-3.7-flash",
			Modalities: []string{"text", "image", "video", "audio"}, StructuredOutput: true, ZDR: true,
			UpstreamProviders: []string{"Google", "Google AI Studio"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	cert, err := resolver.Resolve(RoleFillerVideo, LaneCertification, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !cert.Certified || cert.AllowFallbacks || cert.PolicyVersion != "policy-2026-08-24" || cert.CapabilityVersion != "openrouter-models-2026-08-24" {
		t.Fatalf("certification route = %+v", cert)
	}
	resilient, err := resolver.Resolve(RoleFillerVideo, LaneResilience, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !resilient.AllowFallbacks || resilient.Certified {
		t.Fatalf("resilience route = %+v", resilient)
	}
}

func TestRoleResolverAdvancedOverrideIsUnverifiedAndCapabilityChecked(t *testing.T) {
	resolver, err := NewRoleResolver(RolePolicySnapshot{
		Version: "p1", CertificationArtifact: "report-1",
		Routes: map[InferenceRole]RoleRoute{
			RoleFillerFrames: {Provider: "openrouter", Model: "openai/gpt-5-mini", Modalities: []string{"text", "image"}, StructuredOutput: true},
		},
	}, CompatibilitySnapshot{
		Version: "c1",
		Models: []ModelCompatibility{
			{Provider: "openrouter", Model: "openai/gpt-5-mini", Modalities: []string{"text", "image"}, StructuredOutput: true},
			{Provider: "openrouter", Model: "google/gemini-3.1-flash-lite", Modalities: []string{"text", "image", "video"}, StructuredOutput: true},
			{Provider: "openrouter", Model: "old/text-only", Modalities: []string{"text"}, StructuredOutput: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	override, err := resolver.Resolve(RoleFillerFrames, LaneResilience, &RouteOverride{
		Provider: "openrouter", Model: "google/gemini-3.1-flash-lite",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !override.Overridden || override.Certified || override.Model != "google/gemini-3.1-flash-lite" {
		t.Fatalf("override = %+v", override)
	}
	_, err = resolver.Resolve(RoleFillerFrames, LaneResilience, &RouteOverride{Provider: "openrouter", Model: "old/text-only"})
	var unavailable *RouteUnavailableError
	if !errors.As(err, &unavailable) || unavailable.Role != RoleFillerFrames {
		t.Fatalf("incompatible override error = %v", err)
	}
}

func TestRoleResolverRejectsUncertifiedAutomaticPolicy(t *testing.T) {
	_, err := NewRoleResolver(RolePolicySnapshot{
		Version: "p1",
		Routes:  map[InferenceRole]RoleRoute{RoleLineup: {Provider: "openrouter", Model: "openai/gpt-5-mini", Modalities: []string{"text"}}},
	}, CompatibilitySnapshot{
		Version: "c1",
		Models:  []ModelCompatibility{{Provider: "openrouter", Model: "openai/gpt-5-mini", Modalities: []string{"text"}}},
	})
	if err == nil {
		t.Fatal("automatic policy without a certification artifact must be rejected")
	}
}

func TestRoleResolverOverrideCannotDropCertifiedUpstreamPin(t *testing.T) {
	t.Parallel()
	resolver, err := NewRoleResolver(RolePolicySnapshot{
		Version: "p1", CertificationArtifact: "report-1",
		Routes: map[InferenceRole]RoleRoute{RoleFillerFrames: {
			Provider: "openrouter", Model: "model-a", UpstreamProvider: "provider-a",
			Modalities: []string{"text", "image"},
		}},
	}, CompatibilitySnapshot{Version: "c1", Models: []ModelCompatibility{
		{Provider: "openrouter", Model: "model-a", Modalities: []string{"text", "image"}, UpstreamProviders: []string{"provider-a"}},
		{Provider: "openrouter", Model: "model-b", Modalities: []string{"text", "image"}, UpstreamProviders: []string{"provider-b"}},
	}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = resolver.Resolve(RoleFillerFrames, LaneResilience, &RouteOverride{Provider: "openrouter", Model: "model-b"})
	var unavailable *RouteUnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("override silently dropped provider pin: %v", err)
	}
}
