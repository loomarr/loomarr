package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// hfFixture loads a pinned testkit fixture. This test lives in `package llm` (it
// exercises unexported helpers), so it can't import internal/testkit (which imports
// llm → cycle). Anchoring on this source file's dir reaches the shared fixtures/ tree.
func hfFixture(t *testing.T, rel string) []byte {
	t.Helper()
	_, self, _, _ := runtime.Caller(0)
	p := filepath.Join(filepath.Dir(self), "..", "testkit", "fixtures", rel)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read fixture %q: %v", rel, err)
	}
	return b
}

// The per-repo file parser runs against a PINNED HF capture (no network) — per the
// "parse against the fixture, not remembered field names" rule.
func TestHFModelFiles_FromFixture(t *testing.T) {
	var mf hfModelFiles
	if err := json.Unmarshal(hfFixture(t, "llm/huggingface_model_files.json"), &mf); err != nil {
		t.Fatalf("decode HF files fixture: %v", err)
	}
	// Collect the .gguf files the way hfModelGGUFFiles does.
	files := map[string]int64{}
	for _, s := range mf.Siblings {
		if s.Size > 0 && len(s.Filename) > 5 && s.Filename[len(s.Filename)-5:] == ".gguf" {
			files[s.Filename] = s.Size
		}
	}
	if len(files) == 0 {
		t.Fatal("no .gguf files parsed — capture empty or field names drifted")
	}

	// The sizing quant is the Q4_K_M build — what Ollama's `latest` resolves to and what
	// we actually pull (bare repo). The Qwen3.5-4B repo has a Q4_K_M ~2.7GiB.
	quant, sizeGiB, ok := defaultQuantSize(files)
	if !ok {
		t.Fatal("expected a canonical quant in the Qwen3.5-4B repo")
	}
	if quant != "Q4_K_M" {
		t.Errorf("default quant = %q, want Q4_K_M (the `latest` proxy)", quant)
	}
	if sizeGiB < 2 || sizeGiB > 4 {
		t.Errorf("Q4_K_M size = %.1fGiB, want ~2.7 for a 4B model", sizeGiB)
	}
}

func TestQuantFromFilename(t *testing.T) {
	cases := map[string]string{
		"Qwen3.5-4B-Q4_K_M.gguf": "Q4_K_M",
		"Qwen3.5-4B-IQ4_XS.gguf": "IQ4_XS",
		"Qwen3.5-4B-Q8_0.gguf":   "Q8_0",
		"Qwen3.5-4B-BF16.gguf":   "BF16",
		// The deepseek-v4 repo's odd name: the OLD last-dash heuristic scraped "F32" and
		// built a tag Ollama 400'd on. We now pick the canonical Q8_0 token instead.
		"DeepSeek-V4-Flash-MTP-Q4K-Q8_0-F32.gguf": "Q8_0",
		// A non-canonical quant token ("w2Q2K") must NOT be offered — no canonical match.
		"DeepSeek-V4-Flash-IQ2XXS-w2Q2K-AProjQ8.gguf": "",
		"model-00001-of-00002.gguf":                   "", // a shard part, not a quant
		"weird-no-quant-name.gguf":                    "",
		// Whole-token boundary: "Q4_0" must not match inside "Q4_K_M".
		"m-Q4_K_M.gguf": "Q4_K_M",
	}
	for name, want := range cases {
		if got := quantFromFilename(name); got != want {
			t.Errorf("quantFromFilename(%q) = %q, want %q", name, got, want)
		}
	}
}

// A repo whose files carry NO canonical quant (only odd custom names) must produce no
// candidate — better to drop it than mis-size / hand Ollama something it 400s on.
func TestDefaultQuantSize_DropsNonCanonicalOnlyRepo(t *testing.T) {
	files := map[string]int64{
		"DeepSeek-V4-Flash-IQ2XXS-w2Q2K-AProjQ8.gguf": int64(80 * bytesPerGiB),
		"DeepSeek-V4-Flash-DSpark-support.gguf":       int64(5 * bytesPerGiB),
	}
	if _, _, ok := defaultQuantSize(files); ok {
		t.Error("a repo with no canonical quant must yield no candidate")
	}
}

