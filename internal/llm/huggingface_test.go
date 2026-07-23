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
	}
	for id, want := range cases {
		if got := isToolCapableFamily(id); got != want {
			t.Errorf("isToolCapableFamily(%q) = %v, want %v", id, got, want)
		}
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
