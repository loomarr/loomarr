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
	segmentScreeningSubjectMaxBytes = 64 << 10
	segmentScreeningSubjectKind     = "screening-subjects"
)

func (r *FileSegmentScreeningEvidenceRepository) PutSegmentScreeningSubject(ctx context.Context, subject filler.SegmentScreeningSubject) error {
	if r == nil || r.files == nil {
		return fmt.Errorf("segment screening evidence repository is unavailable")
	}
	if err := filler.ValidateSegmentScreeningSubject(subject); err != nil {
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

func (r *FileSegmentScreeningEvidenceRepository) GetSegmentScreeningSubject(ctx context.Context, subjectSHA256 string) (filler.SegmentScreeningSubject, error) {
	if r == nil || r.files == nil || !filler.IsContentHash(subjectSHA256) {
		return filler.SegmentScreeningSubject{}, fmt.Errorf("segment screening subject identity is invalid")
	}
	raw, err := r.files.ReadBlob(ctx, segmentScreeningSubjectKind, subjectSHA256, segmentScreeningSubjectMaxBytes)
	if err != nil {
		return filler.SegmentScreeningSubject{}, fmt.Errorf("read segment screening subject: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var subject filler.SegmentScreeningSubject
	if err := decoder.Decode(&subject); err != nil {
		return filler.SegmentScreeningSubject{}, fmt.Errorf("decode segment screening subject: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return filler.SegmentScreeningSubject{}, fmt.Errorf("decode segment screening subject: trailing JSON")
	}
	if subject.SHA256 != subjectSHA256 {
		return filler.SegmentScreeningSubject{}, fmt.Errorf("segment screening subject path does not match its identity")
	}
	if err := filler.ValidateSegmentScreeningSubject(subject); err != nil {
		return filler.SegmentScreeningSubject{}, err
	}
	return subject, nil
}