func TestDefaultQuantSize_PrefersQ4KMThenSmallest(t *testing.T) {
	// Q4_K_M present → it's the `latest` proxy regardless of size ordering.
	q, gib, ok := defaultQuantSize(map[string]int64{
		"m-Q3_K_S.gguf": int64(2.0 * bytesPerGiB),
		"m-Q4_K_M.gguf": int64(4.5 * bytesPerGiB),
		"m-Q8_0.gguf":   int64(9.0 * bytesPerGiB),
	})
	if !ok || q != "Q4_K_M" || gib != 4.5 {
		t.Fatalf("defaultQuantSize = %q (%.1f), want Q4_K_M (4.5)", q, gib)
	}
	// No Q4_K_M → fall back to the SMALLEST canonical quant (light default stand-in).
	q, gib, ok = defaultQuantSize(map[string]int64{
		"m-Q6_K.gguf":   int64(6.5 * bytesPerGiB),
		"m-Q3_K_S.gguf": int64(2.0 * bytesPerGiB),
		"m-Q8_0.gguf":   int64(9.0 * bytesPerGiB),
	})
	if !ok || q != "Q3_K_S" || gib != 2.0 {
		t.Fatalf("defaultQuantSize = %q (%.1f), want Q3_K_S (2.0)", q, gib)
	}
}

