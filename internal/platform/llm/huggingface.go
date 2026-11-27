package llm

import (
	"context"

	"sort"
	"strconv"
	"strings"
	"sync"
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
