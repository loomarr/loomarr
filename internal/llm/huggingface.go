package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/loomarr/loomarr/internal/httpx"
)

// The "browse downloadable models" source (§8.1). There is NO first-party Ollama API
// for enumerating pullable models — /api/search is unshipped, ollama.com serves HTML
// only. Hugging Face's model API IS a live, anonymous source of GGUF repos, AND it
// exposes each repo's per-file GGUF sizes BEFORE download — so Loomarr can rank
// downloadable models against the machine's VRAM the same way it ranks installed ones.
//
// The flow: pull the most-popular GGUF repos, read each repo's quant files + sizes,
// pick the best quant that fits this machine, drop repos where nothing fits, and return
// a list ranked best-first — each with a `pullRef` (`hf.co/<repo>:<quant>`) that
// `ollama pull` consumes directly. No search box, no hardcoded model list.

const (
	hfModelsURL = "https://huggingface.co/api/models"
	// hfPopularLimit is how many popular repos we PULL from HF. We over-fetch because the
	// family allowlist (toolCapableFamilies) then drops the majority — only known
	// tool-capable families survive — so a small final list needs a wide candidate pool.
	hfPopularLimit = 100
	// hfKeepLimit caps the FINAL list after family-filtering + sizing (we stop doing
	// per-repo file calls once we have this many, to bound the fan-out).
	hfKeepLimit = 24
	// hfMaxConcurrent caps concurrent per-repo file lookups (be a good HF citizen).
	hfMaxConcurrent = 6
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

// DiscoverModel is one downloadable candidate, already sized + fit-ranked for THIS
// machine (§8.1). Unlike an installed model, tool-capability is still unknown until it
// is pulled and probed — but VRAM fit IS known, from the chosen quant's real file size.
type DiscoverModel struct {
	// ID is the Hugging Face repo id, e.g. "unsloth/Qwen3.5-4B-GGUF".
	ID string `json:"id"`
	// Label is a human-friendly name derived from the id, e.g. "Qwen3.5 4B" — the org
	// prefix and the "-GGUF" suffix stripped, dashes spaced. The id stays the identity.
	Label string `json:"label"`
	// Quant is the chosen quantization (e.g. "Q4_K_M") — the best-fitting one for the
	// machine among the repo's GGUF files.
	Quant string `json:"quant"`
	// PullRef is the exact `ollama pull <ref>` argument: "hf.co/" + ID + ":" + Quant.
	PullRef string `json:"pullRef"`
	// SizeGiB is the chosen quant's real on-disk size (a VRAM proxy), from HF's file list.
	SizeGiB float64 `json:"sizeGiB"`
	// Fit is the verdict of the chosen quant against detected VRAM (fits|tight|wont_fit).
	Fit Fit `json:"fit"`
	// Downloads is the HF popularity signal. It is a RANKING TIEBREAK ONLY and is NOT shown
	// to the user (§8.1: rank for Loomarr, not for Hugging Face).
	Downloads int64 `json:"downloads"`
	// Role is the plain-English job this model plays in the choice the user is making —
	// balanced|faster|higher_quality (§8.1). Drives the row copy + grouping.
	Role DiscoverRole `json:"role"`
	// Recommended flags the single safe-default pick for this machine (§8.1): the top-ranked
	// balanced model that fits comfortably. Exactly one model in a list carries it (or none,
	// if nothing fits comfortably).
	Recommended bool `json:"recommended"`
	// Note is a plain-English one-liner (why pick this) derived from role + fit — §8.1.
	// Presentation only; empty when nothing meaningful can be said.
	Note string `json:"note,omitempty"`
}

// DiscoverRole is the plain-English job a downloadable model plays in the user's choice
// (§8.1) — the framing that replaces quant/downloads jargon in the UI.
type DiscoverRole string

const (
	// RoleBalanced is the all-rounder: enough params to reason well, comfortable to run.
	RoleBalanced DiscoverRole = "balanced"
	// RoleFaster is smaller/lighter — quicker, lower quality, more headroom.
	RoleFaster DiscoverRole = "faster"
	// RoleHigherQuality is larger — best output that still fits (may be tight).
	RoleHigherQuality DiscoverRole = "higher_quality"
)

// hfListItem is the subset of the HF model-LIST JSON we parse.
type hfListItem struct {
	ID          string   `json:"id"`
	Downloads   int64    `json:"downloads"`
	Tags        []string `json:"tags"`
	PipelineTag string   `json:"pipeline_tag"`
	Private     bool     `json:"private"`
}

// hfModelFiles is the subset of the HF single-model JSON (with ?blobs=true) we parse:
// each sibling file's name + size.
type hfModelFiles struct {
	Siblings []struct {
		Filename string `json:"rfilename"`
		Size     int64  `json:"size"`
	} `json:"siblings"`
}

// DiscoverCompatible browses Hugging Face for the most popular GGUF models, sizes each
// against detected VRAM, and returns only those with a quant that fits — ranked
// best-first (largest comfortably-fitting model wins). vramGiB<=0 (no GPU detected)
// means fit is unknown; everything is returned as "tight" rather than filtered out, so
// the operator still sees a browseable list. Best-effort: any HF failure returns an
// empty slice + the error so the caller can degrade to a "browse on huggingface.co"
// link. No search parameter — this is the compatible set for this machine.
//
// installedTags is the caller's list of already-pulled Ollama tags (probe.PulledModels);
// models the user already has are filtered out BEFORE ranking/recommend, so the download
// list never offers something installed and the recommended pick is always a fresh one.
func DiscoverCompatible(ctx context.Context, vramGiB float64, installedTags []string) ([]DiscoverModel, error) {
	repos, err := hfPopularGGUF(ctx)
	if err != nil {
		return nil, err
	}
	// Size each repo concurrently (bounded), picking its best-fitting quant.
	out := make([]DiscoverModel, 0, len(repos))
	var mu sync.Mutex
	sem := make(chan struct{}, hfMaxConcurrent)
	var wg sync.WaitGroup
	for _, r := range repos {
		wg.Add(1)
		go func(r hfListItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			files, err := hfModelGGUFFiles(ctx, r.ID)
			if err != nil || len(files) == 0 {
				return // a repo we can't size just doesn't appear
			}
			quant, sizeGiB, ok := defaultQuantSize(files)
			if !ok {
				return // no canonical GGUF quant → can't size/trust it → drop it
			}
			// Sanity guard: a sized build implausibly small for the repo's advertised
			// parameter count means we picked a partial/indexer shard, not the real model
			// (e.g. a "27B" repo whose Q8_0 file is 0.6GB). Showing "27B · ~0.6GiB" reads
			// as broken — drop it rather than mislead. ~0.4GiB per billion params is a
			// generous floor (Q2-ish); real quants are well above it.
			if params := paramsFromID(r.ID); params > 0 && sizeGiB < params*0.4 {
				return
			}
			info := ModelInfo{ApproxVRAMGiB: sizeGiB}
			fit := classifyFit(info, vramGiB)
			if fit == FitWontFit {
				return // the default download wouldn't fit this machine → drop it
			}
			mu.Lock()
			out = append(out, DiscoverModel{
				ID:    r.ID,
				Label: repoLabel(r.ID),
				Quant: quant,
				// Pull the BARE repo (Ollama's implicit `:latest`). A synthesized `:quant`
				// tag is NOT reliable — Ollama resolves tags against the repo's own
				// manifest, and many repos (e.g. odd MoE builds) expose only `latest`, so
				// `hf.co/repo:Q8_0` 400s "tag not available". `latest` always resolves and
				// is the Q4_K_M-class default we size against, so what we show == what pulls.
				PullRef:   "hf.co/" + r.ID,
				SizeGiB:   sizeGiB,
				Fit:       fit,
				Downloads: r.Downloads,
			})
			mu.Unlock()
		}(r)
	}
	wg.Wait()

	// Collapse re-uploads of the SAME weights to one row per canonical model, keeping the
	// most-trusted uploader (then most-downloaded) as the representative (§8.1 curation
	// step 2). This is what turns "three Qwen3.5 9B rows" into one.
	out = dedupeByCanonical(out)

	// Drop models the user already has (§8.1) — before ranking/recommend, so the download
	// list never offers something installed and the recommended pick is always fresh.
	out = FilterInstalled(out, installedTags)

	// Rank for LOOMARR, not for Hugging Face (§8.1). The order is fitness-for-this-app:
	//   1. fit        — a comfortably-fitting model runs well; a tight one spills/slows.
	//   2. reliability — known-good grounded tool-caller FAMILY (curated, durable).
	//   3. size fit   — mid sizes (7–14B) reason well AND run comfortably; extremes lose.
	//   4. popularity — LAST tiebreak only (better-supported repo), never the headline.
	// Deterministic (stable + id tiebreak) for a testable UI.
	sort.SliceStable(out, func(a, b int) bool {
		return discoverRank(out[a]) > discoverRank(out[b])
	})
	if len(out) > hfKeepLimit {
		out = out[:hfKeepLimit] // a browse list, not an exhaustive index
	}
	// Stamp role + human note once each survivor's final fit is settled, then flag the one
	// recommended pick (the top-ranked balanced model that fits comfortably).
	for i := range out {
		out[i].Role = discoverRole(paramsFromID(out[i].ID))
		out[i].Note = discoverNote(out[i].Role, out[i].Fit)
	}
	markRecommended(out)
	return out, nil
}

// discoverRank is the fitness-for-Loomarr score used to order the download list (§8.1) —
// higher is better. It packs the priority tiers into one comparable integer so a single
// stable sort orders the list: fit dominates, then family reliability, then size-quality,
// then popularity as a last, bounded tiebreak. Downloads is log-scaled and capped so it
// can only ever break ties WITHIN an equal fit+reliability+size tier — never outrank a
// comfortably-fitting reliable model with a merely-popular one.
func discoverRank(m DiscoverModel) int {
	params := paramsFromID(m.ID)
	// fitOrder is 0=fits/1=tight/2=wont_fit; invert so fits scores highest.
	fitScore := 2 - fitOrder(m.Fit)        // 0..2
	rel := familyReliabilityScore(m.ID)    // 0..3
	sizeQ := sizeQualityScore(params)      // 0..3
	pop := popularityTiebreak(m.Downloads) // 0..9
	return fitScore*100_000 + rel*10_000 + sizeQ*1_000 + pop
}

// popularityTiebreak maps a raw download count to a small 0..9 bucket (roughly log10), so
// popularity can only break ties within a tier — it can never dominate fit or reliability.
func popularityTiebreak(downloads int64) int {
	switch {
	case downloads >= 5_000_000:
		return 9
	case downloads >= 1_000_000:
		return 8
	case downloads >= 500_000:
		return 7
	case downloads >= 100_000:
		return 6
	case downloads >= 50_000:
		return 5
	case downloads >= 10_000:
		return 4
	case downloads >= 1_000:
		return 3
	case downloads > 0:
		return 2
	default:
		return 0
	}
}

// markRecommended flags exactly one model (or none) as the safe default for this machine
// (§8.1): the FIRST balanced model that fits comfortably in the already-ranked list. If no
// balanced model fits comfortably, we fall back to the first comfortably-fitting model of
// any role; if nothing fits comfortably, nothing is recommended (the user still picks).
func markRecommended(out []DiscoverModel) {
	pick := -1
	for i := range out {
		if out[i].Fit == FitFits && out[i].Role == RoleBalanced {
			pick = i
			break
		}
	}
	if pick < 0 {
		for i := range out {
			if out[i].Fit == FitFits {
				pick = i
				break
			}
		}
	}
	if pick >= 0 {
		out[pick].Recommended = true
	}
}

// versionedFamily extracts a family token that KEEPS the generation number attached, so
// re-uploads of one release collapse but different generations stay apart: "Qwen3.5-9B" →
// "qwen3.5", "Qwen2.5-…" → "qwen2.5", "Llama-3.2-1B" → "llama3.2", "gemma-4-12b" →
// "gemma4". It finds the base family via familyFromID, then, if the id carries a version
// token adjacent to it (a bare number like "3.5"/"2.5"/"3.2", or a fused "Qwen3"), folds
// that into the token. "" when there's no known family.
func versionedFamily(id string) string {
	fam := familyFromID(id)
	if fam == "" {
		return ""
	}
	low := strings.ToLower(id)
	if i := strings.LastIndex(low, "/"); i >= 0 {
		low = low[i+1:] // repo name only; the org can't carry the version
	}
	// A version already fused onto the family in the name (e.g. "qwen3", "qwen3.5").
	for _, tok := range strings.FieldsFunc(low, func(r rune) bool { return r == '-' || r == '_' }) {
		if strings.HasPrefix(tok, fam) {
			rest := tok[len(fam):]
			if rest != "" && isVersionToken(rest) {
				return fam + rest // "qwen" + "3.5" → "qwen3.5"
			}
		}
	}
	// A version in the NEXT token after the family (e.g. "gemma-4-…", "llama-3.2-…").
	toks := strings.FieldsFunc(low, func(r rune) bool { return r == '-' || r == '_' })
	for i, tok := range toks {
		if tok == fam && i+1 < len(toks) && isVersionToken(toks[i+1]) {
			return fam + toks[i+1] // "gemma" + "4" → "gemma4"
		}
	}
	return fam // no version token found — the family alone
}

// isVersionToken reports whether s looks like a release number ("3", "3.5", "2.5", "4") —
// a run of digits and dots. Distinguishes a version from a param size ("9b", carrying a
// trailing letter) or arbitrary words.
func isVersionToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return true
}

