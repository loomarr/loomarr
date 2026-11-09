package httpcore

import (
	"context"

	"github.com/loomarr/loomarr/internal/platform/settings"
)

// FeatureGate is the settings read the suggestion readiness check needs.
type FeatureGate interface {
	Features(ctx context.Context) map[string]bool
	// MissingRequirements explains a false feature verdict with the registry-owned keys.
	MissingRequirements(ctx context.Context, feature string) []string
}

// SuggestionConfigurationError maps the settings-owned readiness explanation to one user-facing
// recovery contract. Proposal creation and channel refinement share it so a missing grounding
// source cannot be called an AI failure on one surface and something else on another. A nil gate
// (unit tests) reports ready, so a wired dependency still serves.
func SuggestionConfigurationError(ctx context.Context, gate FeatureGate) error {
	if gate == nil || gate.Features(ctx)["suggestions"] {
		return nil
	}
	aiMissing := false
	tmdbMissing := false
	for _, key := range gate.MissingRequirements(ctx, "suggestions") {
		switch key {
		case settings.LLMProvider.Key(), settings.LLMModel.Key():
			aiMissing = true
		case settings.TMDBAPIKey.Key():
			tmdbMissing = true
		}
	}
	if tmdbMissing && !aiMissing {
		return GroundingNotConfigured(
			"TMDB is needed for channel suggestions",
			"Connect TMDB in Settings → Connections so Loomarr can match channel descriptions to real titles.",
		)
	}
	return FeatureNotConfigured(
		"AI isn't set up",
		"Connect an AI provider and select a tool-capable lineup model in Settings → AI to build channels from a sentence.",
	)
}
