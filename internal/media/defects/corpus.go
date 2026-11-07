package defects

import (
	"embed"
	"encoding/json"
	"fmt"
	"strconv"
)

// CorpusVersion names the committed synthetic corpus the bars are scored against (#2034 step 4c).
// A new corpus is a new file and a new version; an old one is never edited, so a score always
// names exactly the labels it was taken against.
const CorpusVersion = "defect-corpus-v1"

//go:embed corpus/defect-corpus-v1.json
var corpusFiles embed.FS

// Corpus is the committed synthetic corpus: invented clips, each damaged one known way at one
// known severity and labelled per era by how it was made.
type Corpus struct {
	Version    string           `json:"version"`
	Eras       []int            `json:"eras"`
	Thresholds CorpusThresholds `json:"thresholds"`
	Cases      []CorpusCase     `json:"cases"`
}

// CorpusThresholds are what the floor must score on the corpus.
type CorpusThresholds struct {
	// MaxFalseFails is how many clips labelled pass the detectors may fail outright, in any era.
	MaxFalseFails int `json:"maxFalseFails"`
	// MinCaught is the share of clips labelled fail that the detectors fail or send to the judge.
	MinCaught float64 `json:"minCaught"`
}

// CorpusCase is one damaged clip and its label in each era.
type CorpusCase struct {
	Damage   string           `json:"damage"`
	Severity float64          `json:"severity"`
	Labels   map[string]Label `json:"labels"`
}

// LoadCorpus reads the committed corpus and checks that every case is labelled for every era.
func LoadCorpus() (Corpus, error) {
	raw, err := corpusFiles.ReadFile("corpus/" + CorpusVersion + ".json")
	if err != nil {
		return Corpus{}, err
	}
	var c Corpus
	if err := json.Unmarshal(raw, &c); err != nil {
		return Corpus{}, fmt.Errorf("defect corpus: %w", err)
	}
	if c.Version != CorpusVersion {
		return Corpus{}, fmt.Errorf("defect corpus: file says %q, want %q", c.Version, CorpusVersion)
	}
	for _, cs := range c.Cases {
		for _, era := range c.Eras {
			if l := cs.Labels[strconv.Itoa(era)]; l != LabelPass && l != LabelFail {
				return Corpus{}, fmt.Errorf("defect corpus: %s %g has no label for %d", cs.Damage, cs.Severity, era)
			}
		}
	}
	return c, nil
}
