package recommend

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

const corpusManifestPath = "testdata/channel-recommendation-v1.json"

//go:embed testdata/channel-recommendation-v1.json testdata/channel-recommendation-snapshots-v1.json
var corpusFiles embed.FS

type Thresholds struct {
	MinRelevance          float64 `json:"minRelevance"`
	MinNovelty            float64 `json:"minNovelty"`
	MinDiversity          float64 `json:"minDiversity"`
	MinCatalogFeasibility float64 `json:"minCatalogFeasibility"`
	MinPolicySafety       float64 `json:"minPolicySafety"`
	MinSchemaValidity     float64 `json:"minSchemaValidity"`
	MinAbstention         float64 `json:"minAbstention"`
}

type FixtureIdentity struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type Corpus struct {
	SchemaVersion         int             `json:"schemaVersion"`
	Version               string          `json:"version"`
	Split                 string          `json:"split"`
	PromptVersion         string          `json:"promptVersion"`
	SchemaVersionName     string          `json:"schemaVersionName"`
	ScorerVersion         string          `json:"scorerVersion"`
	HardMetrics           []string        `json:"hardMetrics"`
	QualityMetrics        []string        `json:"qualityMetrics"`
	Thresholds            Thresholds      `json:"thresholds"`
	SelectionMargin       float64         `json:"selectionMargin"`
	AllowedTrainingSplits []string        `json:"allowedTrainingSplits"`
	Fixture               FixtureIdentity `json:"fixture"`
	Cases                 []Case          `json:"-"`
}

type Expectation struct {
	MinConcepts          int      `json:"minConcepts"`
	MaxConcepts          int      `json:"maxConcepts"`
	AllowAbstention      bool     `json:"allowAbstention"`
	RequiredEvidenceIDs  []string `json:"requiredEvidenceIds,omitempty"`
	RequiredIntentTerms  []string `json:"requiredIntentTerms,omitempty"`
	ForbiddenIntentTerms []string `json:"forbiddenIntentTerms,omitempty"`
}

type Case struct {
	ID          string
	Axes        []string
	Snapshot    Snapshot
	Expectation Expectation
}

type fixtureFile struct {
	SchemaVersion int               `json:"schemaVersion"`
	Cases         []fixtureCaseWire `json:"cases"`
}

type fixtureCaseWire struct {
	ID          string          `json:"id"`
	Axes        []string        `json:"axes"`
	Snapshot    json.RawMessage `json:"snapshot"`
	Expectation Expectation     `json:"expectation"`
}

// LoadCorpus verifies the embedded held-out fixture digest and every closed
// snapshot schema before returning any certification case.
func LoadCorpus() (Corpus, error) {
	manifestBlob, err := corpusFiles.ReadFile(corpusManifestPath)
	if err != nil {
		return Corpus{}, fmt.Errorf("read recommendation manifest: %w", err)
	}
	var corpus Corpus
	if err := json.Unmarshal(manifestBlob, &corpus); err != nil {
		return Corpus{}, fmt.Errorf("decode recommendation manifest: %w", err)
	}
	fixtureBlob, err := corpusFiles.ReadFile(corpus.Fixture.Path)
	if err != nil {
		return Corpus{}, fmt.Errorf("read recommendation fixture: %w", err)
	}
	digest := sha256.Sum256(fixtureBlob)
	if hex.EncodeToString(digest[:]) != corpus.Fixture.SHA256 {
		return Corpus{}, fmt.Errorf("recommendation fixture digest mismatch")
	}
	var fixture fixtureFile
	if err := json.Unmarshal(fixtureBlob, &fixture); err != nil {
		return Corpus{}, fmt.Errorf("decode recommendation fixture: %w", err)
	}
	if corpus.SchemaVersion != 1 || corpus.Version == "" || corpus.Split != "certification" ||
		corpus.PromptVersion == "" || corpus.SchemaVersionName == "" || corpus.ScorerVersion == "" ||
		fixture.SchemaVersion != 1 || len(fixture.Cases) == 0 {
		return Corpus{}, fmt.Errorf("invalid recommendation corpus identity")
	}
	seen := make(map[string]bool, len(fixture.Cases))
	for _, wire := range fixture.Cases {
		snapshot, err := DecodeSnapshot(wire.Snapshot)
		if err != nil {
			return Corpus{}, fmt.Errorf("case %q: %w", wire.ID, err)
		}
		if wire.ID == "" || seen[wire.ID] || snapshot.ID != wire.ID || len(wire.Axes) == 0 ||
			wire.Expectation.MinConcepts < 0 || wire.Expectation.MaxConcepts < wire.Expectation.MinConcepts {
			return Corpus{}, fmt.Errorf("invalid recommendation case %q", wire.ID)
		}
		seen[wire.ID] = true
		corpus.Cases = append(corpus.Cases, Case{ID: wire.ID, Axes: wire.Axes, Snapshot: snapshot, Expectation: wire.Expectation})
	}
	for _, split := range corpus.AllowedTrainingSplits {
		if split == corpus.Split {
			return Corpus{}, fmt.Errorf("certification split cannot be used for training")
		}
	}
	return corpus, nil
}