// canonicalKey reduces a repo id to a stable identity for the SAME underlying model,
// ignoring uploader, quant, and packaging — so re-uploads collapse to one row (§8.1
// curation step 2). Two ids with the same key are treated as the same model; only the
// most-trusted/popular one survives dedup.
//
// The key is versioned-family + parameter size + base-variant tag — everything that names
// a distinct model to a user, and nothing that merely re-packages identical weights (org
// prefix, "-GGUF" suffix, quant token). Examples:
//
//	"unsloth/Qwen3.5-9B-GGUF"          ─┐
//	"bartowski/Qwen3.5-9B-GGUF"         ├─ all → "qwen3.5/9"   (one row)
//	"randomguy/Qwen3.5-9B-Q4_K_M-GGUF" ─┘
//	"google/gemma-4-12b-it-GGUF"        → "gemma4/12/it"   (stays apart from the 4b + the base)
//	"google/gemma-4-4b-it-GGUF"         → "gemma4/4/it"
//	"meta-llama/Llama-3.2-1B-Instruct"  → "llama3.2/1/instruct"
//	"weird/no-param-model-GGUF"         → ""   (no size → unkeyable → never merged)
//
// The variant tag ("it"/"instruct"/"chat") IS part of the key: a base and its instruct
// build are genuinely different picks, so they must not collapse into each other. A missing
// size yields "" — dedupeByCanonical then leaves that candidate standing alone rather than
// risk merging two genuinely different models on family alone.
func canonicalKey(id string) string {
	fam := versionedFamily(id)
	params := paramsFromID(id)
	if fam == "" || params <= 0 {
		return "" // can't confidently identify the model → don't merge it with anything
	}
	// Trim a trailing ".0" so paramsFromID's float ("9") and ("9.0") match textually.
	size := strconv.FormatFloat(params, 'f', -1, 64)
	key := fam + "/" + size
	if v := variantTag(id); v != "" {
		key += "/" + v
	}
	return key
}

