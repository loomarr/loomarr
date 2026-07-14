package llm

import "sort"

// This file is the curated, in-code catalog of LOCAL (Ollama) models Loomarr knows
// how to recommend (§8.1). It is deliberately NOT a live registry scrape: a fixed,
// reviewable list makes the "best model for this machine" recommendation
// deterministic and auditable, and lets us encode Loomarr-specific truth (which
// models actually tool-call well, which need a newer Ollama) that a registry can't.
//
// Keep this list short and honest — every entry must be a model we've reason to
// believe does grounded tool-calling + JSON. When a better current-gen model lands,
// add it here (that's the sanctioned update path), don't reach for a registry API.

// ModelInfo describes one catalog model. Sizes are approximate on-disk/VRAM
// footprint for the default quant Ollama pulls (Q4_K_M unless noted) — enough to
// rank fit, not an exact allocator.
type ModelInfo struct {
	// Tag is the exact Ollama pull tag (what `ollama pull <tag>` / LLM_MODEL wants).
	Tag string `json:"tag"`
	// Label is a human name for the UI.
	Label string `json:"label"`
	// ApproxVRAMGiB is the rough resident footprint of the default quant. Fit is
	// judged against detected GPU VRAM; a model needs headroom for the context
	// window on top of the weights, which the "tight" band accounts for.
	ApproxVRAMGiB float64 `json:"approxVramGiB"`
	// MinOllama is the minimum Ollama version that runs this model well ("" = any
	// reasonably current release). Compared lexically-by-segment (see versionAtLeast).
	MinOllama string `json:"minOllama,omitempty"`
	// Why is a one-line rationale shown in the UI ("current-gen, official Tools badge").
	Why string `json:"why"`
	// Rank orders recommendation preference among models that fit — higher is more
	// preferred (better tool-caller / more current). The recommended default is the
	// highest-ranked model that fits AND the runtime supports.
	Rank int `json:"-"`
}

// localCatalog is the curated truth. Ordered loosely best→simplest for readability;
// Rank (not slice order) drives recommendation. Footprints are for the default quant.
//
// Rationale for the current picks (2026-07, RTX-3080-Ti-class 12GB target):
//   - qwen3:8b            — strong tool-caller, current-gen, runs on today's Ollama.
//   - qwen3.5:9b          — newer, official Tools badge, but needs a newer runtime.
//   - llama3.1:8b         — reliable fallback, universally compatible.
//   - qwen3:14b (Q6_K)    — best quality that still fits 12GB with a modest context;
//     Q6 because stock Q4 degrades tool-calling/JSON (§8/§15).
var localCatalog = []ModelInfo{
	{Tag: "qwen3.5:9b", Label: "Qwen3.5 9B", ApproxVRAMGiB: 6.6, MinOllama: "0.14.0", Rank: 40,
		Why: "Current-gen, official tool-calling; needs a newer Ollama runtime."},
	{Tag: "qwen3:14b", Label: "Qwen3 14B (Q6_K)", ApproxVRAMGiB: 12.1, MinOllama: "", Rank: 35,
		Why: "Highest quality that fits a 12GB card; use the Q6_K quant (Q4 degrades tool-use)."},
	{Tag: "qwen3:8b", Label: "Qwen3 8B", ApproxVRAMGiB: 5.2, MinOllama: "", Rank: 30,
		Why: "Strong tool-caller, current-gen, runs on today's Ollama — a safe default."},
	{Tag: "llama3.1:8b", Label: "Llama 3.1 8B", ApproxVRAMGiB: 4.9, MinOllama: "", Rank: 10,
		Why: "Reliable, universally compatible fallback."},
}

// Fit is a coarse verdict of whether a model fits the detected hardware.
type Fit string

const (
	FitFits    Fit = "fits"     // comfortable headroom for weights + context
	FitTight   Fit = "tight"    // runs, but little headroom (small context / possible spill)
	FitWontFit Fit = "wont_fit" // weights exceed detected VRAM — will spill to CPU / OOM
)

