package assessment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"reflect"

	"github.com/loomarr/loomarr/internal/filler/structure"
	"github.com/loomarr/loomarr/internal/filler/structure/window"
)

const (
	structureWindowAssessmentMaximumBytes  = 1 << 20
	structureWindowCallRecordMaximumBytes  = 1 << 20
	structureWindowCallPublicationMaxBytes = 64 << 10
	structureWindowStitchMaximumBytes      = 4 << 20
)

// PutStructureWindowAssessmentEvidence publishes provider blobs, semantic assessment, and call
// record before the operation publication. A resumable operation is therefore fully replayable.
func (r *FileStructureAssessmentEvidenceRepository) PutStructureWindowAssessmentEvidence(ctx context.Context, recorded window.RecordedAssessment) error {
	if r == nil || r.root == "" {
		return errors.New("structure window evidence repository is unavailable")
	}
	if err := window.ValidateRecordedAssessment(recorded); err != nil {
		return err
	}
	if window.RetryableCallRecord(recorded.Record) {
		return fmt.Errorf("publish structure window call: %w", ErrStructureWindowRetryable)
	}
	assessmentRaw, err := json.Marshal(recorded.Assessment)
	if err != nil || len(assessmentRaw) == 0 || len(assessmentRaw) > structureWindowAssessmentMaximumBytes {
		return errors.New("marshal bounded structure window assessment")
	}
	recordRaw, err := json.Marshal(recorded.Record)
	if err != nil || len(recordRaw) == 0 || len(recordRaw) > structureWindowCallRecordMaximumBytes {
		return errors.New("marshal bounded structure window call record")
	}
	publication, err := window.NewCallPublication(recorded.Record)
	if err != nil {
		return err
	}
	publicationRaw, err := json.Marshal(publication)
	if err != nil || len(publicationRaw) == 0 || len(publicationRaw) > structureWindowCallPublicationMaxBytes {
		return errors.New("marshal bounded structure window call publication")
	}
	if recorded.Record.ResponseSHA256 != "" {
		if err := r.putImmutable(ctx, r.blobPath("responses", recorded.Record.ResponseSHA256), recorded.RawResponse, structureAssessmentResponseMaxBytes); err != nil {
			return fmt.Errorf("persist structure window raw response: %w", err)
		}
	}
	if recorded.Record.StructuredOutputSHA256 != "" {
		if err := r.putImmutable(ctx, r.blobPath("outputs", recorded.Record.StructuredOutputSHA256), []byte(recorded.StructuredOutput), structureAssessmentResponseMaxBytes); err != nil {
			return fmt.Errorf("persist structure window structured output: %w", err)
		}
	}
	if err := r.putImmutable(ctx, r.blobPath("window-assessments", recorded.Assessment.SHA256), assessmentRaw, structureWindowAssessmentMaximumBytes); err != nil {
		return fmt.Errorf("persist structure window assessment: %w", err)
	}
	if err := r.putImmutable(ctx, r.blobPath("window-call-records", recorded.Record.SHA256), recordRaw, structureWindowCallRecordMaximumBytes); err != nil {
		return fmt.Errorf("persist structure window call record: %w", err)
	}
	if err := r.putImmutable(ctx, r.blobPath("window-call-publications", publication.OperationSHA256), publicationRaw, structureWindowCallPublicationMaxBytes); err != nil {
		return fmt.Errorf("publish structure window call: %w", err)
	}
	return nil
}

func (r *FileStructureAssessmentEvidenceRepository) FindStructureWindowAssessmentEvidence(ctx context.Context, set window.MediaSet, ordinal int, profile structure.AssessorProfile) (window.RecordedAssessment, bool, error) {
	if r == nil || r.root == "" || window.ValidateMediaSet(set) != nil ||
		ordinal < 0 || ordinal >= len(set.Windows) || structure.ValidateAssessorProfile(profile) != nil {
		return window.RecordedAssessment{}, false, errors.New("structure window call operation is invalid")
	}
	operationSHA256 := window.CallOperationSHA256(set, ordinal, profile)
	raw, err := r.readImmutable(ctx, r.blobPath("window-call-publications", operationSHA256), structureWindowCallPublicationMaxBytes)
	if errors.Is(err, fs.ErrNotExist) {
		return window.RecordedAssessment{}, false, nil
	}
	if err != nil {
		return window.RecordedAssessment{}, false, fmt.Errorf("read structure window call publication: %w", err)
	}
	var publication window.CallPublication
	if err := decodeStructureWindowEvidence(raw, &publication); err != nil || publication.OperationSHA256 != operationSHA256 {
		return window.RecordedAssessment{}, false, errors.New("decode structure window call publication")
	}
	recorded, err := r.GetStructureWindowAssessmentEvidence(ctx, set, publication.RecordSHA256)
	if err != nil {
		return window.RecordedAssessment{}, false, err
	}
	if err := window.ValidateCallPublication(publication, recorded.Record); err != nil {
		return window.RecordedAssessment{}, false, err
	}
	return recorded, true, nil
}