// installedTagKey reduces an installed Ollama tag to a canonical model key comparable with
// canonicalKey (§8.1), so "already installed" can be matched against a downloadable repo.
// It handles both tag shapes Ollama uses:
//   - a plain registry tag: "qwen3:8b"           → canonicalKey("qwen3-8b")  = "qwen3/8"
//   - an hf.co pull:  "hf.co/unsloth/Qwen3-8B-GGUF:latest" → keyed off the repo id
//
// Returns "" when the tag can't be confidently keyed (then it just won't match anything,
// which is the safe default — we'd rather show a maybe-dup than hide a real model).
func installedTagKey(tag string) string {
	// hf.co pulls carry the repo id directly — key off it like any discover repo.
	if rest, ok := strings.CutPrefix(tag, "hf.co/"); ok {
		repo := rest
		if before, _, found := strings.Cut(rest, ":"); found {
			repo = before // drop the ":latest"/":Q4_K_M" suffix
		}
		return canonicalKey(repo)
	}
	// A plain registry tag "qwen3:8b" → treat "qwen3-8b" as a repo name for canonicalKey.
	name, _, _ := strings.Cut(tag, ":")
	size := ""
	if before, after, found := strings.Cut(tag, ":"); found {
		name, size = before, after
	}
	// Reshape "qwen3" + "8b" into a canonicalKey-parseable "qwen3-8b" (size already has "b").
	return canonicalKey(name + "-" + size)
}

