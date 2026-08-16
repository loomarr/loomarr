package llm

import (
	"sort"
	"strconv"
	"strings"
)

// The LOCAL model catalog is discovered LIVE, not hardcoded (§8.1). There is no
// registry API for "what can I download" — Ollama's /api/search is unshipped and no
// tool (Open WebUI included) enumerates a downloadable list; the ecosystem pattern is
// "browse ollama.com, pull by name, and let the app tell you what fits". So Loomarr
// builds this catalog from what the operator has actually PULLED (probe.Installed via
// /api/tags), exposes Ollama's TOOL-CAPABILITY evidence (/api/show capabilities), and
// ranks verified options by VRAM fit using each model's real on-disk size. Unsupported
// and unverified models remain visible so the UI can explain or verify them, but they
// are never recommended for grounded curation. Discovery + pull-by-name live in
// probe.go / the pull endpoint.

// ModelInfo describes one catalog model, built from the live probe.
type ModelInfo struct {
	// Tag is the exact Ollama pull tag (what `ollama pull <tag>` / LLM_MODEL wants).
	Tag string `json:"tag"`
	// Label is a human name for the UI (derived from the tag).
	Label string `json:"label"`
	// ApproxVRAMGiB is the resident footprint — the model's real on-disk size (a
	// good VRAM proxy), from /api/tags. Fit is judged against detected GPU VRAM.
	ApproxVRAMGiB float64 `json:"approxVramGiB"`
	// Why is a one-line rationale shown in the UI (family · params · quant).
	Why string `json:"why"`
	// Rank orders recommendation preference among models that fit — higher is more
	// preferred. Derived from parameter size (bigger ⇒ more capable, within fit).
	Rank int `json:"-"`
}

// localCatalog is the curated truth. Ordered loosely best→simplest for readability;
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
	Fit            Fit            `json:"fit"`
	Pulled         bool           `json:"pulled"`         // already present in the local Ollama
	RuntimeOK      bool           `json:"runtimeOk"`      // an installed model runs on the installed Ollama (always true here)
	Tools          bool           `json:"tools"`          // Ollama reports tool-calling — REQUIRED to ground suggestions
	ToolCapability ToolCapability `json:"toolCapability"` // verified|unsupported|unverified
	Recommended    bool           `json:"recommended"`    // the single best-fit default for this machine
}

// annotateCatalog builds the local catalog LIVE from the probe's installed models
// (§8.1): every installed model is included with its capability evidence, and each is
// ranked by VRAM fit from its real on-disk size. It flags the single recommended
// VERIFIED default: the largest model that fits comfortably; failing that, the largest
// verified model that fits tight; nothing if no verified model fits.
func annotateCatalog(p Probe) []CatalogEntry {
	entries := make([]CatalogEntry, 0, len(p.Installed))
	for _, m := range p.Installed {
		capability := normalizedInstalledCapability(m)
		info := ModelInfo{
			Tag:           m.Tag,
			Label:         modelLabel(m.Tag),
			ApproxVRAMGiB: m.VRAMGiB,
			Why:           installedWhy(m, capability),
			Rank:          paramRank(m.ParameterSize),
		}
		entries = append(entries, CatalogEntry{
			ModelInfo:      info,
			Fit:            classifyFit(info, p.VRAMGiB),
			Pulled:         true, // everything discovered here is, by definition, pulled
			RuntimeOK:      true, // the installed model runs on the installed Ollama
			Tools:          capability == ToolCapabilityVerified,
			ToolCapability: capability,
		})
	}
	// Recommend the largest comfortably-fitting model (bigger ⇒ more capable), falling
	// back to the largest that at least fits tight; none fit ⇒ no recommendation.
	best := -1
	bestScore := -1
	for i, e := range entries {
		if e.Fit == FitWontFit || e.ToolCapability != ToolCapabilityVerified {
			continue
		}
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
		if capabilityOrder(entries[a].ToolCapability) != capabilityOrder(entries[b].ToolCapability) {
			return capabilityOrder(entries[a].ToolCapability) < capabilityOrder(entries[b].ToolCapability)
		}
		if fitOrder(entries[a].Fit) != fitOrder(entries[b].Fit) {
			return fitOrder(entries[a].Fit) < fitOrder(entries[b].Fit)
		}
		return entries[a].Rank > entries[b].Rank
	})
	return entries
}

func normalizedInstalledCapability(model InstalledModel) ToolCapability {
	if model.ToolCapability != "" {
		return model.ToolCapability
	}
	if model.Tools {
		return ToolCapabilityVerified
	}
	return ToolCapabilityUnverified
}

func capabilityOrder(capability ToolCapability) int {
	switch capability {
	case ToolCapabilityVerified:
		return 0
	case ToolCapabilityUnverified:
		return 1
	default:
		return 2
	}
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

// modelLabel turns an Ollama tag into a friendly display name: "qwen3:8b" → "Qwen3 8B".
// Best-effort presentation only — the tag remains the identity everywhere.
func modelLabel(tag string) string {
	name, size, ok := strings.Cut(tag, ":")
	label := strings.ToUpper(name[:1]) + name[1:]
	if ok && size != "" && size != "latest" {
		label += " " + strings.ToUpper(size)
	}
	return label
}

// installedWhy is the one-line rationale from what Ollama actually reports about the
// model — family, parameter size, quant — rather than a hand-authored blurb.
func installedWhy(m InstalledModel, capability ToolCapability) string {
	parts := make([]string, 0, 3)
	if m.Family != "" {
		parts = append(parts, m.Family)
	}
	if m.ParameterSize != "" {
		parts = append(parts, m.ParameterSize)
	}
	if m.Quant != "" {
		parts = append(parts, m.Quant)
	}
	if len(parts) == 0 {
		switch capability {
		case ToolCapabilityVerified:
			return "Tool-capable local model."
		case ToolCapabilityUnsupported:
			return "Local model without tool calling."
		default:
			return "Local model; tool calling is unverified."
		}
	}
	var suffix string
	switch capability {
	case ToolCapabilityVerified:
		suffix = "tool-calling"
	case ToolCapabilityUnsupported:
		suffix = "no tool-calling"
	default:
		suffix = "tool-calling unverified"
	}
	return strings.Join(parts, " · ") + " · " + suffix
}

// paramRank scores a model by parameter count so the recommendation prefers the most
// capable model that fits (bigger ⇒ generally better grounding). Parses "7.6B"/"14B"/
// "500M" → an integer millions-of-params; unknown → 0 (ranks last, still selectable).
func paramRank(paramSize string) int {
	s := strings.TrimSpace(strings.ToUpper(paramSize))
	if s == "" {
		return 0
	}
	mult := 1.0
	switch {
	case strings.HasSuffix(s, "B"):
		mult, s = 1000, strings.TrimSuffix(s, "B")
	case strings.HasSuffix(s, "M"):
		mult, s = 1, strings.TrimSuffix(s, "M")
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return int(n * mult)
}
