package llm

import (
	"slices"
	"strings"
)

// toolCapableFamilies is the curated allowlist of model families Loomarr is confident are
// TOOL-CAPABLE — the one thing that decides whether a model can ground suggestions, and
// the one thing Hugging Face's metadata does NOT reliably expose before download (verified
// live: HF's `conversational`/`pipeline_tag` tags don't separate tool-callers from
// completion-only models — Mistral has no conversational tag, Qwen is tagged
// image-text-to-text, DeepSeek-V4 is text-generation but reports no `tools` to Ollama).
// So the download list only offers repos whose id contains one of these family names.
//
// This IS a maintained list — the deliberate tradeoff for "only offer models that work"
// (a curated-but-accurate gate beats an inaccurate live heuristic). Keep it to broad
// FAMILY names (not exact model ids or sizes), which change slowly. To add a family:
// append its lowercase name here; that's the only change needed. A model that slips
// through anyway (odd repo naming) is still caught after pull by Ollama's tools check and
// shown as unusable — this list is the pre-download filter, not the last line of defense.
var toolCapableFamilies = []string{
	"qwen", "llama", "mistral", "mixtral", "gemma", "phi", "command-r", "commandr",
	"hermes", "granite", "smollm", "aya", "nemotron", "tulu", "olmo", "falcon",
	"internlm", "yi-", "glm-", "deepseek-v3", "deepseek-v2",
}

// rejectRemixTokens marks community fine-tune "soup" that must match as a WHOLE token
// (delimiter-bounded), because the word could otherwise appear inside a legitimate name —
// short/ambiguous markers like "erp"/"rp". Kept narrow so a clean "-instruct"/"-it" never
// trips (design §8.1 curation step 1). Sibling to toolCapableFamilies: the family gate
// decides USABLE, this decides LEGIBLE.
var rejectRemixTokens = []string{
	"uncensored", "abliterated", "heretic", "roleplay", "erp", "nsfw",
	"aggressive", "hauhaucs", "agentic", "tau2", "vntl",
}

// rejectRemixSubstrings mark soup that is junk regardless of adjacent characters — recipe
// "composer2.5"/"fable5" tokens fuse a digit onto the word so a whole-token match misses
// them, and a "coder" build is a code model, never a lineup model. These are distinctive
// enough that a plain substring match won't catch a real base/instruct model.
var rejectRemixSubstrings = []string{
	"composer", "fable", "coder", "-mtp", "mtp-", "-rp-",
}

// isFineTuneSoup reports whether a repo id looks like a community remix or wrong-purpose
// build rather than a base/instruct model. Whole-token markers use a delimiter-bounded
// match; substring markers (fused recipe tokens, "coder") match anywhere. A hit drops the
// repo even though its family passed the tool-capable gate.
func isFineTuneSoup(id string) bool {
	up := strings.ToUpper(id)
	for _, part := range rejectRemixTokens {
		if quantTokenPresent(up, strings.ToUpper(part)) {
			return true
		}
	}
	low := strings.ToLower(id)
	for _, sub := range rejectRemixSubstrings {
		if strings.Contains(low, sub) {
			return true
		}
	}
	return false
}

// visionMarkers flag a family's MULTIMODAL line — builds that advertise vision/completion
// but NOT tool-calling to Ollama, confirmed only after pull (§8.1 curation step 1a). The
// family substring gate lets these ride in (they ARE "gemma"/"qwen"); grounding needs
// tools, so a pulled vision model downloads fine yet never enters the installed catalog —
// the "downloaded it but nothing happened" trap. Their ids carry the marker before
// download, so we drop them here. Delimiter-bounded tokens, kept narrow:
//   - "VL"        — Qwen's vision-language line (Qwen3-VL-*, Qwen2.5-VL-*)
//   - "E2B"/"E4B" — Gemma's efficiency-MULTIMODAL builds (gemma-4-E4B-it), NOT text "-4b-"
var visionMarkers = []string{"VL", "E2B", "E4B"}

// isVisionVariant reports whether a repo id names a known non-tool-calling multimodal
// variant (§8.1 step 1a), by a delimiter-bounded token match so "E4B" fires only as a
// standalone token ("gemma-4-E4B-it") and never inside another run.
func isVisionVariant(id string) bool {
	up := strings.ToUpper(id)
	for _, m := range visionMarkers {
		if quantTokenPresent(up, m) {
			return true
		}
	}
	return false
}

// trustedUploaders are packagers Loomarr trusts to publish a clean, correctly-quantized
// GGUF of a base model — the lowercase org prefix of the repo id. When several repos
// collapse to the same canonical model (§8.1 curation step 2), a trusted uploader's repo
// is chosen as the representative over an unknown re-uploader. Broad + slow-changing, like
// the family allowlist. Not a gate — an untrusted uploader still appears if it's the only
// source of a model; trust only breaks the who-represents-this-model tie.
var trustedUploaders = []string{
	"unsloth", "bartowski", "ggml-org", "lmstudio-community",
	"qwen", "google", "meta-llama", "microsoft", "mistralai", "ibm-granite",
}

// uploaderTrust returns 0 for a trusted uploader (sorts first) or 1 otherwise, read from
// the org prefix of the repo id. A bare id with no "/" is untrusted (1).
func uploaderTrust(id string) int {
	org := id
	if before, _, ok := strings.Cut(id, "/"); ok {
		org = before
	}
	if slices.Contains(trustedUploaders, strings.ToLower(org)) {
		return 0
	}
	return 1
}

