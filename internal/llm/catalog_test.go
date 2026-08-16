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

// installed is a test helper: an InstalledModel with the size (in GiB) and params a
// scenario needs, tool-capable by default. The catalog is built LIVE from these — the
// whole point of §8.1 — so tests seed Probe.Installed, never a hardcoded catalog.
func installed(tag string, gib float64, params string, tools bool) InstalledModel {
	capability := ToolCapabilityUnsupported
	if tools {
		capability = ToolCapabilityVerified
	}
	return InstalledModel{
		Tag:            tag,
		SizeBytes:      int64(gib * bytesPerGiB),
		VRAMGiB:        gib,
		ParameterSize:  params,
		Family:         "test",
		Quant:          "Q4_K_M",
		Tools:          tools,
		ToolCapability: capability,
	}
}

// Every installed model stays visible so unsupported and unverified are actionable,
// but only verified models can be recommended or selected for grounded curation.
func TestAnnotateCatalog_ExposesToolCapabilityHonestly(t *testing.T) {
	unverified := installed("show-unavailable:8b", 5.0, "8B", false)
	unverified.ToolCapability = ToolCapabilityUnverified
	p := Probe{
		VRAMGiB:   12,
		Reachable: true,
		Installed: []InstalledModel{
			installed("qwen3:8b", 5.2, "8B", true),
			installed("deepseek-r1:14b", 9.0, "14B", false),
			unverified,
		},
	}
	entries := annotateCatalog(p)
	if len(entries) != 3 {
		t.Fatalf("got %d catalog entries, want all 3 installed models", len(entries))
	}
	want := map[string]ToolCapability{
		"qwen3:8b":            ToolCapabilityVerified,
		"deepseek-r1:14b":     ToolCapabilityUnsupported,
		"show-unavailable:8b": ToolCapabilityUnverified,
	}
	for _, entry := range entries {
		if got := entry.ToolCapability; got != want[entry.Tag] {
			t.Errorf("%s capability = %q, want %q", entry.Tag, got, want[entry.Tag])
		}
		if entry.Tools != (entry.ToolCapability == ToolCapabilityVerified) {
			t.Errorf("%s compatibility Tools disagrees with capability", entry.Tag)
		}
		if entry.Recommended && entry.ToolCapability != ToolCapabilityVerified {
			t.Errorf("%s is %q but was recommended", entry.Tag, entry.ToolCapability)
		}
	}
}

// Among the models that fit, the recommendation is the largest (bigger ⇒ more capable
// grounding, within the VRAM budget). A model that won't fit is never recommended.
func TestAnnotateCatalog_Recommendation(t *testing.T) {
	// 12GB card. 8b (5.2) and 4b (2.6) both fit comfortably; 14b (12.1) is over the
	// card. Best comfortable fit is the larger of {8b, 4b} → 8b.
	p := Probe{
		VRAMGiB:   12,
		Reachable: true,
		Installed: []InstalledModel{
			installed("qwen3:4b", 2.6, "4B", true),
			installed("qwen3:8b", 5.2, "8B", true),
			installed("qwen3:14b", 12.1, "14B", true), // won't fit (weights > 12GB)
		},
	}
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
		t.Errorf("recommended = %q, want qwen3:8b (largest that fits comfortably)", rec.Tag)
	}
	// The 14b must be present but flagged won't-fit and never recommended.
	for _, e := range entries {
		if e.Tag == "qwen3:14b" {
			if e.Fit != FitWontFit {
				t.Errorf("qwen3:14b fit = %q, want wont_fit on a 12GB card", e.Fit)
			}
			if e.Recommended {
				t.Error("a won't-fit model must not be recommended")
			}
		}
	}
}

// A tiny/no GPU: nothing fits comfortably, but we still recommend the best tight fit
// rather than leaving the user with no guidance.
func TestAnnotateCatalog_LowVRAMStillRecommends(t *testing.T) {
	// 5GB card. llama3.1:8b (4.9) fits tight (≤5 weights, but < headroom); the 14b
	// (9.0) won't fit at all. Recommend the tight-but-runnable one.
	p := Probe{
		VRAMGiB:   5.0,
		Reachable: true,
		Installed: []InstalledModel{
			installed("llama3.1:8b", 4.9, "8B", true),
			installed("qwen3:14b", 9.0, "14B", true), // wont_fit
		},
	}
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
	if rec != "llama3.1:8b" {
		t.Errorf("low-VRAM recommendation = %q, want llama3.1:8b", rec)
	}
}

// Nothing installed (or nothing tool-capable) → an empty catalog and no recommendation.
// The UI shows the browse-to-download surface instead of a false recommendation.
func TestAnnotateCatalog_NoneInstalled(t *testing.T) {
	if got := annotateCatalog(Probe{VRAMGiB: 12, Reachable: true}); len(got) != 0 {
		t.Errorf("empty Installed → %d entries, want 0", len(got))
	}
}
