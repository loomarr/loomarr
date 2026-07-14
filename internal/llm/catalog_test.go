package llm

import "testing"

func TestClassifyFit(t *testing.T) {
	m := ModelInfo{Tag: "qwen3:8b", ApproxVRAMGiB: 5.2}
	tests := []struct {
		vram float64
		want Fit
	}{
		{12, FitFits},     // 12 >= 5.2*1.3 (6.76) → comfortable
		{6.8, FitFits},    // just above the headroom boundary (5.2*1.3≈6.76)
		{6.0, FitTight},   // above weights, below headroom
		{5.2, FitTight},   // exactly the weights
		{4.0, FitWontFit}, // below weights
		{0, FitTight},     // unknown VRAM → never claim fits/wont
	}
	for _, tt := range tests {
		if got := classifyFit(m, tt.vram); got != tt.want {
			t.Errorf("classifyFit(vram=%.2f) = %q, want %q", tt.vram, got, tt.want)
		}
	}
}

func TestVersionAtLeast(t *testing.T) {
	tests := []struct {
		have, want string
		ok         bool
	}{
		{"0.14.0", "0.14.0", true},
		{"0.14.1", "0.14.0", true},
		{"0.13.5", "0.14.0", false}, // the qwen3.5 gate
		{"0.14.0", "", true},        // no minimum
		{"", "0.14.0", false},       // unknown runtime, has a minimum → can't confirm
		{"1.0.0", "0.14.0", true},
		{"0.14.0-rc1", "0.14.0", true}, // pre-release suffix trimmed
	}
	for _, tt := range tests {
		if got := versionAtLeast(tt.have, tt.want); got != tt.ok {
			t.Errorf("versionAtLeast(%q,%q) = %v, want %v", tt.have, tt.want, got, tt.ok)
		}
	}
}

// The recommendation must be the best model that BOTH fits and the runtime supports.
func TestAnnotateCatalog_Recommendation(t *testing.T) {
	// 12GB card, current Ollama that pre-dates qwen3.5's requirement (0.13.5 < 0.14.0),
	// nothing pulled. Expect qwen3:8b recommended: qwen3.5 is runtime-blocked, qwen3:14b
	// is "tight" at 12.1 vs 12 (needs headroom), so the best comfortable+supported is 8b.
	p := Probe{VRAMGiB: 12, OllamaVersion: "0.13.5", Reachable: true}
	entries := annotateCatalog(p)
	var rec *CatalogEntry
	for i := range entries {
		if entries[i].Recommended {
			if rec != nil {
				t.Fatal("more than one model marked recommended")
			}
			rec = &entries[i]
		}
	}
	if rec == nil {
		t.Fatal("no model recommended")
	}
	if rec.Tag != "qwen3:8b" {
		t.Errorf("recommended = %q, want qwen3:8b (qwen3.5 runtime-blocked, 14b tight)", rec.Tag)
	}
	// qwen3.5 must be present but runtime-blocked (not recommended).
	for _, e := range entries {
		if e.Tag == "qwen3.5:9b" {
			if e.RuntimeOK {
				t.Error("qwen3.5:9b should be runtime-blocked on Ollama 0.13.5")
			}
			if e.Recommended {
				t.Error("a runtime-blocked model must not be recommended")
			}
		}
	}
}

// On a newer runtime, the current-gen qwen3.5 wins.
func TestAnnotateCatalog_NewerRuntimePrefersCurrentGen(t *testing.T) {
	p := Probe{VRAMGiB: 12, OllamaVersion: "0.14.2", Reachable: true, PulledModels: []string{"qwen3:8b"}}
	entries := annotateCatalog(p)
	for _, e := range entries {
		if e.Recommended && e.Tag != "qwen3.5:9b" {
			t.Errorf("recommended = %q, want qwen3.5:9b on a supporting runtime", e.Tag)
		}
		if e.Tag == "qwen3:8b" && !e.Pulled {
			t.Error("qwen3:8b should be marked pulled")
		}
	}
}

// A tiny/no GPU: nothing "fits", but we still recommend the best tight+supported
// model rather than leaving the user with no guidance.
func TestAnnotateCatalog_LowVRAMStillRecommends(t *testing.T) {
	p := Probe{VRAMGiB: 5.0, OllamaVersion: "0.13.5", Reachable: true}
	entries := annotateCatalog(p)
	var rec string
	for _, e := range entries {
		if e.Recommended {
			rec = e.Tag
		}
	}
	if rec == "" {
		t.Fatal("expected a fallback recommendation even when nothing fits comfortably")
	}
	// llama3.1:8b (4.9) is the only one at/under 5.0GB weights → the others are wont_fit.
	if rec != "llama3.1:8b" {
		t.Errorf("low-VRAM recommendation = %q, want llama3.1:8b", rec)
	}
}