func TestParamsFromID(t *testing.T) {
	cases := map[string]float64{
		"unsloth/Qwen3.5-4B-GGUF":                       4,
		"prism-ml/Bonsai-27B-gguf":                      27,
		"rippertnt/HyperCLOVAX-SEED-Text-Instruct-1.5B": 1.5,
		"u/no-param-name-GGUF":                          0,
		"u/gemma-4-26B-A4B-it-GGUF":                     26, // picks the larger of 4B/26B
	}
	for id, want := range cases {
		if got := paramsFromID(id); got != want {
			t.Errorf("paramsFromID(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestRepoLabel(t *testing.T) {
	cases := map[string]string{
		"unsloth/Qwen3.5-4B-GGUF":                                   "Qwen3.5 4B",
		"deepreinforce-ai/Ornith-1.0-9B-GGUF":                       "Ornith 1.0 9B",
		"rippertnt/HyperCLOVAX-SEED-Text-Instruct-1.5B-Q4_K_M-GGUF": "HyperCLOVAX SEED Text Instruct 1.5B",
		"lmg-anon/vntl-llama3-8b-v2-gguf":                           "vntl llama3 8b v2",
		"bare-id-no-slash":                                          "bare id no slash",
	}
	for id, want := range cases {
		if got := repoLabel(id); got != want {
			t.Errorf("repoLabel(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestIsToolCapableFamily(t *testing.T) {
	cases := map[string]bool{
		"unsloth/Qwen3.5-4B-GGUF":             true,
		"bartowski/Mistral-7B-Instruct-GGUF":  true,
		"lmstudio-community/gemma-4-E4B-GGUF": true,
		"meta-llama/Llama-3.3-70B-GGUF":       true,
		// Completion/reasoning-only or non-chat repos we can't ground with — excluded.
		"antirez/deepseek-v4-gguf":      false, // reports no tools to Ollama (live)
		"someone/RandomExperiment-GGUF": false,
		"org/bark-tts-GGUF":             false,
		// A substring of a family name is NOT the family — "Gemmable" is not Gemma.
		"user/Gemmable-4-12B-MTP-GGUF": false,
	}
	for id, want := range cases {
		if got := isToolCapableFamily(id); got != want {
			t.Errorf("isToolCapableFamily(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestIsFineTuneSoup(t *testing.T) {
	cases := map[string]bool{
		// Clean base/instruct builds — kept.
		"unsloth/Qwen3.5-9B-GGUF":          false,
		"google/gemma-4-12b-it-GGUF":       false,
		"meta-llama/Llama-3.2-1B-Instruct": false,
		// Community remixes that pass the family substring gate — dropped.
		"someone/Qwen3.5-9B-Uncensored-HauhauCS-Aggressive-GGUF": true,
		"x/gemma-4-12B-agentic-fable5-composer2.5-v2-3.5x-tau2":  true,
		"y/Qwen3-VL-8B-Instruct-abliterated-GGUF":                true,
		"z/Gemma-3-1B-it-GLM-Heretic-Uncensored-Thinking-GGUF":   true,
		"lmg-anon/vntl-llama3-8b-v2-gguf":                        true, // JA-translation fine-tune
		// Fused recipe tokens (digit welded onto the marker) + wrong-purpose builds.
		"user/gemma-4-12B-coder-fable5-composer2.5-v1": true, // coder + composer2.5 + fable5
		"user/Qwen3-8B-MTP-GGUF":                       true, // speculative-decoding recipe
	}
	for id, want := range cases {
		if got := isFineTuneSoup(id); got != want {
			t.Errorf("isFineTuneSoup(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestIsVisionVariant(t *testing.T) {
	cases := map[string]bool{
		// Multimodal variants that download but don't tool-call — dropped.
		"Qwen/Qwen3-VL-30B-A3B-Instruct-GGUF":    true,
		"unsloth/Qwen3-VL-2B-Instruct-GGUF":      true,
		"lmstudio-community/gemma-4-E4B-it-GGUF": true,
		"unsloth/gemma-4-E2B-it-GGUF":            true,
		// Plain text builds — kept (the marker must be a standalone token, not a substring).
		"unsloth/gemma-4-12b-it-GGUF":      false,
		"unsloth/Qwen3-8B-GGUF":            false,
		"meta-llama/Llama-3.2-1B-Instruct": false,
		// A "4B" text size must NOT be read as the "E4B" vision marker.
		"unsloth/Qwen3-4B-GGUF": false,
	}
	for id, want := range cases {
		if got := isVisionVariant(id); got != want {
			t.Errorf("isVisionVariant(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestCanonicalKey(t *testing.T) {
	cases := map[string]string{
		// Re-uploads of the same weights collapse to one key.
		"unsloth/Qwen3.5-9B-GGUF":          "qwen3.5/9",
		"bartowski/Qwen3.5-9B-GGUF":        "qwen3.5/9",
		"randomguy/Qwen3.5-9B-Q4_K_M-GGUF": "qwen3.5/9",
		// Different generation / size / variant stay apart.
		"foo/Qwen2.5-9B-GGUF":              "qwen2.5/9",
		"google/gemma-4-12b-it-GGUF":       "gemma4/12/instruct",
		"google/gemma-4-4b-it-GGUF":        "gemma4/4/instruct",
		"meta-llama/Llama-3.2-1B-Instruct": "llama3.2/1/instruct",
		// "it" and "instruct" normalize to the same variant → same key.
		"x/gemma-4-12b-instruct-GGUF": "gemma4/12/instruct",
		// No size token → unkeyable (never merged).
		"weird/no-param-model-GGUF": "",
		// No known family → unkeyable.
		"weird/Bonsai-27B-GGUF": "",
	}
	for id, want := range cases {
		if got := canonicalKey(id); got != want {
			t.Errorf("canonicalKey(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestDedupeByCanonical_KeepsTrustedRepresentative(t *testing.T) {
	in := []DiscoverModel{
		// Same model, three uploaders. The one trusted uploader (unsloth) must win even
		// with far fewer downloads than the untrusted re-uploaders.
		{ID: "randomguy/Qwen3.5-9B-GGUF", Downloads: 900_000},
		{ID: "unsloth/Qwen3.5-9B-GGUF", Downloads: 100_000},
		{ID: "otherguy/Qwen3.5-9B-GGUF", Downloads: 500_000},
		// A different model — untouched.
		{ID: "google/gemma-4-12b-it-GGUF", Downloads: 400_000},
		// Unkeyable — must stand alone, never merged into anything.
		{ID: "weird/Bonsai-27B-GGUF", Downloads: 10_000},
	}
	out := dedupeByCanonical(in)
	if len(out) != 3 {
		t.Fatalf("dedupe → %d rows, want 3 (one qwen, one gemma, one bonsai): %+v", len(out), out)
	}
	// The Qwen representative must be the trusted uploader (unsloth), not the popular one.
	var qwen *DiscoverModel
	for i := range out {
		if canonicalKey(out[i].ID) == "qwen3.5/9" {
			qwen = &out[i]
		}
	}
	if qwen == nil || qwen.ID != "unsloth/Qwen3.5-9B-GGUF" {
		t.Errorf("qwen representative = %v, want unsloth (trusted beats popular)", qwen)
	}
}

func TestDiscoverRole(t *testing.T) {
	cases := map[float64]DiscoverRole{
		1:  RoleFaster,
		4:  RoleFaster,
		8:  RoleBalanced,
		12: RoleHigherQuality, // 12B is a larger, higher-quality pick — not an all-rounder
		27: RoleHigherQuality,
		0:  RoleBalanced, // unknown size → default bucket
	}
	for params, want := range cases {
		if got := discoverRole(params); got != want {
			t.Errorf("discoverRole(%v) = %q, want %q", params, got, want)
		}
	}
}

func TestDiscoverNote(t *testing.T) {
	cases := []struct {
		role DiscoverRole
		fit  Fit
		want string
	}{
		{RoleBalanced, FitFits, "Best all-round pick — fits comfortably"},
		{RoleFaster, FitFits, "Faster and lighter — fits comfortably"},
		{RoleHigherQuality, FitTight, "Higher quality — fits, but tight on VRAM"},
		{RoleBalanced, FitWontFit, "Best all-round pick"}, // no fit clause when it won't fit
	}
	for _, c := range cases {
		if got := discoverNote(c.role, c.fit); got != c.want {
			t.Errorf("discoverNote(%v, %v) = %q, want %q", c.role, c.fit, got, c.want)
		}
	}
}

// The ranking must put fitness-for-Loomarr ahead of raw popularity: a comfortably-fitting
// reliable mid-size model must outrank a wildly-popular model that only fits tight, and a
// reliable family must outrank an unvouched one at equal fit + size.
func TestDiscoverRank_FitnessBeatsPopularity(t *testing.T) {
	// A hugely popular 3B that only fits tight.
	popularTight := DiscoverModel{ID: "unsloth/Qwen3.5-3B-GGUF", Fit: FitTight, Downloads: 9_000_000}
	// A modestly-popular reliable 8B that fits comfortably — the better pick for Loomarr.
	comfyReliable := DiscoverModel{ID: "unsloth/Qwen3.5-8B-GGUF", Fit: FitFits, Downloads: 200_000}
	if discoverRank(comfyReliable) <= discoverRank(popularTight) {
		t.Error("a comfortably-fitting reliable model must outrank a popular tight one")
	}

	// Equal fit + size: reliable family (qwen) beats an unvouched one (bonsai passes gate
	// only via a family substring — use a real unvouched family: falcon is tier 1).
	reliable := DiscoverModel{ID: "unsloth/Qwen3.5-8B-GGUF", Fit: FitFits, Downloads: 100_000}
	unvouched := DiscoverModel{ID: "tiiuae/Falcon-8B-GGUF", Fit: FitFits, Downloads: 100_000}
	if discoverRank(reliable) <= discoverRank(unvouched) {
		t.Error("a reliable family must outrank an unvouched one at equal fit + size")
	}
}

func TestMarkRecommended_PicksBalancedComfortable(t *testing.T) {
	out := []DiscoverModel{
		// A higher-quality model that fits comes first in rank order…
		{ID: "a/Gemma-4-27B-GGUF", Fit: FitFits, Role: RoleHigherQuality},
		// …but the recommended pick must be the first BALANCED comfortable model.
		{ID: "b/Qwen3.5-8B-GGUF", Fit: FitFits, Role: RoleBalanced},
		{ID: "c/Qwen3.5-4B-GGUF", Fit: FitFits, Role: RoleFaster},
	}
	markRecommended(out)
	if out[1].ID != "b/Qwen3.5-8B-GGUF" || !out[1].Recommended {
		t.Errorf("recommended should be the first balanced comfortable model; got %+v", out)
	}
	// Exactly one recommended.
	n := 0
	for _, m := range out {
		if m.Recommended {
			n++
		}
	}
	if n != 1 {
		t.Errorf("want exactly 1 recommended, got %d", n)
	}
}

func TestMarkRecommended_FallsBackWhenNoBalancedFits(t *testing.T) {
	out := []DiscoverModel{
		{ID: "a/Gemma-4-27B-GGUF", Fit: FitTight, Role: RoleHigherQuality},
		{ID: "b/Qwen3.5-4B-GGUF", Fit: FitFits, Role: RoleFaster}, // only comfortable one
	}
	markRecommended(out)
	if !out[1].Recommended {
		t.Error("with no balanced comfortable model, fall back to first comfortable one")
	}
}

func TestIsChatGGUF_RejectsNonText(t *testing.T) {
	cases := []struct {
		name string
		m    hfListItem
		keep bool
	}{
		{"chat pipeline", hfListItem{ID: "u/Qwen-GGUF", PipelineTag: "text-generation"}, true},
		{"unknown pipeline kept", hfListItem{ID: "u/mystery-GGUF"}, true},
		{"embeddings pipeline", hfListItem{ID: "u/e5-GGUF", PipelineTag: "feature-extraction"}, false},
		{"tts pipeline", hfListItem{ID: "u/bark-GGUF", PipelineTag: "text-to-speech"}, false},
		{"reranker tag", hfListItem{ID: "u/rr-GGUF", Tags: []string{"gguf", "reranker"}}, false},
	}
	for _, c := range cases {
		if got := isChatGGUF(c.m); got != c.keep {
			t.Errorf("%s: isChatGGUF = %v, want %v", c.name, got, c.keep)
		}
	}
}
