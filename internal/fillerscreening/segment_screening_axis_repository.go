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
	segmentScreeningAxisRecordMaxBytes = 64 << 10
	segmentScreeningAxisRawMaxBytes    = 1 << 20

	segmentScreeningAxisRawKind    = "screening-axis-raw"
	segmentScreeningAxisRecordKind = "screening-axis-records"
)

func (r *FileSegmentScreeningEvidenceRepository) PutSegmentScreeningAxisEvidence(ctx context.Context, recorded RecordedSegmentScreeningAxisEvidence) error {
	if r == nil || r.files == nil {
		return fmt.Errorf("segment screening evidence repository is unavailable")
	}
	if err := ValidateRecordedSegmentScreeningAxisEvidence(recorded); err != nil {
		return err
	}
	if len(recorded.RawEvidence) > segmentScreeningAxisRawMaxBytes {
		return fmt.Errorf("segment screening raw evidence exceeds its ceiling")
	}
	raw, err := json.Marshal(recorded.Evidence)
	if err != nil || len(raw) > segmentScreeningAxisRecordMaxBytes {
		return fmt.Errorf("marshal segment screening axis evidence")
	}
	if err := r.files.PutBlob(ctx, segmentScreeningAxisRawKind, recorded.Evidence.RawEvidenceSHA256, recorded.RawEvidence, segmentScreeningAxisRawMaxBytes); err != nil {
		return fmt.Errorf("persist segment screening raw evidence: %w", err)
	}
	if err := r.files.PutBlob(ctx, segmentScreeningAxisRecordKind, recorded.Evidence.SHA256, raw, segmentScreeningAxisRecordMaxBytes); err != nil {
		return fmt.Errorf("persist segment screening axis record: %w", err)
	}
	if err := r.putSegmentScreeningOperation(ctx, recorded.Evidence); err != nil {
		return err
	}
	return nil
}

func (r *FileSegmentScreeningEvidenceRepository) GetSegmentScreeningAxisEvidence(ctx context.Context, evidenceSHA256 string) (RecordedSegmentScreeningAxisEvidence, error) {
	evidence, err := r.GetSegmentScreeningAxisRecord(ctx, evidenceSHA256)
	if err != nil {
		return RecordedSegmentScreeningAxisEvidence{}, err
	}
	rawEvidence, err := r.files.ReadBlob(ctx, segmentScreeningAxisRawKind, evidence.RawEvidenceSHA256, segmentScreeningAxisRawMaxBytes)
	if err != nil {
		return RecordedSegmentScreeningAxisEvidence{}, fmt.Errorf("read segment screening raw evidence: %w", err)
	}
	recorded := RecordedSegmentScreeningAxisEvidence{Evidence: evidence, RawEvidence: rawEvidence}
	if err := ValidateRecordedSegmentScreeningAxisEvidence(recorded); err != nil {
		return RecordedSegmentScreeningAxisEvidence{}, err
	}
	return recorded, nil
}

// GetSegmentScreeningAxisRecord opens only the provider-neutral record. Browser-safe read models
// use it to reproduce closed evaluator identity without loading private raw evidence into memory.
func (r *FileSegmentScreeningEvidenceRepository) GetSegmentScreeningAxisRecord(ctx context.Context, evidenceSHA256 string) (SegmentScreeningAxisEvidence, error) {
	if r == nil || r.files == nil || !filler.IsContentHash(evidenceSHA256) {
		return SegmentScreeningAxisEvidence{}, fmt.Errorf("segment screening axis evidence identity is invalid")
	}
	raw, err := r.files.ReadBlob(ctx, segmentScreeningAxisRecordKind, evidenceSHA256, segmentScreeningAxisRecordMaxBytes)
	if err != nil {
		return SegmentScreeningAxisEvidence{}, fmt.Errorf("read segment screening axis record: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var evidence SegmentScreeningAxisEvidence
	if err := decoder.Decode(&evidence); err != nil {
		return SegmentScreeningAxisEvidence{}, fmt.Errorf("decode segment screening axis record: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return SegmentScreeningAxisEvidence{}, fmt.Errorf("decode segment screening axis record: trailing JSON")
	}
	if evidence.SHA256 != evidenceSHA256 || ValidateSegmentScreeningAxisEvidence(evidence) != nil {
		return SegmentScreeningAxisEvidence{}, fmt.Errorf("segment screening axis path does not match valid evidence")
	}
	return evidence, nil
}

var _ SegmentScreeningCertificationEvidenceReader = (*FileSegmentScreeningEvidenceRepository)(nil)
