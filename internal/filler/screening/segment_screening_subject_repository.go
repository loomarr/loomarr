package screening

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/split"
)

const (
	segmentScreeningSubjectMaxBytes = 64 << 10
	segmentScreeningSubjectKind     = "screening-subjects"
)

func (r *FileSegmentScreeningEvidenceRepository) PutSegmentScreeningSubject(ctx context.Context, subject asset.SegmentScreeningSubject) error {
	if r == nil || r.files == nil {
		return fmt.Errorf("segment screening evidence repository is unavailable")
	}
	if err := asset.ValidateSegmentScreeningSubject(subject); err != nil {
		return err
	}
	raw, err := json.Marshal(subject)
	if err != nil || len(raw) > segmentScreeningSubjectMaxBytes {
		return fmt.Errorf("marshal segment screening subject")
	}
	if err := r.files.PutBlob(ctx, segmentScreeningSubjectKind, subject.SHA256, raw, segmentScreeningSubjectMaxBytes); err != nil {
		return fmt.Errorf("persist segment screening subject: %w", err)
	}
	return nil
}

func (r *FileSegmentScreeningEvidenceRepository) GetSegmentScreeningSubject(ctx context.Context, subjectSHA256 string) (asset.SegmentScreeningSubject, error) {
	if r == nil || r.files == nil || !split.IsContentHash(subjectSHA256) {
		return asset.SegmentScreeningSubject{}, fmt.Errorf("segment screening subject identity is invalid")
	}
	raw, err := r.files.ReadBlob(ctx, segmentScreeningSubjectKind, subjectSHA256, segmentScreeningSubjectMaxBytes)
	if err != nil {
		return asset.SegmentScreeningSubject{}, fmt.Errorf("read segment screening subject: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var subject asset.SegmentScreeningSubject
	if err := decoder.Decode(&subject); err != nil {
		return asset.SegmentScreeningSubject{}, fmt.Errorf("decode segment screening subject: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return asset.SegmentScreeningSubject{}, fmt.Errorf("decode segment screening subject: trailing JSON")
	}
	if subject.SHA256 != subjectSHA256 {
		return asset.SegmentScreeningSubject{}, fmt.Errorf("segment screening subject path does not match its identity")
	}
	if err := asset.ValidateSegmentScreeningSubject(subject); err != nil {
		return asset.SegmentScreeningSubject{}, err
	}
	return subject, nil
}