// FilterInstalled drops downloadable models the user already has, matching by canonical
// model key OR exact hf.co repo ref against the installed tags (§8.1). This is what stops
// the download list from offering "Qwen3 8B" when `qwen3:8b` is already pulled and active.
// installedTags is the flat list of Ollama tags (probe.PulledModels). Order preserved.
func FilterInstalled(models []DiscoverModel, installedTags []string) []DiscoverModel {
	if len(installedTags) == 0 {
		return models
	}
	installedKeys := make(map[string]struct{}, len(installedTags))
	installedRefs := make(map[string]struct{}, len(installedTags))
	for _, t := range installedTags {
		if k := installedTagKey(t); k != "" {
			installedKeys[k] = struct{}{}
		}
		// Also record the bare hf.co repo ref, so a pulled "hf.co/<repo>:latest" matches a
		// discover pullRef of "hf.co/<repo>" even when the key can't be derived.
		if rest, ok := strings.CutPrefix(t, "hf.co/"); ok {
			repo := rest
			if before, _, found := strings.Cut(rest, ":"); found {
				repo = before
			}
			installedRefs["hf.co/"+repo] = struct{}{}
		}
	}
	out := models[:0:0] // fresh backing array; don't alias the input
	for _, m := range models {
		if _, ok := installedRefs[m.PullRef]; ok {
			continue
		}
		if k := canonicalKey(m.ID); k != "" {
			if _, ok := installedKeys[k]; ok {
				continue
			}
		}
		out = append(out, m)
	}
	return out
}