// Fit-band tuning. A model needs its weights PLUS room for the KV-cache/context on
// top; we treat "fits" as needing ~1.3× the weight footprint in VRAM, and "tight"
// as anything from the bare weights up to that. Below the bare weights → won't fit.
//
// TODO(maintainer): these multipliers are a judgment call — 1.3× headroom is
// conservative-ish for an 8k context on an 8B model. If you'd rather bias toward
// recommending bigger models on a 12GB card (accepting a smaller context), lower
// fitsHeadroom toward 1.15. Kept explicit so it's easy to tune from real runs.
const (
	fitsHeadroom = 1.30 // VRAM ≥ weights×this ⇒ "fits"
)

// classifyFit judges a model against detected VRAM. vramGiB<=0 means unknown
// (no GPU detected / probe failed) → we never claim "fits" or "won't fit" on
// guesswork; everything becomes "tight" so the UI shows the catalog without lying.
func classifyFit(m ModelInfo, vramGiB float64) Fit {
	if vramGiB <= 0 {
		return FitTight
	}
	switch {
	case vramGiB >= m.ApproxVRAMGiB*fitsHeadroom:
		return FitFits
	case vramGiB >= m.ApproxVRAMGiB:
		return FitTight
	default:
		return FitWontFit
	}
}

// CatalogEntry is one model annotated for a specific machine (fit + availability).
type CatalogEntry struct {
	ModelInfo
	Fit         Fit  `json:"fit"`
	Pulled      bool `json:"pulled"`      // already present in the local Ollama
	RuntimeOK   bool `json:"runtimeOk"`   // the detected Ollama version satisfies MinOllama
	Recommended bool `json:"recommended"` // the single best-fit default for this machine
}

// annotateCatalog scores every catalog model against a probe result and flags the
// single recommended default: the highest-Rank model that both fits comfortably AND
// the runtime supports. If none "fits", it falls back to the highest-Rank "tight"
// model the runtime supports (better a working tight model than no recommendation);
// if the runtime supports none, nothing is recommended (the UI then explains why).
func annotateCatalog(p Probe) []CatalogEntry {
	pulled := map[string]bool{}
	for _, t := range p.PulledModels {
		pulled[t] = true
	}
	entries := make([]CatalogEntry, 0, len(localCatalog))
	for _, m := range localCatalog {
		runtimeOK := versionAtLeast(p.OllamaVersion, m.MinOllama)
		entries = append(entries, CatalogEntry{
			ModelInfo: m,
			Fit:       classifyFit(m, p.VRAMGiB),
			Pulled:    pulled[m.Tag],
			RuntimeOK: runtimeOK,
		})
	}
	// Pick the recommendation: prefer a comfortably-fitting, runtime-supported model
	// with the highest Rank; fall back to a tight one if nothing fits comfortably.
	best := -1
	bestScore := -1
	for i, e := range entries {
		if !e.RuntimeOK || e.Fit == FitWontFit {
			continue
		}
		// Comfortable fit outranks a tight one regardless of Rank, then Rank breaks ties.
		score := e.Rank
		if e.Fit == FitFits {
			score += 1000
		}
		if score > bestScore {
			bestScore, best = score, i
		}
	}
	if best >= 0 {
		entries[best].Recommended = true
	}
	// Stable display order: recommended first, then fit (fits>tight>wont), then Rank.
	sort.SliceStable(entries, func(a, b int) bool {
		if entries[a].Recommended != entries[b].Recommended {
			return entries[a].Recommended
		}
		if fitOrder(entries[a].Fit) != fitOrder(entries[b].Fit) {
			return fitOrder(entries[a].Fit) < fitOrder(entries[b].Fit)
		}
		return entries[a].Rank > entries[b].Rank
	})
	return entries
}

func fitOrder(f Fit) int {
	switch f {
	case FitFits:
		return 0
	case FitTight:
		return 1
	default:
		return 2
	}
}
