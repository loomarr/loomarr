package filler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

const (
	structureAssessmentRecordMaxBytes   = 1 << 20
	structureAssessmentResponseMaxBytes = fillerstructure.AssessmentMaximumResponseBytes
)

// FileStructureAssessmentEvidenceRepository is the private content-addressed store for complete-
// timeline call records and response bytes. Blobs publish before their record, so a visible record
// is always loadable after a crash; unreferenced blobs are harmless and may be swept later.
type FileStructureAssessmentEvidenceRepository struct {
	root string
}

func NewFileStructureAssessmentEvidenceRepository(root string) (*FileStructureAssessmentEvidenceRepository, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root {
		return nil, fmt.Errorf("structure assessment evidence requires a clean absolute root")
	}
	return &FileStructureAssessmentEvidenceRepository{root: root}, nil
}

func (r *FileStructureAssessmentEvidenceRepository) PutStructureAssessmentEvidence(ctx context.Context, recorded fillerstructure.RecordedAssessment) error {
	if r == nil || r.root == "" {
		return fmt.Errorf("structure assessment evidence repository is unavailable")
	}
	if err := fillerstructure.ValidateRecordedAssessment(recorded); err != nil {
		return err
	}
	if len(recorded.RawResponse) > structureAssessmentResponseMaxBytes || len(recorded.StructuredOutput) > structureAssessmentResponseMaxBytes {
		return fmt.Errorf("structure assessment response exceeds the evidence ceiling")
	}
	recordRaw, err := json.Marshal(recorded.Record)
	if err != nil || len(recordRaw) > structureAssessmentRecordMaxBytes {
		return fmt.Errorf("marshal structure assessment record")
	}
	if recorded.Record.ResponseSHA256 != "" {
		if err := r.putImmutable(ctx, r.blobPath("responses", recorded.Record.ResponseSHA256), recorded.RawResponse, structureAssessmentResponseMaxBytes); err != nil {
			return fmt.Errorf("persist structure assessment raw response: %w", err)
		}
	}
	if recorded.Record.StructuredOutputSHA256 != "" {
		if err := r.putImmutable(ctx, r.blobPath("outputs", recorded.Record.StructuredOutputSHA256), []byte(recorded.StructuredOutput), structureAssessmentResponseMaxBytes); err != nil {
			return fmt.Errorf("persist structure assessment structured output: %w", err)
		}
	}
	if err := r.putImmutable(ctx, r.blobPath("records", recorded.Record.SHA256), recordRaw, structureAssessmentRecordMaxBytes); err != nil {
		return fmt.Errorf("persist structure assessment record: %w", err)
	}
	return nil
}

func (r *FileStructureAssessmentEvidenceRepository) GetStructureAssessmentEvidence(ctx context.Context, assessmentSHA256 string) (fillerstructure.RecordedAssessment, error) {
	if r == nil || r.root == "" || !structureEvidenceDigest(assessmentSHA256) {
		return fillerstructure.RecordedAssessment{}, fmt.Errorf("structure assessment evidence identity is invalid")
	}
	recordRaw, err := r.readImmutable(ctx, r.blobPath("records", assessmentSHA256), structureAssessmentRecordMaxBytes)
	if err != nil {
		return fillerstructure.RecordedAssessment{}, fmt.Errorf("read structure assessment record: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(recordRaw))
	decoder.DisallowUnknownFields()
	var record fillerstructure.AssessmentRecord
	if err := decoder.Decode(&record); err != nil {
		return fillerstructure.RecordedAssessment{}, fmt.Errorf("decode structure assessment record: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fillerstructure.RecordedAssessment{}, fmt.Errorf("decode structure assessment record: trailing JSON")
	}
	if record.SHA256 != assessmentSHA256 {
		return fillerstructure.RecordedAssessment{}, fmt.Errorf("structure assessment record path does not match its identity")
	}
	var rawResponse, structuredOutput []byte
	if record.ResponseSHA256 != "" {
		rawResponse, err = r.readImmutable(ctx, r.blobPath("responses", record.ResponseSHA256), structureAssessmentResponseMaxBytes)
		if err != nil {
			return fillerstructure.RecordedAssessment{}, fmt.Errorf("read structure assessment raw response: %w", err)
		}
	}
	if record.StructuredOutputSHA256 != "" {
		structuredOutput, err = r.readImmutable(ctx, r.blobPath("outputs", record.StructuredOutputSHA256), structureAssessmentResponseMaxBytes)
		if err != nil {
			return fillerstructure.RecordedAssessment{}, fmt.Errorf("read structure assessment structured output: %w", err)
		}
	}
	recorded := fillerstructure.RecordedAssessment{Record: record, RawResponse: rawResponse, StructuredOutput: string(structuredOutput)}
	if err := fillerstructure.ValidateRecordedAssessment(recorded); err != nil {
		return fillerstructure.RecordedAssessment{}, err
	}
	return recorded, nil
}

func (r *FileStructureAssessmentEvidenceRepository) blobPath(kind, digest string) string {
	return filepath.Join(r.root, kind, digest[:2], digest+".json")
}

var _ StructureAssessmentEvidenceRepository = (*FileStructureAssessmentEvidenceRepository)(nil)
