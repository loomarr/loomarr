package recommend_test

import (
	"encoding/json"
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

func TestLoadDevelopmentCorpusIsDigestDisjointFromCertification(t *testing.T) {
	certification, err := recommend.LoadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	development, err := recommend.LoadDevelopmentCorpus()
	if err != nil {
		t.Fatal(err)
	}
	if development.Version != "channel-recommendation-development-v1" || development.Split != "development" {
		t.Fatalf("development identity = %+v", development)
	}
	if err := recommend.VerifyCorpusDisjoint(development, certification); err != nil {
		t.Fatalf("development/certification split = %v", err)
	}
	if development.Fixture.SHA256 == certification.Fixture.SHA256 {
		t.Fatal("development reused the certification fixture digest")
	}
}

func TestVerifyCorpusDisjointRejectsReusedIDsAndSnapshots(t *testing.T) {
	certification, err := recommend.LoadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	development, err := recommend.LoadDevelopmentCorpus()
	if err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*recommend.Corpus){
		"case id": func(corpus *recommend.Corpus) {
			corpus.Cases[0].ID = certification.Cases[0].ID
		},
		"snapshot content": func(corpus *recommend.Corpus) {
			blob, marshalErr := json.Marshal(certification.Cases[0].Snapshot)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var reused recommend.Snapshot
			if unmarshalErr := json.Unmarshal(blob, &reused); unmarshalErr != nil {
				t.Fatal(unmarshalErr)
			}
			reused.ID = corpus.Cases[0].ID
			corpus.Cases[0].Snapshot = reused
		},
	} {
		t.Run(name, func(t *testing.T) {
			copyCorpus := development
			copyCorpus.Cases = append([]recommend.Case(nil), development.Cases...)
			mutate(&copyCorpus)
			if err := recommend.VerifyCorpusDisjoint(copyCorpus, certification); err == nil {
				t.Fatal("reused certification material was accepted")
			}
		})
	}
}
