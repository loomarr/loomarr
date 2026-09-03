package filler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/loomarr/loomarr/internal/fillerstructurewindow"
)

const (
	structureWindowAssessmentMaximumBytes = 1 << 20
	structureWindowStitchMaximumBytes     = 4 << 20
)

func (r *FileStructureAssessmentEvidenceRepository) PutStructureWindowAssessment(ctx context.Context, set fillerstructurewindow.MediaSet, assessment fillerstructurewindow.Assessment) error {
	if r == nil || r.root == "" {
		return errors.New("structure window evidence repository is unavailable")
	}
	if err := fillerstructurewindow.ValidateAssessment(set, assessment); err != nil {
		return err
	}
	raw, err := json.Marshal(assessment)
	if err != nil || len(raw) == 0 || len(raw) > structureWindowAssessmentMaximumBytes {
		return errors.New("marshal bounded structure window assessment")
	}
	if err := r.putImmutable(ctx, r.blobPath("window-assessments", assessment.SHA256), raw, structureWindowAssessmentMaximumBytes); err != nil {
		return fmt.Errorf("persist structure window assessment: %w", err)
	}
	return nil
}

func (r *FileStructureAssessmentEvidenceRepository) GetStructureWindowAssessment(ctx context.Context, set fillerstructurewindow.MediaSet, assessmentSHA256 string) (fillerstructurewindow.Assessment, error) {
	if r == nil || r.root == "" || !structureEvidenceDigest(assessmentSHA256) {
		return fillerstructurewindow.Assessment{}, errors.New("structure window assessment identity is invalid")
	}
	raw, err := r.readImmutable(ctx, r.blobPath("window-assessments", assessmentSHA256), structureWindowAssessmentMaximumBytes)
	if err != nil {
		return fillerstructurewindow.Assessment{}, fmt.Errorf("read structure window assessment: %w", err)
	}
	var assessment fillerstructurewindow.Assessment
	if err := decodeStructureWindowEvidence(raw, &assessment); err != nil {
		return fillerstructurewindow.Assessment{}, fmt.Errorf("decode structure window assessment: %w", err)
	}
	if assessment.SHA256 != assessmentSHA256 {
		return fillerstructurewindow.Assessment{}, errors.New("structure window assessment path does not match content")
	}
	if err := fillerstructurewindow.ValidateAssessment(set, assessment); err != nil {
		return fillerstructurewindow.Assessment{}, err
	}
	return assessment, nil
}

func (r *FileStructureAssessmentEvidenceRepository) PutStructureWindowStitch(ctx context.Context, stitch fillerstructurewindow.StitchResult) error {
	if r == nil || r.root == "" {
		return errors.New("structure window evidence repository is unavailable")
	}
	if err := fillerstructurewindow.ValidateStitchResult(stitch); err != nil {
		return err
	}
	raw, err := json.Marshal(stitch)
	if err != nil || len(raw) == 0 || len(raw) > structureWindowStitchMaximumBytes {
		return errors.New("marshal bounded structure window stitch")
	}
	if err := r.putImmutable(ctx, r.blobPath("window-stitches", stitch.SHA256), raw, structureWindowStitchMaximumBytes); err != nil {
		return fmt.Errorf("persist structure window stitch: %w", err)
	}
	return nil
}

func (r *FileStructureAssessmentEvidenceRepository) GetStructureWindowStitch(ctx context.Context, stitchSHA256 string) (fillerstructurewindow.StitchResult, error) {
	if r == nil || r.root == "" || !structureEvidenceDigest(stitchSHA256) {
		return fillerstructurewindow.StitchResult{}, errors.New("structure window stitch identity is invalid")
	}
	raw, err := r.readImmutable(ctx, r.blobPath("window-stitches", stitchSHA256), structureWindowStitchMaximumBytes)
	if err != nil {
		return fillerstructurewindow.StitchResult{}, fmt.Errorf("read structure window stitch: %w", err)
	}
	var stitch fillerstructurewindow.StitchResult
	if err := decodeStructureWindowEvidence(raw, &stitch); err != nil {
		return fillerstructurewindow.StitchResult{}, fmt.Errorf("decode structure window stitch: %w", err)
	}
	if stitch.SHA256 != stitchSHA256 {
		return fillerstructurewindow.StitchResult{}, errors.New("structure window stitch path does not match content")
	}
	if err := fillerstructurewindow.ValidateStitchResult(stitch); err != nil {
		return fillerstructurewindow.StitchResult{}, err
	}
	return stitch, nil
}

func decodeStructureWindowEvidence(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON value")
	}
	return nil
}

var _ StructureWindowEvidenceRepository = (*FileStructureAssessmentEvidenceRepository)(nil)
