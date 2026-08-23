package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// The final §17 tranche: the domain counters that aren't latencies and aren't
// store state — LLM token usage, filler fallback-ladder depth, and slot-drift
// substitutions. Each is hooked at its own subsystem's natural point.

var (
	// llmTokens counts tokens consumed by grounded generation, split prompt vs
	// completion (§17 LLM tokens). Cost is deliberately NOT a metric: it is
	// tokens × a per-model posted rate that drifts and is hosted/model-specific,
	// so it belongs in a dashboard recording rule over this series, not baked
	// into the request path where it would rot.
	llmTokens = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loomarr", Subsystem: "llm", Name: "tokens_total",
		Help: "LLM tokens consumed by grounded generation, by kind.",
	}, []string{"kind"})

	// fillerPods counts assembled filler pods by the fallback-ladder rung reached
	// (§17: how often matching degrades). exact = a clean era+audience match;
	// bumper_card = the bottom rung, only the embedded card.
	fillerPods = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loomarr", Subsystem: "filler", Name: "pods_total",
		Help: "Assembled filler pods by fallback-ladder match level.",
	}, []string{"match_level"})

	// fillerRotationAirings counts actual internal-playout clip starts, never previews or
	// reconciles. Both labels are bounded domain states; channel and clip identities are omitted.
	fillerRotationAirings = promauto.NewCounterVec(prometheus.CounterOpts{
		Namespace: "loomarr", Subsystem: "filler", Name: "rotation_airings_total",
		Help: "Actual filler clip airings by repeat state and cooldown pressure.",
	}, []string{"repeat", "cooldown"})

	// slotSubstitutions counts scheduled programs demoted to a placeholder because
	// the title vanished from the library between reconciles (§17 slot drift).
	slotSubstitutions = promauto.NewCounter(prometheus.CounterOpts{
		Namespace: "loomarr", Subsystem: "channel", Name: "slot_substitutions_total",
		Help: "Scheduled programs demoted to a placeholder because the title vanished (slot drift).",
	})
)

// LLMTokens records prompt/completion token usage from one generation call. A
// zero count (a provider that doesn't report usage) is skipped, so the series
// reflects real accounting rather than phantom zeros.
func LLMTokens(promptTokens, completionTokens int) {
	if promptTokens > 0 {
		llmTokens.WithLabelValues("prompt").Add(float64(promptTokens))
	}
	if completionTokens > 0 {
		llmTokens.WithLabelValues("completion").Add(float64(completionTokens))
	}
}

// FillerPodAssembled records one assembled pod under its ladder match level.
func FillerPodAssembled(matchLevel string) { fillerPods.WithLabelValues(matchLevel).Inc() }

// FillerRotationAired records one actual clip start. A pinned recent clip is an intentional
// override, while relaxed means the unpinned pool was depleted inside the preferred cooldown.
func FillerRotationAired(repeated, relaxed, pinned bool) {
	repeat := "fresh"
	if repeated {
		repeat = "repeat"
	}
	cooldown := "ready"
	if pinned && repeated {
		cooldown = "override"
	} else if relaxed {
		cooldown = "relaxed"
	}
	fillerRotationAirings.WithLabelValues(repeat, cooldown).Inc()
}

// SlotSubstitutions records n programs demoted to placeholders in one reconcile.
func SlotSubstitutions(n int) {
	if n > 0 {
		slotSubstitutions.Add(float64(n))
	}
}
