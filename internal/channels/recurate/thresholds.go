package recurate

import (
	"context"

	"github.com/loomarr/loomarr/internal/platform/settings"
)

// SettingsThresholds reads Thresholds from the live settings (§8.2). It reads PER CALL, so raising
// recurate.min_score_pct / recurate.max_titles takes effect without a restart, like every other
// setting (config-design §3).
type SettingsThresholds struct{ set settings.Reader }

// NewSettingsThresholds returns the thresholds over the live settings view.
func NewSettingsThresholds(set settings.Reader) SettingsThresholds {
	return SettingsThresholds{set: set}
}

// MinScorePct is the minimum match score, as a percentage, a title needs to be kept.
func (t SettingsThresholds) MinScorePct(context.Context) int {
	return settings.RecurateMinScorePct.Get(t.set)
}

// MaxTitles is the most titles a self-updating channel keeps.
func (t SettingsThresholds) MaxTitles(context.Context) int {
	return settings.RecurateMaxTitles.Get(t.set)
}
