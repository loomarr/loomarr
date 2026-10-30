package fillerscreening

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/loomarr/loomarr/internal/filler"
)

const (
	segmentScreeningEvidenceMaxBytes = 64 << 10
	segmentScreeningEvidenceKind     = "screenings"
)

// SegmentScreeningEvidenceRepository stores provider-neutral axis evidence and its four-axis
// aggregate. Concrete evaluators own their private raw-evidence settlement.
type SegmentScreeningEvidenceRepository interface {
	PutSegmentScreeningSubject(context.Context, filler.SegmentScreeningSubject) error
	PutSegmentScreeningAxisEvidence(context.Context, RecordedSegmentScreeningAxisEvidence) error
	PutSegmentScreeningEvidence(context.Context, filler.SegmentScreeningEvidence) error
}

// FileSegmentScreeningEvidenceRepository stores rendered-child aggregates separately from
// complete-timeline assessment records while reusing the same private immutable-file discipline.
type FileSegmentScreeningEvidenceRepository struct {
	files *filler.FileStructureAssessmentEvidenceRepository
}

func NewFileSegmentScreeningEvidenceRepository(root string) (*FileSegmentScreeningEvidenceRepository, error) {
	files, err := filler.NewFileStructureAssessmentEvidenceRepository(root)
	if err != nil {
		return nil, err
	}
	return &FileSegmentScreeningEvidenceRepository{files: files}, nil
}

func (r *FileSegmentScreeningEvidenceRepository) PutSegmentScreeningEvidence(ctx context.Context, evidence filler.SegmentScreeningEvidence) error {
	if r == nil || r.files == nil {
		return fmt.Errorf("segment screening evidence repository is unavailable")
	}
	if err := filler.ValidateSegmentScreeningEvidence(evidence); err != nil {
		return err
	}
	raw, err := json.Marshal(evidence)
	if err != nil || len(raw) > segmentScreeningEvidenceMaxBytes {
		return fmt.Errorf("marshal segment screening evidence")
	}
	if err := r.files.PutBlob(ctx, segmentScreeningEvidenceKind, evidence.SHA256, raw, segmentScreeningEvidenceMaxBytes); err != nil {
		return fmt.Errorf("persist segment screening evidence: %w", err)
	}
	return nil
}

func (r *FileSegmentScreeningEvidenceRepository) GetSegmentScreeningEvidence(ctx context.Context, evidenceSHA256 string) (filler.SegmentScreeningEvidence, error) {
	if r == nil || r.files == nil || !filler.IsContentHash(evidenceSHA256) {
		return filler.SegmentScreeningEvidence{}, fmt.Errorf("segment screening evidence identity is invalid")
	}
	raw, err := r.files.ReadBlob(ctx, segmentScreeningEvidenceKind, evidenceSHA256, segmentScreeningEvidenceMaxBytes)
	if err != nil {
		return filler.SegmentScreeningEvidence{}, fmt.Errorf("read segment screening evidence: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var evidence filler.SegmentScreeningEvidence
	if err := decoder.Decode(&evidence); err != nil {
		return filler.SegmentScreeningEvidence{}, fmt.Errorf("decode segment screening evidence: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return filler.SegmentScreeningEvidence{}, fmt.Errorf("decode segment screening evidence: trailing JSON")
	}
	if evidence.SHA256 != evidenceSHA256 {
		return filler.SegmentScreeningEvidence{}, fmt.Errorf("segment screening path does not match its identity")
	}
	if err := filler.ValidateSegmentScreeningEvidence(evidence); err != nil {
		return filler.SegmentScreeningEvidence{}, err
	}
	return evidence, nil
}

var _ SegmentScreeningEvidenceRepository = (*FileSegmentScreeningEvidenceRepository)(nil)