// variantTag returns the base-instruction variant a repo id advertises ("it"/"instruct"/
// "chat"), normalized to the canonical "instruct"/"chat" form, or "" for a plain base
// model. Matched as a delimiter-bounded token so it can't fire inside another word.
func variantTag(id string) string {
	up := strings.ToUpper(id)
	// "it" and "instruct" both mean instruction-tuned → one canonical token so
	// "gemma-4-12b-it" and "gemma-4-12b-instruct" collapse together.
	if quantTokenPresent(up, "IT") || quantTokenPresent(up, "INSTRUCT") {
		return "instruct"
	}
	if quantTokenPresent(up, "CHAT") {
		return "chat"
	}
	return ""
}

// familyFromID returns the tool-capable family name a repo id belongs to (the first
// toolCapableFamilies entry it contains, lowercased), or "" if none — a normalized family
// token for canonicalKey that keeps "qwen3.5" vs "qwen2.5" style distinctions via params.
func familyFromID(id string) string {
	low := strings.ToLower(id)
	for _, fam := range toolCapableFamilies {
		if strings.Contains(low, fam) {
			return strings.Trim(fam, "-") // "yi-"/"glm-" carry a trailing dash in the table
		}
	}
	return ""
}

// dedupeByCanonical collapses candidates that are the SAME underlying model (same weights,
// different uploader/quant/packaging) to a single representative row (§8.1 curation step
// 2). Grouping is by canonicalKey(id); within a group the representative is the most-
// TRUSTED uploader, then the most-DOWNLOADED — so a clean unsloth/bartowski build wins
// over a random re-upload. The representative keeps its OWN download count (a widely-pulled
// canonical model still ranks high). Order-independent: a stable pre-sort makes the winner
// deterministic regardless of HF's return order. Candidates whose id yields an empty key
// are never merged (each stands alone) — we don't guess two unknowns are the same model.
func dedupeByCanonical(in []DiscoverModel) []DiscoverModel {
	// Sort a copy so the FIRST candidate seen per key is already the winner: trusted
	// uploader first, then higher downloads, then id for a total order.
	ranked := make([]DiscoverModel, len(in))
	copy(ranked, in)
	sort.SliceStable(ranked, func(a, b int) bool {
		if ta, tb := uploaderTrust(ranked[a].ID), uploaderTrust(ranked[b].ID); ta != tb {
			return ta < tb
		}
		if ranked[a].Downloads != ranked[b].Downloads {
			return ranked[a].Downloads > ranked[b].Downloads
		}
		return ranked[a].ID < ranked[b].ID
	})
	seen := make(map[string]struct{}, len(ranked))
	out := make([]DiscoverModel, 0, len(ranked))
	for _, m := range ranked {
		key := canonicalKey(m.ID)
		if key == "" {
			out = append(out, m) // unkeyable → never merged with anything
			continue
		}
		if _, dup := seen[key]; dup {
			continue // a more-trusted/more-popular repo already represents this model
		}
		seen[key] = struct{}{}
		out = append(out, m)
	}
	return out
}

