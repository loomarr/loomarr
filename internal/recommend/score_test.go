package recommend_test

import (
	"testing"

	"github.com/loomarr/loomarr/internal/recommend"
)

func TestScoreCaseSeparatesQualityFromHardFailuresAndRewardsCorrectAbstention(t *testing.T) {
	corpus, err := recommend.LoadCorpus()
	if err != nil {
		t.Fatal(err)
	}
	family := corpusCase(t, corpus, "family-safe")
	familyRaw := []byte(`{"concepts":[{"name":"Family Animation Club","intent":{"description":"A family-safe animation channel"},"evidenceIds":["library:genre:animation","constraint:audience:family"]}]}`)
	familyResult := recommend.ScoreCase(family, familyRaw)
	if !familyResult.Passed || len(familyResult.HardFailures) != 0 {
		t.Fatalf("family result = %+v", familyResult)
	}
	if familyResult.Quality.Relevance != 1 || familyResult.Quality.CatalogFeasibility != 1 || familyResult.Quality.PolicySafety != 1 || familyResult.Quality.Abstention != 1 {
		t.Fatalf("family quality = %+v", familyResult.Quality)
	}

	conflicting := corpusCase(t, corpus, "conflicting-preferences")
	abstained := recommend.ScoreCase(conflicting, []byte(`{"concepts":[]}`))
	if !abstained.Passed || abstained.Quality.Abstention != 1 {
		t.Fatalf("conflicting abstention = %+v", abstained)
	}
}

func corpusCase(t *testing.T, corpus recommend.Corpus, id string) recommend.Case {
	t.Helper()
	for _, c := range corpus.Cases {
		if c.ID == id {
			return c
		}
	}
	t.Fatalf("missing corpus case %q", id)
	return recommend.Case{}
}
