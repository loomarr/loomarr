package store

import "testing"

func TestInferenceCacheKeyIncludesEverySemanticVersion(t *testing.T) {
	t.Parallel()
	base := InferenceEvaluation{
		ClipHash: "sha256:clip", Role: "filler_frames", Rung: "frames",
		RequestedProvider: "openrouter", RequestedModel: "google/gemini-3.7-flash",
		UpstreamProvider: "Google", Modalities: []string{"image", "text"},
		DerivativeBytes: 2048, DerivativeDurationMS: 30_000, DerivativePixels: 2_073_600,
		Versions: InferenceVersions{
			Evidence: "e1", Extractor: "x1", Prompt: "p1", Schema: "s1", Taxonomy: "t1",
			AdmissionPolicy: "a1", RolePolicy: "r1", CapabilitySnapshot: "c1",
		},
	}
	want := InferenceCacheKey(base)
	if want == "" {
		t.Fatal("empty inference cache key")
	}
	changes := map[string]func(*InferenceEvaluation){
		"evidence":            func(e *InferenceEvaluation) { e.Versions.Evidence = "e2" },
		"extractor":           func(e *InferenceEvaluation) { e.Versions.Extractor = "x2" },
		"prompt":              func(e *InferenceEvaluation) { e.Versions.Prompt = "p2" },
		"schema":              func(e *InferenceEvaluation) { e.Versions.Schema = "s2" },
		"taxonomy":            func(e *InferenceEvaluation) { e.Versions.Taxonomy = "t2" },
		"admission policy":    func(e *InferenceEvaluation) { e.Versions.AdmissionPolicy = "a2" },
		"role policy":         func(e *InferenceEvaluation) { e.Versions.RolePolicy = "r2" },
		"capability snapshot": func(e *InferenceEvaluation) { e.Versions.CapabilitySnapshot = "c2" },
		"provider":            func(e *InferenceEvaluation) { e.RequestedProvider = "custom" },
		"model":               func(e *InferenceEvaluation) { e.RequestedModel = "other/model" },
		"modality":            func(e *InferenceEvaluation) { e.Modalities = []string{"video", "text"} },
		"derivative":          func(e *InferenceEvaluation) { e.DerivativeBytes++ },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			changed := base
			change(&changed)
			if got := InferenceCacheKey(changed); got == want {
				t.Fatalf("cache key did not change with %s", name)
			}
		})
	}

	reordered := base
	reordered.Modalities = []string{"text", "image"}
	if got := InferenceCacheKey(reordered); got != want {
		t.Fatalf("modality set order changed cache key: %s != %s", got, want)
	}
}
