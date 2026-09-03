package filler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
)

const segmentScreeningEvidenceMaxBytes = 64 << 10

// FileStructureScreeningEvidenceRepository stores exact-span aggregates separately from
// complete-timeline assessment records while reusing the same private immutable-file discipline.
type FileStructureScreeningEvidenceRepository struct {
	files *FileStructureAssessmentEvidenceRepository
}

func NewFileStructureScreeningEvidenceRepository(root string) (*FileStructureScreeningEvidenceRepository, error) {
	files, err := NewFileStructureAssessmentEvidenceRepository(root)
	if err != nil {
		return nil, err
	}
	return &FileStructureScreeningEvidenceRepository{files: files}, nil
}

func (r *FileStructureScreeningEvidenceRepository) PutSegmentScreeningEvidence(ctx context.Context, evidence SegmentScreeningEvidence) error {
	if r == nil || r.files == nil {
		return fmt.Errorf("structure screening evidence repository is unavailable")
	}
	if err := ValidateSegmentScreeningEvidence(evidence); err != nil {
		return err
	}
	raw, err := json.Marshal(evidence)
	if err != nil || len(raw) > segmentScreeningEvidenceMaxBytes {
		return fmt.Errorf("marshal structure screening evidence")
	}
	if err := r.files.putImmutable(ctx, r.path(evidence.SHA256), raw, segmentScreeningEvidenceMaxBytes); err != nil {
		return fmt.Errorf("persist structure screening evidence: %w", err)
	}
	return nil
}

func (r *FileStructureScreeningEvidenceRepository) GetSegmentScreeningEvidence(ctx context.Context, evidenceSHA256 string) (SegmentScreeningEvidence, error) {
	if r == nil || r.files == nil || !structureEvidenceDigest(evidenceSHA256) {
		return SegmentScreeningEvidence{}, fmt.Errorf("structure screening evidence identity is invalid")
	}
	raw, err := r.files.readImmutable(ctx, r.path(evidenceSHA256), segmentScreeningEvidenceMaxBytes)
	if err != nil {
		return SegmentScreeningEvidence{}, fmt.Errorf("read structure screening evidence: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var evidence SegmentScreeningEvidence
	if err := decoder.Decode(&evidence); err != nil {
		return SegmentScreeningEvidence{}, fmt.Errorf("decode structure screening evidence: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return SegmentScreeningEvidence{}, fmt.Errorf("decode structure screening evidence: trailing JSON")
	}
	if evidence.SHA256 != evidenceSHA256 {
		return SegmentScreeningEvidence{}, fmt.Errorf("structure screening path does not match its identity")
	}
	if err := ValidateSegmentScreeningEvidence(evidence); err != nil {
		return SegmentScreeningEvidence{}, err
	}
	return evidence, nil
}

func (r *FileStructureScreeningEvidenceRepository) path(digest string) string {
	return filepath.Join(r.files.root, "screenings", digest[:2], digest+".json")
}

var _ StructureScreeningEvidenceRepository = (*FileStructureScreeningEvidenceRepository)(nil)
