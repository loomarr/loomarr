package httpx

import "slices"

// Target names the remote service a client talks to. It is the identity every
// outbound metric is labelled with, so a caller picks one of the constants below
// instead of inventing a free string; a service the recorder does not know would
// otherwise collapse into target="other" and hide which dependency is failing.
//
// The string values are stable names; Label maps them onto the bounded
// metric label set the Recorder accepts. Adding a service means adding its
// constant here and its label to the Recorder's closed set (TestEveryTargetHasItsOwnMetricLabel
// fails until both agree).
type Target string

// The services Loomarr calls.
const (
	TargetLibrary       Target = "library"
	TargetSeerr         Target = "seerr"
	TargetTMDB          Target = "tmdb"
	TargetArr           Target = "arr"
	TargetTunarr        Target = "tunarr"
	TargetTunarrBulk    Target = "tunarr-bulk"
	TargetLLM           Target = "llm"
	TargetLLMVideo      Target = "llm-video"
	TargetNotifications Target = "notification-provider"
	TargetReference     Target = "reference"

	TargetFillerWebSearch          Target = "filler_web_search"
	TargetFillerStructuredResearch Target = "filler_structured_research"
	TargetFillerStructureWindow    Target = "filler-structure-window-openrouter"
	TargetFillerOpenRouterSnapshot Target = "filler-openrouter-snapshot"
	TargetFillerReviewOllama       Target = "filler-review-ollama"
	TargetFillerReviewOpenRouter   Target = "filler-review-openrouter"
)

// targetLabels maps each Target to its metric label. Several Targets can share
// one label when they are the same remote service (a bulk read and a CRUD call
// to Tunarr) or the same kind of work.
var targetLabels = map[Target]string{
	TargetLibrary:       "library",
	TargetSeerr:         "seerr",
	TargetTMDB:          "tmdb",
	TargetArr:           "arr",
	TargetTunarr:        "tunarr",
	TargetTunarrBulk:    "tunarr",
	TargetLLM:           "llm",
	TargetLLMVideo:      "llm",
	TargetNotifications: "notifications",
	TargetReference:     "reference",

	TargetFillerWebSearch:          "filler_research",
	TargetFillerStructuredResearch: "filler_research",
	TargetFillerStructureWindow:    "filler_openrouter",
	TargetFillerOpenRouterSnapshot: "filler_openrouter",
	TargetFillerReviewOllama:       "filler_review",
	TargetFillerReviewOpenRouter:   "filler_review",
}

// Label is the bounded metric label for the target; an unregistered Target is "other".
func (t Target) Label() string {
	if label, ok := targetLabels[t]; ok {
		return label
	}
	return "other"
}

// Targets lists every registered Target in a stable order.
func Targets() []Target {
	out := make([]Target, 0, len(targetLabels))
	for target := range targetLabels {
		out = append(out, target)
	}
	slices.Sort(out)
	return out
}