// hfPopularGGUF fetches the most-downloaded GGUF repos (the candidate set), filtered to
// plausible text-chat models.
func hfPopularGGUF(ctx context.Context) ([]hfListItem, error) {
	q := url.Values{}
	q.Set("filter", "gguf")
	q.Set("sort", "downloads")
	q.Set("direction", "-1")
	q.Set("limit", fmt.Sprintf("%d", hfPopularLimit))
	reqURL := hfModelsURL + "?" + q.Encode()

	var raw []hfListItem
	if err := hfGetJSON(ctx, reqURL, &raw); err != nil {
		return nil, err
	}
	out := make([]hfListItem, 0, len(raw))
	for _, m := range raw {
		if m.Private || m.ID == "" || !isChatGGUF(m) {
			continue
		}
		// Only known tool-capable families — the one gate that keeps the download list to
		// models that can actually ground suggestions (HF exposes no reliable pre-download
		// tool-calling signal; see toolCapableFamilies).
		if !isToolCapableFamily(m.ID) {
			continue
		}
		// Drop community fine-tune remixes that slip through the family substring match
		// (§8.1 curation step 1) — "…-Heretic-Uncensored", "…-agentic-composer2.5".
		if isFineTuneSoup(m.ID) {
			continue
		}
		// Drop known multimodal variants that advertise vision but not tools (§8.1 step 1a)
		// — Qwen3-VL-*, gemma-4-E4B-* — so we never recommend a model that downloads but
		// can't ground (the "downloaded it but nothing happened" trap).
		if isVisionVariant(m.ID) {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

// hfModelGGUFFiles returns the repo's GGUF files (name → size in bytes) via ?blobs=true.
func hfModelGGUFFiles(ctx context.Context, id string) (map[string]int64, error) {
	reqURL := hfModelsURL + "/" + id + "?blobs=true"
	var mf hfModelFiles
	if err := hfGetJSON(ctx, reqURL, &mf); err != nil {
		return nil, err
	}
	files := make(map[string]int64, len(mf.Siblings))
	for _, s := range mf.Siblings {
		if strings.HasSuffix(strings.ToLower(s.Filename), ".gguf") && s.Size > 0 {
			files[s.Filename] = s.Size
		}
	}
	return files, nil
}

// hfGetJSON does a bounded anonymous GET and decodes JSON into v.
func hfGetJSON(ctx context.Context, reqURL string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := httpx.New(httpx.TimeoutProbe).Do(req)
	if err != nil {
		return fmt.Errorf("hugging face: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("hugging face: status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("hugging face: decode: %w", err)
	}
	return nil
}

// defaultQuantSize returns the quant + size Ollama's `latest` tag will actually pull for
// this repo — which is what we both SIZE against and DOWNLOAD (we pull the bare repo, so
// what we show must match what `latest` resolves to). Ollama's `latest` for an hf.co repo
// is the Q4_K_M build when present (the sane default), so we look for a Q4_K_M-class file
// first; failing that, the smallest canonical quant (the closest stand-in for a lightweight
// default). ok=false if the repo exposes no canonical GGUF at all — then we can't trust
// what `latest` is, so we drop it rather than mis-size it.
func defaultQuantSize(files map[string]int64) (quant string, sizeGiB float64, ok bool) {
	type cand struct {
		quant string
		gib   float64
	}
	cands := make([]cand, 0, len(files))
	for name, size := range files {
		q := quantFromFilename(name)
		if q == "" {
			continue
		}
		cands = append(cands, cand{quant: q, gib: float64(size) / bytesPerGiB})
	}
	if len(cands) == 0 {
		return "", 0, false
	}
	// Prefer the Q4_K_M build — what Ollama's `latest` resolves to when present.
	for _, c := range cands {
		if strings.EqualFold(c.quant, "Q4_K_M") {
			return c.quant, round1(c.gib), true
		}
	}
	// Otherwise the smallest canonical quant — the closest stand-in for a light default.
	sort.SliceStable(cands, func(a, b int) bool { return cands[a].gib < cands[b].gib })
	return cands[0].quant, round1(cands[0].gib), true
}

// canonicalQuants are the quantization tags Ollama recognizes as an `hf.co/<repo>:<tag>`
// selector. We ONLY offer one of these — a synthesized tag from an odd filename token
// (e.g. "F32" scraped off "…-MTP-Q4K-Q8_0-F32.gguf") makes Ollama reject the pull with a
// 400. Ordered so the regex prefers the longer, more specific match (Q4_K_M before Q4_0).
var canonicalQuants = []string{
	"IQ1_S", "IQ1_M", "IQ2_XXS", "IQ2_XS", "IQ2_S", "IQ2_M", "IQ3_XXS", "IQ3_XS", "IQ3_S", "IQ3_M",
	"IQ4_XS", "IQ4_NL",
	"Q2_K_S", "Q2_K", "Q3_K_L", "Q3_K_M", "Q3_K_S", "Q4_K_M", "Q4_K_S", "Q4_0", "Q4_1",
	"Q5_K_M", "Q5_K_S", "Q5_0", "Q5_1", "Q6_K", "Q8_0", "Q8_K",
	"BF16", "F16", "F32",
}

// quantFromFilename extracts a CANONICAL Ollama quant tag from a GGUF filename, or "" if
// the file carries none. It matches a canonical quant as a dash/underscore-delimited token
// anywhere in the name (case-insensitive) and returns its canonical uppercase form:
//
//	"Qwen3.5-4B-Q4_K_M.gguf"                 → "Q4_K_M"
//	"model-IQ4_XS.gguf"                      → "IQ4_XS"
//	"DeepSeek-V4-Flash-MTP-Q4K-Q8_0-F32.gguf"→ "Q8_0"  (a real canonical quant, not "F32"/"Q4K")
//	"weird-custom-w2Q2K-build.gguf"          → ""      (no canonical quant → not offered)
//
// Returning only canonical quants is what keeps us from ever handing Ollama a tag it 400s
// on. Multi-part shards ("-of-") are skipped (Ollama pulls those as a whole, not by tag).
func quantFromFilename(name string) string {
	base := strings.TrimSuffix(name, ".gguf")
	base = strings.TrimSuffix(base, ".GGUF")
	if strings.Contains(base, "-of-") {
		return "" // a shard part, not a standalone quant
	}
	up := strings.ToUpper(base)
	for _, q := range canonicalQuants {
		if quantTokenPresent(up, q) {
			return q
		}
	}
	return ""
}

// quantTokenPresent reports whether canonical quant q appears in up (already uppercased)
// as a whole token — bounded by a delimiter (dash/underscore/dot) or string end on each
// side — so "Q4_0" doesn't spuriously match inside "Q4_K_M" and "Q4K" (non-canonical)
// never matches "Q4_K_M".
func quantTokenPresent(up, q string) bool {
	from := 0
	for {
		i := strings.Index(up[from:], q)
		if i < 0 {
			return false
		}
		i += from
		leftOK := i == 0 || isQuantDelim(up[i-1])
		end := i + len(q)
		rightOK := end == len(up) || isQuantDelim(up[end])
		if leftOK && rightOK {
			return true
		}
		from = i + 1
	}
}

// isQuantDelim reports whether b bounds a token in a GGUF filename or repo id. "/" is
// included so a marker at the very start of a repo name (right after the "org/" prefix,
// e.g. "lmg-anon/vntl-…") is still seen as a standalone token.
func isQuantDelim(b byte) bool { return b == '-' || b == '_' || b == '.' || b == '/' }

// round1 rounds to one decimal for display stability.
func round1(f float64) float64 {
	return float64(int(f*10+0.5)) / 10
}

// paramsFromID reads the advertised parameter count (billions) from a repo id's name —
// the "27B"/"4B"/"1.5B" token: "unsloth/Qwen3.5-4B-GGUF" → 4, "prism-ml/Bonsai-27B-gguf"
// → 27, "…-1.5B-…" → 1.5. Returns 0 when there's no such token (then no size sanity check
// is applied). Only a "B" (billions) token counts; an "M" or bare number is ignored.
func paramsFromID(id string) float64 {
	name := id
	if i := strings.LastIndex(id, "/"); i >= 0 {
		name = id[i+1:]
	}
	best := 0.0
	// Split on '-'/'_' only (NOT '.', so "1.5" stays intact) and look for a "<num>B" token.
	for _, tok := range strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' }) {
		best = maxParamToken(tok, best)
	}
	return best
}

// maxParamToken parses a single token like "27B"/"1.5B"/"4b" to billions-of-params,
// returning max(cur, parsed) — so a name with several numbers picks the params-looking one.
func maxParamToken(tok string, cur float64) float64 {
	if len(tok) < 2 {
		return cur
	}
	last := tok[len(tok)-1]
	if last != 'B' && last != 'b' {
		return cur
	}
	numPart := tok[:len(tok)-1]
	// Allow a leading non-numeric run to be trimmed (e.g. "Qwen3.5" isn't a param token,
	// but "4" in "4B" is) — parse only if the remainder is a clean number.
	if v, err := strconv.ParseFloat(numPart, 64); err == nil && v > 0 && v < 2000 {
		if v > cur {
			return v
		}
	}
	return cur
}

// repoLabel turns an HF repo id into a human-friendly name for the UI, best-effort:
//
//	"unsloth/Qwen3.5-4B-GGUF"                        → "Qwen3.5 4B"
//	"deepreinforce-ai/Ornith-1.0-9B-GGUF"            → "Ornith 1.0 9B"
//	"rippertnt/HyperCLOVAX-…-1.5B-Q4_K_M-GGUF"       → "HyperCLOVAX … 1.5B"
//
// It drops the org prefix, the "-GGUF" suffix, and a trailing embedded quant token, then
// spaces the dashes. The raw id stays the identity everywhere; this is presentation only.
func repoLabel(id string) string {
	name := id
	if i := strings.LastIndex(id, "/"); i >= 0 {
		name = id[i+1:] // drop the org/user prefix
	}
	// Drop a trailing "-GGUF"/"-gguf".
	for _, suf := range []string{"-GGUF", "-gguf", "-Gguf"} {
		name = strings.TrimSuffix(name, suf)
	}
	// Drop a trailing embedded quant token (e.g. "…-1.5B-Q4_K_M" → "…-1.5B").
	if i := strings.LastIndex(name, "-"); i >= 0 {
		if quantFromFilename(name[i+1:]+".gguf") != "" {
			name = name[:i]
		}
	}
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.Join(strings.Fields(name), " ") // collapse repeated spaces
	if name == "" {
		return id // never return empty — fall back to the raw id
	}
	return name
}

// isChatGGUF is the relevance filter: keep only repos likely to be a text-chat model
// Ollama can run for grounded suggestions, dropping the noise HF's broad `gguf` filter
// returns (embeddings, rerankers, TTS/audio, pure image/vision pipelines). Tool-
// capability + the final fit verdict are confirmed after pull (probe.go). We err toward
// inclusion: an unknown pipeline is kept.
func isChatGGUF(m hfListItem) bool {
	switch m.PipelineTag {
	case "feature-extraction", "sentence-similarity", "text-to-speech",
		"automatic-speech-recognition", "text-to-image", "image-classification":
		return false
	}
	for _, t := range m.Tags {
		low := strings.ToLower(t)
		for _, r := range rejectTags {
			if low == r {
				return false
			}
		}
	}
	// Some non-chat repos carry no telling tag/pipeline (a plain "gguf" embedding model
	// slipped through in the live check) — reject on obvious name signals as a backstop.
	id := strings.ToLower(m.ID)
	for _, bad := range rejectNameParts {
		if strings.Contains(id, bad) {
			return false
		}
	}
	return true
}

// rejectNameParts catch non-chat repos whose tags don't reveal them, by an unambiguous
// substring in the repo id (an "embedding" repo is never a chat model). Kept narrow so
// it can't accidentally drop a real chat model.
var rejectNameParts = []string{"embedding", "embed-", "-embed", "reranker", "rerank-", "-tts", "whisper", "bge-", "-bge"}

// rejectTags are HF tags that mark a repo as not a text-chat model we'd run for
// grounding. Kept small + obvious — a coarse noise filter, not a curated allowlist.
var rejectTags = []string{
	"sentence-transformers", "text-to-speech", "text-to-image",
	"automatic-speech-recognition", "reranker", "embeddings",
}