// familyReliability scores how well a model FAMILY is known to work as a grounded
// tool-caller for Loomarr — the durable "quality by family, not exact id" judgment §8.1
// applies to hosted models, brought to the local side. Higher is better; 0 is an unknown
// (still tool-capable — it passed the family gate — just not one we've vouched for). This
// is the one piece of curated judgment metadata can't give us: which families actually
// reason well enough to pick themed titles. Keyed by versionedFamily's token stem.
var familyReliability = map[string]int{
	// Frontier open families with strong instruct + tool-calling track records.
	"qwen": 3, "llama": 3, "gemma": 3, "mistral": 3, "phi": 3,
	// Solid, well-supported, slightly less battle-tested for our grounding use.
	"granite": 2, "command-r": 2, "commandr": 2, "hermes": 2, "mixtral": 2,
	"aya": 2, "internlm": 2, "glm": 2, "yi": 2,
	// Everything else that passed the family gate defaults to 1 (usable, unvouched).
}

// familyReliabilityScore returns the reliability tier for a repo id's family (0 if the
// family stem isn't in the table but the repo passed the gate — treated as tier 1 by the
// caller). versionedFamily gives "qwen3.5"; we match on the leading alpha stem ("qwen").
func familyReliabilityScore(id string) int {
	fam := versionedFamily(id)
	// Strip the trailing version digits/dots to get the stem ("qwen3.5" → "qwen").
	stem := strings.TrimRight(fam, "0123456789.")
	if score, ok := familyReliability[stem]; ok {
		return score
	}
	if fam != "" {
		return 1 // passed the gate but unvouched
	}
	return 0
}

// sizeQualityScore scores a model's PARAMETER size for grounded reasoning-vs-speed balance
// (§8.1): the mid sizes (7–14B) reason well enough to pick themed titles while still
// running comfortably, so they score highest; a 1B toy and an exotic 30B that barely fits
// both score lower. Deterministic, params-only. 0 for an unknown size.
func sizeQualityScore(params float64) int {
	switch {
	case params <= 0:
		return 0
	case params < 3:
		return 1 // tiny — fast, but weakest at grounded reasoning
	case params < 7:
		return 2 // small — decent
	case params <= 14:
		return 3 // the sweet spot for grounded tool-calling on consumer GPUs
	case params <= 24:
		return 2 // large — strong, but heavy
	default:
		return 1 // very large — usually tight, diminishing returns locally
	}
}

// discoverRole assigns the plain-English job a model plays in the user's choice (§8.1),
// from its parameter size: tiny/small → faster, mid → balanced, large → higher_quality.
// This is the framing the UI leads with, in place of quant/download jargon.
func discoverRole(params float64) DiscoverRole {
	switch {
	case params <= 0:
		return RoleBalanced // unknown size — treat as the default bucket
	case params < 6:
		return RoleFaster // 1–5B: quicker, lighter
	case params <= 9:
		return RoleBalanced // 6–9B: the all-rounder sweet spot for grounded tool-calling
	default:
		return RoleHigherQuality // 10B+: heavier, best output that still fits
	}
}

// discoverNote is the plain-English one-liner shown under a downloadable model (§8.1) —
// deterministic, from role + fit only, so the user reads WHY to pick it in the terms of
// the choice they're actually making (not "Q4_K_M · ~5 GiB").
func discoverNote(role DiscoverRole, fit Fit) string {
	var roleText string
	switch role {
	case RoleFaster:
		roleText = "Faster and lighter"
	case RoleHigherQuality:
		roleText = "Higher quality"
	default:
		roleText = "Best all-round pick"
	}
	fitPart := ""
	switch fit {
	case FitFits:
		fitPart = "fits comfortably"
	case FitTight:
		fitPart = "fits, but tight on VRAM"
	}
	if fitPart == "" {
		return roleText
	}
	return roleText + " — " + fitPart
}

// isToolCapableFamily reports whether a repo id belongs to a curated tool-capable family.
// Matched case-insensitively as a substring of the id (org + repo name).
func isToolCapableFamily(id string) bool {
	low := strings.ToLower(id)
	for _, fam := range toolCapableFamilies {
		if familyTokenPresent(low, fam) {
			return true
		}
	}
	return false
}

// familyTokenPresent reports whether family name fam appears in low (already lowercased) as
// a family token — i.e. NOT immediately followed by another letter, so "gemma" matches
// "gemma-4"/"gemma4"/"gemma_it" but NOT "gemmable" (a non-Gemma repo that merely contains
// the substring). A trailing digit/delimiter/end is fine (that's a version or size). The
// left side isn't constrained: a family can legitimately follow an org prefix or a dash.
func familyTokenPresent(low, fam string) bool {
	from := 0
	for {
		i := strings.Index(low[from:], fam)
		if i < 0 {
			return false
		}
		i += from
		end := i + len(fam)
		// Reject only when the very next char is another LETTER ("gemmable"); a digit
		// ("gemma4"), delimiter, or end-of-string all mark a real family token.
		if end == len(low) || !isASCIILetter(low[end]) {
			return true
		}
		from = i + 1
	}
}

func isASCIILetter(b byte) bool { return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') }
