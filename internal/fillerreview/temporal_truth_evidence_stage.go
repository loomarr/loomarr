package fillerreview

import (
	"fmt"
	"os"
	"path/filepath"
)

// temporalTruthEvidenceStage owns the only publication transition. A failed
// build is removed from its private staging path; a successful build appears at
// the requested output path in one rename.
type temporalTruthEvidenceStage struct {
	path      string
	output    string
	published bool
}

func beginTemporalTruthEvidenceStage(output string) (*temporalTruthEvidenceStage, error) {
	resolved, err := filepath.Abs(output)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(resolved); err == nil || !os.IsNotExist(err) {
		return nil, fmt.Errorf("output already exists: %s", resolved)
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o750); err != nil {
		return nil, err
	}
	path, err := os.MkdirTemp(filepath.Dir(resolved), ".filler-temporal-truth-evidence-*")
	if err != nil {
		return nil, err
	}
	return &temporalTruthEvidenceStage{path: path, output: resolved}, nil
}

func (stage *temporalTruthEvidenceStage) Cleanup() {
	if !stage.published {
		_ = os.RemoveAll(stage.path)
	}
}

func (stage *temporalTruthEvidenceStage) Publish() error {
	if err := os.Rename(stage.path, stage.output); err != nil {
		return err
	}
	stage.published = true
	return nil
}
