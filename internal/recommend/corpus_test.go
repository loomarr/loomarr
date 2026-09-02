package recommend_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/recommend"
)

func TestLoadCorpusVerifiesFrozenHeldOutRecommendationFamilies(t *testing.T) {
	corpus, err := recommend.LoadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if corpus.Version != "channel-recommendation-v1" || corpus.Split != "certification" || corpus.PromptVersion == "" || corpus.ScorerVersion == "" {
		t.Fatalf("corpus identity = %+v", corpus)
	}
	if len(corpus.Cases) != 8 {
		t.Fatalf("recommendation families = %d, want 8", len(corpus.Cases))
	}
	wantAxes := map[string]bool{
		"sparse": false, "broad": false, "repetitive": false, "family": false,
		"seasonal": false, "era-heavy": false, "conflicting": false, "adversarial": false,
	}
	for _, c := range corpus.Cases {
		for _, axis := range c.Axes {
			if _, known := wantAxes[axis]; known {
				wantAxes[axis] = true
			}
		}
		if c.Snapshot.ID == "" || c.Expectation.MaxConcepts < c.Expectation.MinConcepts {
			t.Fatalf("invalid case = %+v", c)
		}
	}
	for axis, seen := range wantAxes {
		if !seen {
			t.Errorf("missing recommendation axis %q", axis)
		}
	}
	for _, split := range corpus.AllowedTrainingSplits {
		if split == corpus.Split {
			t.Fatalf("certification split entered training allowlist: %v", corpus.AllowedTrainingSplits)
		}
	}
}
