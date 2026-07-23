package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/mantonx/loomarr/internal/httpx"
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

// isToolCapableFamily reports whether a repo id belongs to a curated tool-capable family.
// Matched case-insensitively as a substring of the id (org + repo name).
func isToolCapableFamily(id string) bool {
	low := strings.ToLower(id)
	for _, fam := range toolCapableFamilies {
		if strings.Contains(low, fam) {
			return true
		}
	}
	return false
}

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
	// Downloads is the HF popularity signal used for ordering + display.
	Downloads int64 `json:"downloads"`
}

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
func DiscoverCompatible(ctx context.Context, vramGiB float64) ([]DiscoverModel, error) {
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

	// Rank best-first: comfortable fits before tight (a model that comfortably fits runs
	// better), then by POPULARITY. Downloads is a better "which should I pick" signal than
	// chosen-quant size — a 4B at BF16 is larger on disk than a 9B at Q8 but not better —
	// and the popular repos are the well-supported, well-quantized ones. Fit is the gate;
	// popularity is the sort. Deterministic (stable) for a testable UI.
	sort.SliceStable(out, func(a, b int) bool {
		if fitOrder(out[a].Fit) != fitOrder(out[b].Fit) {
			return fitOrder(out[a].Fit) < fitOrder(out[b].Fit)
		}
		return out[a].Downloads > out[b].Downloads
	})
	if len(out) > hfKeepLimit {
		out = out[:hfKeepLimit] // a browse list, not an exhaustive index
	}
	return out, nil
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

func isQuantDelim(b byte) bool { return b == '-' || b == '_' || b == '.' }

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