func (r *FileStructureAssessmentEvidenceRepository) GetStructureWindowAssessmentEvidence(ctx context.Context, set window.MediaSet, recordSHA256 string) (window.RecordedAssessment, error) {
	if r == nil || r.root == "" || !structureEvidenceDigest(recordSHA256) {
		return window.RecordedAssessment{}, errors.New("structure window call record identity is invalid")
	}
	recordRaw, err := r.readImmutable(ctx, r.blobPath("window-call-records", recordSHA256), structureWindowCallRecordMaximumBytes)
	if err != nil {
		return window.RecordedAssessment{}, fmt.Errorf("read structure window call record: %w", err)
	}
	var record window.CallRecord
	if err := decodeStructureWindowEvidence(recordRaw, &record); err != nil {
		return window.RecordedAssessment{}, fmt.Errorf("decode structure window call record: %w", err)
	}
	if record.SHA256 != recordSHA256 || !reflect.DeepEqual(record.MediaSet, set) {
		return window.RecordedAssessment{}, errors.New("structure window call record path or media set does not match")
	}
	assessmentRaw, err := r.readImmutable(ctx, r.blobPath("window-assessments", record.AssessmentSHA256), structureWindowAssessmentMaximumBytes)
	if err != nil {
		return window.RecordedAssessment{}, fmt.Errorf("read structure window assessment: %w", err)
	}
	var assessment window.Assessment
	if err := decodeStructureWindowEvidence(assessmentRaw, &assessment); err != nil {
		return window.RecordedAssessment{}, fmt.Errorf("decode structure window assessment: %w", err)
	}
	var rawResponse, structuredOutput []byte
	if record.ResponseSHA256 != "" {
		rawResponse, err = r.readImmutable(ctx, r.blobPath("responses", record.ResponseSHA256), structureAssessmentResponseMaxBytes)
		if err != nil {
			return window.RecordedAssessment{}, fmt.Errorf("read structure window raw response: %w", err)
		}
	}
	if record.StructuredOutputSHA256 != "" {
		structuredOutput, err = r.readImmutable(ctx, r.blobPath("outputs", record.StructuredOutputSHA256), structureAssessmentResponseMaxBytes)
		if err != nil {
			return window.RecordedAssessment{}, fmt.Errorf("read structure window structured output: %w", err)
		}
	}
	recorded := window.RecordedAssessment{
		Record: record, Assessment: assessment, RawResponse: rawResponse, StructuredOutput: string(structuredOutput),
	}
	if err := window.ValidateRecordedAssessment(recorded); err != nil {
		return window.RecordedAssessment{}, err
	}
	return recorded, nil
}

func (r *FileStructureAssessmentEvidenceRepository) PutStructureWindowStitch(ctx context.Context, stitch window.StitchResult) error {
	if r == nil || r.root == "" {
		return errors.New("structure window evidence repository is unavailable")
	}
	if err := window.ValidateStitchResult(stitch); err != nil {
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

func (r *FileStructureAssessmentEvidenceRepository) GetStructureWindowStitch(ctx context.Context, stitchSHA256 string) (window.StitchResult, error) {
	if r == nil || r.root == "" || !structureEvidenceDigest(stitchSHA256) {
		return window.StitchResult{}, errors.New("structure window stitch identity is invalid")
	}
	raw, err := r.readImmutable(ctx, r.blobPath("window-stitches", stitchSHA256), structureWindowStitchMaximumBytes)
	if err != nil {
		return window.StitchResult{}, fmt.Errorf("read structure window stitch: %w", err)
	}
	var stitch window.StitchResult
	if err := decodeStructureWindowEvidence(raw, &stitch); err != nil {
		return window.StitchResult{}, fmt.Errorf("decode structure window stitch: %w", err)
	}
	if stitch.SHA256 != stitchSHA256 {
		return window.StitchResult{}, errors.New("structure window stitch path does not match content")
	}
	if err := window.ValidateStitchResult(stitch); err != nil {
		return window.StitchResult{}, err
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
