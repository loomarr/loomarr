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

	"github.com/loomarr/loomarr/internal/platform/httpx"
)

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
	if err := checkStatus(resp, "hugging face"); err != nil {
		return err
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
